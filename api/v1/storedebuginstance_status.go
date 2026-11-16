package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StoreDebugInstanceState string

const (
	StoreDebugInstanceStateUnspecified StoreDebugInstanceState = ""
	StoreDebugInstanceStateWait        StoreDebugInstanceState = "wait"
	StoreDebugInstanceStatePending     StoreDebugInstanceState = "pending"
	StoreDebugInstanceStateRunning     StoreDebugInstanceState = "running"
	StoreDebugInstanceStateDone        StoreDebugInstanceState = "done"
	StoreDebugInstanceStateError       StoreDebugInstanceState = "error"
)

// StoreDebugInstanceStatus defines the observed state of StoreDebugInstance.
type StoreDebugInstanceStatus struct {
	State StoreDebugInstanceState `json:"state,omitempty"`

	Conditions []StoreDebugInstanceCondition `json:"conditions,omitempty"`
}

type StoreDebugInstanceCondition struct {
	Type               StoreDebugInstanceState `json:"type,omitempty"`
	LastTransitionTime metav1.Time             `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time             `json:"lastUpdatedTime,omitempty"`
	Message            string                  `json:"message,omitempty"`
	Reason             string                  `json:"reason,omitempty"`
	Status             string                  `json:"status,omitempty"`
}

func (s *StoreDebugInstanceStatus) AddCondition(c StoreDebugInstanceCondition) {
	s.Conditions = addDebugInstanceCondition(s.Conditions, c)
}

func addDebugInstanceCondition(conditions []StoreDebugInstanceCondition, c StoreDebugInstanceCondition) []StoreDebugInstanceCondition {
	if len(conditions) == 0 {
		return append(conditions, c)
	}

	// Update latest condition if the type is the same
	if conditions[len(conditions)-1].Type == c.Type {
		conditions[len(conditions)-1] = c
		return conditions
	}

	// Add condition if the type is different then the last one
	conditions = append(conditions, c)

	if len(conditions) > maxStatusesQuantity {
		conditions = conditions[len(conditions)-maxStatusesQuantity:]
	}
	return conditions
}

func (s *StoreDebugInstance) IsState(states ...StoreDebugInstanceState) bool {
	return slices.Contains(states, s.Status.State)
}

func (s *StoreDebugInstanceStatus) GetLastCondition() StoreDebugInstanceCondition {
	if len(s.Conditions) == 0 {
		return StoreDebugInstanceCondition{}
	}
	return s.Conditions[len(s.Conditions)-1]
}
