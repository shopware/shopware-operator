package deployment

import (
	"context"
	"fmt"
	"math"

	v1 "github.com/shopware/shopware-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	startServersRatio    = 0.25
	minSpareServersRatio = 0.2
	maxSpareServersRatio = 0.375
	memoryPerChildMiB    = 80 //Every PHP-FPM process in an empty shop uses 70.6MiB
)

func getDeploymentCondition(
	deployment *appsv1.Deployment,
	storeReplicas int32,
) v1.DeploymentCondition {
	if deployment.Status.AvailableReplicas == deployment.Status.Replicas {

		// This happens if you use a hpa or scaling the deployment manually
		if deployment.Status.AvailableReplicas != storeReplicas {
			return v1.DeploymentCondition{
				State:          v1.DeploymentStateRunning,
				LastUpdateTime: metav1.Now(),
				Message:        "Deployment is running, but has own scaling",
				Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, storeReplicas),
				StoreReplicas:  storeReplicas,
			}
		}

		return v1.DeploymentCondition{
			State:          v1.DeploymentStateRunning,
			LastUpdateTime: metav1.Now(),
			Message:        "Deployment is running",
			Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, storeReplicas),
			StoreReplicas:  storeReplicas,
		}
	}

	if deployment.Status.AvailableReplicas != deployment.Status.Replicas {
		return v1.DeploymentCondition{
			State:          v1.DeploymentStateScaling,
			LastUpdateTime: metav1.Now(),
			Message:        "Deployment is scaling",
			Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, storeReplicas),
			StoreReplicas:  storeReplicas,
		}
	}

	if deployment.Status.UnavailableReplicas > 0 {
		return v1.DeploymentCondition{
			State:          v1.DeploymentStateError,
			LastUpdateTime: metav1.Now(),
			Message:        "Deployment has UnavailableReplicas",
			Ready:          fmt.Sprintf("%d/%d", deployment.Status.AvailableReplicas, storeReplicas),
			StoreReplicas:  storeReplicas,
		}
	}

	// TODO: log State Unknown with the status of the deployment to make a known status out of it

	return v1.DeploymentCondition{
		State:          v1.DeploymentStateUnknown,
		LastUpdateTime: metav1.Now(),
		Message:        "Unknown state of the deployment",
		Ready:          "0/0",
		StoreReplicas:  storeReplicas,
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

func GetCalculatedPHPFPMValues(memoryLimitMiB int) []corev1.EnvVar {
	maxChildren := int(math.Round(float64(memoryLimitMiB / memoryPerChildMiB)))
	startServers := int(math.Round(float64(maxChildren) * startServersRatio))
	minSpare := int(math.Round(float64(maxChildren) * minSpareServersRatio))
	maxSpare := int(math.Round(float64(maxChildren)*maxSpareServersRatio) + 1)

	return []corev1.EnvVar{
		{
			Name:  "FPM_PM",
			Value: "dynamic",
		},
		{
			Name:  "FPM_PM_MAX_CHILDREN",
			Value: fmt.Sprintf("%d", maxChildren),
		},
		{
			Name:  "FPM_PM_START_SERVERS",
			Value: fmt.Sprintf("%d", startServers),
		},
		{
			Name:  "FPM_PM_MIN_SPARE_SERVERS",
			Value: fmt.Sprintf("%d", minSpare),
		},
		{
			Name:  "FPM_PM_MAX_SPARE_SERVERS",
			Value: fmt.Sprintf("%d", maxSpare),
		},
	}
}
