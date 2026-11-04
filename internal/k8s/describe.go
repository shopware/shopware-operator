package k8s

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DescribeUpdate(oldObject, newObject client.Object) (string, []string) {
	if oldObject == nil || newObject == nil {
		return "only status or resourceVersion changed", nil
	}
	if oldObject.GetDeletionTimestamp().IsZero() && !newObject.GetDeletionTimestamp().IsZero() {
		return "deletion requested", nil
	}
	if oldObject.GetGeneration() != newObject.GetGeneration() {
		return "generation changed", []string{
			fmt.Sprintf("generation:%d->%d", oldObject.GetGeneration(), newObject.GetGeneration()),
		}
	}
	oldHash := oldObject.GetAnnotations()[lastConfigHashAnnotation]
	newHash := newObject.GetAnnotations()[lastConfigHashAnnotation]
	if oldHash != newHash {
		return "config hash annotation changed", []string{fmt.Sprintf("hash:%s->%s", oldHash, newHash)}
	}

	if keys := ChangedMetaKeys(oldObject, newObject); len(keys) != 0 {
		return "labels or annotations changed", keys
	}
	return "only status or resourceVersion changed", []string{
		fmt.Sprintf("resourceVersion:%s->%s", oldObject.GetResourceVersion(), newObject.GetResourceVersion()),
	}
}
