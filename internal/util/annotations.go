package util

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

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
	var configs []map[string]any
	if err := json.Unmarshal([]byte(raw), &configs); err != nil || len(configs) == 0 {
		return raw
	}
	tag := "namespace:" + namespace
	changed := false
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		tags := datadogLogTags(cfg)
		if slices.Contains(tags, tag) {
			continue
		}
		cfg["tags"] = append(tags, tag)
		changed = true
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(configs)
	if err != nil {
		return raw
	}
	return string(out)
}

func datadogLogTags(cfg map[string]any) []string {
	raw, ok := cfg["tags"]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(items)+1)
	for _, item := range items {
		tag, ok := item.(string)
		if ok {
			tags = append(tags, tag)
		}
	}
	return tags
}
