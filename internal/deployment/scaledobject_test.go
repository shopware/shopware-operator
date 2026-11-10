package deployment_test

import (
	"strings"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkerScaledObjectUsesWorkerSpec(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"},
		Spec: v1.StoreSpec{
			Worker: v1.WorkerSpec{
				MinReplicas:       2,
				MaxReplicas:       8,
				CooldownPeriod:    60,
				PollingInterval:   5,
				TargetQueueLength: 250,
			},
		},
	}

	so := deployment.WorkerScaledObject(store, "async", "http://metrics")

	assert.Equal(t, int32(2), *so.Spec.MinReplicaCount)
	assert.Equal(t, int32(8), *so.Spec.MaxReplicaCount)
	assert.Equal(t, int32(60), *so.Spec.CooldownPeriod)
	assert.Equal(t, int32(5), *so.Spec.PollingInterval)
	assert.Equal(t, "250", so.Spec.Triggers[0].Metadata["targetValue"])
	assert.Equal(t, "http://metrics/api/queue/test/test-store/async", so.Spec.Triggers[0].Metadata["url"])
}

func TestWorkerScaledObjectHPANameWithinLimit(t *testing.T) {
	t.Run("short name keeps keda default", func(t *testing.T) {
		store := v1.Store{ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"}}

		so := deployment.WorkerScaledObject(store, "async", "http://metrics")

		assert.Empty(t, so.Spec.Advanced.HorizontalPodAutoscalerConfig.Name)
	})

	t.Run("long name gets truncated hpa name", func(t *testing.T) {
		store := v1.Store{ObjectMeta: metav1.ObjectMeta{Name: "a-very-long-store-name-for-testing", Namespace: "test"}}

		so := deployment.WorkerScaledObject(store, "low_priority", "http://metrics")

		hpaName := so.Spec.Advanced.HorizontalPodAutoscalerConfig.Name
		assert.Greater(t, len("keda-hpa-"+so.Name), 63)
		assert.NotEmpty(t, hpaName)
		assert.LessOrEqual(t, len(hpaName), 63)
		assert.True(t, strings.HasPrefix(hpaName, "keda-hpa-"))
	})
}
