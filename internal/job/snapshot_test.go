package job_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSnapshotCreateJobLabelHyphenUnderscoreSources(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dbs-v5",
			Namespace: "dbs-v5-67091e48-6ddb-4cc3-9b29-fc9f4d782f37",
		},
	}

	snapshot := v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cdf2faf6-4a55-4ee7-b095-dca5dd4aaa57-zc9ht",
			Namespace: "dbs-v5-67091e48-6ddb-4cc3-9b29-fc9f4d782f37",
			Labels: map[string]string{
				"application-id":             "4537857a-b727-489b-aac0-5348e179597d",
				"application-name":           "dbs-v5",
				"component":                  "shopsnapshot.create",
				"deployment-id":              "2d0aab73-c162-4b96-8b30-4241e646f6a5",
				"organization-id":            "f219e284-a238-41c3-b3ea-c061b9a5a5d3",
				"project-id":                 "67091e48-6ddb-4cc3-9b29-fc9f4d782f37",
				"tags.datadoghq.com/service": "shopsnapshot-create",
				"tenant-id":                  "dbs-v5-67091e48-6ddb-4cc3-9b29-fc9f4d782f37",
			},
		},
		Spec: v1.StoreSnapshotSpec{
			Path: "/tmp/snapshot.zip",
			Container: v1.ContainerSpec{
				Image: "shopware:snapshot",
				Annotations: map[string]string{
					"ad.datadoghq.com/operator-snapshot.logs": `[{"source":"shopsnapshot","service":"shopsnapshot-create","tags":["component:shopsnapshot.create","application_id:4537857a-b727-489b-aac0-5348e179597d","application_name:dbs-v5"]}]`,
				},
			},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)
	assert.Equal(t, "4537857a-b727-489b-aac0-5348e179597d", result.Spec.Template.Labels["application-id"])
	assert.Empty(t, result.Spec.Template.Labels["application_id"])
	assert.Contains(t, result.Spec.Template.Spec.Containers[0].Args, "application_id=4537857a-b727-489b-aac0-5348e179597d")
	assert.Contains(t, result.Spec.Template.Spec.Containers[0].Args, "application_name=dbs-v5")
	assert.Contains(t, result.Spec.Template.Spec.Containers[0].Args, "namespace=dbs-v5-67091e48-6ddb-4cc3-9b29-fc9f4d782f37")
	assert.NotContains(t, result.Spec.Template.Spec.Containers[0].Args, "application-id=4537857a-b727-489b-aac0-5348e179597d")
	assert.Contains(t, result.Spec.Template.Annotations["ad.datadoghq.com/operator-snapshot.logs"], "application_id:")
	assert.Contains(t, result.Spec.Template.Annotations["ad.datadoghq.com/operator-snapshot.logs"], "application_name:")
}

func TestSnapshotCreateJobFailsWithoutRetryOnNoSpaceExit(t *testing.T) {
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
			Path: "/tmp/snapshot.zip",
			Container: v1.ContainerSpec{
				Image: "shopware:snapshot",
			},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)
	require.NotNil(t, result.Spec.PodFailurePolicy)
	require.Len(t, result.Spec.PodFailurePolicy.Rules, 1)
	rule := result.Spec.PodFailurePolicy.Rules[0]
	assert.Equal(t, batchv1.PodFailurePolicyActionFailJob, rule.Action)
	require.NotNil(t, rule.OnExitCodes)
	assert.Equal(t, []int32{16}, rule.OnExitCodes.Values)
	assert.Equal(t, job.CONTAINER_NAME_SNAPSHOT, *rule.OnExitCodes.ContainerName)
}

func TestSnapshotCreateJobPassesAWSRegion(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"},
		Spec: v1.StoreSpec{
			S3Storage: v1.S3Storage{
				EndpointURL:       "https://s3.eu-central-1.amazonaws.com",
				PrivateBucketName: "private",
				PublicBucketName:  "public",
				Region:            "eu-central-1",
			},
		},
	}
	snapshot := v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{Name: "test-snapshot", Namespace: "test"},
		Spec: v1.StoreSnapshotSpec{
			Path:      "/tmp/snapshot.zip",
			Container: v1.ContainerSpec{Image: "shopware:snapshot"},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)
	assert.Contains(t, result.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "AWS_REGION", Value: "eu-central-1"})
	assert.Contains(t, result.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "AWS_DEFAULT_REGION", Value: "eu-central-1"})
}

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
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("100Gi"),
											},
										},
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
	assert.Equal(t, []string{
		"restore",
		"--backup-file", "s3://bucket/snapshot.zip",
		"--tempdir", "/snapshot-storage",
		"--temp-size", "100Gi",
		"--label", "namespace=test",
	}, container.Args)
}

func TestSnapshotCreateJobPassesDefaultTempSize(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"},
	}
	snapshot := v1.StoreSnapshotCreate{
		ObjectMeta: metav1.ObjectMeta{Name: "test-snapshot", Namespace: "test"},
		Spec: v1.StoreSnapshotSpec{
			Path:      "/tmp/snapshot.zip",
			Container: v1.ContainerSpec{Image: "shopware:snapshot"},
		},
	}

	result := job.SnapshotCreateJob(store, snapshot)
	assert.Equal(t, []string{
		"create",
		"--backup-file", "/tmp/snapshot.zip",
		"--tempdir", "/temp",
		"--temp-size", "20Gi",
		"--label", "namespace=test",
	}, result.Spec.Template.Spec.Containers[0].Args)
}
