package util_test

import (
	"fmt"
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeSnapshotLabelKey(t *testing.T) {
	require.Equal(t, "application_id", util.NormalizeSnapshotLabelKey("application-id"))
	require.Equal(t, "application_name", util.NormalizeSnapshotLabelKey("application-name"))
	require.Equal(t, "application_id", util.NormalizeSnapshotLabelKey("application_id"))
	require.Equal(t, "tags.datadoghq.com/service", util.NormalizeSnapshotLabelKey("tags.datadoghq.com/service"))
}

func TestLabelMerge(t *testing.T) {
	store := v1.Store{
		ObjectMeta: metav1.ObjectMeta{
			Name: "name",
		},
		Spec: v1.StoreSpec{
			Container: v1.ContainerSpec{
				Labels: map[string]string{
					"test": "test",
					"app":  "selector",
				},
			},
		},
	}

	overwrite := make(map[string]string)
	overwrite["test"] = "overwrite"
	overwrite["test2"] = "test2"

	labels := util.GetDefaultContainerStoreLabels(store, overwrite)
	fmt.Println(labels)
	require.Equal(t, "selector", labels["app"])
	require.Equal(t, "overwrite", labels["test"])
	require.Equal(t, "test2", labels["test2"])
	require.Equal(t, "name", labels[util.ShopwareKey("store.name")])
}
