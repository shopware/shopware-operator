package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestMergeEnv(t *testing.T) {
	t.Run("test overwrite existing env var", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{Name: "ENV2", Value: "value2"},
		}
		src := []corev1.EnvVar{
			{Name: "ENV2", Value: "overwritten"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 2)
		assert.Equal(t, "value1", result[0].Value)
		assert.Equal(t, "overwritten", result[1].Value)
	})

	t.Run("test append new env vars", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
		}
		src := []corev1.EnvVar{
			{Name: "ENV2", Value: "value2"},
			{Name: "ENV3", Value: "value3"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 3)
		assert.Equal(t, "value1", result[0].Value)
		assert.Equal(t, "value2", result[1].Value)
		assert.Equal(t, "value3", result[2].Value)
	})

	t.Run("test overwrite and append", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{Name: "ENV2", Value: "value2"},
		}
		src := []corev1.EnvVar{
			{Name: "ENV2", Value: "overwritten"},
			{Name: "ENV3", Value: "value3"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 3)
		assert.Equal(t, "value1", result[0].Value)
		assert.Equal(t, "overwritten", result[1].Value)
		assert.Equal(t, "value3", result[2].Value)
	})

	t.Run("test empty dst", func(t *testing.T) {
		dst := []corev1.EnvVar{}
		src := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{Name: "ENV2", Value: "value2"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 2)
		assert.Equal(t, "value1", result[0].Value)
		assert.Equal(t, "value2", result[1].Value)
	})

	t.Run("test empty src", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{Name: "ENV2", Value: "value2"},
		}
		src := []corev1.EnvVar{}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 2)
		assert.Equal(t, "value1", result[0].Value)
		assert.Equal(t, "value2", result[1].Value)
	})

	t.Run("test multiple overwrites", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{Name: "ENV2", Value: "value2"},
			{Name: "ENV3", Value: "value3"},
		}
		src := []corev1.EnvVar{
			{Name: "ENV1", Value: "overwritten1"},
			{Name: "ENV3", Value: "overwritten3"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 3)
		assert.Equal(t, "overwritten1", result[0].Value)
		assert.Equal(t, "value2", result[1].Value)
		assert.Equal(t, "overwritten3", result[2].Value)
	})

	t.Run("test with ValueFrom", func(t *testing.T) {
		dst := []corev1.EnvVar{
			{Name: "ENV1", Value: "value1"},
			{
				Name: "ENV2",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "secret1"},
						Key:                  "key1",
					},
				},
			},
		}
		src := []corev1.EnvVar{
			{
				Name: "ENV2",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "secret2"},
						Key:                  "key2",
					},
				},
			},
			{Name: "ENV3", Value: "value3"},
		}

		result := util.MergeEnv(dst, src)

		assert.Len(t, result, 3)
		assert.Equal(t, "value1", result[0].Value)
		assert.NotNil(t, result[1].ValueFrom)
		assert.Equal(t, "secret2", result[1].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "key2", result[1].ValueFrom.SecretKeyRef.Key)
		assert.Equal(t, "value3", result[2].Value)
	})
}
