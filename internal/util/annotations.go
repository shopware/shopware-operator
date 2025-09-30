package util

import (
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetDefaultContainerAnnotations(defaultContainer string, store v1.Store, overwrite map[string]string) map[string]string {
	annotations := make(map[string]string)
	if store.Spec.Container.Annotations != nil {
		annotations = store.Spec.Container.Annotations
	}
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	if overwrite != nil {
		maps.Copy(annotations, overwrite)
	}
	return annotations
}

func GetDefaultContainerExecAnnotations(defaultContainer string, ex v1.StoreExec) map[string]string {
	annotations := make(map[string]string)
	if ex.Spec.Container.Annotations != nil {
		annotations = ex.Spec.Container.Annotations
	}
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	return annotations
}

func GetDefaultContainerSnapshotAnnotations(defaultContainer string, sn v1.StoreSnapshotSpec) map[string]string {
	annotations := make(map[string]string)
	if sn.Container.Annotations != nil {
		annotations = sn.Container.Annotations
	}
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	return annotations
}
