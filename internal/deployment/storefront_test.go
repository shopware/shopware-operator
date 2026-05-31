package deployment_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStorefrontDeployment(t *testing.T) {
	t.Run("test annotation merging", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
					Annotations: map[string]string{
						"shared.key":     "container-value",
						"container.key":  "container-value",
						"container.only": "stays",
					},
				},
				StorefrontDeploymentContainer: v1.ContainerMergeSpec{
					Annotations: map[string]string{
						"shared.key":      "storefront-value",
						"storefront.key":  "storefront-value",
						"storefront.only": "added",
					},
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)

		// Verify annotations are merged correctly
		assert.Equal(t, "storefront-value", result.Annotations["shared.key"], "Shared key should be overwritten by storefront")
		assert.Equal(t, "container-value", result.Annotations["container.key"], "Container-specific key should be preserved")
		assert.Equal(t, "stays", result.Annotations["container.only"], "Container-only annotation should stay")
		assert.Equal(t, "storefront-value", result.Annotations["storefront.key"], "Storefront-specific key should be added")
		assert.Equal(t, "added", result.Annotations["storefront.only"], "Storefront-only annotation should be added")
	})

	t.Run("test container merge spec", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
					Replicas:        2,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"cpu": resource.MustParse("1"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "container-volume",
							MountPath: "/container",
						},
					},
					ExtraEnvs: []corev1.EnvVar{
						{
							Name:  "CONTAINER_ENV",
							Value: "value",
						},
					},
				},
				StorefrontDeploymentContainer: v1.ContainerMergeSpec{
					Image:           "shopware:storefront",
					ImagePullPolicy: "Always",
					Replicas:        3,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"memory": resource.MustParse("1Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "storefront-volume",
							MountPath: "/storefront",
						},
					},
					ExtraEnvs: []corev1.EnvVar{
						{
							Name:  "STOREFRONT_ENV",
							Value: "value",
						},
						{
							Name:  "CONTAINER_ENV",
							Value: "overwritten",
						},
					},
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify image and policy are overwritten
		assert.Equal(t, "shopware:storefront", container.Image)
		assert.Equal(t, corev1.PullPolicy("Always"), container.ImagePullPolicy)

		// Verify replicas
		assert.Equal(t, int32(3), *result.Spec.Replicas)

		// Verify resources are merged
		assert.Equal(t, resource.MustParse("1"), container.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits["memory"])

		// Verify volume mounts are merged
		assert.Len(t, container.VolumeMounts, 2)
		assert.Equal(t, "container-volume", container.VolumeMounts[0].Name)
		assert.Equal(t, "storefront-volume", container.VolumeMounts[1].Name)
		assert.Equal(t, "/storefront", container.VolumeMounts[1].MountPath)

		// Verify env vars are merged
		hasStorefrontEnv := false
		hasContainerEnv := false
		for _, env := range container.Env {
			if env.Name == "STOREFRONT_ENV" {
				hasStorefrontEnv = true
				assert.Equal(t, "value", env.Value)
			}
			if env.Name == "CONTAINER_ENV" {
				hasContainerEnv = true
				assert.Equal(t, "overwritten", env.Value)
			}
		}
		assert.True(t, hasStorefrontEnv, "Storefront env var should be present")
		assert.True(t, hasContainerEnv, "Container env var should be present and overwritten")
	})

	t.Run("test container security context merge", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: util.Int64(1000),
					},
				},
				StorefrontDeploymentContainer: v1.ContainerMergeSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsGroup: util.Int64(2000),
					},
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)

		// Verify pod security context is overwritten.
		assert.NotNil(t, result.Spec.Template.Spec.SecurityContext)
		assert.Equal(t, int64(2000), *result.Spec.Template.Spec.SecurityContext.RunAsGroup)
		assert.Nil(t, result.Spec.Template.Spec.SecurityContext.RunAsUser)
	})

	t.Run("test storefront container security context is restricted", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image: "shopware:latest",
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)
		container := result.Spec.Template.Spec.Containers[0]

		assert.NotNil(t, container.SecurityContext)
		assert.NotNil(t, container.SecurityContext.AllowPrivilegeEscalation)
		assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
		assert.NotNil(t, container.SecurityContext.Capabilities)
		assert.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
	})

	t.Run("test service account merge", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					ServiceAccountName: "container-sa",
				},
				StorefrontDeploymentContainer: v1.ContainerMergeSpec{
					ServiceAccountName: "storefront-sa",
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)

		// Verify service account is overwritten
		assert.Equal(t, "storefront-sa", result.Spec.Template.Spec.ServiceAccountName)
	})

	t.Run("test probes are configured", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
					Port:            8000,
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify probes are configured
		assert.NotNil(t, container.StartupProbe)
		assert.NotNil(t, container.LivenessProbe)
		assert.NotNil(t, container.ReadinessProbe)
		assert.Equal(t, "/-/fpm/ping", container.StartupProbe.HTTPGet.Path)
		assert.Equal(t, int32(8001), container.StartupProbe.HTTPGet.Port.IntVal)
		assert.Equal(t, "/-/fpm/ping", container.LivenessProbe.HTTPGet.Path)
		assert.Equal(t, int32(8001), container.LivenessProbe.HTTPGet.Port.IntVal)
		assert.Equal(t, "/api/_info/health-check", container.ReadinessProbe.HTTPGet.Path)
		assert.Equal(t, int32(8000), container.ReadinessProbe.HTTPGet.Port.IntVal)

		assert.Equal(t, int32(5), container.StartupProbe.PeriodSeconds)
		assert.Equal(t, int32(18), container.StartupProbe.FailureThreshold)
		assert.Equal(t, int32(10), container.LivenessProbe.PeriodSeconds)
		assert.Equal(t, int32(3), container.LivenessProbe.FailureThreshold)
	})

	t.Run("test frankenphp injects FRANKENPHP_MAX_THREADS", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:frankenphp",
					ImagePullPolicy: "IfNotPresent",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"memory": resource.MustParse("2Gi"),
						},
					},
				},
				FPM: v1.FPMSpec{
					ProcessManagement: "frankenphp",
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify FRANKENPHP_MAX_THREADS is set
		hasFrankenPHPThreads := false
		hasFPMEnv := false
		for _, env := range container.Env {
			if env.Name == "FRANKENPHP_MAX_THREADS" {
				hasFrankenPHPThreads = true
				// 2048 MiB / 50 MiB per thread = 40 threads
				assert.Equal(t, "40", env.Value)
			}
			if env.Name == "FPM_PM" {
				hasFPMEnv = true
			}
		}
		assert.True(t, hasFrankenPHPThreads, "FRANKENPHP_MAX_THREADS should be set")
		assert.False(t, hasFPMEnv, "FPM_PM should NOT be set for frankenphp mode")
	})

	t.Run("test frankenphp does not inject FPM env vars", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image: "shopware:frankenphp",
				},
				FPM: v1.FPMSpec{
					ProcessManagement: "frankenphp",
					MaxChildren:       8,
					StartServers:      4,
				},
				SecretName: "store-secret",
			},
		}

		result := deployment.StorefrontDeployment(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify no FPM env vars are injected
		for _, env := range container.Env {
			assert.NotEqual(t, "FPM_PM", env.Name, "FPM_PM should not be set")
			assert.NotEqual(t, "FPM_PM_MAX_CHILDREN", env.Name, "FPM_PM_MAX_CHILDREN should not be set")
			assert.NotEqual(t, "FPM_PM_START_SERVERS", env.Name, "FPM_PM_START_SERVERS should not be set")
			assert.NotEqual(t, "FPM_LISTEN", env.Name, "FPM_LISTEN should not be set")
		}
	})
}
