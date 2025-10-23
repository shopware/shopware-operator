package job_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCommandJob(t *testing.T) {
	t.Run("test env merging", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
					ExtraEnvs: []corev1.EnvVar{
						{
							Name:  "CONTAINER_ENV",
							Value: "value",
						},
					},
				},
				SecretName: "store-secret",
			},
		}

		exec := v1.StoreExec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-exec",
				Namespace: "test",
			},
			Spec: v1.StoreExecSpec{
				Script: "echo 'test'",
				ExtraEnvs: []corev1.EnvVar{
					{
						Name:  "EXEC_ENV",
						Value: "value",
					},
					{
						Name:  "CONTAINER_ENV",
						Value: "overwritten",
					},
				},
			},
		}

		result := job.CommandJob(store, exec)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify env vars are merged
		hasExecEnv := false
		hasContainerEnv := false
		for _, env := range container.Env {
			if env.Name == "EXEC_ENV" {
				hasExecEnv = true
				assert.Equal(t, "value", env.Value)
			}
			if env.Name == "CONTAINER_ENV" {
				hasContainerEnv = true
				assert.Equal(t, "overwritten", env.Value)
			}
		}
		assert.True(t, hasExecEnv, "Exec env var should be present")
		assert.True(t, hasContainerEnv, "Container env var should be present and overwritten")
	})

	t.Run("test script execution", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
				},
				SecretName: "store-secret",
			},
		}

		exec := v1.StoreExec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-exec",
				Namespace: "test",
			},
			Spec: v1.StoreExecSpec{
				Script: "bin/console cache:clear",
			},
		}

		result := job.CommandJob(store, exec)
		container := result.Spec.Template.Spec.Containers[0]

		// Verify command and args
		assert.Equal(t, []string{"sh", "-c"}, container.Command)
		assert.Equal(t, []string{"bin/console cache:clear"}, container.Args)
		assert.Equal(t, "shopware-command", container.Name)
	})
}

func TestCommandCronJob(t *testing.T) {
	t.Run("test cron schedule", func(t *testing.T) {
		store := v1.Store{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-store",
				Namespace: "test",
			},
			Spec: v1.StoreSpec{
				Container: v1.ContainerSpec{
					Image:           "shopware:latest",
					ImagePullPolicy: "IfNotPresent",
				},
				SecretName: "store-secret",
			},
		}

		suspend := false
		exec := v1.StoreExec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-exec",
				Namespace: "test",
			},
			Spec: v1.StoreExecSpec{
				Script:       "bin/console scheduled-task:run",
				CronSchedule: "*/5 * * * *",
				CronSuspend:  suspend,
			},
		}

		result := job.CommandCronJob(store, exec)

		// Verify cron job spec
		assert.Equal(t, "*/5 * * * *", result.Spec.Schedule)
		assert.Equal(t, &suspend, result.Spec.Suspend)
		assert.Equal(t, "test-exec", result.Name)
		assert.Equal(t, "test", result.Namespace)
	})
}
