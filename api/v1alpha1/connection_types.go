/*
Copyright 2023.

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

package v1alpha1

import (
	"encoding/json"

	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ConnectionSpec defines the desired state of Connection.
type ConnectionSpec struct {
	// Protocol of the connection.
	Protocol ConnectionProtocol `json:"protocol,omitempty"`

	// Parent of the connection specified as a path (/<group>/<group>).
	// Defaults to ROOT if not specified.
	//
	// +optional
	// +kubebuilder:default=/
	Parent *string `json:"parent,omitempty"`

	// Parameter of the connection
	//
	// +optional
	Parameters *ConnectionParameters `json:"parameters,omitempty"`
}

// ConnectionStatus defines the observed state of Connection.
type ConnectionStatus struct {
	// Guacamole internal identifier of the connection.
	// Missing if connection not yet configured.
	//
	// +optional
	Identifier *string `json:"identifier,omitempty"`

	// Guacamole internal identifier of the connection's parent group.
	// Missing if connection not yet configured.
	//
	// +optional
	Parent *string `json:"parent,omitempty"`

	// Conditions represent the latest available observations of an object's state.
	//
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Protocol",type=string,JSONPath=`.spec.protocol`

// Connection is the Schema for the connections API.
type Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConnectionSpec   `json:"spec,omitempty"`
	Status ConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConnectionList contains a list of Connection.
type ConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Connection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Connection{}, &ConnectionList{})
}

// ConnectionProtocol...
type ConnectionProtocol = gen.ConnectionProtocol

// ConnectionParameters...
type ConnectionParameters struct {
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	json.RawMessage `json:",inline"`
}
