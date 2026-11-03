package util

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestAppendDatadogLogNamespace(t *testing.T) {
	in := `[{"source":"shopsnapshot","service":"shopsnapshot-create","tags":["component:shopsnapshot.create"]}]`
	out := appendDatadogLogNamespace(in, "shop-ns")
	assert.Contains(t, out, `"namespace:shop-ns"`)
	assert.Contains(t, out, `"component:shopsnapshot.create"`)
	assert.Equal(t, out, appendDatadogLogNamespace(out, "shop-ns"))
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
