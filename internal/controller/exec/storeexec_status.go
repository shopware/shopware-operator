package exec

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const Error = "Error"

func (r *StoreExecReconciler) reconcileCRStatus(
	ctx context.Context,
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

	// First creation
	if ex.IsState(v1.ExecStateEmpty) {
		// We disable the checks for local development, so you don't need to run
		// portforwards or dns. This is later also important if we plan to install one
		// operator for multiple namespaces.
		ex.Status.State = v1.ExecStateInitializing
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
