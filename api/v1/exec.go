package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatefulState string

const (
	ExecStateEmpty   StatefulState = ""
	ExecStateRunning StatefulState = "running"
	ExecStateDone    StatefulState = "done"
	ExecStateError   StatefulState = "error"
)

type StoreExecSpec struct {
	StoreRef     string `json:"storeRef"`
	CronSchedule string `json:"cronSchedule,omitempty"`
	Script       string `json:"script"`

	ExtraEnvs []corev1.EnvVar `json:"extraEnvs,omitempty"`

	Container ContainerSpec `json:"container,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=stexec
type StoreExec struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoreExecSpec   `json:"spec,omitempty"`
	Status StoreExecStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type StoreExecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoreExec `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoreExec{}, &StoreExecList{})
}
