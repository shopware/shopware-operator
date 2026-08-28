package manager_test

import (
	"context"
	"testing"
	"time"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/manager"
	"github.com/shopware/shopware-operator/internal/manager/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, kedav1alpha1.AddToScheme(scheme))
	return scheme
}

func testStore() *v1.Store {
	return &v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: v1.StoreSpec{
			SecretName: "store-secret",
			Container: v1.ContainerSpec{
				Image:    "shopware:6.7.0",
				Replicas: 1,
			},
			AdminCredentials: v1.Credentials{
				Username: "admin",
			},
			Database: v1.DatabaseSpec{
				Host: "mysql",
				Port: 3306,
				User: "shopware",
				Name: "shopware",
				PasswordSecretRef: v1.SecretRef{
					Name: "db-secret",
					Key:  "password",
				},
			},
		},
	}
}

func dbSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-secret",
			Namespace: "test",
		},
		Data: map[string][]byte{
			"password": []byte("secret"),
		},
	}
}

func newTestManager(t *testing.T, objs ...client.Object) (*manager.StoreStateManager, client.Client) {
	t.Helper()
	scheme := testScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
	m := manager.NewStoreStateManager(&base.Base{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
	})
	return m, c
}

func runningDeployments(store *v1.Store) []client.Object {
	workers, _ := deployment.WorkerDeployments(*store, false)
	all := make([]*appsv1.Deployment, 0, 2+len(workers))
	all = append(all, deployment.StorefrontDeployment(*store), deployment.AdminDeployment(*store))
	all = append(all, workers...)
	objs := make([]client.Object, 0, len(all))
	for _, d := range all {
		d.Status = appsv1.DeploymentStatus{
			Replicas:          1,
			AvailableReplicas: 1,
		}
		objs = append(objs, d)
	}
	return objs
}

func TestReconcileStateEmptyWithDisabledChecks(t *testing.T) {
	store := testStore()
	store.Spec.DisableChecks = true
	m, _ := newTestManager(t)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateSetup, store.Status.State)
}

func TestReconcileStateEmptyWaitsForChecks(t *testing.T) {
	store := testStore()
	m, _ := newTestManager(t)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateWait, store.Status.State)
	assert.NotEmpty(t, store.Status.Conditions)
}

func TestReconcileStateWaitWithAllChecksDisabled(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateWait
	store.Spec.DisableDatabaseCheck = true
	store.Spec.DisableS3Check = true
	store.Spec.DisableFastlyCheck = true
	store.Spec.DisableOpensearchCheck = true
	m, _ := newTestManager(t)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateSetup, store.Status.State)
}

func TestReconcileStateSetupJobSucceeded(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateSetup

	setupJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.GetSetupJobName(*store),
			Namespace: store.Namespace,
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}
	m, _ := newTestManager(t, setupJob)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateInitializing, store.Status.State)
	assert.Equal(t, store.Spec.Container.Image, store.Status.CurrentImageTag)
}

func TestReconcileStateSetupJobPending(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateSetup
	m, _ := newTestManager(t)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateSetup, store.Status.State)
}

func TestReconcileStateInitializingToReady(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateInitializing
	m, _ := newTestManager(t, runningDeployments(store)...)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateReady, store.Status.State)
	assert.Equal(t, v1.DeploymentStateRunning, store.Status.StorefrontState.State)
	assert.Equal(t, v1.DeploymentStateRunning, store.Status.AdminState.State)
	assert.Equal(t, v1.DeploymentStateRunning, store.Status.WorkerState.State)
}

func TestReconcileStateInitializingWaitsForDeployments(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateInitializing
	m, _ := newTestManager(t)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateInitializing, store.Status.State)
}

func TestReconcileStateReadyDetectsImageChange(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = store.Spec.Container.Image

	oldStore := store.DeepCopy()
	oldStore.Spec.Container.Image = "shopware:6.6.0"
	objs := runningDeployments(oldStore)

	m, _ := newTestManager(t, objs...)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateMigration, store.Status.State)
	assert.Equal(t, "shopware:6.6.0", store.Status.CurrentImageTag)
}

