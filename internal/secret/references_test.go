package secret

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func referenceTestStore() *v1.Store {
	store := &v1.Store{ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: "shop"}}
	store.Spec.Database.PasswordSecretRef = v1.SecretRef{Name: "db", Key: "password"}
	store.Spec.Database.HostRef = v1.SecretRef{Name: "db", Key: "host"}
	store.Spec.Database.TLS.SecretName = "db-tls"
	store.Spec.OpensearchSpec.PasswordSecretRef = v1.SecretRef{Name: "opensearch", Key: "password"}
	store.Spec.ShopConfiguration.Fastly.TokenRef = v1.SecretRef{Name: "fastly", Key: "token"}
	store.Spec.ShopConfiguration.Fastly.ServiceRef = v1.SecretRef{Name: "fastly", Key: "service"}
	store.Spec.AdminCredentials.UsernameSecretRef = v1.SecretRef{Name: "admin", Key: "username"}
	store.Spec.AdminCredentials.PasswordSecretRef = v1.SecretRef{Name: "admin", Key: "password"}
	store.Spec.Tideways.APIKeyRef = v1.SecretRef{Name: "tideways", Key: "key"}
	store.Spec.S3Storage.AccessKeyRef = v1.SecretRef{Name: "s3", Key: "access"}
	store.Spec.S3Storage.SecretAccessKeyRef = v1.SecretRef{Name: "s3", Key: "secret"}
	return store
}

func TestReferencedSecretNames(t *testing.T) {
	require.ElementsMatch(t,
		[]string{"db", "db-tls", "opensearch", "fastly", "admin", "tideways", "s3"},
		referencedSecretNames(referenceTestStore()),
	)
	require.Empty(t, referencedSecretNames(&v1.Store{}))
}

func TestIsReferencedByStore(t *testing.T) {
	store := referenceTestStore()

	tests := []struct {
		name      string
		secret    string
		namespace string
		want      bool
	}{
		{name: "referenced secret", secret: "db", namespace: "shop", want: true},
		{name: "other namespace", secret: "db", namespace: "other", want: false},
		{name: "unreferenced secret", secret: "unknown", namespace: "shop", want: false},
		{name: "store secret", secret: "store-secret", namespace: "shop", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tt.secret, Namespace: tt.namespace}}
			require.Equal(t, tt.want, IsReferencedByStore(store, obj))
		})
	}
}
