package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=store-snap-restore
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StoreSnapshotRestore is the Schema for the storesnapshotrestores API.
type StoreSnapshotRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoreSnapshotSpec   `json:"spec,omitempty"`
	Status StoreSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StoreSnapshotRestoreList contains a list of StoreSnapshotRestore.
type StoreSnapshotRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoreSnapshotRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoreSnapshotRestore{}, &StoreSnapshotRestoreList{})
}

// GetObjectMeta implements SnapshotResource interface
func (s *StoreSnapshotRestore) GetObjectMeta() metav1.Object {
	return s
}

func (s *StoreSnapshotRestore) GetRuntimeObject() runtime.Object {
	return s
}

// GetSpec implements SnapshotResource interface
func (s *StoreSnapshotRestore) GetSpec() StoreSnapshotSpec {
	return s.Spec
}

// GetStatus implements SnapshotResource interface
func (s *StoreSnapshotRestore) GetStatus() *StoreSnapshotStatus {
	return &s.Status
}
