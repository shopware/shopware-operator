package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatefulState string

const (
	ExecStateEmpty   StatefulState = ""
	ExecStateWait    StatefulState = "wait"
	ExecStateRunning StatefulState = "running"
	ExecStateDone    StatefulState = "done"
	ExecStateError   StatefulState = "error"
)

type StoreExecSpec struct {
	StoreRef     string `json:"storeRef"`
	CronSchedule string `json:"cronSchedule,omitempty"`

	// +kubebuilder:default=false
	CronSuspend bool   `json:"cronSuspend,omitempty"`
	Script      string `json:"script"`

	// +kubebuilder:default=3
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// The two cleanup periods are pointers rather than values because `omitempty`
	// does not drop a zero metav1.Duration: a value field would send an explicit
	// "0s" from typed clients and defeat the defaults. Keep those defaults in sync
	// with defaultCleanupPeriod* in internal/controller/storeexec_controller.go.

	// How long a successfully finished StoreExec is kept before it is deleted.
	// Zero disables cleanup for successful executions.
	// +optional
	// +kubebuilder:default="5m"
	CleanupPeriodSuccessfulExec *metav1.Duration `json:"cleanupPeriodSuccessfulExec,omitempty"`

	// How long a failed StoreExec is kept before it is deleted.
	// Zero disables cleanup for failed executions.
	// +optional
	// +kubebuilder:default="1h"
	CleanupPeriodErrorExec *metav1.Duration `json:"cleanupPeriodErrorExec,omitempty"`

	ExtraEnvs []corev1.EnvVar `json:"extraEnvs,omitempty"`

	Container ContainerSpec `json:"container,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="MaxRetries",type=string,JSONPath=".spec.maxRetries"
// +kubebuilder:printcolumn:name="CronSchedule",type=string,JSONPath=".spec.cronSchedule"
// +kubebuilder:printcolumn:name="CronSuspend",type=string,JSONPath=".spec.cronSuspend"
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
