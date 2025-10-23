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
	StateEmpty          StatefulAppState = ""
	StateWait           StatefulAppState = "waiting"
	StateSetup          StatefulAppState = "setup"
	StateSetupError     StatefulAppState = "setup_error"
	StateInitializing   StatefulAppState = "initializing"
	StateMigration      StatefulAppState = "migrating"
	StateMigrationError StatefulAppState = "migrating_error"
	StateReady          StatefulAppState = "ready"
)

const (
	DeploymentStateUnknown  DeploymentState = "unknown"
	DeploymentStateError    DeploymentState = "error"
	DeploymentStateNotFound DeploymentState = "not-found"
	DeploymentStateRunning  DeploymentState = "running"
	DeploymentStateScaling  DeploymentState = "scaling"
)

type StoreCondition struct {
	Type               string      `json:"type,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	LastUpdateTime     metav1.Time `json:"lastUpdatedTime,omitempty"`
	Message            string      `json:"message,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	Status             string      `json:"status,omitempty"`
}

type StoreConditionsList struct {
	Conditions []StoreCondition `json:"conditions,omitempty"`
}

type DeploymentCondition struct {
	State          DeploymentState `json:"state,omitempty"`
	LastUpdateTime metav1.Time     `json:"lastUpdatedTime,omitempty"`
	Message        string          `json:"message,omitempty"`
	Ready          string          `json:"ready,omitempty"`
	StoreReplicas  int32           `json:"storeReplicas,omitempty"`
}

type StoreStatus struct {
	State           StatefulAppState `json:"state,omitempty"`
	Message         string           `json:"message,omitempty"`
	CurrentImageTag string           `json:"currentImageTag,omitempty"`

	WorkerState     DeploymentCondition `json:"workerState,omitempty"`
	AdminState      DeploymentCondition `json:"adminState,omitempty"`
	StorefrontState DeploymentCondition `json:"storefrontState,omitempty"`

	StoreConditionsList `json:",inline"`
}

func (s *StoreConditionsList) AddCondition(c StoreCondition) {
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

func (s *StoreConditionsList) GetLastCondition() StoreCondition {
	if len(s.Conditions) == 0 {
		return StoreCondition{}
	}
	return s.Conditions[len(s.Conditions)-1]
}

func (s *Store) IsState(states ...StatefulAppState) bool {
	return slices.Contains(states, s.Status.State)
}
