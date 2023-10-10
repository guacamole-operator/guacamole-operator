package transformer

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/manifest"
)

func isDeployment(obj *manifest.Object) bool {
	return obj.Kind == "Deployment"
}

func getProxyVariables() []corev1.EnvVar {
	proxyEnv := []corev1.EnvVar{{
		Name:  "HTTPS_PROXY",
		Value: os.Getenv("HTTPS_PROXY"),
	}, {
		Name:  "HTTP_PROXY",
		Value: os.Getenv("HTTP_PROXY"),
	}, {
		Name:  "NO_PROXY",
		Value: os.Getenv("NO_PROXY"),
	}}

	return proxyEnv
}
