package controller

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *StoreReconciler) reconcileCRStatus(
	ctx context.Context,
	store *v1.Store,
	reconcileError error,
) error {
	if store == nil || store.DeletionTimestamp != nil {
		return nil
	}

	if reconcileError != nil {
		store.Status.AddCondition(
			v1.StoreCondition{
				Type:               store.Status.State,
				LastTransitionTime: metav1.Time{},
				LastUpdateTime:     metav1.NewTime(time.Now()),
				Message:            reconcileError.Error(),
				Reason:             "ReconcileError",
				Status:             Error,
			},
		)
	}

	printWarningForEnvs(ctx, store)

	// First creation
	if store.IsState(v1.StateEmpty) || store.IsState(v1.StateWait) {
		// We disable the checks for local development, so you don't need to run
		// portforwards or dns. This is later also important if we plan to install one
		// operator for multiple namespaces.
		if r.DisableServiceChecks || store.Spec.DisableChecks {
			store.Status.State = v1.StateSetup
		} else {
			store.Status.State = v1.StateWait
		}
	}

	if store.IsState(v1.StateWait) {
		if !store.Spec.DisableDatabaseCheck {
			store.Status.State = r.checkDatabaseServices(ctx, store)
		}

		if !store.Spec.DisableS3Check && store.Spec.S3Storage.AccessKeyRef.Key != "" {
			store.Status.State = r.checkS3Services(ctx, store)
		}
	}

	if store.IsState(v1.StateSetup) {
		store.Status.State = r.stateSetup(ctx, store)
	}

	if store.IsState(v1.StateInitializing) {
		store.Status.State = r.stateInitializing(ctx, store)
		store.Status.CurrentImageTag = store.Spec.Container.Image
	}

	if store.IsState(v1.StateReady) {
		currentImage, err := deployment.GetStoreDeploymentImage(ctx, *store, r.Client)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				store.Status.State = v1.StateInitializing
			}
		}
		store.Status.CurrentImageTag = currentImage
		store.Status.State = r.stateReady(ctx, store)
	}

	if store.IsState(v1.StateMigration) {
		store.Status.State = r.stateMigration(ctx, store)
		if store.IsState(v1.StateInitializing) {
			log.FromContext(ctx).Info("Update current image tag")
			r.Recorder.Event(store, "Normal", "Finish Migration",
				fmt.Sprintf("Migration in Store %s/%s finished. From tag %s to %s ",
					store.Namespace,
					store.Name,
					store.Status.CurrentImageTag,
					store.Spec.Container.Image))
			store.Status.CurrentImageTag = store.Spec.Container.Image
		}
	}

	log.FromContext(ctx).Info("Update store status", "status", store.Status)
	return writeStoreStatus(ctx, r.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}, store.Status)
}

func printWarningForEnvs(ctx context.Context, store *v1.Store) {
	l := log.FromContext(ctx)

	envs := store.GetEnv()
	for _, obj2 := range store.Spec.Container.ExtraEnvs {
		if slices.ContainsFunc(envs, func(c corev1.EnvVar) bool { return c.Name == obj2.Name }) {
			l.Info("Overwriting env var. If you can, please use the crd to define it", "name", obj2.Name)
		}
	}
}

func (r *StoreReconciler) checkDatabaseServices(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateWait,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for database connection",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	secret, err := k8s.GetSecret(ctx, r.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.Database.PasswordSecretRef.Name,
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	var password []byte
	var ok bool
	if password, ok = secret.Data[store.Spec.Database.PasswordSecretRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"PasswordSecretRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.Database.PasswordSecretRef.Key,
			store.Spec.Database.PasswordSecretRef.Name,
		)
		con.Status = Error
		return v1.StateWait
	}

	dbHost, err := util.GetDBHost(ctx, *store, r.Client)
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	err = util.TestSQLConnection(ctx, &store.Spec.Database, dbHost, password)
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = Ready
	con.Reason = "Database ping passed"
	return v1.StateSetup
}

func (r *StoreReconciler) checkS3Services(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateWait,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for s3 connection",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	secret, err := k8s.GetSecret(ctx, r.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.S3Storage.SecretAccessKeyRef.Name,
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	var ok bool
	var secretAccessKey []byte
	if secretAccessKey, ok = secret.Data[store.Spec.S3Storage.SecretAccessKeyRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The SecretAccessKeyRef dosn't contain the specified key '%s' in the secret '%s'",
			store.Spec.S3Storage.SecretAccessKeyRef.Key,
			store.Spec.S3Storage.SecretAccessKeyRef.Name,
		)
		con.Status = Error
		return v1.StateWait
	}

	secret, err = k8s.GetSecret(ctx, r.Client, types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Spec.S3Storage.AccessKeyRef.Name,
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	var accessKey []byte
	if accessKey, ok = secret.Data[store.Spec.S3Storage.AccessKeyRef.Key]; !ok {
		con.Reason = fmt.Sprintf(
			"The AccessKeyRef doesn't contain the specified key '%s' in the secret '%s'",
			store.Spec.S3Storage.AccessKeyRef.Key,
			store.Spec.S3Storage.AccessKeyRef.Name,
		)
		con.Status = Error
		return v1.StateWait
	}

	err = util.TestS3Connection(ctx, store.Spec.S3Storage, aws.Credentials{
		AccessKeyID:     string(accessKey),
		SecretAccessKey: string(secretAccessKey),
	})
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateWait
	}

	con.LastTransitionTime = metav1.Now()
	con.Status = Ready
	con.Reason = "S3 connection test passed"
	return v1.StateSetup
}

