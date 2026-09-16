package migration

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/manager/base"
	batchv1 "k8s.io/api/batch/v1"
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
		Type:               string(v1.StateMigration),
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for migration job to finish",
		Reason:             "",
		Status:             "",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	migration, err := job.GetMigrationJob(ctx, m.Client, *store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logging.FromContext(ctx).Info("Migration job is not found")
			return v1.StateMigration
		}
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateMigration
	}

	// Controller is to fast so we need to check the migration job
	if migration == nil {
		logging.FromContext(ctx).Info("Migration is nil")
		return v1.StateMigration
	}

	jobState, err := job.IsJobContainerDone(ctx, m.Client, migration, job.CONTAINER_NAME_MIGRATION_JOB)
	if err != nil {
		con.Reason = err.Error()
		con.Status = base.Error
		return v1.StateMigration
	}

	if jobState.IsDone() && jobState.HasErrors() {
		con.Message = "Migration is Done but has Errors. Check logs for more details." + migrationDuration(migration)
		con.Reason = fmt.Sprintf("Exit code: %d", jobState.ExitCode)
		con.Status = base.Error
		con.Type = string(v1.StateMigrationError)
		con.LastTransitionTime = metav1.Now()
		return v1.StateMigrationError
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		con.Message = "Migration finished." + migrationDuration(migration)
		con.Status = base.Ready
		con.LastTransitionTime = metav1.Now()
		m.Eventf(store, "Finish Migration",
			"Migration in Store %s/%s finished. From tag %s to %s ",
			store.Namespace,
			store.Name,
			store.Status.CurrentImageTag,
			store.Spec.Container.Image)
		store.Status.CurrentImageTag = store.Spec.Container.Image
		return v1.StateInitializing
	}

	con.Message = fmt.Sprintf(
		"Waiting for migration job to finish. Active jobs: %d, Failed jobs: %d",
		migration.Status.Active,
		migration.Status.Failed,
	)

	return v1.StateMigration
}

func migrationDuration(job *batchv1.Job) string {
	if job.Status.StartTime == nil {
		return ""
	}
	end := metav1.Now()
	if job.Status.CompletionTime != nil {
		end = *job.Status.CompletionTime
	}
	return fmt.Sprintf(" (Duration %s)", end.Sub(job.Status.StartTime.Time).Round(time.Second))
}

func (m *Manager) ResourceHandler(ctx context.Context, store *v1.Store) error {
	log := logging.FromContext(ctx)

	if store.IsState(v1.StateMigrationError) {
		log.Warn("Migration job has errors check migration logs. Waiting for new Image")
	}

	if err := m.ReconcileMigrationJob(ctx, store); err != nil {
		return fmt.Errorf("migration: %w", err)
	}

	store.Spec.ScheduledTask.Suspend = true
	log.Info("Overwrite Suspend for ScheduledTask because of migration")
	if err := m.ReconcileScheduledTask(ctx, store); err != nil {
		return fmt.Errorf("cronjob: %w", err)
	}

	log.Info("wait for migration to finish")
	return nil
}
