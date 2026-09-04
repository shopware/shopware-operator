package ready

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Manager struct {
	*base.Base
}

func New(b *base.Base) *Manager {
	return &Manager{Base: b}
}

func (m *Manager) StateHandler(ctx context.Context, store *v1.Store) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               string(v1.StateReady),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Store is running waiting for image updates to migrate",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	currentImage, err := deployment.GetStoreDeploymentImage(ctx, *store, m.Client)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.StateInitializing
		}
		con.Status = base.Error
		con.Reason = fmt.Sprintf("get deployment: %s", err.Error())
		return v1.StateReady
	}
	store.Status.CurrentImageTag = currentImage

	if currentImage != store.Spec.Container.Image {
		logging.FromContext(ctx).
			With(zap.String("currentImage", currentImage), zap.String("containerImage", store.Spec.Container.Image)).
			Info("Change to state migration")

		con.Reason = "Detected image change, switch to migration"
		con.Status = base.Ready
		con.LastTransitionTime = metav1.Now()
		return v1.StateMigration
	}

	if !m.AllDeploymentsRunning(ctx, store) {
		con.Reason = "Deployments are not running anymore"
		return v1.StateInitializing
	}

	con.Status = base.Ready
	return v1.StateReady
}

func (m *Manager) ResourceHandler(ctx context.Context, store *v1.Store) error {
	log := logging.FromContext(ctx)

	// Should be optional because we check the image in the status and switch to migration state. This should
	// also be the prefired way to update a store. But might cause issues needs more testing.
	if store.Status.CurrentImageTag != store.Spec.Container.Image {
		log.Info("wait for migration to finish")
		if err := m.ReconcileMigrationJob(ctx, store); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
		return nil
	}

	log.Debug("reconcile deployment")
	if err := m.ReconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	log.Debug("reconcile services")
	if err := m.ReconcileServices(ctx, store); err != nil {
		return fmt.Errorf("service: %w", err)
	}

	log.Debug("reconcile CronJob scheduledTask")
	if err := m.ReconcileScheduledTask(ctx, store); err != nil {
		return fmt.Errorf("cronjob: %w", err)
	}

	log.Debug("reconcile horizontalPodAutoscaler")
	if err := m.ReconcileHorizontalPodAutoscaler(ctx, store); err != nil {
		return fmt.Errorf("hpa: %w", err)
	}

	return nil
}
