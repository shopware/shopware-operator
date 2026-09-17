package manager

import (
	"context"
	"errors"

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
		log := logging.FromContext(ctx)
		var statsErr *deployment.QueueStatsError
		if errors.As(err, &statsErr) {
			log = log.With(
				zap.String("pod", statsErr.Pod),
				zap.String("container", statsErr.Container),
				zap.String("command", statsErr.Command),
				zap.String("stdout", statsErr.Stdout),
				zap.String("stderr", statsErr.Stderr),
			)
		}
		log.Errorw("failed to get queue stats from admin pod, no worker deployments will be created", zap.Error(err))
		queueState := store.Status.QueueState
		queueState.LastUpdateTime = metav1.Now()
		queueState.Error = err.Error()
		store.Status.QueueState = queueState
		return
	}

	store.Status.QueueState = v1.QueueCondition{
		LastUpdateTime:        metav1.Now(),
		Transports:            stats,
		UncountableTransports: uncountable,
	}
	metrics.UpdateQueueMetrics(store.Namespace, store.Name, stats)
}
