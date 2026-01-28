package secret

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/require"
)

func TestReferencedSecretNames(t *testing.T) {
	store := createTestStore()
	store.Spec.Database.HostRef = v1.SecretRef{Name: "db-host", Key: "host"}
	store.Spec.Database.TLS.SecretName = "db-tls"
	store.Spec.OpensearchSpec.PasswordSecretRef = v1.SecretRef{Name: "opensearch", Key: "password"}
	store.Spec.ShopConfiguration.Fastly.TokenRef = v1.SecretRef{Name: "fastly", Key: "token"}
	store.Spec.ShopConfiguration.Fastly.ServiceRef = v1.SecretRef{Name: "fastly", Key: "service"}
	store.Spec.AdminCredentials.UsernameSecretRef = v1.SecretRef{Name: "admin", Key: "username"}
	store.Spec.AdminCredentials.PasswordSecretRef = v1.SecretRef{Name: "admin", Key: "password"}
	store.Spec.S3Storage.AccessKeyRef = v1.SecretRef{Name: "s3", Key: "access"}
	store.Spec.S3Storage.SecretAccessKeyRef = v1.SecretRef{Name: "s3", Key: "secret"}
	store.Spec.Tideways.APIKeyRef = v1.SecretRef{Name: "tideways", Key: "key"}

	require.ElementsMatch(t, []string{
		"test-store-secret",
		"db-secret",
		"db-host",
		"db-tls",
		"opensearch",
		"fastly",
		"admin",
		"s3",
		"tideways",
	}, ReferencedSecretNames(store))
}

func TestReferencesSecret(t *testing.T) {
	store := createTestStore()
	store.Spec.S3Storage.AccessKeyRef = v1.SecretRef{Name: "s3", Key: "access"}

	require.True(t, ReferencesSecret(store, "test-store-secret"))
	require.True(t, ReferencesSecret(store, "db-secret"))
	require.True(t, ReferencesSecret(store, "s3"))
	require.False(t, ReferencesSecret(store, "other-store-secret"))
	require.False(t, ReferencesSecret(store, ""))
}
