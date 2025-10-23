package job_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMigrationJob(t *testing.T) {
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
				MigrationJobContainer: v1.ContainerMergeSpec{
					Annotations: map[string]string{
						"shared.key":     "migration-value",
						"migration.key":  "migration-value",
						"migration.only": "added",
					},
				},
				MigrationScript: "/migrate.sh",
				SecretName:      "store-secret",
			},
			Status: v1.StoreStatus{
				CurrentImageTag: "shopware:old",
			},
		}

		result := job.MigrationJob(store)

		// Verify annotations are merged correctly
		assert.Equal(t, "migration-value", result.Annotations["shared.key"], "Shared key should be overwritten by migration")
		assert.Equal(t, "container-value", result.Annotations["container.key"], "Container-specific key should be preserved")
		assert.Equal(t, "stays", result.Annotations["container.only"], "Container-only annotation should stay")
		assert.Equal(t, "migration-value", result.Annotations["migration.key"], "Migration-specific key should be added")
		assert.Equal(t, "added", result.Annotations["migration.only"], "Migration-only annotation should be added")
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
				MigrationJobContainer: v1.ContainerMergeSpec{
					Image:           "shopware:migration",
					ImagePullPolicy: "Always",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"memory": resource.MustParse("1Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "migration-volume",
							MountPath: "/migration",
						},
					},
					ExtraEnvs: []corev1.EnvVar{
						{
							Name:  "MIGRATION_ENV",
							Value: "value",
						},
						{
							Name:  "CONTAINER_ENV",
							Value: "overwritten",
						},
					},
				},
				MigrationScript: "/migrate.sh",
				SecretName:      "store-secret",
			},
			Status: v1.StoreStatus{
				CurrentImageTag: "shopware:old",
			},
		}

		result := job.MigrationJob(store)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify image and policy are overwritten
		assert.Equal(t, "shopware:migration", container.Image)
		assert.Equal(t, corev1.PullPolicy("Always"), container.ImagePullPolicy)

		// Verify resources are merged
		assert.Equal(t, resource.MustParse("1"), container.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits["memory"])

		// Verify volume mounts are replaced
		assert.Len(t, container.VolumeMounts, 1)
		assert.Equal(t, "migration-volume", container.VolumeMounts[0].Name)
		assert.Equal(t, "/migration", container.VolumeMounts[0].MountPath)

		// Verify env vars are merged
		hasMigrationEnv := false
		hasContainerEnv := false
		for _, env := range container.Env {
			if env.Name == "MIGRATION_ENV" {
				hasMigrationEnv = true
				assert.Equal(t, "value", env.Value)
			}
			if env.Name == "CONTAINER_ENV" {
				hasContainerEnv = true
				assert.Equal(t, "overwritten", env.Value)
			}
		}
		assert.True(t, hasMigrationEnv, "Migration env var should be present")
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
				MigrationJobContainer: v1.ContainerMergeSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsGroup: util.Int64(2000),
					},
				},
				MigrationScript: "/migrate.sh",
			},
			Status: v1.StoreStatus{
				CurrentImageTag: "shopware:old",
			},
		}

		result := job.MigrationJob(store)

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
				MigrationJobContainer: v1.ContainerMergeSpec{
					ServiceAccountName: "migration-sa",
				},
				MigrationScript: "/migrate.sh",
			},
			Status: v1.StoreStatus{
				CurrentImageTag: "shopware:old",
			},
		}

		result := job.MigrationJob(store)

		// Verify service account is overwritten
		assert.Equal(t, "migration-sa", result.Spec.Template.Spec.ServiceAccountName)
	})
}
