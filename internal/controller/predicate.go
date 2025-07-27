package controller

import (
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type SkipStatusUpdates = TypedSkipStatusPredicate[client.Object]

// TypedSkipStatusPredicate filters out status-only updates.
type TypedSkipStatusPredicate[object client.Object] struct {
	predicate.TypedFuncs[object]
}

// Update returns false if only the status has changed, skipping reconciliation.
func (TypedSkipStatusPredicate[object]) Update(e event.TypedUpdateEvent[object]) bool {
	if isNil(e.ObjectOld) || isNil(e.ObjectNew) {
		return false
	}

	oldStatus, okOld := getStatus(e.ObjectOld)
	newStatus, okNew := getStatus(e.ObjectNew)

	if !okOld || !okNew {
		// Fallback to allow reconcile if we can't extract status
		return true
	}

	// If status changed, skip reconcile
	if !reflect.DeepEqual(oldStatus, newStatus) {
		return false
	}

	// Otherwise, allow reconcile
	return true
}

// getStatus extracts the .Status field using reflection.
func getStatus(obj any) (any, bool) {
	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		return nil, false
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return nil, false
	}

	status := v.FieldByName("Status")
	if !status.IsValid() {
		return nil, false
	}
	return status.Interface(), true
}

// isNil safely checks for nil values via reflection.
func isNil(arg any) bool {
	if arg == nil {
		return true
	}
	v := reflect.ValueOf(arg)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}
