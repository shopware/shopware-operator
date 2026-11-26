package v1_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func findEnv(t *testing.T, envs []corev1.EnvVar, name string) corev1.EnvVar {
	t.Helper()
	for _, env := range envs {
		if env.Name == name {
			return env
		}
	}
	require.Failf(t, "env not found", "%s missing", name)
	return corev1.EnvVar{}
}

func envIndex(envs []corev1.EnvVar, name string) int {
	for i, env := range envs {
		if env.Name == name {
			return i
		}
	}
	return -1
}

func TestWorkerTransport(t *testing.T) {
	assert.Nil(t, v1.WorkerSpec{Adapter: "builtin"}.Transport())
	assert.IsType(t, v1.RedisQueue{}, v1.WorkerSpec{Adapter: "redis"}.Transport())
	assert.IsType(t, v1.SQSSpec{}, v1.WorkerSpec{Adapter: "sqs"}.Transport())
}

func TestWorkerBuiltinHasNoMessengerEnv(t *testing.T) {
	store := &v1.Store{Spec: v1.StoreSpec{Worker: v1.WorkerSpec{Adapter: "builtin"}}}
	assert.Equal(t, -1, envIndex(store.GetEnv(), "MESSENGER_TRANSPORT_DSN"))
}

func TestWorkerSQS(t *testing.T) {
	t.Run("plain DSN", func(t *testing.T) {
		store := &v1.Store{Spec: v1.StoreSpec{Worker: v1.WorkerSpec{
			Adapter: "sqs",
			SQS:     v1.SQSSpec{DSN: "sqs://elasticmq:9324/messages?sslmode=disable", Region: "ignored"},
		}}}

		env := store.GetEnv()
		assert.Equal(t, "sqs://elasticmq:9324/messages?sslmode=disable", findEnv(t, env, "MESSENGER_TRANSPORT_DSN").Value)
		assert.Equal(t, -1, envIndex(env, "MESSENGER_CONSUMER_NAME"))
	})

	t.Run("DSN from secret", func(t *testing.T) {
		store := &v1.Store{Spec: v1.StoreSpec{Worker: v1.WorkerSpec{
			Adapter: "sqs",
			SQS: v1.SQSSpec{
				DSNSecretRef: v1.SecretRef{Name: "queue", Key: "dsn"},
				AccessKeyRef: v1.SecretRef{Name: "queue", Key: "id"},
			},
		}}}

		env := store.GetEnv()
		dsn := findEnv(t, env, "MESSENGER_TRANSPORT_DSN")
		require.NotNil(t, dsn.ValueFrom)
		require.NotNil(t, dsn.ValueFrom.SecretKeyRef)
		assert.Equal(t, "queue", dsn.ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "dsn", dsn.ValueFrom.SecretKeyRef.Key)
		assert.Equal(t, -1, envIndex(env, "MESSENGER_SQS_ACCESS_KEY_ID"))
	})

	t.Run("AWS defaults", func(t *testing.T) {
		spec := v1.SQSSpec{Region: "eu-central-1"}
		assert.Equal(t, "sqs://default/messages?region=eu-central-1", spec.DSNValue())
	})

	t.Run("built from fields with credentials", func(t *testing.T) {
		store := &v1.Store{Spec: v1.StoreSpec{Worker: v1.WorkerSpec{
			Adapter: "sqs",
			SQS: v1.SQSSpec{
				Endpoint:           "sqs.example.org:9324",
				Region:             "eu01",
				Account:            "123456789012",
				QueueName:          "shop",
				Options:            "?sslmode=disable&auto_setup=false",
				AccessKeyRef:       v1.SecretRef{Name: "queue", Key: "id"},
				SecretAccessKeyRef: v1.SecretRef{Name: "queue", Key: "secret"},
			},
		}}}

		env := store.GetEnv()
		assert.Equal(t,
			"sqs://$(MESSENGER_SQS_ACCESS_KEY_ID):$(MESSENGER_SQS_SECRET_ACCESS_KEY)@sqs.example.org:9324/123456789012/shop?region=eu01&sslmode=disable&auto_setup=false",
			findEnv(t, env, "MESSENGER_TRANSPORT_DSN").Value,
		)
		assert.Equal(t, "id", findEnv(t, env, "MESSENGER_SQS_ACCESS_KEY_ID").ValueFrom.SecretKeyRef.Key)
		assert.Equal(t, "secret", findEnv(t, env, "MESSENGER_SQS_SECRET_ACCESS_KEY").ValueFrom.SecretKeyRef.Key)
		assert.Less(t, envIndex(env, "MESSENGER_SQS_ACCESS_KEY_ID"), envIndex(env, "MESSENGER_TRANSPORT_DSN"))
		assert.Less(t, envIndex(env, "MESSENGER_SQS_SECRET_ACCESS_KEY"), envIndex(env, "MESSENGER_TRANSPORT_DSN"))
	})
}
