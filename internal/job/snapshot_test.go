package job_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSnapshotCreateJobUsesRestrictedContainerSecurityContext(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: v1.StoreSpec{
			SecretName: "store-secret",
			Database: v1.DatabaseSpec{
				Name: "shopware",
				TLS:  v1.DatabaseTLS{SecretName: "database-tls"},
			},
			S3Storage: v1.S3Storage{
				EndpointURL:       "https://s3.example.com",
				PrivateBucketName: "private",
				PublicBucketName:  "public",
			},
		},
	}

	snapshot := v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test",
		},
		Spec: v1.StoreSnapshotSpec{
			Path: "/tmp/snapshot.zip",
			Container: v1.ContainerSpec{
				Image: "shopware:snapshot",
			},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)
	container := result.Spec.Template.Spec.Containers[0]

	assert.NotNil(t, container.SecurityContext)
	assert.NotNil(t, container.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
	assert.NotNil(t, container.SecurityContext.Capabilities)
	assert.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
	assert.Contains(t, container.Env, corev1.EnvVar{Name: "DB_SSL_CA", Value: "/etc/shopware/database-tls/ca.crt"})
	assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "shopware-database-tls", MountPath: "/etc/shopware/database-tls", ReadOnly: true})
	assert.Len(t, result.Spec.Template.Spec.Volumes, 2)
	assert.Equal(t, "database-tls", result.Spec.Template.Spec.Volumes[0].Secret.SecretName)
}

func TestSnapshotCreateJobPropagatesContainerAnnotationsToPodTemplate(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
	}

	snapshot := v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test",
		},
		Spec: v1.StoreSnapshotSpec{
			Container: v1.ContainerSpec{
				Annotations: map[string]string{
					"ad.datadoghq.com/operator-snapshot.logs": "[]",
				},
			},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)

	assert.Equal(t, "[]", result.Annotations["ad.datadoghq.com/operator-snapshot.logs"])
	assert.Equal(t, "[]", result.Spec.Template.Annotations["ad.datadoghq.com/operator-snapshot.logs"])
}

func TestSnapshotRestoreJobPropagatesContainerAnnotationsToPodTemplate(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
	}

	snapshot := v1.StoreSnapshotRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "test",
		},
		Spec: v1.StoreSnapshotSpec{
			Container: v1.ContainerSpec{
				Annotations: map[string]string{
					"ad.datadoghq.com/operator-snapshot.logs": "[]",
				},
			},
		},
	}

	result := job.SnapshotRestoreJob(store, snapshot)

	assert.Equal(t, "[]", result.Annotations["ad.datadoghq.com/operator-snapshot.logs"])
	assert.Equal(t, "[]", result.Spec.Template.Annotations["ad.datadoghq.com/operator-snapshot.logs"])
}

func TestSnapshotRestoreJobUsesEphemeralVolumeTempDir(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: v1.StoreSpec{
			SecretName: "store-secret",
			Database: v1.DatabaseSpec{
				Name: "shopware",
			},
			S3Storage: v1.S3Storage{
				EndpointURL:       "https://s3.example.com",
				PrivateBucketName: "private",
				PublicBucketName:  "public",
			},
		},
	}

	snapshot := v1.StoreSnapshotRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "test",
		},
		Spec: v1.StoreSnapshotSpec{
			Path: "s3://bucket/snapshot.zip",
			Container: v1.ContainerSpec{
				Image: "shopware:snapshot",
				Volumes: []corev1.Volume{
					{
						Name: "snapshot-storage",
						VolumeSource: corev1.VolumeSource{
							Ephemeral: &corev1.EphemeralVolumeSource{
								VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
									Spec: corev1.PersistentVolumeClaimSpec{
										AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									},
								},
							},
						},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "snapshot-storage",
						MountPath: "/snapshot-storage",
					},
				},
			},
		},
	}

	result := job.SnapshotRestoreJob(store, snapshot)
	var container *corev1.Container
	for i := range result.Spec.Template.Spec.Containers {
		if result.Spec.Template.Spec.Containers[i].Name == job.CONTAINER_NAME_SNAPSHOT {
			container = &result.Spec.Template.Spec.Containers[i]
			break
		}
	}

	assert.NotNil(t, container)
	assert.Contains(t, container.Args, "--tempdir")
	assert.Contains(t, container.Args, "/snapshot-storage")
	assert.Equal(t, []string{"restore", "--backup-file", "s3://bucket/snapshot.zip", "--tempdir", "/snapshot-storage"}, container.Args)
}
