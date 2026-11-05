package util

import (
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

const (
	DefaultContainerAnnotation     = "kubectl.kubernetes.io/default-container"
	DefaultLogsContainerAnnotation = "kubectl.kubernetes.io/default-logs-container"
	LastConfigHashAnnotation       = "shop.shopware.com/last-config-hash"
	StoreSecretHashAnnotation      = "shop.shopware.com/store-secret-hash"
	OldImageAnnotation             = "shop.shopware.com/store.oldImage"
	NewImageAnnotation             = "shop.shopware.com/store.newImage"
)

func GetDefaultContainerAnnotations(defaultContainer string, store v1.Store, overwrite map[string]string) map[string]string {
	annotations := make(map[string]string)
	maps.Copy(annotations, store.Spec.Container.Annotations)
	annotations[DefaultContainerAnnotation] = defaultContainer
	annotations[DefaultLogsContainerAnnotation] = defaultContainer
	if overwrite != nil {
		maps.Copy(annotations, overwrite)
	}
	return annotations
}

func GetDefaultContainerExecAnnotations(defaultContainer string, ex v1.StoreExec) map[string]string {
	annotations := make(map[string]string)
	maps.Copy(annotations, ex.Spec.Container.Annotations)
	annotations[DefaultContainerAnnotation] = defaultContainer
	annotations[DefaultLogsContainerAnnotation] = defaultContainer
	return annotations
}

func GetDefaultContainerSnapshotAnnotations(defaultContainer string, sn v1.StoreSnapshotSpec) map[string]string {
	annotations := make(map[string]string)
	maps.Copy(annotations, sn.Container.Annotations)
	annotations[DefaultContainerAnnotation] = defaultContainer
	annotations[DefaultLogsContainerAnnotation] = defaultContainer
	return annotations
}
