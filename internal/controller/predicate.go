package controller

import (
	"encoding/json"
	"reflect"

	"github.com/pmezard/go-difflib/difflib"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type SkipStatusUpdates = TypedSkipStatusPredicate[client.Object]

// NewSkipStatusUpdates creates a new SkipStatusUpdates predicate with the given logger and allow list.
// The logger is required to prevent nil pointer panics during logging operations.
func NewSkipStatusUpdates(logger *zap.SugaredLogger, allowList ...client.Object) SkipStatusUpdates {
	return TypedSkipStatusPredicate[client.Object]{
		Logger:    logger,
		AllowList: allowList,
	}
}

// TypedSkipStatusPredicate filters out status-only updates.
type TypedSkipStatusPredicate[object client.Object] struct {
	predicate.TypedFuncs[object]
	Logger    *zap.SugaredLogger
	AllowList []client.Object
}

// Update returns false if only the status has changed, skipping reconciliation.
func (t TypedSkipStatusPredicate[object]) Update(e event.TypedUpdateEvent[object]) (update bool) {
	var oldStatusJson []byte
	var newStatusJson []byte
	var oldObjectJson []byte
	var newObjectJson []byte
	var err error

	// We need to use reflection because the GetObjectKind is empty
	kind := "unknown"
	objType := reflect.TypeOf(e.ObjectNew)
	if objType != nil {
		if objType.Kind() == reflect.Ptr {
			objType = objType.Elem()
		}
		kind = objType.Name()
	}

	defer func() {
		if update && t.Logger != nil {
			// Log the update trigger
			t.Logger.Debugw("Update trigger",
				zap.Bool("triggerReconcile", update),
				zap.String("name", e.ObjectNew.GetName()),
				zap.String("kind", kind),
			)

			// Generate and log status diff
			statusDiff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(oldStatusJson)),
				B:        difflib.SplitLines(string(newStatusJson)),
				FromFile: "Old Status",
				ToFile:   "New Status",
				Context:  3,
			}
			statusDiffText, _ := difflib.GetUnifiedDiffString(statusDiff)
			if statusDiffText != "" && t.Logger != nil {
				t.Logger.Debugf("Status diff: \n%s", statusDiffText)
			}

			// Generate and log spec diff
			specDiff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(oldObjectJson)),
				B:        difflib.SplitLines(string(newObjectJson)),
				FromFile: "Old Spec",
				ToFile:   "New Spec",
				Context:  3,
			}
			specDiffText, _ := difflib.GetUnifiedDiffString(specDiff)
			if specDiffText != "" && t.Logger != nil {
				t.Logger.Debugf("Spec Diff: \n%s", specDiffText)
			}
		}
	}()
	if isNil(e.ObjectOld) || isNil(e.ObjectNew) {
		return false
	}

	oldSpec, okOld := getSpec(e.ObjectOld)
	newSpec, okNew := getSpec(e.ObjectNew)
	if okOld && okNew {
		oldObjectJson, err = json.MarshalIndent(oldSpec, "", "  ")
		if err != nil && t.Logger != nil {
			t.Logger.Warnw("parse old spec json", zap.Error(err))
		}
		newObjectJson, err = json.MarshalIndent(newSpec, "", "  ")
		if err != nil && t.Logger != nil {
			t.Logger.Warnw("parse new spec json", zap.Error(err))
		}
	}

	oldStatus, okOld := getStatus(e.ObjectOld)
	newStatus, okNew := getStatus(e.ObjectNew)
	if !okOld || !okNew {
		// Fallback to allow reconcile if we can't extract status
		return true
	}

	oldStatusJson, err = json.MarshalIndent(oldStatus, "", "  ")
	if err != nil && t.Logger != nil {
		t.Logger.Warnw("parse old status json", zap.Error(err))
	}
	newStatusJson, err = json.MarshalIndent(newStatus, "", "  ")
	if err != nil && t.Logger != nil {
		t.Logger.Warnw("parse new status json", zap.Error(err))
	}

	// If status changed check if object is in allow list and allow reconcile
	if !reflect.DeepEqual(oldStatus, newStatus) {
		return t.isInAllowList(kind)
	}

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

func getSpec(obj any) (any, bool) {
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

	spec := v.FieldByName("Spec")
	if !spec.IsValid() {
		return nil, false
	}
	return spec.Interface(), true
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

// isInAllowList checks if the given kind matches any object type in the allow list.
func (t TypedSkipStatusPredicate[object]) isInAllowList(kind string) bool {
	for _, allowedObj := range t.AllowList {
		objType := reflect.TypeOf(allowedObj)
		if objType != nil {
			if objType.Kind() == reflect.Ptr {
				objType = objType.Elem()
			}
			if objType.Name() == kind {
				return true
			}
		}
	}
	return false
}
