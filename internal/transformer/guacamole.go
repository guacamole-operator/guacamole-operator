package transformer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative/pkg/manifest"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
)

const (
	GuacamoleDeploymentName = "guacamole"
	extensionDLImage        = "ghcr.io/guacamole-operator/extension-dl:025675e"
)

// Guacamole transform the guacamole deployment manifest.
func Guacamole(client client.Client) declarative.ObjectTransform {
	return func(_ context.Context, obj declarative.DeclarativeObject, m *manifest.Objects) error {
		guac := obj.(*v1alpha1.Guacamole)

		if guac.Spec.TLS != nil {
			if err := applyTLSConfiguration(guac.Spec.TLS, m); err != nil {
				return err
			}
		}

		if guac.Spec.Auth.Postgres != nil {
			if err := applyPostgresConfiguration(guac, m); err != nil {
				return err
			}
		}

		if guac.Spec.Auth.OIDC != nil {
			if err := applyOIDCConfiguration(guac, m); err != nil {
				return err
			}
		}

		if guac.Spec.AdditionalSettings != nil {
			if err := applyAdditionalSettings(guac.Spec.AdditionalSettings, m); err != nil {
				return err
			}
		}

		if guac.Spec.Extensions != nil {
			if err := applyExtensions(guac.Spec.Extensions, m); err != nil {
				return err
			}
		}

		// Add instance name to resources.
		if err := addInstanceName(m, guac); err != nil {
			return err
		}

		// Add access information to status.
		return addAccessParameters(client, guac)
	}
}

func applyTLSConfiguration(tls *v1alpha1.TLS, m *manifest.Objects) error {
	if tls.CaCertificates == nil {
		return nil
	}

	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacamoleDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			// Mount CAs.
			ensureVolume(&deployment, corev1.Volume{
				Name: "ca-bundle",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tls.CaCertificates.SecretRef.Name,
					},
				},
			})

			ensureContainerVolumeMount(&deployment, "guacamole", corev1.VolumeMount{
				Name:      "ca-bundle",
				ReadOnly:  true,
				MountPath: "/opt/ca-bundle",
			})

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

func applyPostgresConfiguration(guac *v1alpha1.Guacamole, m *manifest.Objects) error {
	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacamoleDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			// Image version overwritten via kustomize before this transformation runs.
			guacImage := deployment.Spec.Template.Spec.Containers[0].Image
			deployment.Spec.Template.Spec.InitContainers = postgresInitContainer(guacImage)
			deployment.Spec.Template.Spec.InitContainers[1].Env = envVarFromParameters(guac.Spec.Auth.Postgres.Parameter)

			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "initdb",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})

			envs := envVarFromParameters(guac.Spec.Auth.Postgres.Parameter)
			deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, envs...)

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

func postgresInitContainer(guacImage string) []corev1.Container {
	createDB := corev1.Container{
		Name:  "create-init-db",
		Image: guacImage,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "initdb",
				MountPath: "/data",
			},
		},
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			"/opt/guacamole/bin/initdb.sh --postgresql > /data/initdb.sql",
		},
	}

	loadDB := corev1.Container{
		Name:  "load-db",
		Image: "docker.io/library/postgres:alpine",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "initdb",
				MountPath: "/data",
			},
		},
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			`export PGPASSWORD=$POSTGRESQL_PASSWORD
psql -h $POSTGRESQL_HOSTNAME -d $POSTGRESQL_DATABASE -U $POSTGRESQL_USER -p $POSTGRESQL_PORT -a -w -f /data/initdb.sql || true`,
		},
	}

	return []corev1.Container{createDB, loadDB}
}

func applyOIDCConfiguration(guac *v1alpha1.Guacamole, m *manifest.Objects) error {
	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacamoleDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			envs := envVarFromParameters(guac.Spec.Auth.OIDC.Parameter)
			deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, envs...)

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

