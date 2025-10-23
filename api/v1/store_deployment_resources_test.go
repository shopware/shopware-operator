package v1_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestDeploymentResourceIsolation tests that modifying store resources doesn't affect
// container spec overwrites for worker, admin, and storefront deployments.
// This test ensures that DeepCopy properly isolates resources between deployments.
func TestDeploymentResourceIsolation(t *testing.T) {
	// Create a store with base resources set to 1GiB
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:    "shopware:latest",
				Replicas: 2,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000m"),
					},
				},
			},
			// Configure worker with 2GiB
			WorkerDeploymentContainer: v1.ContainerMergeSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("2Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("2Gi"),
						corev1.ResourceCPU:    resource.MustParse("2000m"),
					},
				},
			},
			// Configure admin with 512Mi
			AdminDeploymentContainer: v1.ContainerMergeSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
						corev1.ResourceCPU:    resource.MustParse("250m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
				},
			},
			// Configure storefront with 3GiB
			StorefrontDeploymentContainer: v1.ContainerMergeSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("3Gi"),
						corev1.ResourceCPU:    resource.MustParse("1500m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("3Gi"),
						corev1.ResourceCPU:    resource.MustParse("3000m"),
					},
				},
			},
		},
	}

	// Generate deployments
	adminDeploy := deployment.AdminDeployment(store)
	workerDeploy := deployment.WorkerDeployment(store)
	storefrontDeploy := deployment.StorefrontDeployment(store)

	// Verify Admin deployment has 512Mi resources
	adminContainer := findMainContainer(adminDeploy.Spec.Template.Spec.Containers, "shopware-admin")
	require.NotNil(t, adminContainer, "Admin container should exist")
	assert.Equal(t,
		resource.MustParse("512Mi"),
		adminContainer.Resources.Requests[corev1.ResourceMemory],
		"Admin memory request should be 512Mi")
	assert.Equal(t,
		resource.MustParse("250m"),
		adminContainer.Resources.Requests[corev1.ResourceCPU],
		"Admin CPU request should be 250m")
	assert.Equal(t,
		resource.MustParse("512Mi"),
		adminContainer.Resources.Limits[corev1.ResourceMemory],
		"Admin memory limit should be 512Mi")
	assert.Equal(t,
		resource.MustParse("500m"),
		adminContainer.Resources.Limits[corev1.ResourceCPU],
		"Admin CPU limit should be 500m")

	// Verify Worker deployment has 2GiB resources
	workerContainer := findMainContainer(workerDeploy.Spec.Template.Spec.Containers, "shopware-worker")
	require.NotNil(t, workerContainer, "Worker container should exist")
	assert.Equal(t,
		resource.MustParse("2Gi"),
		workerContainer.Resources.Requests[corev1.ResourceMemory],
		"Worker memory request should be 2Gi")
	assert.Equal(t,
		resource.MustParse("1000m"),
		workerContainer.Resources.Requests[corev1.ResourceCPU],
		"Worker CPU request should be 1000m")
	assert.Equal(t,
		resource.MustParse("2Gi"),
		workerContainer.Resources.Limits[corev1.ResourceMemory],
		"Worker memory limit should be 2Gi")
	assert.Equal(t,
		resource.MustParse("2000m"),
		workerContainer.Resources.Limits[corev1.ResourceCPU],
		"Worker CPU limit should be 2000m")

	// Verify Storefront deployment has 3GiB resources
	storefrontContainer := findMainContainer(storefrontDeploy.Spec.Template.Spec.Containers, "shopware-storefront")
	require.NotNil(t, storefrontContainer, "Storefront container should exist")
	assert.Equal(t,
		resource.MustParse("3Gi"),
		storefrontContainer.Resources.Requests[corev1.ResourceMemory],
		"Storefront memory request should be 3Gi")
	assert.Equal(t,
		resource.MustParse("1500m"),
		storefrontContainer.Resources.Requests[corev1.ResourceCPU],
		"Storefront CPU request should be 1500m")
	assert.Equal(t,
		resource.MustParse("3Gi"),
		storefrontContainer.Resources.Limits[corev1.ResourceMemory],
		"Storefront memory limit should be 3Gi")
	assert.Equal(t,
		resource.MustParse("3000m"),
		storefrontContainer.Resources.Limits[corev1.ResourceCPU],
		"Storefront CPU limit should be 3000m")

	// Additional verification: Ensure that none of the deployments have the base 1Gi resource
	// (unless that's what was explicitly configured for that deployment)
	assert.NotEqual(t,
		resource.MustParse("1Gi"),
		workerContainer.Resources.Requests[corev1.ResourceMemory],
		"Worker should not have base store resource of 1Gi")
	assert.NotEqual(t,
		resource.MustParse("1Gi"),
		storefrontContainer.Resources.Requests[corev1.ResourceMemory],
		"Storefront should not have base store resource of 1Gi")
}

