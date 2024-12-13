package util

import (
	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetDefaultStoreLabels(store *v1.Store) map[string]string {
	return map[string]string{
		"store": store.Name,
	}
}

func GetDefaultStoreExecLabels(store *v1.Store, ex *v1.StoreExec) map[string]string {
	return map[string]string{
		"store":     store.Name,
		"storeexec": ex.Name,
	}
}
