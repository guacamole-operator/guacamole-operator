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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/addon/pkg/status"
	"sigs.k8s.io/kubebuilder-declarative-pattern/pkg/patterns/declarative"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	"github.com/guacamole-operator/guacamole-operator/internal/transformer"
)

// guacamoleFinalizer is the arbitrary string representing the resource's finalizer.
const guacamoleFinalizer = "guacamole.guacamole-operator.github.io/finalizer"

var _ reconcile.Reconciler = &GuacamoleReconciler{}

// GuacamoleReconciler reconciles a Guacamole object.
type GuacamoleReconciler struct {
	declarative.Reconciler
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	EnableListener bool
	Listener       Listener
	watchLabels    declarative.LabelMaker
}

// Listener defines an interface for a Guacamole CloudEvent listener.
type Listener interface {
	Add(namespace, name, url string)
	Remove(namespace, name string)
	Listen(ctx context.Context, eventCh chan<- GuacamoleWrappedEvent, errCh chan<- error, doneCh chan<- struct{})
}

// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=guacamoles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=guacamoles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=guacamole-operator.github.io,resources=guacamoles/finalizers,verbs=update
//
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;secrets;configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete;patch
//
// For WithApplyPrune.
// +kubebuilder:rbac:groups=*,resources=*,verbs=list

// setupReconciler configures the reconciler.
func (r *GuacamoleReconciler) setupReconciler(mgr ctrl.Manager) error {
	r.watchLabels = declarative.SourceLabel(mgr.GetScheme())

	return r.Init(mgr, &v1alpha1.Guacamole{},
		declarative.WithOwner(declarative.SourceAsOwner),
		declarative.WithLabels(r.watchLabels),
		declarative.WithStatus(status.NewKstatusCheck(mgr.GetClient(), &r.Reconciler)),
		declarative.WithApplyPrune(),
		// Transformation for Guacd is executed first to avoid changes on instance resources
		// made by Guacamole transformation.
		declarative.WithObjectTransform(transformer.Guacd(mgr.GetClient()), addon.ApplyPatches),
		declarative.WithObjectTransform(transformer.Guacamole(mgr.GetClient()), addon.ApplyPatches),
		declarative.WithApplyKustomize(),
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuacamoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	addon.Init()

	if err := r.setupReconciler(mgr); err != nil {
		return err
	}

	c, err := controller.New("guacamole-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Guacamole
	err = c.Watch(source.Kind(mgr.GetCache(), &v1alpha1.Guacamole{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.Guacamole]{}))
	if err != nil {
		return err
	}

	// Watch for changes to deployed objects.
	err = declarative.WatchChildren(declarative.WatchChildrenOptions{Manager: mgr, Controller: c, Reconciler: r, LabelMaker: r.watchLabels})
	if err != nil {
		return err
	}

	return nil
}

// Reconcile runs the declarative.Reconciler logic and adds custom finalizer logic.
func (r *GuacamoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch instance.
	instance := &v1alpha1.Guacamole{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get instance.")
		return ctrl.Result{}, err
	}

	isMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(instance, guacamoleFinalizer) {
			r.Listener.Remove(instance.GetNamespace(), instance.GetName())

			if controllerutil.RemoveFinalizer(instance, guacamoleFinalizer) {
				if err := r.Update(ctx, instance); err != nil {
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
	if !controllerutil.ContainsFinalizer(instance, guacamoleFinalizer) {
		logger.Info("Add finalizer.")
		controllerutil.AddFinalizer(instance, guacamoleFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			// Error updating the object - requeue the request.
			logger.Error(err, "Failed to update instance after adding finalizer")
			return ctrl.Result{}, err
		}
	}

	// Run declarative.Reconiler logic.
	result, err := r.Reconciler.Reconcile(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If instance is marked to have the cloudevents extension,
	// add it to the listener instance.
	_, ok := instance.GetAnnotations()["extension.guacamole-operator.github.io/cloudevents"]
	if ok && r.EnableListener {
		instanceURL, err := r.findWebSocketURL(ctx, instance)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("error finding WebSocket URL: %w", err)
		}

		r.Listener.Add(instance.GetNamespace(), instance.GetName(), instanceURL)
	}

	return result, nil
}

// findWebSocketURL retrieves access parameters for the WebSocket API provided
// by the custom Guacamole `cloudevents` extension.
func (r *GuacamoleReconciler) findWebSocketURL(ctx context.Context, obj *v1alpha1.Guacamole) (string, error) {
	name := fmt.Sprintf("guacamole-%s", obj.GetName())

	var svc corev1.Service
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: obj.GetNamespace()}, &svc); err != nil {
		return "", err
	}

	var wsPort *corev1.ServicePort
	for _, p := range svc.Spec.Ports {
		if p.Name == "ws" {
			wsPort = &p
		}
	}

	if wsPort == nil {
		return "", fmt.Errorf("no port with name 'ws' found")
	}

	url := fmt.Sprintf("ws://guacamole-%s.%s.svc.cluster.local:%d", obj.GetName(), obj.GetNamespace(), wsPort.Port)
	return url, nil
}
