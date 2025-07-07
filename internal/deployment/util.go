package deployment

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getDeploymentCondition(
	deployment *appsv1.Deployment,
) v1.DeploymentCondition {
	if deployment.Status.AvailableReplicas >= deployment.Status.Replicas {
		return v1.DeploymentCondition{
			State:          v1.DeploymentStateRunning,
			LastUpdateTime: metav1.Now(),
			Message:        "Deployment is running",
			Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, deployment.Status.Replicas),
		}
	}

	if deployment.Status.UnavailableReplicas >= 0 {
		return v1.DeploymentCondition{
			State:          v1.DeploymentStateError,
			LastUpdateTime: metav1.Now(),
			Message:        "Deployment has UnavailableReplicas",
			Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, deployment.Status.Replicas),
		}
	}

	// TODO: log State Unknown with the status of the deployment to make a known status out of it

	return v1.DeploymentCondition{
		State:          v1.DeploymentStateUnknown,
		LastUpdateTime: metav1.Now(),
		Message:        "Unknown state of the deployment",
		Ready:          "0/0",
	}
}

func GetStoreDeploymentImage(
	ctx context.Context,
	store v1.Store,
	client client.Client,
) (string, error) {
	setup := StorefrontDeployment(store)
	search := &appsv1.Deployment{
		ObjectMeta: setup.ObjectMeta,
	}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: setup.Namespace,
		Name:      setup.Name,
	}, search)
	if err != nil {
		return "", err
	}

	for _, container := range search.Spec.Template.Spec.Containers {
		if container.Name == DEPLOYMENT_STOREFRONT_CONTAINER_NAME {
			return container.Image, nil
		}
	}
	return "", fmt.Errorf("could not find storefront deployment container")
}
