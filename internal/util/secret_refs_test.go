package util_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestGetSecretRefs(t *testing.T) {
	store := v1.Store{
		Spec: v1.StoreSpec{
			Database: v1.DatabaseSpec{
				HostRef:           v1.SecretRef{Name: "db", Key: "host"},
				PasswordSecretRef: v1.SecretRef{Name: "db", Key: "password"},
				TLS:               v1.DatabaseTLS{SecretName: "db-tls"},
			},
			OpensearchSpec: v1.OpensearchSpec{
				PasswordSecretRef: v1.SecretRef{Name: "opensearch", Key: "password"},
			},
			ShopConfiguration: v1.Configuration{
				Fastly: v1.FastlySpec{
					TokenRef:   v1.SecretRef{Name: "fastly", Key: "token"},
					ServiceRef: v1.SecretRef{Name: "fastly", Key: "service"},
				},
			},
			AdminCredentials: v1.Credentials{
				UsernameSecretRef: v1.SecretRef{Name: "admin", Key: "username"},
				PasswordSecretRef: v1.SecretRef{Name: "admin", Key: "password"},
			},
			Tideways: v1.TidewaysSpec{
				APIKeyRef: v1.SecretRef{Name: "tideways", Key: "api-key"},
			},
			S3Storage: v1.S3Storage{
				AccessKeyRef:       v1.SecretRef{Name: "s3", Key: "access"},
				SecretAccessKeyRef: v1.SecretRef{Name: "s3", Key: "secret"},
			},
		},
	}

	assert.ElementsMatch(t, []v1.SecretRef{
		{Name: "db", Key: "host"},
		{Name: "db", Key: "password"},
		{Name: "opensearch", Key: "password"},
		{Name: "fastly", Key: "token"},
		{Name: "fastly", Key: "service"},
		{Name: "admin", Key: "username"},
		{Name: "admin", Key: "password"},
		{Name: "tideways", Key: "api-key"},
		{Name: "s3", Key: "access"},
		{Name: "s3", Key: "secret"},
	}, util.GetSecretRefs(store))

	assert.ElementsMatch(t,
		[]string{"db", "opensearch", "fastly", "admin", "tideways", "s3"},
		util.GetReferencedSecretNames(store),
	)
}

func TestGetReferencedSecretNamesSkipsEmpty(t *testing.T) {
	assert.Empty(t, util.GetReferencedSecretNames(v1.Store{}))
}
