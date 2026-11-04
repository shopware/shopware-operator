package k8s_test

import (
	"encoding/base64"
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

func TestObjectDiffRedactsSecretValues(t *testing.T) {
	oldSecret := &corev1.Secret{Data: map[string][]byte{
		"aws-secret-access-key": []byte("old-secret-value"),
		"removed":               []byte("removed-secret-value"),
		"unchanged":             []byte("unchanged-secret-value"),
	}}
	newSecret := &corev1.Secret{
		Data: map[string][]byte{
			"aws-secret-access-key": []byte("new-secret-value"),
			"unchanged":             []byte("unchanged-secret-value"),
		},
		StringData: map[string]string{"added": "added-secret-value"},
	}

	diff, err := k8s.ObjectDiff(oldSecret, newSecret)
	require.NoError(t, err)

	for _, value := range []string{"old-secret-value", "new-secret-value", "removed-secret-value", "added-secret-value"} {
		assert.NotContains(t, diff, value)
		assert.NotContains(t, diff, base64.StdEncoding.EncodeToString([]byte(value)))
	}
	assert.Contains(t, diff, `~ data/aws-secret-access-key: "<redacted>" -> "<redacted, changed>"`)
	assert.Contains(t, diff, `- data/removed: "<redacted>"`)
	assert.Contains(t, diff, `+ stringData/added: "<redacted>"`)
	assert.NotContains(t, diff, "data/unchanged")
}

func TestObjectHashSecretUsesValues(t *testing.T) {
	base := &corev1.Secret{Data: map[string][]byte{"key": []byte("value")}}
	changed := base.DeepCopy()
	changed.Data["key"] = []byte("other")

	baseHash, err := k8s.ObjectHash(base)
	require.NoError(t, err)
	changedHash, err := k8s.ObjectHash(changed)
	require.NoError(t, err)

	assert.NotEqual(t, baseHash, changedHash)
}