func (r *StoreReconciler) stateSetup(ctx context.Context, store *v1.Store) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateSetup,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for setup job to finish",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	setup, err := job.GetSetupJob(ctx, r.Client, *store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.StateSetup
		}
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateSetup
	}

	// Controller is to fast so we need to check the setup job
	if setup == nil {
		return v1.StateSetup
	}

	jobState, err := job.IsJobContainerDone(ctx, r.Client, setup, job.CONTAINER_NAME_SETUP_JOB)
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateSetup
	}

	if jobState.IsDone() && jobState.HasErrors() {
		con.Message = "Setup is Done but has Errors. Check logs for more details"
		con.Reason = fmt.Sprintf("Exit code: %d", jobState.ExitCode)
		con.Status = Error
		con.LastTransitionTime = metav1.Now()
		return v1.StateSetup
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		con.Message = "Setup finished"
		con.Status = Ready
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

func (r *StoreReconciler) stateMigration(ctx context.Context, store *v1.Store) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateMigration,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for migration job to finish",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	migration, err := job.GetMigrationJob(ctx, r.Client, *store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.FromContext(ctx).Info("Migration job is not found")
			return v1.StateMigration
		}
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateMigration
	}

	// Controller is to fast so we need to check the migration job
	if migration == nil {
		log.FromContext(ctx).Info("Migration is nil")
		return v1.StateMigration
	}

	jobState, err := job.IsJobContainerDone(ctx, r.Client, migration, job.CONTAINER_NAME_MIGRATION_JOB)
	if err != nil {
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateMigration
	}

	if jobState.IsDone() && jobState.HasErrors() {
		con.Message = "Migration is Done but has Errors. Check logs for more details"
		con.Reason = fmt.Sprintf("Exit code: %d", jobState.ExitCode)
		con.Status = Error
		con.LastTransitionTime = metav1.Now()
		return v1.StateMigration
	}

	if jobState.IsDone() && !jobState.HasErrors() {
		con.Message = "Migration finished"
		con.Status = Ready
		con.LastTransitionTime = metav1.Now()
		return v1.StateInitializing
	}

	con.Message = fmt.Sprintf(
		"Waiting for migration job to finish. Active jobs: %d, Failed jobs: %d",
		migration.Status.Active,
		migration.Status.Failed,
	)

	return v1.StateMigration
}

func (r *StoreReconciler) stateInitializing(
	ctx context.Context,
	store *v1.Store,
) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateInitializing,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Waiting for ingress to finish",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	deployment, err := deployment.GetStorefrontDeployment(ctx, *store, r.Client)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.StateInitializing
		}
		con.Reason = err.Error()
		con.Status = Error
		return v1.StateInitializing
	}

	// This can be nil, even if err is not nil :shrug:
	if deployment == nil {
		return v1.StateInitializing
	}

	if deployment.Status.ReadyReplicas == store.Spec.Container.Replicas {
		con.Message = "Initialization finished"
		con.Status = Ready
		con.LastTransitionTime = metav1.Now()
		return v1.StateReady
	}

	con.Message = fmt.Sprintf(
		"Waiting for deployment to get ready. Target replicas: %d, Ready replicas: %d",
		store.Spec.Container.Replicas,
		deployment.Status.ReadyReplicas,
	)

	return v1.StateInitializing
}

func (r *StoreReconciler) stateReady(ctx context.Context, store *v1.Store) v1.StatefulAppState {
	con := v1.StoreCondition{
		Type:               v1.StateReady,
		LastTransitionTime: metav1.Time{},
		LastUpdateTime:     metav1.Now(),
		Message:            "Store is running waiting for image updates to migrate",
		Reason:             "",
		Status:             "True",
	}
	defer func() {
		store.Status.AddCondition(con)
	}()

	currentImage, err := deployment.GetStoreDeploymentImage(ctx, *store, r.Client)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.StateInitializing
		}
		con.Status = Error
		con.Reason = fmt.Sprintf("get deployment: %s", err.Error())
		return v1.StateReady
	}

	if currentImage == store.Spec.Container.Image {
		return v1.StateReady
	} else {
		log.FromContext(ctx).
			WithValues("currentImage", currentImage, "containerImage", store.Spec.Container.Image).
			Info("Change to state migration")
		return v1.StateMigration
	}
}

func writeStoreStatus(
	ctx context.Context,
	cl client.Client,
	nn types.NamespacedName,
	status v1.StoreStatus,
) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		cr := &v1.Store{}
		if err := cl.Get(ctx, nn, cr); err != nil {
			return fmt.Errorf("write status: %w", err)
		}

		cr.Status = status
		return cl.Status().Update(ctx, cr)
	})
}
