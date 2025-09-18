package util

import (
	"fmt"
	"maps"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetDefaultStoreLabels(store v1.Store) map[string]string {
	return map[string]string{
		"shop.shopware.com/store.name": store.Name,
	}
}

func GetDefaultContainerStoreLabels(store v1.Store, overwrite map[string]string) map[string]string {
	labels := make(map[string]string)
	if store.Spec.Container.Labels != nil {
		maps.Copy(labels, store.Spec.Container.Labels)
	}
	labels["shop.shopware.com/store.name"] = store.Name
	if overwrite != nil {
		maps.Copy(labels, overwrite)
	}
	return labels
}

func GetDefaultStoreExecLabels(store v1.Store, ex v1.StoreExec) map[string]string {
	labels := make(map[string]string)
	if store.Spec.Container.Labels != nil {
		maps.Copy(labels, store.Spec.Container.Labels)
	}
	labels["shop.shopware.com/store.name"] = store.Name
	labels["shop.shopware.com/storeexec.name"] = ex.Name
	return labels
}

func GetDefaultStoreSnapshotLabels(store v1.Store, meta metav1.ObjectMeta) map[string]string {
	labels := make(map[string]string)
	if store.Spec.Container.Labels != nil {
		maps.Copy(labels, store.Spec.Container.Labels)
	}
	labels["shop.shopware.com/store.name"] = store.Name
	labels["shop.shopware.com/storesnapshot.name"] = meta.Name
	return labels
}

func GetDefaultStoreInstanceDebugLabels(store v1.Store, storeDebugInstance v1.StoreDebugInstance) map[string]string {
	labels := GetDefaultContainerStoreLabels(store, storeDebugInstance.Spec.ExtraLabels)

	// we don't need to check for errors here, because the duration is validated in the controller
	duration, _ := time.ParseDuration(storeDebugInstance.Spec.Duration)
	validUntil := storeDebugInstance.CreationTimestamp.Add(duration)

	labels["shop.shopware.com/store.debug"] = "true"
	labels["shop.shopware.com/store.debug.instance"] = storeDebugInstance.Name
	labels["shop.shopware.com/store.debug.validUntil"] = fmt.Sprintf("%d", validUntil.UnixNano())

	return labels
}

func GetAdminDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels["shop.shopware.com/store.app"] = "shopware-admin"
	return labels
}

func GetStorefrontDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels["shop.shopware.com/store.app"] = "shopware-storefront"
	return labels
}

func GetWorkerDeploymentMatchLabel() map[string]string {
	labels := make(map[string]string)
	labels["shop.shopware.com/store.app"] = "shopware-worker"
	return labels
}
