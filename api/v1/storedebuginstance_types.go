/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StoreDebugInstanceSpec defines the desired state of StoreDebugInstance.
type StoreDebugInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// StoreRef is the reference to the store to debug
	StoreRef string `json:"storeRef,omitempty"`
	// Duration is the duration of the debug instance after which it will be deleted
	// e.g. 1h or 30m
	// +default="1h"
	Duration string `json:"duration,omitempty"`
	// ExtraLabels is the extra labels to add to the debug instance
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`
	// ExtraContainerPorts is the extra ports to add to the debug instance
	// if it should be exposed to the outside world
	ExtraContainerPorts []corev1.ContainerPort `json:"extraContainerPorts,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.state"
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=stdi
// StoreDebugInstance is the Schema for the storedebuginstances API.
type StoreDebugInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StoreDebugInstanceSpec   `json:"spec,omitempty"`
	Status StoreDebugInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StoreDebugInstanceList contains a list of StoreDebugInstance.
type StoreDebugInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StoreDebugInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoreDebugInstance{}, &StoreDebugInstanceList{})
}
