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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ConnectionSpec defines the desired state of Connection.
type ConnectionSpec struct {
	// GuacamoleRef references the instance this connection belongs to.
	GuacamoleRef GuacamoleRef `json:"guacamoleRef"`

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

	// Permissions.
	//
	// +optional
	Permissions *ConnectionPermissions `json:"permissions,omitempty"`
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

// GuacamoleRef...
type GuacamoleRef struct {
	// Name of the Guacamole instance.
	Name string `json:"name"`
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

// ConnectionPermissions...
type ConnectionPermissions struct {
	// Users with permissions on the connection.
	// As in upstream UI, users will get READ permissions.
	//
	// +optional
	Users []ConnectionUser `json:"users,omitempty"`
	// User groups with permissions on the connection.
	// As in upstream UI, groups will get READ permissions.
	//
	// +optional
	Groups []ConnectionGroup `json:"groups,omitempty"`
}

// ConnectionUser...
type ConnectionUser struct {
	// User identifier.
	ID string `json:"id"`
}

// ConnectionUser...
type ConnectionGroup struct {
	// Group identifier.
	ID string `json:"id"`
}
