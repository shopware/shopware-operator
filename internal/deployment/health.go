package deployment

import (
	"context"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const crashLoopBackOffReason = "CrashLoopBackOff"

func GetCrashLoopBackOffPods(
	ctx context.Context,
	store v1.Store,
	c client.Client,
) ([]string, error) {
	var crashing []string

	workers, err := WorkerDeployments(store)
	if err != nil {
		return nil, err
	}

	deployments := []*appsv1.Deployment{
		StorefrontDeployment(store),
		AdminDeployment(store),
	}
	deployments = append(deployments, workers...)

	for _, d := range deployments {
		search := &appsv1.Deployment{}
		if err := c.Get(ctx, types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}, search); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}

		selector, err := metav1.LabelSelectorAsSelector(search.Spec.Selector)
		if err != nil {
			return nil, err
		}

		pods := &corev1.PodList{}
		if err := c.List(ctx, pods,
			client.InNamespace(search.Namespace),
			client.MatchingLabelsSelector{Selector: selector},
		); err != nil {
			return nil, err
		}

		for _, pod := range pods.Items {
			statuses := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)
			for _, cs := range statuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason == crashLoopBackOffReason {
					crashing = append(crashing, pod.Name)
					break
				}
			}
		}
	}

	return crashing, nil
}
