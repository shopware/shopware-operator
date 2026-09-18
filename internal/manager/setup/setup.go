package setup

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
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
		Type:               string(v1.StateSetup),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for setup job to finish",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	setup, err := job.GetSetupJob(ctx, m.Client, *store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.StateSetup
		}
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateSetup
	}

	// Controller is to fast so we need to check the setup job
	if setup == nil {
		return v1.StateSetup
	}

	jobState, err := job.IsJobContainerDone(ctx, m.Client, setup, job.CONTAINER_NAME_SETUP_JOB)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateSetup
	}

	if jobState.IsDone() && jobState.HasErrors() {
		con.Message = "Setup is Done but has Errors. Check logs for more details"
		con.Reason = fmt.Sprintf("Exit code: %d", jobState.ExitCode)
		con.Status = base.Error
		con.Type = string(v1.StateSetupError)
		con.LastTransitionTime = metav1.Now()
		return v1.StateSetupError
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		con.Message = "Setup finished"
		con.Status = base.Ready
		con.LastTransitionTime = metav1.Now()
		return v1.StateInitializing
	}

	con.Message = fmt.Sprintf(
		"Waiting for setup job to finish (Notice sidecars are counted). Active jobs: %d, Failed jobs: %d",
		setup.Status.Active,
		setup.Status.Failed,
	)

	return v1.StateSetup
}

func (m *Manager) ResourceHandler(ctx context.Context, store *v1.Store) error {
	log := logging.FromContext(ctx)

	if store.IsState(v1.StateSetupError) {
		log.Warn("Setup job has errors check setup logs. Waiting for new Image")
	}

	if err := m.ReconcileSetupJob(ctx, store); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	log.Info("Wait for setup to finish")
	return nil
}
