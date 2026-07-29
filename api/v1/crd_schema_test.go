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
