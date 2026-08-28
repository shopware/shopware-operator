package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const QueueHandlerPath = "/api/queue/"

type queueCountResponse struct {
	Store     string `json:"store"`
	Namespace string `json:"namespace"`
	Queue     string `json:"queue"`
	Count     int64  `json:"count"`
}

type queueCacheEntry struct {
	fetchedAt  time.Time
	transports []v1.QueueTransportStats
}

// QueueStats serves live queue lengths for the KEDA metrics-api scaler. The
// counts are fetched from the admin pod on demand and cached for a short TTL,
// so freshness is driven by the scaler polling instead of the reconcile
// interval of the store.
type QueueStats struct {
	Client     client.Client
	Clientset  *kubernetes.Clientset
	RestConfig *rest.Config
	TTL        time.Duration

	mu    sync.Mutex
	cache map[types.NamespacedName]queueCacheEntry
}

func NewQueueStats(
	c client.Client,
	clientset *kubernetes.Clientset,
	restConfig *rest.Config,
	ttl time.Duration,
) *QueueStats {
	return &QueueStats{
		Client:     c,
		Clientset:  clientset,
		RestConfig: restConfig,
		TTL:        ttl,
		cache:      make(map[types.NamespacedName]queueCacheEntry),
	}
}

// Handler serves GET /api/queue/<namespace>/<store>/<queue> as JSON.
func (q *QueueStats) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, QueueHandlerPath), "/"), "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			http.Error(w, "expected path /api/queue/<namespace>/<store>/<queue>", http.StatusBadRequest)
			return
		}
		namespace, name, queue := parts[0], parts[1], parts[2]

		store := &v1.Store{}
		if err := q.Client.Get(r.Context(), types.NamespacedName{Namespace: namespace, Name: name}, store); err != nil {
			if k8serrors.IsNotFound(err) {
				http.Error(w, "store not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, transport := range q.transports(r.Context(), store) {
			if transport.Name != queue {
				continue
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(queueCountResponse{
				Store:     name,
				Namespace: namespace,
				Queue:     queue,
				Count:     transport.Count,
			})
			return
		}

		http.Error(w, "queue not found for store", http.StatusNotFound)
	})
}

func (q *QueueStats) transports(ctx context.Context, store *v1.Store) []v1.QueueTransportStats {
	if q.Clientset == nil || q.RestConfig == nil {
		return store.Status.QueueState.Transports
	}

	nn := types.NamespacedName{Namespace: store.Namespace, Name: store.Name}

	q.mu.Lock()
	defer q.mu.Unlock()

	if entry, ok := q.cache[nn]; ok && time.Since(entry.fetchedAt) < q.TTL {
		return entry.transports
	}

	stats, _, err := deployment.GetAdminQueueStats(ctx, q.Client, q.Clientset, q.RestConfig, *store)
	if err != nil {
		// The last reconciled state is better than no answer for the scaler.
		return store.Status.QueueState.Transports
	}

	q.cache[nn] = queueCacheEntry{
		fetchedAt:  time.Now(),
		transports: stats,
	}
	UpdateQueueMetrics(store.Namespace, store.Name, stats)
	return stats
}
