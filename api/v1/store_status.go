package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxStatusesQuantity = 6

type (
	StatefulAppState string
	DeploymentState  string
)

const (
	StateEmpty        StatefulAppState = ""
	StateWait         StatefulAppState = "waiting"
	StateSetup        StatefulAppState = "setup"
	StateInitializing StatefulAppState = "initializing"
	StateMigration    StatefulAppState = "migrating"
	StateReady        StatefulAppState = "ready"
)

const (
	DeploymentStateUnknown  DeploymentState = "unknown"
	DeploymentStateError    DeploymentState = "error"
	DeploymentStateNotFound DeploymentState = "not-found"
	DeploymentStateRunning  DeploymentState = "running"
)

type StoreCondition struct {
	Type               StatefulAppState `json:"type,omitempty"`
	LastTransitionTime metav1.Time      `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time      `json:"lastUpdatedTime,omitempty"`
	Message            string           `json:"message,omitempty"`
	Reason             string           `json:"reason,omitempty"`
	Status             string           `json:"status,omitempty"`
}

type DeploymentCondition struct {
	State          DeploymentState `json:"state,omitempty"`
	LastUpdateTime metav1.Time     `json:"lastUpdatedTime,omitempty"`
	Message        string          `json:"message,omitempty"`
	Ready          string          `json:"ready,omitempty"`
}

type StoreStatus struct {
	State           StatefulAppState `json:"state,omitempty"`
	Message         string           `json:"message,omitempty"`
	CurrentImageTag string           `json:"currentImageTag,omitempty"`

	WorkerState     DeploymentCondition `json:"workerState,omitempty"`
	AdminState      DeploymentCondition `json:"adminState,omitempty"`
	StorefrontState DeploymentCondition `json:"storefrontState,omitempty"`

	Conditions []StoreCondition `json:"conditions,omitempty"`
}

func (s *StoreStatus) AddCondition(c StoreCondition) {
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

	// Remove oldest conditions if the length exceeds maxStatusesQuantity
	if len(s.Conditions) > maxStatusesQuantity {
		s.Conditions = s.Conditions[len(s.Conditions)-maxStatusesQuantity:]
	}
}

func (s *StoreStatus) GetLastCondition() StoreCondition {
	if len(s.Conditions) == 0 {
		return StoreCondition{}
	}
	return s.Conditions[len(s.Conditions)-1]
}

func (s *Store) IsState(states ...StatefulAppState) bool {
	return slices.Contains(states, s.Status.State)
}
