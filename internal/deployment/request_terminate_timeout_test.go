package deployment_test

import (
	"testing"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const requestTerminateTimeoutEnv = "FPM_REQUEST_TERMINATE_TIMEOUT"

func seconds(s int) *metav1.Duration {
	return &metav1.Duration{Duration: time.Duration(s) * time.Second}
}

func lookupEnv(envs []corev1.EnvVar, name string) (string, bool) {
	for _, env := range envs {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func requestTerminateTimeouts(store v1.Store) (admin, storefront string, adminSet, storefrontSet bool) {
	admin, adminSet = lookupEnv(deployment.AdminDeployment(store).Spec.Template.Spec.Containers[0].Env, requestTerminateTimeoutEnv)
	storefront, storefrontSet = lookupEnv(deployment.StorefrontDeployment(store).Spec.Template.Spec.Containers[0].Env, requestTerminateTimeoutEnv)
	return
}

func newRequestTerminateTimeoutStore(base, admin, storefront *metav1.Duration) v1.Store {
	return v1.Store{
		ObjectMeta: metav1.ObjectMeta{Name: "test-store", Namespace: "test"},
		Spec: v1.StoreSpec{
			Container:                     v1.ContainerSpec{Image: "shopware:latest", RequestTerminateTimeout: base},
			AdminDeploymentContainer:      v1.ContainerMergeSpec{RequestTerminateTimeout: admin},
			StorefrontDeploymentContainer: v1.ContainerMergeSpec{RequestTerminateTimeout: storefront},
		},
	}
}

func TestRequestTerminateTimeout(t *testing.T) {
	t.Run("admin and storefront are configured separately", func(t *testing.T) {
		admin, storefront, adminSet, storefrontSet := requestTerminateTimeouts(newRequestTerminateTimeoutStore(nil, seconds(300), seconds(65)))

		assert.True(t, adminSet)
		assert.Equal(t, "300", admin)
		assert.True(t, storefrontSet)
		assert.Equal(t, "65", storefront)
	})

	t.Run("shared container value applies to both", func(t *testing.T) {
		admin, storefront, adminSet, storefrontSet := requestTerminateTimeouts(newRequestTerminateTimeoutStore(seconds(65), nil, nil))

		assert.True(t, adminSet)
		assert.Equal(t, "65", admin)
		assert.True(t, storefrontSet)
		assert.Equal(t, "65", storefront)
	})

	t.Run("deployment value overrides the shared container value", func(t *testing.T) {
		admin, storefront, _, _ := requestTerminateTimeouts(newRequestTerminateTimeoutStore(seconds(65), seconds(300), nil))

		assert.Equal(t, "300", admin)
		assert.Equal(t, "65", storefront)
	})

	t.Run("a deployment can opt out of the shared container value", func(t *testing.T) {
		_, storefront, adminSet, storefrontSet := requestTerminateTimeouts(newRequestTerminateTimeoutStore(seconds(65), seconds(0), nil))

		assert.False(t, adminSet)
		assert.True(t, storefrontSet)
		assert.Equal(t, "65", storefront)
	})

	t.Run("unset keeps the php-fpm default", func(t *testing.T) {
		_, _, adminSet, storefrontSet := requestTerminateTimeouts(newRequestTerminateTimeoutStore(nil, nil, nil))

		assert.False(t, adminSet)
		assert.False(t, storefrontSet)
	})

	t.Run("extraEnvs win over the field", func(t *testing.T) {
		store := newRequestTerminateTimeoutStore(nil, nil, seconds(65))
		store.Spec.StorefrontDeploymentContainer.ExtraEnvs = []corev1.EnvVar{{Name: requestTerminateTimeoutEnv, Value: "10"}}

		_, storefront, _, _ := requestTerminateTimeouts(store)
		assert.Equal(t, "10", storefront)
	})
}
