/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/pod"
	"github.com/shopware/shopware-operator/internal/service"
	corev1 "k8s.io/api/core/v1"
)

// StoreDebugInstanceReconciler reconciles a StoreDebugInstance object
type StoreDebugInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Logger   *zap.SugaredLogger
}

// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storedebuginstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storedebuginstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storedebuginstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get
// +kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;create;delete;
// +kubebuilder:rbac:groups="",namespace=default,resources=services,verbs=get;list;watch;create;delete;

func (r *StoreDebugInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	log := logging.FromContext(ctx).
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("name", req.Name))

	var store *shopv1.Store
	var storeDebugInstance *shopv1.StoreDebugInstance
	rr = ctrl.Result{RequeueAfter: 10 * time.Second}
	defer func() {
		if err := r.reconcileCRStatus(ctx, store, storeDebugInstance, err); err != nil {
			log.Errorw("failed to update status", zap.Error(err))
		}
	}()

	storeDebugInstance, err = k8s.GetStoreDebugInstance(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return rr, nil
		}
		log.Errorw("get CR store debug instance", zap.Error(err))
	}

	// validate duration
	_, err = time.ParseDuration(storeDebugInstance.Spec.Duration)
	if err != nil {
		return rr, fmt.Errorf("invalid duration: %w", err)
	}

	store, err = k8s.GetStore(ctx, r.Client, types.NamespacedName{
		Namespace: req.Namespace,
		Name:      storeDebugInstance.Spec.StoreRef,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Skip reconcile, because store is not found", zap.String("storeRef", storeDebugInstance.Spec.StoreRef))
			return rr, nil
		}
		log.Errorw("get CR store", zap.Error(err))
		return rr, nil
	}

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateDone) {
		pod := pod.DebugPod(*store, *storeDebugInstance)
		if err := r.Delete(ctx, pod); err != nil {
			log.Errorw("failed to delete pod", zap.Error(err))
			return rr, nil
		}

		svc := service.DebugService(*store, *storeDebugInstance)
		if err := r.Delete(ctx, svc); err != nil {
			log.Errorw("failed to delete service", zap.Error(err))
			return rr, nil
		}

		rr.Requeue = false
		return rr, nil
	}

	if !store.IsState(shopv1.StateReady) {
		log.Info("Skip reconcile, because store is not ready yet.", zap.Any("store", store.Status))
		return rr, nil
	}

	log = log.With(zap.String("store", storeDebugInstance.Spec.StoreRef))
	log.Info("Do reconcile on store debug instance")

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateUnspecified) {
		log.Info("skip reconcile because state is unspecified")
		return rr, nil
	}

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateDone, shopv1.StoreDebugInstanceStateError) {
		return ctrl.Result{Requeue: false}, nil
	}

	if err := r.reconcilePod(ctx, store, storeDebugInstance); err != nil {
		log.Errorw("exec error", zap.Error(err))
		return rr, nil
	}

	if err := r.reconcileService(ctx, store, storeDebugInstance); err != nil {
		log.Errorw("failed to reconcile service", zap.Error(err))
		return rr, nil
	}

	log.Info("Reconcile finished for store debug instance")
	rr.RequeueAfter = 20 * time.Second
	return rr, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreDebugInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.StoreDebugInstance{}).
		Named("storedebuginstance").
		Complete(r)
}

func (r *StoreDebugInstanceReconciler) reconcilePod(ctx context.Context, store *shopv1.Store, storeDebugInstance *shopv1.StoreDebugInstance) error {
	pod := pod.DebugPod(*store, *storeDebugInstance)

	existingPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}, existingPod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.Recorder.Event(store, "Normal", "Create debug pod",
				fmt.Sprintf("Creating debug pod %s in namespace %s",
					storeDebugInstance.Name,
					storeDebugInstance.Namespace))
			if err := k8s.EnsurePod(ctx, r.Client, storeDebugInstance, pod, r.Scheme, true); err != nil {
				return fmt.Errorf("create debug pod: %w", err)
			}
			return nil
		}
		return fmt.Errorf("get debug pod: %w", err)
	}

	return nil
}

func (r *StoreDebugInstanceReconciler) reconcileService(ctx context.Context, store *shopv1.Store, storeDebugInstance *shopv1.StoreDebugInstance) error {
	svc := service.DebugService(*store, *storeDebugInstance)

	changed, err := k8s.HasObjectChanged(ctx, r.Client, svc)
	if err != nil {
		return fmt.Errorf("reconcile debug service: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Debug service update",
			fmt.Sprintf("Update debug service %s in namespace %s",
				storeDebugInstance.Name,
				storeDebugInstance.Namespace))
		if err := k8s.EnsureService(ctx, r.Client, storeDebugInstance, svc, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile debug service: %w", err)
		}
	}

	return nil
}
