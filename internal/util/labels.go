package util

import (
	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetDefaultLabels(store *v1.Store) map[string]string {
	return map[string]string{
		"store": store.Name,
	}
}
