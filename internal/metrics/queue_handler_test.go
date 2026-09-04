package metrics_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestQueueCountHandler(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))

	store := &v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"},
		Status: v1.StoreStatus{
			QueueState: v1.QueueCondition{
				Transports: []v1.QueueTransportStats{
					{Name: "async", Count: 42},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store).Build()
	handler := metrics.NewQueueStats(c, nil, nil, time.Minute).Handler()

	t.Run("returns queue count", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/queue/test/test-store/async", nil))

		require.Equal(t, 200, rec.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(42), resp["count"])
	})

	t.Run("unknown queue returns 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/queue/test/test-store/unknown", nil))
		assert.Equal(t, 404, rec.Code)
	})

	t.Run("unknown store returns 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/queue/test/other/async", nil))
		assert.Equal(t, 404, rec.Code)
	})

	t.Run("invalid path returns 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/queue/test", nil))
		assert.Equal(t, 400, rec.Code)
	})
}
