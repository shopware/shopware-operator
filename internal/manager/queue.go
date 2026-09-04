package manager

import (
	"context"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/metrics"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *StoreStateManager) UpdateQueueState(ctx context.Context, store *v1.Store) {
	if m.Clientset == nil || m.RestConfig == nil {
		return
	}
	if !store.IsState(v1.StateReady) {
		return
	}
	if store.Status.AdminState.State != v1.DeploymentStateRunning {
		return
	}

	stats, uncountable, err := deployment.GetAdminQueueStats(ctx, m.Client, m.Clientset, m.RestConfig, *store)
	if err != nil {
		logging.FromContext(ctx).Debugw("failed to get queue stats from admin pod", zap.Error(err))
		store.Status.QueueState = v1.QueueCondition{
			LastUpdateTime: metav1.Now(),
			Error:          err.Error(),
		}
		return
	}

	store.Status.QueueState = v1.QueueCondition{
		LastUpdateTime:        metav1.Now(),
		Transports:            stats,
		UncountableTransports: uncountable,
	}
	metrics.UpdateQueueMetrics(store.Namespace, store.Name, stats)
}
