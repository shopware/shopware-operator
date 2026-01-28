package secret

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const credentialsSecretName = "credentials"

func storeWithCopiedCredentials() *v1.Store {
	store := createTestStore()
	store.Spec.AdminCredentials.Username = ""
	store.Spec.AdminCredentials.UsernameSecretRef = v1.SecretRef{Name: credentialsSecretName, Key: "admin-user"}
	store.Spec.S3Storage.AccessKeyRef = v1.SecretRef{Name: credentialsSecretName, Key: "s3-access"}
	store.Spec.S3Storage.SecretAccessKeyRef = v1.SecretRef{Name: credentialsSecretName, Key: "s3-secret"}
	store.Spec.Tideways.Enabled = true
	store.Spec.Tideways.APIKeyRef = v1.SecretRef{Name: credentialsSecretName, Key: "tideways"}
	return store
}

func credentialsSecret(admin, access, secretKey, tideways string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: credentialsSecretName, Namespace: "default"},
		Data: map[string][]byte{
			"admin-user": []byte(admin),
			"s3-access":  []byte(access),
			"s3-secret":  []byte(secretKey),
			"tideways":   []byte(tideways),
		},
	}
}

func ensureWithObjects(t *testing.T, store *v1.Store, objects ...client.Object) *corev1.Secret {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	objects = append(objects, store, createDBSecret())
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

	storeSecret, err := EnsureStoreSecret(context.Background(), c, &mockEventRecorder{}, store)
	require.NoError(t, err)
	return storeSecret
}

func existingStoreSecret(store *v1.Store, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: store.GetSecretName(), Namespace: store.Namespace},
		Data:       data,
	}
}

func TestEnsureStoreSecret_CopiesReferencedCredentials(t *testing.T) {
	store := storeWithCopiedCredentials()

	storeSecret := ensureWithObjects(t, store, credentialsSecret("admin", "access", "secret", "tideways-key"))

	require.Equal(t, "admin", string(storeSecret.Data[v1.StoreSecretAdminUsernameKey]))
	require.Equal(t, "access", string(storeSecret.Data[v1.StoreSecretAWSAccessKeyIDKey]))
	require.Equal(t, "secret", string(storeSecret.Data[v1.StoreSecretAWSSecretAccessKeyKey]))
	require.Equal(t, "tideways-key", string(storeSecret.Data[v1.StoreSecretTidewaysAPIKeyKey]))
}

func TestEnsureStoreSecret_RotatesCopiedCredentials(t *testing.T) {
	store := storeWithCopiedCredentials()
	existing := existingStoreSecret(store, map[string][]byte{
		v1.StoreSecretAdminUsernameKey:      []byte("old-admin"),
		v1.StoreSecretAWSAccessKeyIDKey:     []byte("old-access"),
		v1.StoreSecretAWSSecretAccessKeyKey: []byte("old-secret"),
		v1.StoreSecretTidewaysAPIKeyKey:     []byte("old-tideways"),
	})

	storeSecret := ensureWithObjects(t, store, existing, credentialsSecret("new-admin", "new-access", "new-secret", "new-tideways"))

	require.Equal(t, "new-admin", string(storeSecret.Data[v1.StoreSecretAdminUsernameKey]))
	require.Equal(t, "new-access", string(storeSecret.Data[v1.StoreSecretAWSAccessKeyIDKey]))
	require.Equal(t, "new-secret", string(storeSecret.Data[v1.StoreSecretAWSSecretAccessKeyKey]))
	require.Equal(t, "new-tideways", string(storeSecret.Data[v1.StoreSecretTidewaysAPIKeyKey]))
}

func TestEnsureStoreSecret_RemovesStaleCopiedCredentials(t *testing.T) {
	staleData := func() map[string][]byte {
		return map[string][]byte{
			v1.StoreSecretAdminUsernameKey:      []byte("stale-admin"),
			v1.StoreSecretAWSAccessKeyIDKey:     []byte("stale-access"),
			v1.StoreSecretAWSSecretAccessKeyKey: []byte("stale-secret"),
			v1.StoreSecretTidewaysAPIKeyKey:     []byte("stale-tideways"),
		}
	}

	t.Run("references removed", func(t *testing.T) {
		store := createTestStore()
		store.Spec.Tideways.Enabled = true

		storeSecret := ensureWithObjects(t, store, existingStoreSecret(store, staleData()))

		for _, key := range []string{
			v1.StoreSecretAdminUsernameKey,
			v1.StoreSecretAWSAccessKeyIDKey,
			v1.StoreSecretAWSSecretAccessKeyKey,
			v1.StoreSecretTidewaysAPIKeyKey,
		} {
			require.NotContains(t, storeSecret.Data, key)
		}
	})

	t.Run("tideways disabled", func(t *testing.T) {
		store := storeWithCopiedCredentials()
		store.Spec.Tideways.Enabled = false

		storeSecret := ensureWithObjects(t, store,
			existingStoreSecret(store, staleData()),
			credentialsSecret("admin", "access", "secret", "tideways-key"),
		)

		require.NotContains(t, storeSecret.Data, v1.StoreSecretTidewaysAPIKeyKey)
		require.Equal(t, "admin", string(storeSecret.Data[v1.StoreSecretAdminUsernameKey]))
		require.Equal(t, "access", string(storeSecret.Data[v1.StoreSecretAWSAccessKeyIDKey]))
		require.Equal(t, "secret", string(storeSecret.Data[v1.StoreSecretAWSSecretAccessKeyKey]))
	})
}

func TestEnsureStoreSecret_MissingCopiedCredentialFails(t *testing.T) {
	store := storeWithCopiedCredentials()
	credentials := credentialsSecret("admin", "access", "secret", "tideways-key")
	delete(credentials.Data, "s3-secret")

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(store, createDBSecret(), credentials).Build()

	_, err := EnsureStoreSecret(context.Background(), c, &mockEventRecorder{}, store)
	require.Error(t, err)
}
