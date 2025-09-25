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

	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

// Send EVENT
// StoreSnapshotCreateReconciler reconciles a StoreSnapshot object
type StoreSnapshotCreateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Logger   *zap.SugaredLogger
}

var (
	shortRequeue = ctrl.Result{RequeueAfter: 10 * time.Second}
	longRequeue  = ctrl.Result{RequeueAfter: 5 * time.Minute}
)

// TODO: Filter if the state is failed or succeeded, because then we don't reconcile finished snapshots
// SetupWithManager sets up the controller with the Manager.
func (r *StoreSnapshotCreateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.StoreSnapshotCreate{}).
		Owns(&batchv1.Job{}).
		WithEventFilter(SkipStatusUpdates{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotscreate,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotscreate/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=storesnapshotscreate/finalizers,verbs=update
// +kubebuilder:rbac:groups="batch",namespace=default,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=shop.shopware.com,namespace=default,resources=stores,verbs=get;list;update;patch
// +kubebuilder:rbac:groups="",namespace=default,resources=persistentvolumes,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",namespace=default,resources=persistentvolumeclaims,verbs=get;list;watch;create;delete

func (r *StoreSnapshotCreateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("name", req.Name))

	ctx = logging.WithLogger(ctx, logger)
	logger.Info("Reconciling create snapshots")

	snapshotCreate, err := k8s.GetStoreSnapshotCreate(ctx, r.Client, req.NamespacedName)
	if err == nil {
		logger.Info("Processing StoreSnapshotCreate")
		return r.reconcileCreate(ctx, req, snapshotCreate), nil
	} else if !k8serrors.IsNotFound(err) {
		logger.Errorw("get StoreSnapshotCreate", zap.Error(err))
		return shortRequeue, nil
	}

	logger.Info("No StoreSnapshotCreate resource found")
	return shortRequeue, nil
}

func (r *StoreSnapshotCreateReconciler) reconcileCreate(ctx context.Context, req ctrl.Request, snapshot *v1.StoreSnapshotCreate) ctrl.Result {
	logger := logging.FromContext(ctx).With(
		zap.String("snapshot", snapshot.Name),
		zap.String("namespace", snapshot.Namespace),
		zap.String("store", snapshot.Spec.StoreNameRef),
		zap.String("snapshot_type", "create"),
	)

	store, err := k8s.GetStore(ctx, r.Client, client.ObjectKey{Namespace: req.Namespace, Name: snapshot.Spec.StoreNameRef})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Warnw("store not found", zap.Error(err))
			return longRequeue
		}
		logger.Errorw("get store", zap.Error(err))
		return shortRequeue
	}

	if !snapshot.Status.IsState(v1.SnapshotStateFailed, v1.SnapshotStateSucceeded) {
		defer func() {
			if err := r.reconcileCreateCRStatus(ctx, *store, snapshot); err != nil {
				if err != nil {
					logger.Errorw("reconcile snapshot create status", zap.Error(err))
				}
			}

			err = writeSnapshotCreateStatus(ctx, r.Client, types.NamespacedName{
				Namespace: snapshot.Namespace,
				Name:      snapshot.Name,
			}, snapshot.Status)
			if err != nil {
				logger.Errorw("write snapshot status", zap.Error(err))
			}
		}()

		// Create PV and PVC for snapshot storage
		// pv := volume.SnapshotPersistentVolume(*store, snapshot.ObjectMeta)
		// if err := k8s.EnsurePersistentVolume(ctx, r.Client, store, pv, r.Scheme, true); err != nil {
		// 	logger.Errorw("reconcile snapshot PV", zap.Error(err))
		// 	return shortRequeue
		// }
		//
		// pvc := volume.SnapshotPersistentVolumeClaim(*store, snapshot.ObjectMeta)
		// if err := k8s.EnsurePersistentVolumeClaim(ctx, r.Client, store, pvc, r.Scheme, true); err != nil {
		// 	logger.Errorw("reconcile snapshot PVC", zap.Error(err))
		// 	return shortRequeue
		// }

		obj := job.SnapshotCreateJob(*store, *snapshot)
		if err := r.reconcileSnapshotJob(ctx, store, snapshot, obj); err != nil {
			logger.Errorw("reconcile snapshot create job", zap.Error(err))
			return shortRequeue
		}
		return shortRequeue
	}

	return longRequeue
}

func (r *StoreSnapshotCreateReconciler) reconcileCreateCRStatus(ctx context.Context, store v1.Store, snapshot *v1.StoreSnapshotCreate) error {
	if snapshot.Status.IsState(v1.SnapshotStateEmpty) {
		snapshot.Status.State = v1.SnapshotStatePending
	}

	snapshotJob, err := job.GetSnapshotCreateJob(ctx, r.Client, store, *snapshot)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			snapshot.Status.State = v1.SnapshotStatePending
			return nil
		}
		snapshot.Status.State = v1.SnapshotStatePending
		snapshot.Status.Message = "Error in getting snapshot job"
		return nil
	}

	// Controller is to fast so we need to check the setup job
	if snapshotJob == nil {
		snapshot.Status.State = v1.SnapshotStatePending
		snapshot.Status.Message = "Waiting for snapshot job to be created"
		return nil
	}

	jobState, err := job.IsJobContainerDone(ctx, r.Client, snapshotJob, job.CONTAINER_NAME_SNAPSHOT)
	if err != nil {
		snapshot.Status.State = v1.SnapshotStateFailed
		snapshot.Status.Message = "Error in getting snapshot state job"
		snapshot.Status.CompletedAt = metav1.Now()
		return fmt.Errorf("get snapshot job state: %w", err)
	}

	if jobState.IsDone() && jobState.HasErrors() {
		snapshot.Status.State = v1.SnapshotStateFailed
		snapshot.Status.Message = fmt.Sprintf("Snapshot is Done but has Errors. Check logs for more details. Exit code: %d", jobState.ExitCode)
		snapshot.Status.CompletedAt = metav1.Now()
		return nil
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		snapshot.Status.State = v1.SnapshotStateSucceeded
		snapshot.Status.Message = "Snapshot is successfully run"
		snapshot.Status.CompletedAt = metav1.Now()
		return nil
	}

	snapshot.Status.State = v1.SnapshotStateRunning
	snapshot.Status.Message = fmt.Sprintf(
		"Waiting for snapshot job to finish. Active jobs: %d, Failed jobs: %d",
		snapshotJob.Status.Active,
		snapshotJob.Status.Failed,
	)

	return nil
}

func (r *StoreSnapshotCreateReconciler) reconcileSnapshotJob(ctx context.Context, store *v1.Store, owner metav1.Object, obj *batchv1.Job) (err error) {
	var changed bool
	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}
	if changed {
		r.Recorder.Event(store, "Normal", "Diff snapshot job hash",
			fmt.Sprintf("Update Store %s snapshot job in namespace %s. Diff hash",
				store.Name,
				store.Namespace))
		if err := k8s.EnsureJob(ctx, r.Client, owner, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready snapshot job: %w", err)
		}
	}
	return nil
}

func writeSnapshotCreateStatus(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status v1.StoreSnapshotStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &v1.StoreSnapshotCreate{}
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		cr.Status = status
		return cl.Status().Update(ctx, cr)
	})
}
