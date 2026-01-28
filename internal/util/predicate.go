package util

import (
	"maps"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const LastConfigHashAnnotation = "shopware.com/last-config-hash"

var ControllerManagedAnnotations = []string{"deployment.kubernetes.io/revision"}

func HashAnnotationChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			return e.ObjectOld.GetAnnotations()[LastConfigHashAnnotation] !=
				e.ObjectNew.GetAnnotations()[LastConfigHashAnnotation]
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func MetadataChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			return !maps.Equal(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
				!maps.Equal(withoutControllerManagedAnnotations(e.ObjectOld.GetAnnotations()),
					withoutControllerManagedAnnotations(e.ObjectNew.GetAnnotations()))
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func withoutControllerManagedAnnotations(annotations map[string]string) map[string]string {
	filtered := maps.Clone(annotations)
	for _, key := range ControllerManagedAnnotations {
		delete(filtered, key)
	}
	return filtered
}

func DeletingPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew != nil && !e.ObjectNew.GetDeletionTimestamp().IsZero()
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func JobLifecyclePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, okOld := e.ObjectOld.(*batchv1.Job)
			newJob, okNew := e.ObjectNew.(*batchv1.Job)
			if !okOld || !okNew || oldJob == nil || newJob == nil {
				return false
			}
			return oldJob.GetAnnotations()[LastConfigHashAnnotation] != newJob.GetAnnotations()[LastConfigHashAnnotation] ||
				jobProgressChanged(oldJob.Status, newJob.Status)
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func jobProgressChanged(oldStatus, newStatus batchv1.JobStatus) bool {
	if oldStatus.Active != newStatus.Active ||
		oldStatus.Succeeded != newStatus.Succeeded ||
		oldStatus.Failed != newStatus.Failed {
		return true
	}
	return jobFinished(oldStatus) != jobFinished(newStatus)
}

func jobFinished(status batchv1.JobStatus) bool {
	for _, condition := range status.Conditions {
		if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) &&
			condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
