package controller

import (
	"fmt"
	"reflect"
	"slices"
	"sync"

	v1 "github.com/shopware/shopware-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	maxReconcileReasons    = 20
	requeueReconcileReason = "requeue"
)

type reconcileReasons struct {
	mu      sync.Mutex
	reasons map[types.NamespacedName][]string
}

func (r *reconcileReasons) add(key types.NamespacedName, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reasons == nil {
		r.reasons = make(map[types.NamespacedName][]string)
	}
	current := r.reasons[key]
	if len(current) >= maxReconcileReasons || slices.Contains(current, reason) {
		return
	}
	r.reasons[key] = append(current, reason)
}

func (r *reconcileReasons) pop(key types.NamespacedName) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	reasons, ok := r.reasons[key]
	if !ok {
		return []string{requeueReconcileReason}
	}
	delete(r.reasons, key)
	return reasons
}

func (r *reconcileReasons) wrap(label string, inner predicate.Predicate) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.record(inner.Create(e), label, "create", e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return r.record(inner.Update(e), label, "update", e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.record(inner.Delete(e), label, "delete", e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return r.record(inner.Generic(e), label, "generic", e.Object)
		},
	}
}

func (r *reconcileReasons) record(passed bool, label, action string, obj client.Object) bool {
	if !passed || obj == nil {
		return passed
	}
	key, ok := storeKeyFor(obj)
	if ok {
		r.add(key, fmt.Sprintf("%s: %s %s/%s", label, action, objectKind(obj), obj.GetName()))
	}
	return passed
}

func storeKeyFor(obj client.Object) (types.NamespacedName, bool) {
	if _, ok := obj.(*v1.Store); ok {
		return types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, true
	}
	owner := metav1.GetControllerOf(obj)
	if owner == nil || owner.Kind != "Store" {
		return types.NamespacedName{}, false
	}
	return types.NamespacedName{Namespace: obj.GetNamespace(), Name: owner.Name}, true
}

func objectKind(obj client.Object) string {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}
