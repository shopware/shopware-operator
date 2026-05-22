package cronjob_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScheduledTaskJobUsesRestrictedContainerSecurityContext(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image: "shopware:latest",
			},
			ScheduledTask: v1.ScheduledTaskSpec{
				Schedule: "0 * * * *",
				TimeZone: "Etc/UTC",
				Command:  "bin/console scheduled-task:run",
			},
		},
	}

	result := cronjob.ScheduledTaskJob(store)
	container := result.Spec.JobTemplate.Spec.Template.Spec.Containers[0]

	assert.NotNil(t, container.SecurityContext)
	assert.NotNil(t, container.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
	assert.NotNil(t, container.SecurityContext.Capabilities)
	assert.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
}