// TestDeploymentResourceIsolationWithPartialOverrides tests that partial resource
// overrides work correctly - only overriding some resources while keeping others
func TestDeploymentResourceIsolationWithPartialOverrides(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:    "shopware:latest",
				Replicas: 2,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000m"),
					},
				},
			},
			// Worker only overrides memory, CPU should come from base
			WorkerDeploymentContainer: v1.ContainerMergeSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}

	workerDeploy := deployment.WorkerDeployment(store)
	workerContainer := findMainContainer(workerDeploy.Spec.Template.Spec.Containers, "shopware-worker")
	require.NotNil(t, workerContainer, "Worker container should exist")

	// Memory should be overridden to 2Gi
	assert.Equal(t,
		resource.MustParse("2Gi"),
		workerContainer.Resources.Requests[corev1.ResourceMemory],
		"Worker memory request should be overridden to 2Gi")
	assert.Equal(t,
		resource.MustParse("2Gi"),
		workerContainer.Resources.Limits[corev1.ResourceMemory],
		"Worker memory limit should be overridden to 2Gi")

	// CPU should come from base spec
	assert.Equal(t,
		resource.MustParse("500m"),
		workerContainer.Resources.Requests[corev1.ResourceCPU],
		"Worker CPU request should inherit base 500m")
	assert.Equal(t,
		resource.MustParse("1000m"),
		workerContainer.Resources.Limits[corev1.ResourceCPU],
		"Worker CPU limit should inherit base 1000m")
}

// TestDeploymentResourceIsolationNoOverrides tests that when no overrides are
// specified, deployments get the base resources
func TestDeploymentResourceIsolationNoOverrides(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "default",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image:    "shopware:latest",
				Replicas: 2,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("1000m"),
					},
				},
			},
			// No overrides specified
		},
	}

	adminDeploy := deployment.AdminDeployment(store)
	workerDeploy := deployment.WorkerDeployment(store)
	storefrontDeploy := deployment.StorefrontDeployment(store)

	// All deployments should have base resources
	adminContainer := findMainContainer(adminDeploy.Spec.Template.Spec.Containers, "shopware-admin")
	workerContainer := findMainContainer(workerDeploy.Spec.Template.Spec.Containers, "shopware-worker")
	storefrontContainer := findMainContainer(storefrontDeploy.Spec.Template.Spec.Containers, "shopware-storefront")

	require.NotNil(t, adminContainer)
	require.NotNil(t, workerContainer)
	require.NotNil(t, storefrontContainer)

	for _, container := range []*corev1.Container{adminContainer, workerContainer, storefrontContainer} {
		assert.Equal(t,
			resource.MustParse("1Gi"),
			container.Resources.Requests[corev1.ResourceMemory],
			"Container should have base memory request of 1Gi")
		assert.Equal(t,
			resource.MustParse("500m"),
			container.Resources.Requests[corev1.ResourceCPU],
			"Container should have base CPU request of 500m")
		assert.Equal(t,
			resource.MustParse("1Gi"),
			container.Resources.Limits[corev1.ResourceMemory],
			"Container should have base memory limit of 1Gi")
		assert.Equal(t,
			resource.MustParse("1000m"),
			container.Resources.Limits[corev1.ResourceCPU],
			"Container should have base CPU limit of 1000m")
	}
}

// Helper function to find the main container in a pod spec by name
func findMainContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}
