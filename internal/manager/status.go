package manager

import (
	"context"
	"fmt"
	"reflect"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/metrics"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
)

func (m *StoreStateManager) ReconcileStatus(
	ctx context.Context,
	store *v1.Store,
	reconcileError error,
) error {
	if store == nil || store.DeletionTimestamp != nil {
		return nil
	}

	err := m.ReconcileState(ctx, store)
	if err != nil {
		store.Status.AddCondition(
			v1.StoreCondition{
				Type:               string(store.Status.State),
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            err.Error(),
				Reason:             "StatusError",
				Status:             base.Error,
			},
		)
	}

	if reconcileError != nil {
		store.Status.AddCondition(
			v1.StoreCondition{
				Type:               string(store.Status.State),
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            reconcileError.Error(),
				Reason:             "ReconcileError",
				Status:             base.Error,
			},
		)
	}
	printWarningForEnvs(ctx, store)

	store.Status.Message = store.Status.GetLastCondition().Message
	store.Status.AdminState = deployment.GetAdminDeploymentCondition(ctx, *store, m.Client)
	m.UpdateQueueState(ctx, store)
	store.Status.WorkerState = deployment.GetWorkerDeploymentCondition(ctx, *store, m.Client, m.EnableKeda)
	store.Status.StorefrontState = deployment.GetStorefrontDeploymentCondition(ctx, *store, m.Client)

	logging.FromContext(ctx).Infow("Update store status", zap.Any("status", store.Status))
	m.sendEvent(ctx, *store, "Update store status")
	metrics.UpdateStoreMetrics(store)

	scheduledCronJob, err := cronjob.GetScheduledCronJob(ctx, m.Client, *store)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			logging.FromContext(ctx).Warnw("failed to get scheduled task cronjob for metrics", zap.Error(err))
		}
		scheduledCronJob = nil
	}
	metrics.UpdateScheduledTaskMetrics(store, scheduledCronJob)

	return m.writeStoreStatus(ctx, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}, store.Status)
}

func printWarningForEnvs(ctx context.Context, store *v1.Store) {
	l := logging.FromContext(ctx)
	for _, o := range store.OverriddenOperatorEnvs() {
		l.Warnw("ExtraEnvs overwrites env var managed by the operator. If you can, please use the crd to define it",
			zap.String("env", o.Extra.Name),
			zap.String("extraEnvValue", o.Extra.Value),
			zap.String("operatorEnvValue", o.Operator.Value))
	}
}

func (m *StoreStateManager) sendEvent(ctx context.Context, store v1.Store, message string) {
	e := event.Event{
		Message:       message,
		Condition:     store.Status.GetLastCondition(),
		DeployedImage: store.Status.CurrentImageTag,
		Labels:        store.Labels,
		KindType:      reflect.TypeOf(store).String(),
	}
	log := logging.FromContext(ctx).With(
		zap.Any("event", e),
	)

	for _, handler := range m.EventHandlers {
		log.Info("Sending event", "handler", reflect.TypeOf(handler).String())
		err := handler.Send(ctx, e)
		if err != nil {
			log.Error(err, "Sending event", "handler", reflect.TypeOf(handler).String())
		}
	}
}

func (m *StoreStateManager) writeStoreStatus(
	ctx context.Context,
	nn types.NamespacedName,
	status v1.StoreStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &v1.Store{}
		if err := m.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		cr.Status = status
		return m.Status().Update(ctx, cr)
	})
}
