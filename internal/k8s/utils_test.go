package k8s_test

import (
	"encoding/base64"
	"context"
	"testing"

	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestService(selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}
}

func TestEnsureServiceRejectsForeignService(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "ns", UID: "owner-uid"}}

	tests := map[string]*corev1.Service{
		"no owner": newTestService(map[string]string{"app": "prod"}),
		"other controller": func() *corev1.Service {
			svc := newTestService(map[string]string{"app": "prod"})
			svc.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "v1", Kind: "ConfigMap", Name: "debug", UID: "other-uid",
				Controller: ptr.To(true),
			}}
			return svc
		}(),
	}

	for name, existing := range tests {
		t.Run(name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(existing).Build()

			err := k8s.EnsureService(context.Background(), cl, owner,
				newTestService(map[string]string{"app": "debug"}), scheme.Scheme, true)
			require.ErrorIs(t, err, k8s.ErrNotControlledByOwner)

			got := new(corev1.Service)
			require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(existing), got))
			assert.Equal(t, map[string]string{"app": "prod"}, got.Spec.Selector)
			assert.Equal(t, existing.OwnerReferences, got.OwnerReferences)
		})
	}
}

func TestEnsureServiceUpdatesOwnedService(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "ns", UID: "owner-uid"}}
	existing := newTestService(map[string]string{"app": "old"})
	existing.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1", Kind: "ConfigMap", Name: "debug", UID: "owner-uid",
		Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(true),
	}}
	cl := fake.NewClientBuilder().WithObjects(existing).Build()

	require.NoError(t, k8s.EnsureService(context.Background(), cl, owner,
		newTestService(map[string]string{"app": "new"}), scheme.Scheme, true))

	got := new(corev1.Service)
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(existing), got))
	assert.Equal(t, map[string]string{"app": "new"}, got.Spec.Selector)
}

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
