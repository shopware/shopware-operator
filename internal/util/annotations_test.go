package util

import (
	"encoding/json"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendDatadogLogNamespace(t *testing.T) {
	in := `[{"source":"shopsnapshot","service":"shopsnapshot-create","tags":["component:shopsnapshot.create"]}]`
	out := appendDatadogLogNamespace(in, "shop-ns")
	assert.Contains(t, out, `"namespace:shop-ns"`)
	assert.Contains(t, out, `"component:shopsnapshot.create"`)
	assert.Equal(t, out, appendDatadogLogNamespace(out, "shop-ns"))
}

func TestAppendDatadogLogNamespaceHandlesSpacedAndCRLFEmptyTags(t *testing.T) {
	for _, in := range []string{
		`[{"source":"shopsnapshot","tags" : []}]`,
		"[{\"source\":\"shopsnapshot\",\"tags\":[\r\n]}]",
	} {
		out := appendDatadogLogNamespace(in, "shop")
		var configs []map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &configs), in)
		require.Len(t, configs, 1)
		assert.Equal(t, []any{"namespace:shop"}, configs[0]["tags"])
		assert.Equal(t, "shopsnapshot", configs[0]["source"])
	}
}

func TestAppendDatadogLogNamespaceDoesNotMatchTagPrefix(t *testing.T) {
	in := `[{"source":"shopsnapshot","extra":true,"tags":["namespace:shop-old"]}]`
	out := appendDatadogLogNamespace(in, "shop")
	var configs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &configs))
	require.Len(t, configs, 1)
	assert.Equal(t, []any{"namespace:shop-old", "namespace:shop"}, configs[0]["tags"])
	assert.Equal(t, true, configs[0]["extra"])
}

func TestAppendDatadogLogNamespacePassesThroughNullConfigs(t *testing.T) {
	in := `[null]`
	assert.Equal(t, in, appendDatadogLogNamespace(in, "shop"))
}

func TestGetDefaultContainerSnapshotAnnotationsAddsNamespaceToDatadogLogs(t *testing.T) {
	spec := v1.StoreSnapshotSpec{
		Container: v1.ContainerSpec{
			Annotations: map[string]string{
				"ad.datadoghq.com/operator-snapshot.logs": `[{"source":"shopsnapshot","tags":["application_id:abc"]}]`,
			},
		},
	}
	got := GetDefaultContainerSnapshotAnnotations("operator-snapshot", spec, "shop-ns")
	assert.Contains(t, got["ad.datadoghq.com/operator-snapshot.logs"], "namespace:shop-ns")
	assert.NotContains(t, spec.Container.Annotations["ad.datadoghq.com/operator-snapshot.logs"], "namespace:shop-ns")
}

func TestGetDefaultContainerSnapshotAnnotationsLeavesEmptyLogs(t *testing.T) {
	spec := v1.StoreSnapshotSpec{
		Container: v1.ContainerSpec{
			Annotations: map[string]string{
				"ad.datadoghq.com/operator-snapshot.logs": "[]",
			},
		},
	}
	got := GetDefaultContainerSnapshotAnnotations("operator-snapshot", spec, "shop-ns")
	assert.Equal(t, "[]", got["ad.datadoghq.com/operator-snapshot.logs"])
}
