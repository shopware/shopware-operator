package manager

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/shopware/shopware-operator/internal/manager/initializing"
	"github.com/shopware/shopware-operator/internal/manager/migration"
	"github.com/shopware/shopware-operator/internal/manager/ready"
	"github.com/shopware/shopware-operator/internal/manager/setup"
	"github.com/shopware/shopware-operator/internal/manager/wait"
	"go.uber.org/zap"
)

const maxStateTransitionsPerReconcile = 5

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

func (m *StoreStateManager) ReconcileState(ctx context.Context, store *v1.Store) {
	for i := 0; i < maxStateTransitionsPerReconcile; i++ {
		mgr, ok := m.managers[store.Status.State]
		if !ok {
			break
		}
		next := mgr.StateHandler(ctx, store)
		if next == store.Status.State {
			break
		}
		logging.FromContext(ctx).Infow("Store state transition",
			zap.String("from", string(store.Status.State)),
			zap.String("to", string(next)))
		store.Status.State = next
	}
}

func (m *StoreStateManager) ReconcileResources(ctx context.Context, store *v1.Store) error {
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
	return mgr.ResourceHandler(ctx, store)
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
