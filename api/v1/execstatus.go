package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StoreExecStatus struct {
	State StatefulState `json:"state,omitempty"`

	Done       string          `json:"ready,omitempty"`
	Conditions []ExecCondition `json:"conditions,omitempty"`
}

type ExecCondition struct {
	Type               StatefulState `json:"type,omitempty"`
	LastTransitionTime metav1.Time   `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time   `json:"lastUpdatedTime,omitempty"`
	Message            string        `json:"message,omitempty"`
	Reason             string        `json:"reason,omitempty"`
	Status             string        `json:"status,omitempty"`
}

func (s *StoreExecStatus) AddCondition(c ExecCondition) {
	if len(s.Conditions) == 0 {
		s.Conditions = append(s.Conditions, c)
		return
	}

	// Update latest condition if the type is the same
	if s.Conditions[len(s.Conditions)-1].Type == c.Type {
		s.Conditions[len(s.Conditions)-1] = c
		return
	}

	// Add condition if the type is different then the last one
	if s.Conditions[len(s.Conditions)-1].Type != c.Type {
		s.Conditions = append(s.Conditions, c)
	}

	if len(s.Conditions) > maxStatusesQuantity {
		s.Conditions = s.Conditions[len(s.Conditions)-maxStatusesQuantity:]
	}
}

func (s *StoreExec) IsState(states ...StatefulState) bool {
	return slices.Contains(states, s.Status.State)
}
