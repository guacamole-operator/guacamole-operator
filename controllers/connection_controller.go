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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=connections/finalizers,verbs=update
//
// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=guacamoles,verbs=get;list
//
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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

	if len(connection.Status.Conditions) == 0 {
		connection.Status.MarkAsUnknown()
		if err := r.Status().Update(ctx, connection); err != nil {
			logger.Error(err, "Failed to update status.")
			return ctrl.Result{}, err
		}
	}

	// Create Guacamole API client.
	config, err := r.getConnectionParams(ctx, connection)
	if err != nil {
		return ctrl.Result{}, err
	}

	guacClient, err := guacclient.New(config)
	if err != nil {
		logger.Error(err, "Could not create Guacamole API client.")

		connection.Status.MarkAsUnsynchronized()
		if err := r.Status().Update(ctx, connection); err != nil {
			logger.Error(err, "Failed to update status.")
			return ctrl.Result{}, err
		}

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
				if err := r.Update(ctx, connection); err != nil {
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
		if err := r.Update(ctx, connection); err != nil {
			// Error updating the object - requeue the request.
			logger.Error(err, "Failed to update instance after adding finalizer")
			return ctrl.Result{}, err
		}
	}

	// Sync state.
	if err := reconciler.Sync(ctx, connection); err != nil {
		logger.Error(err, "Could not sync resource.")

		connection.Status.MarkAsUnsynchronized()
		if err := r.Status().Update(ctx, connection); err != nil {
			logger.Error(err, "Failed to update status.")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	// Update status.
	connection.Status.MarkAsSynchronized()
	if err := r.Status().Update(ctx, connection); err != nil {
		logger.Error(err, "Failed to update status.")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciled.")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	fieldToIndex, err := createGuacamoleIndexer(mgr)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&guacamoleoperatorgithubiov1alpha1.Connection{}).
		Watches(
			&v1alpha1.Guacamole{},
			r.watchGuacamoleRef(fieldToIndex),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// createGuacamoleIndexer creates a local index of Guacamole instaces
// referenced by Connections.
func createGuacamoleIndexer(mgr ctrl.Manager) (string, error) {
	// We build an index for the Guacamole reference within a connection.
	const fieldToIndex string = ".spec.guacamoleRef.Name"

	// The indexer function extracts the index field from a given object.
	indexerFunc := func(obj client.Object) []string {
		connection, ok := obj.(*v1alpha1.Connection)
		if !ok {
			return nil
		}
		if connection.Spec.GuacamoleRef.Name == "" {
			return nil
		}
		return []string{connection.Spec.GuacamoleRef.Name}
	}

	// Build the indexer.
	err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.Connection{}, fieldToIndex, indexerFunc)
	if err != nil {
		return "", err
	}

	return fieldToIndex, nil
}

func (r *ConnectionReconciler) watchGuacamoleRef(indexField string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(r.guacamoleRequestMapFunc(indexField))
}

// guacamoleRequestMapFunc returns a list of Connection resources to be enqueued after
// an event of a corresponding Guacamole resource.
func (r *ConnectionReconciler) guacamoleRequestMapFunc(indexField string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		requests := []reconcile.Request{}

		guacamole, ok := obj.(*v1alpha1.Guacamole)
		if !ok {
			return requests
		}

		// Get all relevant connections.
		listOpts := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(indexField, obj.GetName()),
			Namespace:     guacamole.GetNamespace(),
		}

		var connections v1alpha1.ConnectionList
		if err := r.List(ctx, &connections, listOpts); err != nil {
			return requests
		}

		for _, c := range connections.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      c.GetName(),
					Namespace: c.GetNamespace(),
				},
			})
		}

		return requests
	}
}

// getConnectionParams retrieves access parameters for the Guacamole API.
func (r *ConnectionReconciler) getConnectionParams(ctx context.Context, obj *v1alpha1.Connection) (*guacclient.Config, error) {
	namespace := obj.GetNamespace()

	// Get corresponding Guacamole instance.
	guacRef := obj.Spec.GuacamoleRef.Name

	var guac v1alpha1.Guacamole
	if err := r.Get(ctx, types.NamespacedName{Name: guacRef, Namespace: namespace}, &guac); err != nil {
		return nil, err
	}

	if guac.Status.Access == nil {
		return nil, errors.New("access information missing")
	}

	clientConfig := &guacclient.Config{
		Endpoint: guac.Status.Access.Endpoint,
		Source:   guac.Status.Access.Source,
	}

	// Retrieve credentials for API access.
	secretName := "guacamole-" + guacRef + "-credentials"
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &secret)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("Guacamole access parameters secret not found: %w", err)
	}

	errInvalidParamaters := errors.New("invalid parameters")

	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("Guacamole username parameter missing: %w", errInvalidParamaters)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("Guacamole password parameter missing: %w", errInvalidParamaters)
	}

	clientConfig.Username = string(username)
	clientConfig.Password = string(password)

	// Allow overwriting some parameters. Mainly useful for local testing, where cluster DNS
	// is not available.
	endpoint, ok := secret.Data["endpoint"]
	if ok {
		clientConfig.Endpoint = string(endpoint)
	}

	source, ok := secret.Data["source"]
	if ok {
		clientConfig.Source = string(source)
	}

	insecure, ok := secret.Data["insecure"]
	if ok {
		b, err := strconv.ParseBool(string(insecure))
		if err == nil {
			clientConfig.Insecure = b
		}
	}

	return clientConfig, nil
}
