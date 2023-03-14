package v1alpha1

import corev1 "k8s.io/api/core/v1"

// Authentication configuration for Guacamole.
// At least one method has to be configured.
type Auth struct {
	// +optional
	Postgres *Postgres `json:"postgres,omitempty"`

	// +optional
	OIDC *OIDC `json:"oidc,omitempty"`
}

// Postgres authentication.
type Postgres struct {
	Parameter []Parameter `json:"params"`
}

// OIDC authentication.
type OIDC struct {
	Parameter []Parameter `json:"params"`
}

// Parameter for an authentication method.
type Parameter struct {
	Name      string                   `json:"name"`
	ValueFrom corev1.SecretKeySelector `json:"valueFrom"`
}
