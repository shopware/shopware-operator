package v1_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestStoreDatabaseTLSEnvironmentAndVolume(t *testing.T) {
	store := v1.Store{Spec: v1.StoreSpec{Database: v1.DatabaseSpec{TLS: v1.DatabaseTLS{
		SecretName:                  "database-tls",
		ClientCertificate:           true,
		DontVerifyServerCertificate: true,
	}}}}

	env := map[string]string{}
	for _, variable := range store.GetEnv() {
		env[variable.Name] = variable.Value
	}

	assert.Equal(t, "/etc/shopware/database-tls/ca.crt", env["DATABASE_SSL_CA"])
	assert.Equal(t, "/etc/shopware/database-tls/tls.crt", env["DATABASE_SSL_CERT"])
	assert.Equal(t, "/etc/shopware/database-tls/tls.key", env["DATABASE_SSL_KEY"])
	assert.Equal(t, "1", env["DATABASE_SSL_DONT_VERIFY_SERVER_CERT"])

	volumes := store.GetDatabaseTLSVolumes()
	mounts := store.GetDatabaseTLSVolumeMounts()
	assert.Len(t, volumes, 1)
	assert.Len(t, mounts, 1)
	assert.Equal(t, "shopware-database-tls", volumes[0].Name)
	assert.Equal(t, "database-tls", volumes[0].Secret.SecretName)
	assert.Equal(t, "/etc/shopware/database-tls", mounts[0].MountPath)
	assert.True(t, mounts[0].ReadOnly)
}
