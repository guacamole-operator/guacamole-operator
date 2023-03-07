package transformer

import "sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/manifest"

func isDeployment(obj *manifest.Object) bool {
	return obj.Kind == "Deployment"
}

func isSecret(obj *manifest.Object) bool {
	return obj.Kind == "Secret"
}
