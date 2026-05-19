package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultPodSecurityContextUsesRestrictedDefaultsWhenMissing(t *testing.T) {
	securityContext := util.DefaultPodSecurityContext(nil)

	assert.NotNil(t, securityContext)
	assert.NotNil(t, securityContext.FSGroup)
	assert.Equal(t, int64(82), *securityContext.FSGroup)
	assert.NotNil(t, securityContext.RunAsGroup)
	assert.Equal(t, int64(82), *securityContext.RunAsGroup)
	assert.NotNil(t, securityContext.RunAsNonRoot)
	assert.True(t, *securityContext.RunAsNonRoot)
	assert.NotNil(t, securityContext.RunAsUser)
	assert.Equal(t, int64(82), *securityContext.RunAsUser)
	assert.NotNil(t, securityContext.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, securityContext.SeccompProfile.Type)
}

func TestDefaultPodSecurityContextKeepsProvidedContext(t *testing.T) {
	securityContext := &corev1.PodSecurityContext{
		RunAsUser: util.Int64(1000),
	}

	assert.Same(t, securityContext, util.DefaultPodSecurityContext(securityContext))
}
