package transformer

import (
	"context"
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/manifest"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
)

const (
	GuacdDeploymentName = "guacd"
)

// Guacd transforms the guacd deployment manifest.
func Guacd(client client.Client) declarative.ObjectTransform {
	return func(_ context.Context, obj declarative.DeclarativeObject, m *manifest.Objects) error {
		guac := obj.(*v1alpha1.Guacamole)

		if guac.Spec.Guacd != nil && guac.Spec.Guacd.Metadata != nil {
			if err := applyAnnotations(guac.Spec.Guacd.Metadata, m); err != nil {
				return err
			}
		}

		return nil
	}
}

func applyAnnotations(objMeta *v1alpha1.ObjectMeta, m *manifest.Objects) error {
	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacdDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			annotations := objMeta.Annotations

			// Merge annotations.
			if deployment.ObjectMeta.Annotations == nil {
				deployment.ObjectMeta.Annotations = make(map[string]string)
			}

			maps.Insert(deployment.ObjectMeta.Annotations, maps.All(annotations))

			// Merge template annotations.
			if deployment.Spec.Template.ObjectMeta.Annotations == nil {
				deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
			}
			maps.Insert(deployment.Spec.Template.ObjectMeta.Annotations, maps.All(annotations))

			u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
			if err != nil {
				return err
			}

			obj, err := manifest.NewObject(&unstructured.Unstructured{Object: u})
			if err != nil {
				return err
			}

			m.Items[idx] = obj
			break
		}
	}
	return nil
}
