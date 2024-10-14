package util

import (
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetServiceAccountName(store *v1.Store) string {
	return fmt.Sprintf("%s-store-sa", store.Name)
}