func TestReconcileStateMigrationFinishedWithDuration(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateMigration
	store.Status.CurrentImageTag = "shopware:6.6.0"

	start := metav1.NewTime(time.Now().Add(-90 * time.Second))
	end := metav1.NewTime(time.Now())
	migrationJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.MigrationJob(*store).Name,
			Namespace: store.Namespace,
		},
		Status: batchv1.JobStatus{
			Succeeded:      1,
			StartTime:      &start,
			CompletionTime: &end,
		},
	}

	m, _ := newTestManager(t, migrationJob)

	m.ReconcileState(context.Background(), store)

	assert.Equal(t, v1.StateInitializing, store.Status.State)
	assert.Equal(t, store.Spec.Container.Image, store.Status.CurrentImageTag)

	var migrationCondition v1.StoreCondition
	for _, con := range store.Status.Conditions {
		if con.Type == string(v1.StateMigration) {
			migrationCondition = con
		}
	}
	assert.Contains(t, migrationCondition.Message, "Migration finished. (Duration 1m30s)")
}

func TestReconcileResourcesWaitOnlyCreatesInitResources(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateWait
	m, c := newTestManager(t)

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	pdbs := &policy.PodDisruptionBudgetList{}
	require.NoError(t, c.List(context.Background(), pdbs, client.InNamespace("test")))
	assert.Len(t, pdbs.Items, 3)

	deployments := &appsv1.DeploymentList{}
	require.NoError(t, c.List(context.Background(), deployments, client.InNamespace("test")))
	assert.Empty(t, deployments.Items)

	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "store-secret"}, secret)
	assert.True(t, k8serrors.IsNotFound(err), "store secret must not exist in wait state")
}

func TestReconcileResourcesSetupCreatesSecretAndJob(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateSetup
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	secret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: "store-secret"}, secret))
	assert.Contains(t, secret.Data, "database-url")

	setupJob := &batchv1.Job{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: job.GetSetupJobName(*store)}, setupJob))

	migrationJob := &batchv1.JobList{}
	require.NoError(t, c.List(context.Background(), migrationJob, client.InNamespace("test")))
	assert.Len(t, migrationJob.Items, 1, "only the setup job must exist")
}

func TestReconcileResourcesInitializingCreatesDeployments(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateInitializing
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	deployments := &appsv1.DeploymentList{}
	require.NoError(t, c.List(context.Background(), deployments, client.InNamespace("test")))
	assert.Len(t, deployments.Items, 3)
}

func TestReconcileResourcesMigrationCreatesJobAndSuspendsCron(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateMigration
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	migrationJob := &batchv1.Job{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: job.MigrationJob(*store).Name}, migrationJob))

	cron := &batchv1.CronJob{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: cronjob.ScheduledTaskJob(*store).Name}, cron))
	require.NotNil(t, cron.Spec.Suspend)
	assert.True(t, *cron.Spec.Suspend)
}

func TestReconcileResourcesReadyWithImageChangeCreatesMigrationJob(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = "shopware:6.6.0"
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	migrationJob := &batchv1.Job{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: job.MigrationJob(*store).Name}, migrationJob))

	services := &corev1.ServiceList{}
	require.NoError(t, c.List(context.Background(), services, client.InNamespace("test")))
	assert.Empty(t, services.Items, "no services while waiting for migration")
}

