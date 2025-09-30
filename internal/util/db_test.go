package util_test

import (
	"testing"

	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestDatabaseConnectionStringTest(t *testing.T) {
	dbSpec := &util.DatabaseSpec{
		Host:     "host",
		Port:     1234,
		Version:  "v2",
		User:     "user",
		Name:     "testName",
		SSLMode:  "REQUIRED",
		Options:  "tls-version=TLSv1.3&auth-method=AUTO",
		Password: []byte("password"),
	}

	b := util.GenerateDatabaseURLForShopware(dbSpec)
	assert.Equal(t, "mysql://user:password@host:1234/testName?serverVersion=v2&sslMode=REQUIRED&tls-version=TLSv1.3&auth-method=AUTO", string(b))
}
