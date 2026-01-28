package util

import (
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const LastConfigHashAnnotation = "shopware.com/last-config-hash"

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

func DeploymentStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDeployment, okOld := e.ObjectOld.(*appsv1.Deployment)
			newDeployment, okNew := e.ObjectNew.(*appsv1.Deployment)
			if !okOld || !okNew || oldDeployment == nil || newDeployment == nil {
				return false
			}
			return deploymentStatusChanged(oldDeployment.Status, newDeployment.Status)
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func deploymentStatusChanged(oldStatus, newStatus appsv1.DeploymentStatus) bool {
	if oldStatus.Replicas != newStatus.Replicas ||
		oldStatus.AvailableReplicas != newStatus.AvailableReplicas ||
		oldStatus.UnavailableReplicas != newStatus.UnavailableReplicas {
		return true
	}

	if len(oldStatus.Conditions) != len(newStatus.Conditions) {
		return true
	}
	for i, oldCondition := range oldStatus.Conditions {
		newCondition := newStatus.Conditions[i]
		if oldCondition.Type != newCondition.Type ||
			oldCondition.Status != newCondition.Status ||
			oldCondition.Reason != newCondition.Reason {
			return true
		}
	}
	return false
}
