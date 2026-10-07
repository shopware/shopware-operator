package manager

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/manager/initializing"
	"github.com/shopware/shopware-operator/internal/manager/migration"
	"github.com/shopware/shopware-operator/internal/manager/ready"
	"github.com/shopware/shopware-operator/internal/manager/setup"
	"github.com/shopware/shopware-operator/internal/manager/wait"
	"github.com/shopware/shopware-operator/internal/tracing"
	"go.uber.org/zap"
)

type (
	StateHandler    func(ctx context.Context, store *v1.Store) v1.StatefulAppState
	ResourceHandler func(ctx context.Context, store *v1.Store) error
)

type StateManager interface {
	StateHandler(ctx context.Context, store *v1.Store) v1.StatefulAppState
	ResourceHandler(ctx context.Context, store *v1.Store) error
}

type StoreStateManager struct {
	*base.Base
	managers map[v1.StatefulAppState]StateManager
}

func NewStoreStateManager(b *base.Base) *StoreStateManager {
	waitManager := wait.New(b)
	setupManager := setup.New(b)
	migrationManager := migration.New(b)
	initializingManager := initializing.New(b)
	readyManager := ready.New(b)

	return &StoreStateManager{
		Base: b,
		managers: map[v1.StatefulAppState]StateManager{
			v1.StateEmpty:          waitManager,
			v1.StateWait:           waitManager,
			v1.StateSetup:          setupManager,
			v1.StateSetupError:     setupManager,
			v1.StateInitializing:   initializingManager,
			v1.StateMigration:      migrationManager,
			v1.StateMigrationError: migrationManager,
			v1.StateReady:          readyManager,
		},
	}
}

func (m *StoreStateManager) ReconcileState(ctx context.Context, store *v1.Store) (err error) {
	ctx, span := tracing.Start(ctx, "StoreStateManager.ReconcileState",
		tracing.AttrStateFrom.String(string(store.Status.State)))
	defer tracing.End(span, &err)

	mgr, ok := m.managers[store.Status.State]
	if !ok {
		return fmt.Errorf("state %q is not registered in operator", store.Status.State)
	}
	next := m.runStateHandler(ctx, mgr, store)
	if next != store.Status.State {
		logging.FromContext(ctx).Infow("Store state transition",
			zap.String("from", string(store.Status.State)),
			zap.String("to", string(next)))
	}
	tracing.RecordStateChange(ctx, string(store.Status.State), string(next))
	store.Status.State = next
	return nil
}

func (m *StoreStateManager) runStateHandler(ctx context.Context, mgr StateManager, store *v1.Store) v1.StatefulAppState {
	ctx, span := tracing.Start(ctx, handlerName(mgr)+".StateHandler",
		tracing.AttrState.String(string(store.Status.State)))
	defer span.End()

	next := mgr.StateHandler(ctx, store)
	span.SetAttributes(tracing.AttrStateTo.String(string(next)))
	return next
}

func (m *StoreStateManager) ReconcileResources(ctx context.Context, store *v1.Store) (err error) {
	ctx, span := tracing.Start(ctx, "StoreStateManager.ReconcileResources",
		tracing.AttrState.String(string(store.Status.State)))
	defer tracing.End(span, &err)

	log := logging.FromContext(ctx)
	log.Info("Do reconcile on store")

	if err := m.reconcileInitResources(ctx, store); err != nil {
		return err
	}

	if store.IsState(v1.StateEmpty, v1.StateWait) {
		log.Info("skip some resources because s3/db/fastly/opensearch not ready or state is empty")
		return nil
	}

	log.Debug("reconcile app secrets")
	if err := m.EnsureAppSecrets(ctx, store); err != nil {
		return fmt.Errorf("app secrets: %w", err)
	}

	mgr, ok := m.managers[store.Status.State]
	if !ok {
		return nil
	}
	ctx, handlerSpan := tracing.Start(ctx, handlerName(mgr)+".ResourceHandler",
		tracing.AttrState.String(string(store.Status.State)))
	defer tracing.End(handlerSpan, &err)
	return mgr.ResourceHandler(ctx, store)
}

func handlerName(mgr StateManager) string {
	return strings.TrimPrefix(fmt.Sprintf("%T", mgr), "*")
}

// reconcileInitResources reconciles the initial resources for the store,
// everything which can be already created before logic kicks in
func (m *StoreStateManager) reconcileInitResources(ctx context.Context, store *v1.Store) error {
	log := logging.FromContext(ctx)

	log.Debug("reconcile ingress")
	if err := m.ReconcileIngress(ctx, store); err != nil {
		return fmt.Errorf("ingress: %w", err)
	}

	log.Debug("reconcile gateway httproute")
	if err := m.ReconcileHTTPRoute(ctx, store); err != nil {
		return fmt.Errorf("httproute: %w", err)
	}

	log.Debug("reconcile pdb")
	if err := m.ReconcilePDB(ctx, store); err != nil {
		return fmt.Errorf("pdb: %w", err)
	}

	return nil
}
