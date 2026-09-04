package k8s_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestObjectHashConfigMapBinaryData(t *testing.T) {
	base := &corev1.ConfigMap{Data: map[string]string{"key": "value"}}
	changed := base.DeepCopy()
	changed.BinaryData = map[string][]byte{"blob": {0x01}}

	baseHash, err := k8s.ObjectHash(base)
	require.NoError(t, err)
	changedHash, err := k8s.ObjectHash(changed)
	require.NoError(t, err)

	assert.NotEqual(t, baseHash, changedHash)
}

func TestObjectHashSecretStringData(t *testing.T) {
	base := &corev1.Secret{Data: map[string][]byte{"key": []byte("value")}}
	changed := base.DeepCopy()
	changed.StringData = map[string]string{"plain": "text"}

	baseHash, err := k8s.ObjectHash(base)
	require.NoError(t, err)
	changedHash, err := k8s.ObjectHash(changed)
	require.NoError(t, err)

	assert.NotEqual(t, baseHash, changedHash)
}
