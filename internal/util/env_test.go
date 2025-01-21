package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestEnv(t *testing.T) {
	src := []corev1.EnvVar{
		{
			Name:  "TEST",
			Value: "valueSrc",
		},
	}
	dst := []corev1.EnvVar{
		{
			Name:  "TEST",
			Value: "ValueDst",
		},
		{
			Name:  "SecondValue",
			Value: "ignore",
		},
	}
	merged := util.MergeEnv(dst, src)
	assert.Len(t, merged, 2)
	assert.Equal(t, "valueSrc", merged[0].Value)
}
