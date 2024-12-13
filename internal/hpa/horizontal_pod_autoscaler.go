package hpa

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/util"
	autoscaling "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStoreHPA(
	ctx context.Context,
	store *v1.Store,
	client client.Client,
) (*autoscaling.HorizontalPodAutoscaler, error) {
	hpa := StoreHPA(store)
	search := &autoscaling.HorizontalPodAutoscaler{
		ObjectMeta: hpa.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: hpa.Namespace,
		Name:      hpa.Name,
	}, search)
	return search, err
}

func StoreHPA(store *v1.Store) *autoscaling.HorizontalPodAutoscaler {
	dep := deployment.StorefrontDeployment(store)

	if len(store.Spec.HorizontalPodAutoscaler.Metrics) == 0 {
		util := int32(70)
		store.Spec.HorizontalPodAutoscaler.Metrics = append(
			store.Spec.HorizontalPodAutoscaler.Metrics,
			autoscaling.MetricSpec{
				Type: autoscaling.ResourceMetricSourceType,
				Resource: &autoscaling.ResourceMetricSource{
					Name: "cpu",
					Target: autoscaling.MetricTarget{
						Type:               "Utilization",
						AverageUtilization: &util,
					},
				},
			},
		)
	}

	return &autoscaling.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetStoreHPAName(store),
			Namespace:   store.GetNamespace(),
			Labels:      util.GetDefaultStoreLabels(store),
			Annotations: store.Spec.HorizontalPodAutoscaler.Annotations,
		},
		Spec: autoscaling.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscaling.CrossVersionObjectReference{
				Kind:       dep.Kind,
				Name:       dep.Name,
				APIVersion: dep.APIVersion,
			},
			MinReplicas: store.Spec.HorizontalPodAutoscaler.MinReplicas,
			MaxReplicas: store.Spec.HorizontalPodAutoscaler.MaxReplicas,
			Metrics:     store.Spec.HorizontalPodAutoscaler.Metrics,
			Behavior:    store.Spec.HorizontalPodAutoscaler.Behavior,
		},
	}
}

func GetStoreHPAName(store *v1.Store) string {
	return fmt.Sprintf("store-%s", store.Name)
}
