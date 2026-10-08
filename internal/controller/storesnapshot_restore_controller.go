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

	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

// StoreSnapshotRestoreReconciler reconciles a StoreSnapshot object
type StoreSnapshotRestoreReconciler struct {
	StoreSnapshotBaseReconciler
}

// SetupWithManager sets up the controller with the Manager.
func (r *StoreSnapshotRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.StoreSnapshotRestore{}).
		Owns(&batchv1.Job{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotrestores/finalizers,verbs=update
// +kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get;list;update;patch
// +kubebuilder:rbac:groups="",namespace=default,resources=persistentvolumes,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",namespace=default,resources=persistentvolumeclaims,verbs=get;list;watch;create;delete

func (r *StoreSnapshotRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	getSnapshot := func(ctx context.Context, client client.Client, key types.NamespacedName) (SnapshotResource, error) {
		snapshot, err := k8s.GetStoreSnapshotRestore(ctx, client, key)
		if err != nil {
			return nil, err
		}
		return snapshot, nil
	}

	getJob := func(ctx context.Context, client client.Client, store v1.Store, snapshot SnapshotResource) (*batchv1.Job, error) {
		restoreSnapshot := snapshot.(*v1.StoreSnapshotRestore)
		return job.GetSnapshotRestoreJob(ctx, client, store, *restoreSnapshot)
	}

	createJob := func(store v1.Store, snapshot SnapshotResource) *batchv1.Job {
		restoreSnapshot := snapshot.(*v1.StoreSnapshotRestore)
		return job.SnapshotRestoreJob(store, *restoreSnapshot)
	}

	writeStatus := func(ctx context.Context, client client.Client, key types.NamespacedName, status v1.StoreSnapshotStatus) error {
		return WriteSnapshotStatus(ctx, client, key, status, func() *v1.StoreSnapshotRestore {
			return &v1.StoreSnapshotRestore{}
		})
	}

	return r.ReconcileSnapshot(ctx, req, "restore", getSnapshot, getJob, createJob, writeStatus)
}
