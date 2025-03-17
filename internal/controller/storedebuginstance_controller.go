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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	logging "sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/pod"
)

// StoreDebugInstanceReconciler reconciles a StoreDebugInstance object
type StoreDebugInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=shop.shopware.com,resources=storedebuginstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,resources=storedebuginstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,resources=storedebuginstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=shop.shopware.com,resources=stores,verbs=get
// +kubebuilder:rbac:groups="",namespace=default,resources=pods,verbs=get;list;watch;
func (r *StoreDebugInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	log := log.FromContext(ctx)

	var store *shopv1.Store
	var storeDebugInstance *shopv1.StoreDebugInstance
	rr = ctrl.Result{RequeueAfter: 10 * time.Second}
	defer func() {
		if err := r.reconcileCRStatus(ctx, store, storeDebugInstance, err); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	storeDebugInstance, err = k8s.GetStoreDebugInstance(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return rr, nil
		}
		log.Error(err, "get CR store debug instance")
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
			log.Info("Skip reconcile, because store is not found", "storeRef", storeDebugInstance.Spec.StoreRef)
			return rr, nil
		}
		log.Error(err, "get CR store")
		return rr, nil
	}

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateDone) {
		pod := pod.DebugPod(*store, *storeDebugInstance)
		if err := r.Client.Delete(ctx, pod); err != nil {
			log.Error(err, "failed to delete pod")
			return rr, nil
		}
		rr.Requeue = false
		return rr, nil
	}

	if !store.IsState(shopv1.StateReady) {
		log.Info("Skip reconcile, because store is not ready yet.", "store", store.Status)
		return rr, nil
	}

	log = logging.FromContext(ctx).
		WithName(storeDebugInstance.Name).
		WithValues("store", storeDebugInstance.Spec.StoreRef).
		WithValues("state", storeDebugInstance.Status.State)

	log.Info("Do reconcile on store debug instance")

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateUnspecified) {
		log.Info("skip reconcile because state is unspecified")
		return rr, nil
	}

	if storeDebugInstance.IsState(shopv1.StoreDebugInstanceStateDone, shopv1.StoreDebugInstanceStateError) {
		return ctrl.Result{Requeue: false}, nil
	}

	if err := r.reconcilePod(ctx, store, storeDebugInstance); err != nil {
		log.Error(err, "exec error: %w")
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

func (r *StoreDebugInstanceReconciler) reconcilePod(ctx context.Context, store *shopv1.Store, storeDebugInstance *shopv1.StoreDebugInstance) (err error) {
	var changed bool
	obj := pod.DebugPod(*store, *storeDebugInstance)

	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		r.Recorder.Event(store, "Normal", "Diff command pod hash",
			fmt.Sprintf("Update command pod %s in namespace %s. Diff hash",
				storeDebugInstance.Name,
				storeDebugInstance.Namespace))
		if err := k8s.EnsurePod(ctx, r.Client, storeDebugInstance, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile debug pod: %w", err)
		}
	}

	return nil
}
