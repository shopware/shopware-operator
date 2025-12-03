package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/shopware/shopware-operator/internal/event"
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

var (
	_ SnapshotResource = (*v1.StoreSnapshotRestore)(nil)
	_ SnapshotResource = (*v1.StoreSnapshotCreate)(nil)
)

// SnapshotResource represents a common interface for snapshot resources
type SnapshotResource interface {
	GetRuntimeObject() runtime.Object
	GetObjectMeta() metav1.Object
	GetSpec() v1.StoreSnapshotSpec
	GetStatus() *v1.StoreSnapshotStatus
}

// JobGetter defines how to get the appropriate job for each snapshot type
type JobGetter func(ctx context.Context, client client.Client, store v1.Store, snapshot SnapshotResource) (*batchv1.Job, error)

// JobCreator defines how to create the appropriate job for each snapshot type
type JobCreator func(store v1.Store, snapshot SnapshotResource) *batchv1.Job

// SnapshotGetter defines how to get the snapshot resource from k8s
type SnapshotGetter func(ctx context.Context, client client.Client, key types.NamespacedName) (SnapshotResource, error)

// StatusWriter defines how to write the snapshot status back to k8s
type StatusWriter func(ctx context.Context, client client.Client, key types.NamespacedName, status v1.StoreSnapshotStatus) error

// StoreSnapshotBaseReconciler contains shared logic for snapshot controllers
type StoreSnapshotBaseReconciler struct {
	Client        client.Client
	EventHandlers []event.EventHandler
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	Logger        *zap.SugaredLogger
}

func (r *StoreSnapshotBaseReconciler) reconcileCRStatus(
	ctx context.Context,
	store v1.Store,
	snapshot SnapshotResource,
	getJob JobGetter,
) error {
	status := snapshot.GetStatus()
	if status.IsState(v1.SnapshotStateEmpty) {
		status.State = v1.SnapshotStatePending
	}
	previousState := status.State

	con := v1.StoreCondition{
		Type:               string(v1.SnapshotStatePending),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		con.Type = string(status.State)
		con.Message = status.Message
		if previousState != status.State {
			con.LastTransitionTime = metav1.Now()
		}
		status.AddCondition(con)
	}()

	snapshotJob, err := getJob(ctx, r.Client, store, snapshot)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			status.State = v1.SnapshotStatePending
			return nil
		}
		status.State = v1.SnapshotStatePending
		status.Message = "Error in getting snapshot job"
		con.Status = Error
		con.Reason = fmt.Errorf("InternalError: Error in getting snapshot job: %w", err).Error()
		return nil
	}

	// Controller is too fast so we need to check the setup job
	if snapshotJob == nil {
		status.State = v1.SnapshotStatePending
		status.Message = "Waiting for snapshot job to be created"
		con.Status = ""
		con.Reason = "Waiting for snapshot job to be created"
		return nil
	}

	jobState, err := job.IsJobContainerDone(ctx, r.Client, snapshotJob, job.CONTAINER_NAME_SNAPSHOT)
	if err != nil {
		status.State = v1.SnapshotStateFailed
		status.Message = "Error in getting snapshot state job"
		status.CompletedAt = metav1.Now()
		con.Status = Error
		con.Reason = fmt.Errorf("InternalError: Error in getting snapshot state job: %w", err).Error()
		return fmt.Errorf("get snapshot job state: %w", err)
	}

	if jobState.IsDone() && jobState.HasErrors() {
		status.State = v1.SnapshotStateFailed
		status.Message = fmt.Sprintf("Snapshot is Done but has Errors. Check logs for more details. Exit code: %d", jobState.ExitCode)
		status.CompletedAt = metav1.Now()
		con.Status = Error
		//nolint:staticcheck
		con.Reason = fmt.Errorf("Error in Snapshot, exit code: %d", jobState.ExitCode).Error()
		return nil
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		status.State = v1.SnapshotStateSucceeded
		status.Message = "Snapshot is successfully run"
		status.CompletedAt = metav1.Now()
		con.Status = Ready
		con.Reason = "Snapshot successfully created"
		return nil
	}

	status.State = v1.SnapshotStateRunning
	status.Message = fmt.Sprintf(
		"Waiting for snapshot job to finish. Active jobs: %d, Failed jobs: %d",
		snapshotJob.Status.Active,
		snapshotJob.Status.Failed,
	)
	con.Status = ""
	con.Reason = "Still running"

	return nil
}

