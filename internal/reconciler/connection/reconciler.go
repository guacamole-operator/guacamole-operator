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

	// Concurrency factor for Guacamole API calls.
	concurrency int
}

// New instantiates a reconciler.
func New(client *client.Client, concurrency int) *Reconciler {
	return &Reconciler{
		client:      client,
		concurrency: concurrency,
	}
}

// Sync synchronizes the connection resource.
func (r *Reconciler) Sync(ctx context.Context, obj *v1alpha1.Connection) error {
	// Normalize parameters.
	if obj.Spec.Parameters == nil {
		obj.Spec.Parameters = &v1alpha1.ConnectionParameters{
			RawMessage: []byte("{}"),
		}
	}

	params := gen.ConnectionParameters{}
	err := params.UnmarshalJSON(obj.Spec.Parameters.RawMessage)
	if err != nil {
		return err
	}

	// Resolve connection group.
	parent, parents, err := r.client.ResolveConnectionGroup(ctx, *obj.Spec.Parent)
	if err != nil {
		return err
	}

	// Check if connection already exists.
	exists, cIdent, err := r.client.ConnectionExistsInGroup(ctx, parent, obj.Name)
	if err != nil {
		return err
	}

	// Check if connection exists in old group.
	oldParent := obj.Status.Parent
	if oldParent != nil && *oldParent != parent {
		exists, cIdent, err = r.client.ConnectionExistsInGroup(ctx, *oldParent, obj.Name)
		if err != nil {
			return err
		}
	}

	// Update connection if existent.
	if exists {
		request := gen.ConnectionRequest{
			Name:             obj.Name,
			Protocol:         obj.Spec.Protocol,
			ParentIdentifier: parent,
			Parameters:       params,
			Attributes:       gen.ConnectionAttributes{},
		}

		// Update connection. This can fail when a connection changes its parent
		// and a connection is already in place in the new group. As this connection
		// can be managed by another CR (or manually) fail and do not delete
		// or modify it here.
		response, err := r.client.UpdateConnectionWithResponse(ctx, r.client.Source, cIdent, request)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return errors.New("could not update connection")
		}

		obj.Status.Identifier = &cIdent
		obj.Status.Parent = &parent
	} else {
		// Create connection otherwise.
		request := gen.ConnectionRequest{
			Name:             obj.Name,
			Protocol:         obj.Spec.Protocol,
			ParentIdentifier: parent,
			Parameters:       params,
		}

		response, err := r.client.CreateConnectionWithResponse(ctx, r.client.Source, request)
		if err != nil {
			return err
		}

		if response.JSON200 == nil {
			return errors.New("could not create connection")
		}

		obj.Status.Identifier = &response.JSON200.Identifier
		obj.Status.Parent = &parent
	}

	// Set permissions for connection.
	if obj.Spec.Permissions == nil {
		obj.Spec.Permissions = &v1alpha1.ConnectionPermissions{}
	}

	identifier := *obj.Status.Identifier

	// Sync user permissions on a connection and all parent connection groups.
	err = r.syncUserPermissions(ctx, syncUserPermissionsParams{
		connectionID: identifier,
		users:        obj.Spec.Permissions.Users,
		parents:      parents,
	})
	if err != nil {
		return err
	}

	// Sync permissions of user group on a connection and all parent connection groups.
	err = r.syncUserGroupPermissions(ctx, syncUserGroupPermissionsParams{
		connectionID: identifier,
		groups:       obj.Spec.Permissions.Groups,
		parents:      parents,
	})
	if err != nil {
		return err
	}

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

type syncUserPermissionsParams struct {
	connectionID string
	users        []v1alpha1.ConnectionUser
	parents      []string
}

func (r *Reconciler) syncUserPermissions(ctx context.Context, params syncUserPermissionsParams) error {
	requestedUsers := make([]string, 0, len(params.users))
	for _, user := range params.users {
		requestedUsers = append(requestedUsers, user.ID)
	}

	return r.client.SyncUserPermissions(ctx, client.SyncUserPermissionsParams{
		ConnID:      params.connectionID,
		Users:       requestedUsers,
		Parents:     params.parents,
		Concurrency: r.concurrency,
	})
}

type syncUserGroupPermissionsParams struct {
	connectionID string
	groups       []v1alpha1.ConnectionGroup
	parents      []string
}

func (r *Reconciler) syncUserGroupPermissions(ctx context.Context, params syncUserGroupPermissionsParams) error {
	requestedUserGroups := make([]string, 0, len(params.groups))
	for _, group := range params.groups {
		requestedUserGroups = append(requestedUserGroups, group.ID)
	}

	return r.client.SyncUserGroupPermissions(ctx, requestedUserGroups, params.connectionID, params.parents)
}
