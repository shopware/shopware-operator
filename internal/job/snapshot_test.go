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
}
