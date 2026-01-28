package deployment

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RolloutInProgress(ctx context.Context, c client.Client, store *v1.Store) (bool, error) {
	deployments := &appsv1.DeploymentList{}
	if err := c.List(ctx, deployments, client.InNamespace(store.Namespace)); err != nil {
		return false, fmt.Errorf("list deployments: %w", err)
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]
		if metav1.IsControlledBy(d, store) && isRollingOut(d) {
			return true, nil
		}
	}
	return false, nil
}

func isRollingOut(d *appsv1.Deployment) bool {
	if progressDeadlineExceeded(d) {
		return false
	}
	if d.Generation > d.Status.ObservedGeneration {
		return true
	}
	if d.Spec.Replicas != nil && d.Status.UpdatedReplicas < *d.Spec.Replicas {
		return true
	}
	return d.Status.Replicas > d.Status.UpdatedReplicas ||
		d.Status.AvailableReplicas < d.Status.UpdatedReplicas
}
