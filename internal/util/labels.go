package util

import (
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetPDBLabels(store v1.Store) map[string]string {
	return map[string]string{
		"store-pdb": store.Name,
	}
}

func GetDefaultStoreLabels(store v1.Store) map[string]string {
	return map[string]string{
		"store": store.Name,
	}
}

func GetDefaultContainerStoreLabels(store v1.Store, overwrite map[string]string) map[string]string {
	labels := make(map[string]string)
	if store.Spec.Container.Labels != nil {
		labels = store.Spec.Container.Labels
	}
	labels["store"] = store.Name
	if overwrite != nil {
		maps.Copy(labels, overwrite)
	}
	return labels
}

func GetDefaultStoreExecLabels(store v1.Store, ex v1.StoreExec) map[string]string {
	labels := make(map[string]string)
	if store.Spec.Container.Labels != nil {
		labels = store.Spec.Container.Labels
	}
	labels["store"] = store.Name
	labels["storeexec"] = ex.Name
	return labels
}
