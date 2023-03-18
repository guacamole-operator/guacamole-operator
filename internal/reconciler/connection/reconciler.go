package connection

import (
	"context"
	"errors"
	"net/http"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	"github.com/guacamole-operator/guacamole-operator/internal/client"
	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
)

// Reconciler for the connection resource.
type Reconciler struct {
	// client for the Guacamole API.
	client *client.Client
}

// New instantiates a reconciler.
func New(client *client.Client) *Reconciler {
	return &Reconciler{
		client: client,
	}
}

// Sync synchronizes the connection resource.
func (r *Reconciler) Sync(ctx context.Context, obj *v1alpha1.Connection) error {
	identifier := obj.Status.Identifier

	// Update connection.
	if identifier != nil {
		params := gen.ConnectionParameters{}
		if obj.Spec.Protocol == "vnc" {
			if err := params.FromConnectionParametersVNC(gen.ConnectionParametersVNC{}); err != nil {
				return err
			}
		} else if obj.Spec.Protocol == "rdp" {
			if err := params.FromConnectionParametersRDP(gen.ConnectionParametersRDP{}); err != nil {
				return err
			}
		}

		request := gen.ConnectionRequest{
			Name:             obj.Name,
			Protocol:         obj.Spec.Protocol,
			ParentIdentifier: *obj.Spec.Parent,
			Parameters:       params,
			Attributes:       gen.ConnectionAttributes{},
		}

		response, err := r.client.UpdateConnectionWithResponse(ctx, r.client.Source, *identifier, request)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return errors.New("could not update connection")
		}

		return nil
	}

	// Create connection.
	request := gen.ConnectionRequest{
		Name:             obj.Name,
		Protocol:         obj.Spec.Protocol,
		ParentIdentifier: *obj.Spec.Parent,
	}

	response, err := r.client.CreateConnectionWithResponse(ctx, r.client.Source, request)
	if err != nil {
		return err
	}

	if response.JSON200 == nil {
		return errors.New("could not create connection")
	}

	obj.Status.Identifier = &response.JSON200.Identifier

	return nil
}

// Delete deletes the connection resource.
func (r *Reconciler) Delete(ctx context.Context, obj *v1alpha1.Connection) error {
	// Nothing to do.
	if obj.Status.Identifier == nil {
		return nil
	}

	response, err := r.client.DeleteConnectionWithResponse(ctx, r.client.Source, *obj.Status.Identifier)
	if err != nil {
		return err
	}

	// Assumption that resource is already deleted.
	if response.StatusCode() == http.StatusNotFound {
		return nil
	}

	if response.StatusCode() != http.StatusNoContent {
		return errors.New("could not delete connection")
	}

	return nil
}
