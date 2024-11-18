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

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
)

// StoreExecReconciler reconciles a StoreExec object
type StoreExecReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shopware.com,resources=storeexecs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StoreExec object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *StoreExecReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, err error) {
	log := log.FromContext(ctx)

	rr = ctrl.Result{RequeueAfter: 10 * time.Second}

	var store *v1.Store
	defer func() {
		if err := r.reconcileCRStatus(ctx, store, err); err != nil {
			log.Error(err, "failed to update status")
		}
	}()

	store, err = k8s.GetStoreExec(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "get CR")
		return rr, nil
	}

	// We don't need yet finalizers
	// if store.ObjectMeta.DeletionTimestamp != nil {
	// 	return rr, r.applyFinalizers(ctx, store)
	// }

	if err := r.doReconcile(ctx, store); err != nil {
		log.Error(err, "reconcile")
		return rr, nil
	}

	log.Info("Reconcile finished")
	return rr, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreExecReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.StoreExec{}).
		Complete(r)
}
func (r *StoreExecReconciler) doReconcile(
	ctx context.Context,
	store *v1.StoreExec,
) error {
	log := log.FromContext(ctx).
		WithName(store.Name).
		WithValues("state", store.Status.State)
	log.Info("Do reconcile on store-exec")

	if store.IsState(v1.StateEmpty, v1.StateWait) {
		log.Info("skip reconcile because s3/db not ready or state is empty")
		return nil
	}

	// State Setup
	if store.IsState(v1.StateSetup) {
		if err := r.reconcileSetupJob(ctx, store); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		log.Info("Wait for setup to finish")
		return nil
	}

	// State Initializing
	if store.IsState(v1.StateInitializing) {
		log.Info("reconcile deployment")
		if err := r.reconcileDeployment(ctx, store); err != nil {
			return fmt.Errorf("deployment: %w", err)
		}
		log.Info("Wait for deployment to finish")
		return nil
	}

	// State Update
	if store.IsState(v1.StateMigration) {
		if err := r.reconcileMigrationJob(ctx, store); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
		log.Info("wait for migration to finish")
		return nil
	}

	if store.IsState(v1.StateReady) {
		if store.Status.CurrentImageTag != store.Spec.Container.Image {
			log.Info("wait for migration to finish")
			if err := r.reconcileMigrationJob(ctx, store); err != nil {
				return fmt.Errorf("migration: %w", err)
			}
			log.Info("wait for migration to finish")
			return nil
		}
	}

	// When setup job is still running we will kill it now. This happens when sidecars are used
	// EDIT: This makes more problems then it will help. So we process the way of terminating to
	// the user to close all sidecars correctly.
	// Check if sidecars are active
	if len(store.Spec.Container.ExtraContainers) > 0 && !store.Spec.DisableJobDeletion {
		log.Info("Delete setup/migration job if they are finished because sidecars are used")
		if err := r.completeJobs(ctx, store); err != nil {
			log.Error(err, "Can't cleanup setup and migration jobs")
		}
	}

	log.Info("reconcile deployment")
	if err := r.reconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	log.Info("reconcile services")
	if err := r.reconcileServices(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Info("reconcile ingress")
	if err := r.reconcileIngress(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Info("reconcile horizontalPodAutoscaler")
	if err := r.reconcileHoizontalPodAutoscaler(ctx, store); err != nil {
		return fmt.Errorf("hpa: %w", err)
	}

	return nil
}
