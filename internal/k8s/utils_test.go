package k8s_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	// EnsureService is the strict variant used for debug instances, so it must not
	// adopt a Service without controller reference. Only EnsureStoreService does.
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

func TestEnsureStoreServiceAdoptsServiceWithoutController(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: "ns", UID: "new-uid"}}

	tests := map[string][]metav1.OwnerReference{
		// What is left after the old Store was deleted with orphan propagation.
		"no owner references": nil,
		"owner reference that is not a controller": {{
			APIVersion: "v1", Kind: "ConfigMap", Name: "other", UID: "other-uid",
		}},
	}

	for name, owners := range tests {
		t.Run(name, func(t *testing.T) {
			existing := newTestService(map[string]string{"app": "old"})
			existing.OwnerReferences = owners
			cl := fake.NewClientBuilder().WithObjects(existing).Build()

			require.NoError(t, k8s.EnsureStoreService(context.Background(), cl, owner,
				newTestService(map[string]string{"app": "new"}), scheme.Scheme))

			got := new(corev1.Service)
			require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(existing), got))
			assert.Equal(t, map[string]string{"app": "new"}, got.Spec.Selector)
			assert.True(t, metav1.IsControlledBy(got, owner), "service must be controlled by the new owner")
		})
	}
}

func TestEnsureStoreServiceRejectsServiceControlledByOther(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: "ns", UID: "new-uid"}}
	existing := newTestService(map[string]string{"app": "prod"})
	existing.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1", Kind: "ConfigMap", Name: "other", UID: "other-uid",
		Controller: ptr.To(true),
	}}
	cl := fake.NewClientBuilder().WithObjects(existing).Build()

	err := k8s.EnsureStoreService(context.Background(), cl, owner,
		newTestService(map[string]string{"app": "new"}), scheme.Scheme)
	require.ErrorIs(t, err, k8s.ErrNotControlledByOwner)

	got := new(corev1.Service)
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(existing), got))
	assert.Equal(t, map[string]string{"app": "prod"}, got.Spec.Selector)
	assert.Equal(t, existing.OwnerReferences, got.OwnerReferences)
}

func TestEnsureStoreServiceRequiresOwner(t *testing.T) {
	cl := fake.NewClientBuilder().Build()
	require.Error(t, k8s.EnsureStoreService(context.Background(), cl, nil,
		newTestService(nil), scheme.Scheme))
}

func TestDeleteControlledObject(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "ns", UID: "owner-uid"}}
	controlledBy := func(uid string) []metav1.OwnerReference {
		return []metav1.OwnerReference{{
			APIVersion: "v1", Kind: "ConfigMap", Name: "debug", UID: types.UID(uid),
			Controller: ptr.To(true),
		}}
	}

	tests := []struct {
		name        string
		owners      []metav1.OwnerReference
		wantDeleted bool
	}{
		{name: "controlled by owner", owners: controlledBy("owner-uid"), wantDeleted: true},
		{name: "controlled by other uid", owners: controlledBy("other-uid")},
		{name: "same name and kind but not controller", owners: []metav1.OwnerReference{{
			APIVersion: "v1", Kind: "ConfigMap", Name: "debug", UID: "owner-uid",
		}}},
		{name: "no owner"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := newTestService(nil)
			existing.OwnerReferences = tt.owners
			cl := fake.NewClientBuilder().WithObjects(existing).Build()

			deleted, err := k8s.DeleteControlledObject(context.Background(), cl, owner, newTestService(nil))
			require.NoError(t, err)
			assert.Equal(t, tt.wantDeleted, deleted)

			err = cl.Get(context.Background(), client.ObjectKeyFromObject(existing), new(corev1.Service))
			assert.Equal(t, tt.wantDeleted, k8serrors.IsNotFound(err))
		})
	}

	t.Run("missing object", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()
		deleted, err := k8s.DeleteControlledObject(context.Background(), cl, owner, newTestService(nil))
		require.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("owner without uid", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithObjects(newTestService(nil)).Build()
		noUID := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "ns"}}
		_, err := k8s.DeleteControlledObject(context.Background(), cl, noUID, newTestService(nil))
		require.Error(t, err)
	})
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