func TestReconcileResourcesCreatesWorkerPerQueue(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = store.Spec.Container.Image
	store.Status.QueueState.Transports = []v1.QueueTransportStats{
		{Name: "async", Count: 5},
		{Name: "failed", Count: 1},
		{Name: "low_priority", Count: 0},
	}

	staleWorker := deployment.WorkerDeployment(*store, "mail")
	scheme := testScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dbSecret(), staleWorker).
		Build()
	m := manager.NewStoreStateManager(&base.Base{
		Client:             c,
		Scheme:             scheme,
		Recorder:           record.NewFakeRecorder(100),
		EnableKeda:         true,
		OperatorMetricsURL: "http://shopware-operator.operator.svc.cluster.local:8080",
	})

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	workers := &appsv1.DeploymentList{}
	require.NoError(t, c.List(context.Background(), workers,
		client.InNamespace("test"),
		client.MatchingLabels{"shop.shopware.com/store.app": "shopware-worker"}))

	names := make([]string, 0, len(workers.Items))
	for _, d := range workers.Items {
		names = append(names, d.Name)
	}
	assert.ElementsMatch(t, []string{
		"test-store-store-worker-async",
		"test-store-store-worker-failed",
		"test-store-store-worker-low-priority",
	}, names, "one worker per transport, stale worker cleaned up")

	queueByName := map[string]string{
		"test-store-store-worker-async":        "async",
		"test-store-store-worker-failed":       "failed",
		"test-store-store-worker-low-priority": "low_priority",
	}
	for _, d := range workers.Items {
		assert.Contains(t, d.Spec.Template.Spec.Containers[0].Args[0],
			"messenger:consume "+queueByName[d.Name]+" --time-limit=300")
	}
}

func TestReconcileResourcesCombinedWorkerWithoutKeda(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateInitializing
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	worker := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: "test-store-store-worker"}, worker))
	assert.Contains(t, worker.Spec.Template.Spec.Containers[0].Args[0],
		"messenger:consume failed async low_priority --time-limit=300")
}

func TestReconcileResourcesCreatesScaledObjectsWhenKedaEnabled(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = store.Spec.Container.Image
	store.Status.QueueState.Transports = []v1.QueueTransportStats{
		{Name: "async", Count: 5},
		{Name: "low_priority", Count: 0},
	}

	scheme := testScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dbSecret()).
		Build()
	m := manager.NewStoreStateManager(&base.Base{
		Client:             c,
		Scheme:             scheme,
		Recorder:           record.NewFakeRecorder(100),
		EnableKeda:         true,
		OperatorMetricsURL: "http://shopware-operator.operator.svc.cluster.local:8080",
	})

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	so := &kedav1alpha1.ScaledObject{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: "test-store-store-worker-async"}, so))
	assert.Equal(t, "test-store-store-worker-async", so.Spec.ScaleTargetRef.Name)
	require.Len(t, so.Spec.Triggers, 1)
	assert.Equal(t, "metrics-api", so.Spec.Triggers[0].Type)
	assert.Equal(t,
		"http://shopware-operator.operator.svc.cluster.local:8080/api/queue/test/test-store/async",
		so.Spec.Triggers[0].Metadata["url"])
	assert.Equal(t, "count", so.Spec.Triggers[0].Metadata["valueLocation"])

	list := &kedav1alpha1.ScaledObjectList{}
	require.NoError(t, c.List(context.Background(), list, client.InNamespace("test")))
	assert.Len(t, list.Items, 2)
}

func TestReconcileResourcesNoScaledObjectsWhenKedaDisabled(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = store.Spec.Container.Image
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	list := &kedav1alpha1.ScaledObjectList{}
	require.NoError(t, c.List(context.Background(), list, client.InNamespace("test")))
	assert.Empty(t, list.Items)
}

func TestReconcileResourcesReadyCreatesAllResources(t *testing.T) {
	store := testStore()
	store.Status.State = v1.StateReady
	store.Status.CurrentImageTag = store.Spec.Container.Image
	m, c := newTestManager(t, dbSecret())

	require.NoError(t, m.ReconcileResources(context.Background(), store))

	deployments := &appsv1.DeploymentList{}
	require.NoError(t, c.List(context.Background(), deployments, client.InNamespace("test")))
	assert.Len(t, deployments.Items, 3)

	services := &corev1.ServiceList{}
	require.NoError(t, c.List(context.Background(), services, client.InNamespace("test")))
	assert.Len(t, services.Items, 2)

	cron := &batchv1.CronJob{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "test", Name: cronjob.ScheduledTaskJob(*store).Name}, cron))
	require.NotNil(t, cron.Spec.Suspend)
	assert.False(t, *cron.Spec.Suspend)

	jobs := &batchv1.JobList{}
	require.NoError(t, c.List(context.Background(), jobs, client.InNamespace("test")))
	assert.Empty(t, jobs.Items, "no jobs in steady ready state")
}
