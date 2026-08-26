package crashloop

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
		Type:               string(v1.StateCrashLoop),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Pods are in CrashLoopBackOff",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	if store.Status.CurrentImageTag != store.Spec.Container.Image {
		con.Message = "New image detected, switch to migration"
		con.Status = base.Ready
		con.LastTransitionTime = metav1.Now()
		return v1.StateMigration
	}

	crashing, err := deployment.GetCrashLoopBackOffPods(ctx, *store, m.Client)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateCrashLoop
	}
	if len(crashing) > 0 {
		con.Message = fmt.Sprintf("Pods in CrashLoopBackOff: %s", strings.Join(crashing, ", "))
		con.Status = base.Error
		return v1.StateCrashLoop
	}

	con.Message = "Pods recovered from CrashLoopBackOff"
	con.Status = base.Ready
	con.LastTransitionTime = metav1.Now()
	return v1.StateInitializing
}

func (m *Manager) ResourceHandler(ctx context.Context, store *v1.Store) error {
	logging.FromContext(ctx).Info("reconcile deployment for crashloop state")
	if err := m.ReconcileDeployment(ctx, store); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}
	return nil
}
