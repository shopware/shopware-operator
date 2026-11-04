package v1

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminalDebugInstanceSpec defines the desired state of TerminalDebugInstance.
type TerminalDebugInstanceSpec struct {
	// Duration is the duration of the debug instance after which it will be deleted
	// e.g. 1h or 30m. Zero keeps the instance until it is deleted manually.
	// +default="1h"
	// +kubebuilder:validation:XValidation:rule="self.matches('^(0|([0-9]+([.][0-9]+)?(ns|us|ms|s|m|h))+)$')",message="must be a valid duration, e.g. 30s, 5m or 1h"
	Duration metav1.Duration `json:"duration,omitempty"`
	// PodSelector selects the running pod that is copied into the debug pod.
	// All running pods it matches must belong to the same controller.
	PodSelector metav1.LabelSelector `json:"podSelector"`
	// Container receives the custom image and the extra ports. Empty requires
	// the selected pod to have exactly one container.
	// +optional
	Container string `json:"container,omitempty"`
	// CustomImage replaces the image of the debug container
	// +optional
	CustomImage string `json:"customImage,omitempty"`
	// ExtraContainerPorts are added to the debug container
	// +optional
	ExtraContainerPorts []corev1.ContainerPort `json:"extraContainerPorts,omitempty"`
}

// TerminalDebugInstanceStatus defines the observed state of TerminalDebugInstance.
type TerminalDebugInstanceStatus struct {
	State StoreDebugInstanceState `json:"state,omitempty"`
	// PodName is the debug pod created for this instance
	PodName string `json:"podName,omitempty"`
	// Container is the debug container inside PodName
	Container string `json:"container,omitempty"`

	Conditions []StoreDebugInstanceCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=".status.podName"
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=tdi
// TerminalDebugInstance copies a running pod into a debug pod that is not
// part of the workload's Service, for applications that have no Store.
type TerminalDebugInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TerminalDebugInstanceSpec   `json:"spec,omitempty"`
	Status TerminalDebugInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TerminalDebugInstanceList contains a list of TerminalDebugInstance.
type TerminalDebugInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TerminalDebugInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TerminalDebugInstance{}, &TerminalDebugInstanceList{})
}

func (s *TerminalDebugInstance) IsState(states ...StoreDebugInstanceState) bool {
	return slices.Contains(states, s.Status.State)
}

func (s *TerminalDebugInstanceStatus) AddCondition(c StoreDebugInstanceCondition) {
	s.Conditions = addDebugInstanceCondition(s.Conditions, c)
}
