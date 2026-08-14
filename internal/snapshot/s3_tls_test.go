package snapshot

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseSpecFromConfigRequiresClientCertificateAndKeyTogether(t *testing.T) {
	for _, database := range []config.DatabaseConfig{
		{SSLCert: "/tls/tls.crt"},
		{SSLKey: "/tls/tls.key"},
	} {
		_, err := databaseSpecFromConfig(database)
		require.EqualError(t, err, "database TLS client certificate and key must either both be set or both be empty")
	}
}

func TestDatabaseSpecFromConfigEnablesClientCertificateForACompletePair(t *testing.T) {
	spec, err := databaseSpecFromConfig(config.DatabaseConfig{
		SSLCert: "/tls/tls.crt",
		SSLKey:  "/tls/tls.key",
	})

	require.NoError(t, err)
	assert.True(t, spec.TLSClientCertificate)
}
