package cronjob_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScheduledTaskJobUsesScheduledTaskComponentLabel(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-store",
			Namespace: "test",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Image: "shopware:latest",
				Labels: map[string]string{
					"application-id": "app-id",
					"component":      "store",
				},
			},
			SetupJobContainer: v1.ContainerMergeSpec{
				Labels: map[string]string{
					"component":  "setup",
					"setup-only": "setup",
				},
			},
			ScheduledTask: v1.ScheduledTaskSpec{
				TimeZone: "Etc/UTC",
				Schedule: "*/5 * * * *",
				Command:  "bin/console scheduled-task:run -v -n --no-wait",
			},
		},
	}

	result := cronjob.ScheduledTaskJob(store)

	assert.Equal(t, "scheduled-task", result.Labels["component"])
	assert.Equal(t, "scheduled-task", result.Spec.JobTemplate.Labels["component"])
	assert.Equal(t, "scheduled-task", result.Spec.JobTemplate.Spec.Template.Labels["component"])
	assert.Equal(t, "scheduled-task", result.Labels[util.ShopwareKey("store.type")])
	assert.Equal(t, "app-id", result.Labels["application-id"])
	assert.NotContains(t, result.Labels, "setup-only")
}
