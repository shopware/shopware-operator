package deployment_test

import (
	"strings"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetQueueWorkerDeploymentName(t *testing.T) {
	store := v1.Store{ObjectMeta: metav1.ObjectMeta{Name: "test-store"}}

	t.Run("short queue keeps full name", func(t *testing.T) {
		assert.Equal(t, "test-store-store-worker-low-priority",
			deployment.GetQueueWorkerDeploymentName(store, "low_priority"))
	})

	t.Run("long queue is truncated to 63 chars with hash", func(t *testing.T) {
		longQueue := strings.Repeat("very_long_queue_", 5)
		name := deployment.GetQueueWorkerDeploymentName(store, longQueue)
		assert.LessOrEqual(t, len(name), 63)
		assert.True(t, strings.HasPrefix(name, "test-store-store-worker-very-long-queue-"))
	})

	t.Run("long queues stay unique and deterministic", func(t *testing.T) {
		queueA := strings.Repeat("very_long_queue_", 5) + "a"
		queueB := strings.Repeat("very_long_queue_", 5) + "b"
		nameA := deployment.GetQueueWorkerDeploymentName(store, queueA)
		nameB := deployment.GetQueueWorkerDeploymentName(store, queueB)
		assert.NotEqual(t, nameA, nameB)
		assert.Equal(t, nameA, deployment.GetQueueWorkerDeploymentName(store, queueA))
	})
}
