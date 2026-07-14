package util

import (
	"fmt"
	"maps"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

const shopwareLabelPrefix = "shop.shopware.com/"

func ShopwareKey(key string) string {
	return shopwareLabelPrefix + key
}

func GetDefaultStoreLabels(store v1.Store) map[string]string {
	return map[string]string{
		ShopwareKey("store.name"): store.Name,
	}
}

func overrideStoreLabels(store v1.Store, extra map[string]string) map[string]string {
	labels := GetDefaultStoreLabels(store)
	if store.Spec.Container.Labels != nil {
		maps.Copy(labels, store.Spec.Container.Labels)
	}
	if extra != nil {
		maps.Copy(labels, extra)
	}
	return labels
}

func GetDefaultContainerStoreLabels(store v1.Store, extra map[string]string) map[string]string {
	return overrideStoreLabels(store, extra)
}

func GetDefaultStoreExecLabels(store v1.Store, extra map[string]string, name, execType string) map[string]string {
	labels := overrideStoreLabels(store, extra)
	labels[ShopwareKey("storeexec.name")] = name
	labels[ShopwareKey("storeexec.type")] = execType
	return labels
}

func GetDefaultStoreSnapshotLabels(store v1.Store, overwrite map[string]string, name, subCommand string) map[string]string {
	labels := overrideStoreLabels(store, overwrite)
	labels[ShopwareKey("storesnapshot.name")] = name
	labels[ShopwareKey("storesnapshot.type")] = subCommand
	return labels
}

func GetDefaultStoreInstanceDebugLabels(store v1.Store, storeDebugInstance v1.StoreDebugInstance) map[string]string {
	labels := GetDefaultContainerStoreLabels(store, storeDebugInstance.Spec.ExtraLabels)

	// we don't need to check for errors here, because the duration is validated in the controller
	duration, _ := time.ParseDuration(storeDebugInstance.Spec.Duration)
	validUntil := storeDebugInstance.CreationTimestamp.Add(duration)

	labels[ShopwareKey("store.debug")] = "true"
	labels[ShopwareKey("store.debug.instance")] = storeDebugInstance.Name
	labels[ShopwareKey("store.debug.validUntil")] = fmt.Sprintf("%d", validUntil.UnixNano())

	return labels
}

func GetAdminDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels[ShopwareKey("store.app")] = "shopware-admin"
	return labels
}

func GetStorefrontDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels[ShopwareKey("store.app")] = "shopware-storefront"
	return labels
}

func GetWorkerDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels[ShopwareKey("store.app")] = "shopware-worker"
	return labels
}
