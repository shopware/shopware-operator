package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newStore() *v1.Store {
	return &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
	}
}

func TestUpdateStoreMetricsState(t *testing.T) {
	store := newStore()
	store.Status.State = v1.StateReady

	UpdateStoreMetrics(store)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(1), testutil.ToFloat64(storeState.WithLabelValues("shop", "default", string(v1.StateReady))))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeState.WithLabelValues("shop", "default", string(v1.StateWait))))
}

func TestUpdateStoreMetricsUsageDataConsent(t *testing.T) {
	store := newStore()
	store.Spec.ShopConfiguration.UsageDataConsent = "allowed"

	UpdateStoreMetrics(store)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(1), testutil.ToFloat64(storeUsageDataConsent.WithLabelValues("shop", "default")))

	store.Spec.ShopConfiguration.UsageDataConsent = "revoked"
	UpdateStoreMetrics(store)

	assert.Equal(t, float64(0), testutil.ToFloat64(storeUsageDataConsent.WithLabelValues("shop", "default")))
}

func TestUpdateStoreMetricsHPA(t *testing.T) {
	store := newStore()
	minReplicas := int32(2)
	store.Spec.HorizontalPodAutoscaler = v1.HPASpec{
		Enabled:     true,
		MinReplicas: &minReplicas,
		MaxReplicas: 5,
	}

	UpdateStoreMetrics(store)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(1), testutil.ToFloat64(storeHPAEnabled.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(2), testutil.ToFloat64(storeHPAMinReplicas.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(5), testutil.ToFloat64(storeHPAMaxReplicas.WithLabelValues("shop", "default")))

	store.Spec.HorizontalPodAutoscaler = v1.HPASpec{Enabled: false}
	UpdateStoreMetrics(store)

	assert.Equal(t, float64(0), testutil.ToFloat64(storeHPAEnabled.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeHPAMinReplicas.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeHPAMaxReplicas.WithLabelValues("shop", "default")))
}

func TestSetDeploymentMetricsParsesReady(t *testing.T) {
	store := newStore()
	store.Status.AdminState = v1.DeploymentCondition{
		State:         v1.DeploymentStateRunning,
		Ready:         "2/3",
		StoreReplicas: 3,
	}

	UpdateStoreMetrics(store)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(2), testutil.ToFloat64(storeDeploymentReplicasAvailable.WithLabelValues("shop", "default", "admin")))
	assert.Equal(t, float64(3), testutil.ToFloat64(storeDeploymentReplicasDesired.WithLabelValues("shop", "default", "admin")))
	assert.Equal(t, float64(1), testutil.ToFloat64(storeDeploymentState.WithLabelValues("shop", "default", "admin", string(v1.DeploymentStateRunning))))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeDeploymentState.WithLabelValues("shop", "default", "admin", string(v1.DeploymentStateError))))
}

func TestUpdateScheduledTaskMetricsNilCronJob(t *testing.T) {
	store := newStore()

	UpdateScheduledTaskMetrics(store, nil)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(0), testutil.ToFloat64(storeScheduledTaskSuspended.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeScheduledTaskLastRunStatus.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(0), testutil.ToFloat64(storeScheduledTaskLastSuccessTime.WithLabelValues("shop", "default")))
}

func TestUpdateScheduledTaskMetricsSuspended(t *testing.T) {
	store := newStore()
	suspend := true
	cronJob := &batchv1.CronJob{Spec: batchv1.CronJobSpec{Suspend: &suspend}}

	UpdateScheduledTaskMetrics(store, cronJob)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(1), testutil.ToFloat64(storeScheduledTaskSuspended.WithLabelValues("shop", "default")))
}

func TestUpdateScheduledTaskMetricsLastRunSuccess(t *testing.T) {
	store := newStore()
	scheduleTime := metav1.NewTime(time.Unix(100, 0))
	successTime := metav1.NewTime(time.Unix(200, 0))
	cronJob := &batchv1.CronJob{Status: batchv1.CronJobStatus{
		LastScheduleTime:   &scheduleTime,
		LastSuccessfulTime: &successTime,
	}}

	UpdateScheduledTaskMetrics(store, cronJob)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(1), testutil.ToFloat64(storeScheduledTaskLastRunStatus.WithLabelValues("shop", "default")))
	assert.Equal(t, float64(200), testutil.ToFloat64(storeScheduledTaskLastSuccessTime.WithLabelValues("shop", "default")))
}

func TestUpdateScheduledTaskMetricsLastRunFailed(t *testing.T) {
	store := newStore()
	successTime := metav1.NewTime(time.Unix(100, 0))
	scheduleTime := metav1.NewTime(time.Unix(200, 0))
	cronJob := &batchv1.CronJob{Status: batchv1.CronJobStatus{
		LastScheduleTime:   &scheduleTime,
		LastSuccessfulTime: &successTime,
	}}

	UpdateScheduledTaskMetrics(store, cronJob)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(-1), testutil.ToFloat64(storeScheduledTaskLastRunStatus.WithLabelValues("shop", "default")))
}

func TestUpdateScheduledTaskMetricsLastRunInProgress(t *testing.T) {
	store := newStore()
	successTime := metav1.NewTime(time.Unix(100, 0))
	scheduleTime := metav1.NewTime(time.Unix(200, 0))
	cronJob := &batchv1.CronJob{Status: batchv1.CronJobStatus{
		LastScheduleTime:   &scheduleTime,
		LastSuccessfulTime: &successTime,
		Active:             []corev1.ObjectReference{{Name: "shop-scheduled-task-1"}},
	}}

	UpdateScheduledTaskMetrics(store, cronJob)
	defer RemoveStoreMetrics(store)

	assert.Equal(t, float64(0), testutil.ToFloat64(storeScheduledTaskLastRunStatus.WithLabelValues("shop", "default")))
}

func TestFmtScan(t *testing.T) {
	var available, desired int
	fmtScan("4/7", &available, &desired)

	assert.Equal(t, 4, available)
	assert.Equal(t, 7, desired)
}
