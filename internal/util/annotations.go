package util

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

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

func GetDefaultContainerSnapshotAnnotations(defaultContainer string, sn v1.StoreSnapshotSpec, namespace string) map[string]string {
	annotations := make(map[string]string)
	if sn.Container.Annotations != nil {
		maps.Copy(annotations, sn.Container.Annotations)
	}
	annotations["kubectl.kubernetes.io/default-container"] = defaultContainer
	annotations["kubectl.kubernetes.io/default-logs-container"] = defaultContainer
	key := fmt.Sprintf("ad.datadoghq.com/%s.logs", defaultContainer)
	if raw, ok := annotations[key]; ok {
		annotations[key] = appendDatadogLogNamespace(raw, namespace)
	}
	return annotations
}

// appendDatadogLogNamespace adds namespace:<ns> to an existing Autodiscovery
// logs JSON tags list. Namespace is for Datadog only, not S3 object tags.
func appendDatadogLogNamespace(raw, namespace string) string {
	if namespace == "" {
		return raw
	}
	tag := "namespace:" + namespace
	if strings.Contains(raw, tag) {
		return raw
	}
	marker := `"tags":[`
	idx := strings.Index(raw, marker)
	if idx == -1 {
		marker = `"tags": [`
		idx = strings.Index(raw, marker)
	}
	if idx == -1 {
		return raw
	}
	insertAt := idx + len(marker)
	quoted := strconv.Quote(tag)
	rest := strings.TrimLeft(raw[insertAt:], " \t\n")
	if strings.HasPrefix(rest, "]") {
		return raw[:insertAt] + quoted + raw[insertAt:]
	}
	return raw[:insertAt] + quoted + "," + raw[insertAt:]
}
