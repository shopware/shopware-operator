package v1_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCRDUsesCompactContainerOverrideSchemas(t *testing.T) {
	t.Parallel()

	crd, err := os.ReadFile("../../config/crd/bases/shop.shopware.com_stores.yaml")
	require.NoError(t, err)
	require.Less(t, len(crd), 600*1024, "Store CRD must remain below 600 KiB")

	for _, fieldName := range []string{
		"adminDeploymentContainer",
		"workerDeploymentContainer",
		"storefrontDeploymentContainer",
		"setupJobContainer",
		"migrationJobContainer",
	} {
		assert.Contains(t, string(crd), fieldName+":")
	}

	assert.Equal(t, 5, bytes.Count(crd, []byte("x-kubernetes-preserve-unknown-fields: true")))
}

func TestStoreCRDContainsSecretRefValidations(t *testing.T) {
	t.Parallel()

	crd, err := os.ReadFile("../../config/crd/bases/shop.shopware.com_stores.yaml")
	require.NoError(t, err)

	for _, expected := range []string{
		"rule: (has(self.host) && size(self.host) > 0) || has(self.hostRef)",
		"message: either database host or hostRef must be set",
		"rule: '!self.enabled || has(self.passwordSecretRef)'",
		"rule: (has(self.username) && size(self.username) > 0) || has(self.usernameSecretRef)",
		"message: admin username or usernameSecretRef is required",
		"rule: '!has(self.serviceRef) || has(self.tokenRef)'",
		"message: fastly tokenRef is required when serviceRef is set",
	} {
		assert.Contains(t, string(crd), expected)
	}
}

func TestCRDsLimitMetadataNameLength(t *testing.T) {
	t.Parallel()

	for _, file := range []string{
		"shop.shopware.com_stores.yaml",
		"shop.shopware.com_storeexecs.yaml",
		"shop.shopware.com_storedebuginstances.yaml",
		"shop.shopware.com_storesnapshotcreates.yaml",
		"shop.shopware.com_storesnapshotrestores.yaml",
	} {
		crd, err := os.ReadFile("../../config/crd/bases/" + file)
		require.NoError(t, err)
		assert.Contains(t, string(crd), "size(self.metadata.name) <= 63", file)
	}
}
