package k8s

import (
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	StoreSecretHashAnnotation = "shopware.com/store-secret-hash"
	changeReasonSecretChanged = "store secret differs from deployed object (secret hash changed)"
)

func SecretHash(secret *corev1.Secret) (string, error) {
	return ObjectHash(secret)
}

func SetSecretHash(obj runtime.Object, hash string) {
	template := podTemplate(obj)
	if template == nil || hash == "" {
		return
	}
	annotations := maps.Clone(template.Annotations)
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[StoreSecretHashAnnotation] = hash
	template.Annotations = annotations
}

func secretHashChanged(old, new runtime.Object) bool {
	desired := secretHashOf(new)
	return desired != "" && desired != secretHashOf(old)
}

func secretHashOf(obj runtime.Object) string {
	template := podTemplate(obj)
	if template == nil {
		return ""
	}
	return template.Annotations[StoreSecretHashAnnotation]
}

func withoutSecretHash(obj runtime.Object) runtime.Object {
	if secretHashOf(obj) == "" {
		return obj
	}
	copied := obj.DeepCopyObject()
	template := podTemplate(copied)
	template.Annotations = maps.Clone(template.Annotations)
	delete(template.Annotations, StoreSecretHashAnnotation)
	return copied
}

func podTemplate(obj runtime.Object) *corev1.PodTemplateSpec {
	switch object := obj.(type) {
	case *appsv1.Deployment:
		return &object.Spec.Template
	case *appsv1.StatefulSet:
		return &object.Spec.Template
	case *batchv1.Job:
		return &object.Spec.Template
	case *batchv1.CronJob:
		return &object.Spec.JobTemplate.Spec.Template
	default:
		return nil
	}
}
