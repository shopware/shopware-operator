package util_test

import (
	"testing"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/util"
	"github.com/stretchr/testify/require"
)

func TestGetDefaultContainerAnnotationsDoesNotShareStoreMap(t *testing.T) {
	store := v1.Store{Spec: v1.StoreSpec{Container: v1.ContainerSpec{
		Annotations: map[string]string{"custom": "value"},
	}}}

	storefront := util.GetDefaultContainerAnnotations("shopware-storefront", store, map[string]string{"storefront": "only"})
	admin := util.GetDefaultContainerAnnotations("shopware-admin", store, nil)
	storefront[util.LastConfigHashAnnotation] = "storefront-hash"

	require.Equal(t, map[string]string{"custom": "value"}, store.Spec.Container.Annotations)
	require.Equal(t, "shopware-storefront", storefront["kubectl.kubernetes.io/default-container"])
	require.Equal(t, "shopware-admin", admin["kubectl.kubernetes.io/default-container"])
	require.NotContains(t, admin, "storefront")
	require.NotContains(t, admin, util.LastConfigHashAnnotation)
}

func TestGetDefaultContainerExecAnnotationsDoesNotShareMap(t *testing.T) {
	ex := v1.StoreExec{Spec: v1.StoreExecSpec{Container: v1.ContainerSpec{
		Annotations: map[string]string{"custom": "value"},
	}}}

	annotations := util.GetDefaultContainerExecAnnotations("shopware", ex)
	annotations["added"] = "x"

	require.Equal(t, map[string]string{"custom": "value"}, ex.Spec.Container.Annotations)
}

func TestGetDefaultContainerSnapshotAnnotationsDoesNotShareMap(t *testing.T) {
	sn := v1.StoreSnapshotSpec{Container: v1.ContainerSpec{
		Annotations: map[string]string{"custom": "value"},
	}}

	annotations := util.GetDefaultContainerSnapshotAnnotations("snapshot", sn)
	annotations["added"] = "x"

	require.Equal(t, map[string]string{"custom": "value"}, sn.Container.Annotations)
}