// ReconcileSnapshot provides the main reconciliation logic for snapshot controllers
func (r *StoreSnapshotBaseReconciler) ReconcileSnapshot(
	ctx context.Context,
	req ctrl.Request,
	snapshotType string,
	getSnapshot SnapshotGetter,
	getJob JobGetter,
	createJob JobCreator,
	writeStatus StatusWriter,
) (ctrl.Result, error) {
	logger := r.Logger.
		With(zap.String("namespace", req.Namespace)).
		With(zap.String("service", "shopware-operator-snapshot")).
		With(zap.String("type", snapshotType)).
		With(zap.String("name", req.Name))

	ctx = logging.WithLogger(ctx, logger)

	snapshot, err := getSnapshot(ctx, r.Client, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Warnw("snapshot not found, stop execution", zap.Error(err))
			return noRequeue, nil
		} else {
			logger.Errorw("get snapshot unknown error, stop execution", zap.Error(err))
			return noRequeue, nil
		}
	}

	logger.Info("Processing snapshot")
	return r.reconcileSnapshotResource(ctx, req, snapshot, snapshotType, getJob, createJob, writeStatus), nil
}

func (r *StoreSnapshotBaseReconciler) reconcileSnapshotResource(
	ctx context.Context,
	req ctrl.Request,
	snapshot SnapshotResource,
	snapshotType string,
	getJob JobGetter,
	createJob JobCreator,
	writeStatus StatusWriter,
) ctrl.Result {
	logger := logging.FromContext(ctx).With(
		zap.String("name", snapshot.GetObjectMeta().GetName()),
		zap.String("namespace", snapshot.GetObjectMeta().GetNamespace()),
		zap.String("store-ref", snapshot.GetSpec().StoreNameRef),
	)

	store, err := k8s.GetStore(ctx, r.Client, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      snapshot.GetSpec().StoreNameRef,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Warnw("store not found", zap.Error(err))
			return longRequeue
		}
		logger.Errorw("get store", zap.Error(err))
		return longRequeue
	}

	// Skip reconciliation for terminal states (failed or succeeded)
	if snapshot.GetStatus().IsState(v1.SnapshotStateFailed, v1.SnapshotStateSucceeded) {
		return noRequeue
	}

	// Always check status for running or pending snapshots
	defer func() {
		if err := r.reconcileCRStatus(ctx, *store, snapshot, getJob); err != nil {
			logger.Errorw(fmt.Sprintf("reconcile snapshot %s status", snapshotType), zap.Error(err))
		}

		r.sendEvent(ctx, snapshot)
		err = writeStatus(ctx, r.Client, types.NamespacedName{
			Namespace: snapshot.GetObjectMeta().GetNamespace(),
			Name:      snapshot.GetObjectMeta().GetName(),
		}, *snapshot.GetStatus())
		if err != nil {
			logger.Errorw("write snapshot status", zap.Error(err))
		}
	}()

	// Only create/update job if not in running state
	if !snapshot.GetStatus().IsState(v1.SnapshotStateRunning) {
		obj := createJob(*store, snapshot)
		if err := r.reconcileSnapshotJob(ctx, snapshot, snapshot.GetObjectMeta(), obj); err != nil {
			logger.Errorw(fmt.Sprintf("reconcile snapshot %s job", snapshotType), zap.Error(err))
			return shortRequeue
		}
	}

	return longRequeue
}

func (r *StoreSnapshotBaseReconciler) reconcileSnapshotJob(ctx context.Context, snap SnapshotResource, owner metav1.Object, obj *batchv1.Job) (err error) {
	var changed bool
	if changed, err = k8s.HasObjectChanged(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}
	if changed {
		r.Recorder.Event(snap.GetRuntimeObject(), "Normal", "Diff snapshot job hash",
			fmt.Sprintf("Update Store %s snapshot job in namespace %s. Diff hash",
				snap.GetObjectMeta().GetName(),
				snap.GetObjectMeta().GetNamespace()))
		if err := k8s.EnsureJob(ctx, r.Client, owner, obj, r.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready snapshot job: %w", err)
		}
	}
	return nil
}

// WriteSnapshotStatus provides a generic status writer
func WriteSnapshotStatus[T client.Object](
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status v1.StoreSnapshotStatus,
	newResource func() T,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := newResource()
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		// Use reflection to set the Status field
		switch resource := any(cr).(type) {
		case *v1.StoreSnapshotCreate:
			resource.Status = status
		case *v1.StoreSnapshotRestore:
			resource.Status = status
		default:
			return fmt.Errorf("unsupported resource type")
		}

		return cl.Status().Update(ctx, cr)
	})
}

func (r *StoreSnapshotBaseReconciler) sendEvent(ctx context.Context, snapshot SnapshotResource) {
	e := event.Event{
		Message:       snapshot.GetStatus().Message,
		Condition:     snapshot.GetStatus().GetLastCondition(),
		DeployedImage: snapshot.GetSpec().Container.Image,
		Labels:        snapshot.GetObjectMeta().GetLabels(),
		KindType:      reflect.TypeOf(snapshot).String(),
	}

	log := logging.FromContext(ctx).With(
		zap.Any("event", e),
	)

	for _, handler := range r.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}
