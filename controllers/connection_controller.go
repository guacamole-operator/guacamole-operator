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

package controllers

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	guacamoleoperatorgithubiov1alpha1 "github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	guacclient "github.com/guacamole-operator/guacamole-operator/internal/client"
	reconciler "github.com/guacamole-operator/guacamole-operator/internal/reconciler/connection"
)

// ConnectionReconciler reconciles a Connection object.
type ConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("connection", req.NamespacedName)

	// Fetch instance.
	connection := &v1alpha1.Connection{}
	if err := r.Get(ctx, req.NamespacedName, connection); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get instance.")
		return ctrl.Result{}, err
	}

	// Create Guacamole API client.
	config, err := r.getConnectionParamsFromSecret(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	guacClient, err := guacclient.New(config)
	if err != nil {
		logger.Error(err, "Could not create Guacamole API client.")
		return ctrl.Result{}, err
	}

	// Instantiate reconciler.
	reconciler := reconciler.New(guacClient)

	// Check if instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. If so, process the
	// finalizer and end the reconcile cycle.
	isMarkedToBeDeleted := connection.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(connection, connectionFinalizer) {
			// Run finalization logic for finalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalize(ctx, connection, reconciler); err != nil {
				logger.Error(err, "Failed to finalize instance.")
				return ctrl.Result{}, err
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			if controllerutil.RemoveFinalizer(connection, connectionFinalizer) {
				err := r.Update(ctx, connection)
				if err != nil {
					// Error updating the object - requeue the request.
					logger.Error(err, "Failed to update instance after removing finalizer.")
					return ctrl.Result{}, err
				}
			}

			logger.Info("Instance finalized.")
		}
		return ctrl.Result{}, nil
	}

	// Instance is not marked for deletion, add finalizer.
	if !controllerutil.ContainsFinalizer(connection, connectionFinalizer) {
		logger.Info("Add finalizer.")
		controllerutil.AddFinalizer(connection, connectionFinalizer)
		err = r.Update(ctx, connection)
		if err != nil {
			// Error updating the object - requeue the request.
			logger.Error(err, "Failed to update instance after adding finalizer")
			return ctrl.Result{}, err
		}
	}

	// Sync state.
	if err := reconciler.Sync(ctx, connection); err != nil {
		logger.Error(err, "Could not sync resource.")
		return ctrl.Result{}, err
	}

	// Update status.
	err = r.Status().Update(ctx, connection)
	if err != nil {
		logger.Error(err, "Failed to update status.")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&guacamoleoperatorgithubiov1alpha1.Connection{}).
		Complete(r)
}

// getConnectionParamsFromSecret retrieves access parameters for the Guacamole API from secret.
func (r *ConnectionReconciler) getConnectionParamsFromSecret(ctx context.Context, namespace string) (*guacclient.Config, error) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "guacamole-credentials",
			Namespace: namespace,
		},
	}

	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &secret)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("Guacamole access parameters secret not found: %w", err)
	}

	errInvalidParamaters := errors.New("invalid parameters")

	server, ok := secret.Data["server"]
	if !ok {
		return nil, fmt.Errorf("Guacamole server parameter missing: %w", errInvalidParamaters)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("Guacamole username parameter missing: %w", errInvalidParamaters)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("Guacamole password parameter missing: %w", errInvalidParamaters)
	}

	source, ok := secret.Data["source"]
	if !ok {
		return nil, fmt.Errorf("Guacamole source parameter missing: %w", errInvalidParamaters)
	}

	return &guacclient.Config{
		Server:   string(server),
		Username: string(username),
		Password: string(password),
		Insecure: false,
		Source:   string(source),
	}, nil
}