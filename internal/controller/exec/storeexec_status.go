package exec

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const Error = "Error"

func (r *StoreExecReconciler) reconcileCRStatus(
	ctx context.Context,
	store *v1.Store,
	ex *v1.StoreExec,
	reconcileError error,
) error {
	if ex == nil || ex.ObjectMeta.DeletionTimestamp != nil {
		return nil
	}

	if reconcileError != nil {
		ex.Status.AddCondition(
			v1.ExecCondition{
				Type:               ex.Status.State,
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            reconcileError.Error(),
				Reason:             "ReconcileError",
				Status:             Error,
			},
		)
	}

	if ex.IsState(v1.ExecStateEmpty) {
		ex.Status.State = v1.ExecStateRunning
	}

	if ex.IsState(v1.ExecStateRunning) {
		ex.Status.State = r.stateRunning(ctx, store, ex)
	}

	log.FromContext(ctx).Info("Update exec status", "status", ex.Status)
	return writeStatus(ctx, r.Client, types.NamespacedName{
		Namespace: ex.Namespace,
		Name:      ex.Name,
	}, ex.Status)
}

func writeStatus(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status v1.StoreExecStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &v1.StoreExec{}
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		cr.Status = status
		return cl.Status().Update(ctx, cr)
	})
}

func (r *StoreExecReconciler) stateRunning(ctx context.Context, store *v1.Store, ex *v1.StoreExec) v1.StatefulState {
	con := v1.ExecCondition{
		Type:               v1.ExecStateRunning,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for command job to get started",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		ex.Status.AddCondition(con)
	}()

	command, err := job.GetCommandJob(ctx, r.Client, store, ex)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.ExecStateRunning
		}
		con.Reason = err.Error()
		con.Status = Error
		return v1.ExecStateRunning
	}

	// Controller is to fast so we need to check the command job
	if command == nil {
		return v1.ExecStateRunning
	}

	jobState, err := job.IsJobContainerDone(ctx, r.Client, command, job.CONTAINER_NAME_COMMAND)
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.ExecStateRunning
	}

	if jobState.IsDone() && jobState.HasErrors() {
		con.Message = "Command is Done but has Errors. Check logs for more details"
		con.Reason = fmt.Sprintf("Exit code: %d", jobState.ExitCode)
		con.Status = Error
		con.LastTransitionTime = metav1.Now()
		return v1.ExecStateError
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		con.Message = "Command finished"
		con.LastTransitionTime = metav1.Now()
		return v1.ExecStateDone
	}

	con.Message = fmt.Sprintf(
		"Waiting for command job to finish (Notice sidecars are counted). Active jobs: %d, Failed jobs: %d",
		command.Status.Active,
		command.Status.Failed,
	)

	return v1.ExecStateRunning
}
