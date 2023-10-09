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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	addonv1alpha1 "sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon/pkg/apis/v1alpha1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GuacamoleSpec defines the desired state of Guacamole.
type GuacamoleSpec struct {
	addonv1alpha1.CommonSpec `json:",inline"`
	addonv1alpha1.PatchSpec  `json:",inline"`

	// Authentication method configuration (required).
	Auth Auth `json:"auth,omitempty"`

	// Additional TLS settings.
	// +optional
	TLS *TLS `json:"tls,omitempty"`

	// Additional settings.
	// +optional
	AdditionalSettings map[string]string `json:"additionalSettings,omitempty"`

	// Extensions to provision.
	// +optional
	Extensions []Extension `json:"extensions,omitempty"`
}

// GuacamoleStatus defines the observed state of Guacamole.
type GuacamoleStatus struct {
	addonv1alpha1.CommonStatus `json:",inline"`

	// Information about how to connect to the deployed instance.
	// Used by other resources to dynamically connect to
	// an API client.
	//
	// +optional
	Access *Access `json:"access,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Guacamole is the Schema for the guacamoles API.
type Guacamole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GuacamoleSpec   `json:"spec,omitempty"`
	Status GuacamoleStatus `json:"status,omitempty"`
}

var _ addonv1alpha1.CommonObject = &Guacamole{}

func (o *Guacamole) ComponentName() string {
	return "guacamole"
}

func (o *Guacamole) CommonSpec() addonv1alpha1.CommonSpec {
	return o.Spec.CommonSpec
}

func (o *Guacamole) PatchSpec() addonv1alpha1.PatchSpec {
	return o.Spec.PatchSpec
}

func (o *Guacamole) GetCommonStatus() addonv1alpha1.CommonStatus {
	return o.Status.CommonStatus
}

func (o *Guacamole) SetCommonStatus(s addonv1alpha1.CommonStatus) {
	o.Status.CommonStatus = s
}

//+kubebuilder:object:root=true

// GuacamoleList contains a list of Guacamole.
type GuacamoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Guacamole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Guacamole{}, &GuacamoleList{})
}

// Extension...
type Extension struct {
	// URI for the extension.
	URI string `json:"uri"`
}

// Access...
type Access struct {
	// Endpoint of the Guacamole API.
	Endpoint string `json:"endpoint"`
	// Authentication source.
	Source string `json:"source"`
}