func applyAdditionalSettings(values map[string]string, m *manifest.Objects) error {
	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacamoleDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			settings := normalizeSettings(values)
			envs := envVarFromMap(settings)

			currentEnvs := deployment.Spec.Template.Spec.Containers[0].Env

			currentEnvs = append(
				currentEnvs,
				envs...,
			)

			// Sort environment variables to avoid reconcile loop due to
			// a changed order.
			sort.SliceStable(currentEnvs, func(i, j int) bool {
				return currentEnvs[i].Name < currentEnvs[j].Name
			})

			deployment.Spec.Template.Spec.Containers[0].Env = currentEnvs

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

func applyExtensions(extensions []v1alpha1.Extension, m *manifest.Objects) error {
	var extUris []string
	for _, e := range extensions {
		extUris = append(extUris, e.URI)
	}

	for idx, item := range m.Items {
		if isDeployment(item) && item.GetName() == GuacamoleDeploymentName {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			// Apply init container for extension download.
			// TODO: Append instead of replace.
			downloaderContainer := corev1.Container{
				Name:  "extension-dl",
				Image: extensionDLImage,
				Args:  extUris,
			}

			// Apply cluster proxy.
			for _, v := range getProxyVariables() {
				if v.Value == "" {
					continue
				}

				downloaderContainer.Env = append(downloaderContainer.Env, v)
			}

			deployment.Spec.Template.Spec.InitContainers = []corev1.Container{downloaderContainer}

			// Mount emptyDir volume to place extensions.
			ensureVolume(&deployment, corev1.Volume{
				Name: "extensions",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})

			ensureInitContainerVolumeMount(&deployment, "extension-dl", corev1.VolumeMount{
				Name: "extensions",
				// GUACAMOLE_HOME=/tmp/guacamole required.
				MountPath: "/extensions",
			})

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

func addInstanceName(m *manifest.Objects, guac *v1alpha1.Guacamole) error {
	for idx, item := range m.Items {
		instance := item.GetName() + "-" + guac.Name
		if err := item.SetName(instance); err != nil {
			return err
		}

		if isDeployment(item) {
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredObject().Object, &deployment)
			if err != nil {
				return fmt.Errorf("error converting deployment from unstructured: %w", err)
			}

			if deployment.Spec.Template.Spec.ServiceAccountName != "" {
				deployment.Spec.Template.Spec.ServiceAccountName = instance
			}

			if deployment.Name == GuacamoleDeploymentName+"-"+guac.Name {
				envs := deployment.Spec.Template.Spec.Containers[0].Env
				for j, e := range envs {
					if e.Name == "GUACD_HOSTNAME" {
						e.Value += "-" + guac.Name
					}
					envs[j] = e
				}

				deployment.Spec.Template.Spec.Containers[0].Env = envs
			}

			u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
			if err != nil {
				return err
			}

			obj, err := manifest.NewObject(&unstructured.Unstructured{Object: u})
			if err != nil {
				return err
			}

			m.Items[idx] = obj
		}
	}

	return nil
}

func addAccessParameters(client client.Client, guac *v1alpha1.Guacamole) error {
	instance := "guacamole" + "-" + guac.Name
	ns := guac.Namespace

	source := ""
	if guac.Spec.Auth.Postgres != nil {
		source = "postgresql"
	}

	guac.Status.Access = &v1alpha1.Access{
		Endpoint: fmt.Sprintf("http://%s.%s.svc.cluster.local/guacamole/api", instance, ns),
		Source:   source,
	}

	return client.Status().Update(context.Background(), guac)
}

func envVarFromParameters(params []v1alpha1.Parameter) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0)

	for _, p := range params {
		vars = append(vars, corev1.EnvVar{
			Name: p.Name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: p.ValueFrom.LocalObjectReference,
					Key:                  p.ValueFrom.Key,
				},
			},
		})
	}

	return vars
}

func envVarFromMap(params map[string]string) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0)

	for k, v := range params {
		vars = append(vars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	return vars
}

func ensureVolume(deployment *appsv1.Deployment, volume corev1.Volume) {
	for i, v := range deployment.Spec.Template.Spec.Volumes {
		if v.Name == volume.Name {
			deployment.Spec.Template.Spec.Volumes[i] = volume
			return
		}
	}

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, volume)
}

func ensureContainerVolumeMount(deployment *appsv1.Deployment, container string, mount corev1.VolumeMount) {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == container {
			for j, m := range c.VolumeMounts {
				if m.Name == mount.Name {
					deployment.Spec.Template.Spec.Containers[i].VolumeMounts[j] = mount
					return
				}
			}

			deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(
				deployment.Spec.Template.Spec.Containers[i].VolumeMounts,
				mount,
			)
		}
	}
}

func ensureInitContainerVolumeMount(deployment *appsv1.Deployment, container string, mount corev1.VolumeMount) {
	for i, c := range deployment.Spec.Template.Spec.InitContainers {
		if c.Name == container {
			for j, m := range c.VolumeMounts {
				if m.Name == mount.Name {
					deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts[j] = mount
					return
				}
			}

			deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(
				deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts,
				mount,
			)
		}
	}
}

func normalizeSettings(values map[string]string) map[string]string {
	newValues := make(map[string]string, len(values))

	for k, v := range values {
		newKey := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		newValues[newKey] = v
	}

	return newValues
}
