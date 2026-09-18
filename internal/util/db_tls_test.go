package util

import (
	"context"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDBSpecLoadsDatabaseTLSSecret(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec: v1.StoreSpec{Database: v1.DatabaseSpec{
			PasswordSecretRef: v1.SecretRef{Name: "database", Key: "password"},
			TLS:               v1.DatabaseTLS{SecretName: "database-tls", ClientCertificate: true, DontVerifyServerCertificate: true},
		}},
	}
	client := fake.NewClientBuilder().WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "test"}, Data: map[string][]byte{"password": []byte("secret")}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-tls", Namespace: "test"}, Data: map[string][]byte{"ca.crt": []byte("ca"), "tls.crt": []byte("cert"), "tls.key": []byte("key")}},
	).Build()

	spec, err := GetDBSpec(context.Background(), store, client)
	require.NoError(t, err)
	assert.Equal(t, []byte("ca"), spec.TLSCA)
	assert.Equal(t, []byte("cert"), spec.TLSCert)
	assert.Equal(t, []byte("key"), spec.TLSKey)
	assert.True(t, spec.TLSDontVerifyServerCertificate)
}

func TestGetDBSpecRequiresTLSSecretForLegacyRequiredMode(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec: v1.StoreSpec{Database: v1.DatabaseSpec{
			PasswordSecretRef: v1.SecretRef{Name: "database", Key: "password"},
			SSLMode:           "REQUIRED", //nolint:staticcheck
		}},
	}
	client := fake.NewClientBuilder().WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "test"}, Data: map[string][]byte{"password": []byte("secret")}},
	).Build()

	_, err := GetDBSpec(context.Background(), store, client)
	require.EqualError(t, err, "database tls secretName is required when sslMode is REQUIRED")
}

func TestGetDBSpecRequiresDatabaseTLSCA(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
		Spec: v1.StoreSpec{Database: v1.DatabaseSpec{
			PasswordSecretRef: v1.SecretRef{Name: "database", Key: "password"},
			TLS:               v1.DatabaseTLS{SecretName: "database-tls"},
		}},
	}
	client := fake.NewClientBuilder().WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "test"}, Data: map[string][]byte{"password": []byte("secret")}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "database-tls", Namespace: "test"}},
	).Build()

	_, err := GetDBSpec(context.Background(), store, client)
	require.EqualError(t, err, "ca.crt key not found in database tls secret database-tls")
}
