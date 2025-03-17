package controller

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/pod"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *StoreDebugInstanceReconciler) reconcileCRStatus(
	ctx context.Context,
	store *v1.Store,
	storeDebugInstance *v1.StoreDebugInstance,
	reconcileError error,
) error {
	if storeDebugInstance == nil || storeDebugInstance.DeletionTimestamp != nil {
		return nil
	}

	if reconcileError != nil {
		storeDebugInstance.Status.AddCondition(
			v1.StoreDebugInstanceCondition{
				Type:               storeDebugInstance.Status.State,
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            reconcileError.Error(),
				Reason:             "ReconcileError",
				Status:             Error,
			},
		)
	}

	if store == nil {
		storeDebugInstance.Status.State = v1.StoreDebugInstanceStateWait
		storeDebugInstance.Status.AddCondition(
			v1.StoreDebugInstanceCondition{
				Type:               storeDebugInstance.Status.State,
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            fmt.Sprintf("StoreRef not found (stores/%s), waiting for store to be created", storeDebugInstance.Spec.StoreRef),
				Reason:             "StoreRef error",
				Status:             Error,
			},
		)
	} else {
		if storeDebugInstance.IsState(v1.StoreDebugInstanceStateUnspecified) {
			storeDebugInstance.Status.State = v1.StoreDebugInstanceStatePending
		}
	}

	if storeDebugInstance.IsState(v1.StoreDebugInstanceStateRunning, v1.StoreDebugInstanceStatePending) {
		storeDebugInstance.Status.State = r.stateRunning(ctx, store, storeDebugInstance)
	}

	log.FromContext(ctx).Info("Update store debug instance status", "status", storeDebugInstance.Status)
	return writeStoreDebugInstanceStatus(ctx, r.Client, types.NamespacedName{
		Namespace: storeDebugInstance.Namespace,
		Name:      storeDebugInstance.Name,
	}, storeDebugInstance.Status)
}

func writeStoreDebugInstanceStatus(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status v1.StoreDebugInstanceStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &v1.StoreDebugInstance{}
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		cr.Status = status
		return cl.Status().Update(ctx, cr)
	})
}

func (r *StoreDebugInstanceReconciler) stateRunning(ctx context.Context, store *v1.Store, storeDebugInstance *v1.StoreDebugInstance) v1.StoreDebugInstanceState {
	con := v1.StoreDebugInstanceCondition{
		Type:               v1.StoreDebugInstanceStateRunning,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for pod to get started",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		storeDebugInstance.Status.AddCondition(con)
	}()

	duration, _ := time.ParseDuration(storeDebugInstance.Spec.Duration)
	if time.Now().After(storeDebugInstance.CreationTimestamp.Add(duration)) {
		con.Message = "Store debug instance expired"
		con.Status = string(v1.StoreDebugInstanceStateDone)
		return v1.StoreDebugInstanceStateDone
	}

	pod, err := pod.GetDebugPod(ctx, r.Client, *store, *storeDebugInstance)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			con.Message = "Pod not found, waiting for creation"
			return v1.StoreDebugInstanceStatePending
		}
		con.Reason = err.Error()
		con.Status = Error
		return v1.StoreDebugInstanceStateError
	}

	if pod == nil {
		con.Message = "Pod not found, waiting for creation"
		return v1.StoreDebugInstanceStatePending
	}

	if pod.Status.Phase == corev1.PodPending {
		con.Message = "Pod is pending, waiting for resources"
		return v1.StoreDebugInstanceStatePending
	}

	if pod.Status.Phase == corev1.PodRunning {
		con.Message = "Pod is running successfully"
		con.LastTransitionTime = metav1.Now()
		return v1.StoreDebugInstanceStateRunning
	}

	if pod.Status.Phase == corev1.PodFailed {
		con.Message = "Pod failed to start"
		con.Reason = "PodFailed"
		con.Status = Error
		return v1.StoreDebugInstanceStateError
	}

	con.Message = fmt.Sprintf("Pod in unknown state: %s", pod.Status.Phase)
	return v1.StoreDebugInstanceStatePending
}
