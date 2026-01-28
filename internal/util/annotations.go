package util

import (
	"maps"

	v1 "github.com/shopware/shopware-operator/api/v1"
)

func GetDefaultContainerAnnotations(defaultContainer string, store v1.Store, overwrite map[string]string) map[string]string {
	annotations := make(map[string]string, len(store.Spec.Container.Annotations)+2)
	maps.Copy(annotations, store.Spec.Container.Annotations)
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	if overwrite != nil {
		maps.Copy(annotations, overwrite)
	}
	return annotations
}

func GetDefaultContainerExecAnnotations(defaultContainer string, ex v1.StoreExec) map[string]string {
	annotations := make(map[string]string, len(ex.Spec.Container.Annotations)+2)
	maps.Copy(annotations, ex.Spec.Container.Annotations)
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	return annotations
}

func GetDefaultContainerSnapshotAnnotations(defaultContainer string, sn v1.StoreSnapshotSpec) map[string]string {
	annotations := make(map[string]string, len(sn.Container.Annotations)+2)
	maps.Copy(annotations, sn.Container.Annotations)
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	return annotations
}
