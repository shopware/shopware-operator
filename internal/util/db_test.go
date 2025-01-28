package util_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestDatabaseConnectionStringTest(t *testing.T) {
	dbSpec := &v1.DatabaseSpec{
		Host:              "host",
		HostRef:           v1.SecretRef{},
		Port:              1234,
		Version:           "v2",
		User:              "user",
		Name:              "testName",
		SSLMode:           "REQUIRED",
		Options:           "tls-version=TLSv1.3&auth-method=AUTO",
		PasswordSecretRef: v1.SecretRef{},
	}

	b := util.GenerateDatabaseURLForShopware(dbSpec, "host", []byte("password"))
	assert.Equal(t, "mysql://user:password@host:1234/testName?serverVersion=v2&sslMode=REQUIRED&tls-version=TLSv1.3&auth-method=AUTO", string(b))
}
