package controllers

import (
	"context"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	"github.com/guacamole-operator/guacamole-operator/internal/reconciler/connection"
)

// connectionFinalizer is the arbitrary string representing the resource's finalizer.
const connectionFinalizer = "connection.guacamole-operator.github.io/finalizer"

// finalize handles the finalizer logic.
// Objects with an owner reference pointing to this controller are deleted automatically, custom
// actions are handled here.
func (r *ConnectionReconciler) finalize(ctx context.Context, obj *v1alpha1.Connection, reconciler *connection.Reconciler) error {
	return reconciler.Delete(ctx, obj)
}
