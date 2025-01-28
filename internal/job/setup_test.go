package job_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStoreContainer(t *testing.T) {
}

func TestSetupJob(t *testing.T) {
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
				SetupJobContainer: v1.ContainerMergeSpec{
					Annotations: map[string]string{
						"shared.key": "setup-value",
						"setup.key":  "setup-value",
						"setup.only": "added",
					},
				},
				SetupScript: "/setup.sh",
				AdminCredentials: v1.Credentials{
					Username: "admin",
					Password: "abcd123",
				},
				SecretName: "store-secret",
			},
		}

		expected := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store-setup",
				Namespace: "test",
				Labels: map[string]string{
					"store": "test-store",
					"type":  "setup",
				},
				Annotations: map[string]string{
					"shared.key":     "setup-value",     // Should be overwritten by setup
					"container.key":  "container-value", // Should stay from container
					"container.only": "stays",           // Should stay from container
					"setup.key":      "setup-value",     // Should be added from setup
					"setup.only":     "added",           // Should be added from setup
					"kubectl.kubernetes.io/default-container":      "shopware-setup",
					"kubectl.kubernetes.io/default-logs-container": "shopware-setup",
				},
			},
			Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: "Never",
						Containers: []corev1.Container{
							{
								Name:            "shopware-setup",
								Image:           "shopware:latest",
								ImagePullPolicy: "IfNotPresent",
								Command:         []string{"sh", "-c"},
								Args:            []string{"/setup.sh"},
							},
						},
					},
				},
			},
		}

		result := job.SetupJob(store)

		// Basic assertions
		assert.Equal(t, expected.Name, result.Name)
		assert.Equal(t, expected.Namespace, result.Namespace)
		assert.Equal(t, expected.Labels, result.Labels)

		// Detailed annotation assertions
		assert.Equal(t, expected.Annotations, result.Annotations, "Annotations should match expected values")

		// Verify specific annotation merging behavior
		assert.Equal(t, "setup-value", result.Annotations["shared.key"], "Shared key should be overwritten by setup")
		assert.Equal(t, "container-value", result.Annotations["container.key"], "Container-specific key should be preserved")
		assert.Equal(t, "stays", result.Annotations["container.only"], "Container-only annotation should stay")
		assert.Equal(t, "setup-value", result.Annotations["setup.key"], "Setup-specific key should be added")
		assert.Equal(t, "added", result.Annotations["setup.only"], "Setup-only annotation should be added")

		// Container assertions
		container := result.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "shopware-setup", container.Name)
		assert.Equal(t, store.Spec.Container.Image, container.Image)
		assert.Equal(t, store.Spec.Container.ImagePullPolicy, container.ImagePullPolicy)
		assert.Equal(t, []string{"sh", "-c"}, container.Command)
		assert.Equal(t, []string{store.Spec.SetupScript}, container.Args)
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
				SetupJobContainer: v1.ContainerMergeSpec{
					Image:           "shopware:setup",
					ImagePullPolicy: "Always",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"memory": resource.MustParse("1Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "setup-volume",
							MountPath: "/setup",
						},
					},
					ExtraEnvs: []corev1.EnvVar{
						{
							Name:  "SETUP_ENV",
							Value: "value",
						},
					},
				},
				SetupScript: "/setup.sh",
				SecretName:  "store-secret",
			},
		}

		result := job.SetupJob(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify image and policy are overwritten
		assert.Equal(t, "shopware:setup", container.Image)
		assert.Equal(t, corev1.PullPolicy("Always"), container.ImagePullPolicy)

		// Verify resources are merged
		assert.Equal(t, resource.MustParse("1"), container.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits["memory"])

		// Verify volume mounts are replaced
		assert.Len(t, container.VolumeMounts, 1)
		assert.Equal(t, "setup-volume", container.VolumeMounts[0].Name)
		assert.Equal(t, "/setup", container.VolumeMounts[0].MountPath)

		// Verify env vars are replaced
		hasSetupEnv := false
		for _, env := range container.Env {
			if env.Name == "SETUP_ENV" {
				hasSetupEnv = true
				assert.Equal(t, "value", env.Value)
			}
			assert.NotEqual(t, "CONTAINER_ENV", env.Name, "Container env should be replaced")
		}
		assert.True(t, hasSetupEnv, "Setup env var should be present")
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
				SetupJobContainer: v1.ContainerMergeSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsGroup: util.Int64(2000),
					},
				},
				SetupScript: "/setup.sh",
			},
		}

		result := job.SetupJob(store)

		// Verify security context is overwritten
		assert.NotNil(t, result.Spec.Template.Spec.SecurityContext)
		assert.Equal(t, int64(2000), *result.Spec.Template.Spec.SecurityContext.RunAsGroup)
		assert.Nil(t, result.Spec.Template.Spec.SecurityContext.RunAsUser)
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
				SetupJobContainer: v1.ContainerMergeSpec{
					ServiceAccountName: "setup-sa",
				},
				SetupScript: "/setup.sh",
			},
		}

		result := job.SetupJob(store)

		// Verify service account is overwritten
		assert.Equal(t, "setup-sa", result.Spec.Template.Spec.ServiceAccountName)
	})
}
