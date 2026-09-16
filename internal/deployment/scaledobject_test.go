package deployment_test

import (
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
