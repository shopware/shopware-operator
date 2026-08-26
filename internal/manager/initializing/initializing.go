package initializing

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
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
		Type:               string(v1.StateInitializing),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for deployments to get ready",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	store.Status.CurrentImageTag = store.Spec.Container.Image

	crashing, err := deployment.GetCrashLoopBackOffPods(ctx, *store, m.Client)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateInitializing
	}
	if len(crashing) > 0 {
		con.Type = string(v1.StateCrashLoop)
		con.Message = fmt.Sprintf("Pods in CrashLoopBackOff: %s", strings.Join(crashing, ", "))
		con.Status = base.Error
		con.LastTransitionTime = metav1.Now()
		return v1.StateCrashLoop
	}

	if !m.AllDeploymentsRunning(ctx, store) {
		return v1.StateInitializing
	}

	con.Message = "Initialization finished"
	con.Status = base.Ready
	con.LastTransitionTime = metav1.Now()
	return v1.StateReady
}

func (m *Manager) ResourceHandler(ctx context.Context, store *v1.Store) error {
	logging.FromContext(ctx).Info("reconcile deployment for initializing state")
	if err := m.ReconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}
	return nil
}
