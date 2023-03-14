package v1alpha1

import corev1 "k8s.io/api/core/v1"

// TLS settings for Guacamole.
type TLS struct {
	// +optional
	CaCertificates *CaCertificates `json:"caCertificates,omitempty"`
}

type CaCertificates struct {
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}
