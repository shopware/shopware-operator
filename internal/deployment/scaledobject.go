package deployment

import (
	"context"
	"fmt"
	"strconv"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WorkerScaledObjects(store v1.Store, metricsURL string) ([]*kedav1alpha1.ScaledObject, error) {
	queues := WorkerQueues(store)
	objects := make([]*kedav1alpha1.ScaledObject, 0, len(queues))
	for _, queue := range queues {
		if queue == "" {
			return nil, fmt.Errorf("empty queue name for store %s/%s", store.Namespace, store.Name)
		}
		objects = append(objects, WorkerScaledObject(store, queue, metricsURL))
	}
	return objects, nil
}

func WorkerScaledObject(store v1.Store, queue string, metricsURL string) *kedav1alpha1.ScaledObject {
	worker := store.Spec.Worker

	labels := util.GetDefaultStoreLabels(store)
	labels[util.ShopwareKey("store.app")] = "shopware-worker"
	labels[util.ShopwareKey("worker.queue")] = truncateWithHash(queue)

	return &kedav1alpha1.ScaledObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ScaledObject",
			APIVersion: "keda.sh/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetQueueWorkerDeploymentName(store, queue),
			Namespace: store.Namespace,
			Labels:    labels,
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				Name: GetQueueWorkerDeploymentName(store, queue),
			},
			MinReplicaCount: &worker.MinReplicas,
			MaxReplicaCount: &worker.MaxReplicas,
			CooldownPeriod:  &worker.CooldownPeriod,
			PollingInterval: &worker.PollingInterval,
			Advanced: &kedav1alpha1.AdvancedConfig{
				HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							Policies: []autoscalingv2.HPAScalingPolicy{
								{Type: "Pods", Value: 1, PeriodSeconds: 10},
							},
						},
					},
				},
			},
			Triggers: []kedav1alpha1.ScaleTriggers{
				{
					Type: "metrics-api",
					Metadata: map[string]string{
						"url": fmt.Sprintf("%s/api/queue/%s/%s/%s",
							metricsURL, store.Namespace, store.Name, queue),
						"valueLocation": "count",
						"targetValue":   strconv.Itoa(int(worker.TargetQueueLength)),
					},
				},
			},
		},
	}
}

func CleanupObsoleteWorkerScaledObjects(
	ctx context.Context,
	c client.Client,
	store v1.Store,
	desiredNames map[string]struct{},
) error {
	list := &kedav1alpha1.ScaledObjectList{}
	if err := c.List(ctx, list,
		client.InNamespace(store.Namespace),
		client.MatchingLabels(util.GetWorkerDeploymentMatchLabel()),
	); err != nil {
		return fmt.Errorf("list worker scaledobjects: %w", err)
	}

	for i := range list.Items {
		so := &list.Items[i]
		if _, ok := desiredNames[so.Name]; ok {
			continue
		}
		if err := c.Delete(ctx, so); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete obsolete worker scaledobject %s: %w", so.Name, err)
		}
	}

	return nil
}
