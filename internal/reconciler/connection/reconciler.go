package connection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	"github.com/guacamole-operator/guacamole-operator/internal/apierror"
	"github.com/guacamole-operator/guacamole-operator/internal/client"
	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
	"github.com/guacamole-operator/guacamole-operator/internal/set"
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
	parent, parents, err := r.resolveConnectionGroup(ctx, obj)
	if err != nil {
		return err
	}

	// Check if connection already exists.
	exists, cIdent, err := r.connectionExistsInGroup(ctx, parent, obj.Name)
	if err != nil {
		return err
	}

	// Check if connection exists in old group.
	oldParent := obj.Status.Parent
	if oldParent != nil && *oldParent != parent {
		exists, cIdent, err = r.connectionExistsInGroup(ctx, *oldParent, obj.Name)
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

	// User permission.
	{
		requestedUsers := set.New()
		for _, user := range obj.Spec.Permissions.Users {
			requestedUsers.Add(user.ID)
		}

		connUsers, err := r.getConnectionUsers(ctx, identifier)
		if err != nil {
			return err
		}

		currentUsers := set.FromSlice(connUsers)

		usersToAdd := set.Difference(requestedUsers, currentUsers)
		if err := r.addConnectionUsers(ctx, identifier, parents, usersToAdd.ToSlice()); err != nil {
			return err
		}

		usersToDelete := set.Difference(currentUsers, requestedUsers)
		if err := r.deleteConnectionUsers(ctx, identifier, usersToDelete.ToSlice()); err != nil {
			return err
		}
	}

	// User group permissions.
	{
		requestedGroups := set.New()
		for _, group := range obj.Spec.Permissions.Groups {
			requestedGroups.Add(group.ID)
		}

		connGroups, err := r.getConnectionGroups(ctx, identifier)
		if err != nil {
			return err
		}

		currentGroups := set.FromSlice(connGroups)

		groupsToAdd := set.Difference(requestedGroups, currentGroups)
		if err := r.addConnectionGroups(ctx, identifier, parents, groupsToAdd.ToSlice()); err != nil {
			return err
		}

		groupsToDelete := set.Difference(currentGroups, requestedGroups)
		if err := r.deleteConnectionGroups(ctx, identifier, groupsToDelete.ToSlice()); err != nil {
			return err
		}
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

func (r *Reconciler) connectionExistsInGroup(ctx context.Context, parent string, name string) (bool, string, error) {
	exists := false
	identifier := ""

	response, err := r.client.GetConnectionGroupTreeWithResponse(ctx, r.client.Source, parent)
	if err != nil {
		return exists, identifier, err
	}

	if response.JSON200 == nil {
		return exists, identifier, errors.New("could not retrieve connection group tree")
	}

	if response.JSON200.ChildConnections == nil {
		return exists, identifier, nil
	}

	for _, c := range *response.JSON200.ChildConnections {
		if c.Name == name {
			exists = true
			identifier = c.Identifier
		}
	}

	return exists, identifier, nil
}

// resolveConnectionGroup resolves a connection group path to the internal identifier.
// Missing groups will be created automatically. Returns the direct parent identifier
// and a list of all parent connection groups.
func (r *Reconciler) resolveConnectionGroup(ctx context.Context, obj *v1alpha1.Connection) (parent string, parents []string, err error) {
	path := *obj.Spec.Parent
	separator := "/"

	// Ensure leading / just in case.
	if !strings.HasPrefix(path, separator) {
		path = separator + path
	}

	// Ensure path start with ROOT.
	path = "ROOT" + path

	// Remove last /.
	path = strings.TrimSuffix(path, separator)

	// Split groups.
	// [ROOT, g1, g2, ...]
	groups := strings.Split(path, separator)

	// Just ROOT.
	if len(groups) == 1 {
		return "ROOT", nil, nil
	}

	// Retrieve current connection groups.
	response, err := r.client.GetConnectionGroupTreeWithResponse(ctx, r.client.Source, "ROOT")
	if err != nil {
		return "", nil, err
	}

	if response.JSON200 == nil {
		return "", nil, errors.New("could not get connection group tree")
	}

	tree := response.JSON200

	// Iterate over all other groups and create them if necessary.
	currentParent := "ROOT"
	existingGroups := tree.ChildConnectionGroups
	for i, group := range groups {
		// ROOT is always there.
		if i == 0 {
			continue
		}

		// No groups at all, create it.
		if existingGroups == nil {
			request := gen.ConnectionGroup{
				Name:             group,
				ParentIdentifier: currentParent,
				Type:             gen.ConnectionGroupTypeORGANIZATIONAL,
			}
			response, err := r.client.CreateConnectionGroupWithResponse(ctx, r.client.Source, request)
			if err != nil {
				return "", nil, err
			}

			if response.JSON200 == nil {
				return "", nil, fmt.Errorf("could not create connection group %s", group)
			}

			currentParent = *response.JSON200.Identifier
			parents = append(parents, currentParent)
			continue
		}

		// Check sub groups of this level.
		found := false
		idx := 0
		for i, subGroup := range *existingGroups {
			// Group exists.
			if subGroup.Name == group {
				currentParent = *subGroup.Identifier
				parents = append(parents, currentParent)
				found = true
				idx = i
				break
			}
		}

		// Group has to be created.
		if !found {
			request := gen.ConnectionGroup{
				Name:             group,
				ParentIdentifier: currentParent,
				Type:             gen.ConnectionGroupTypeORGANIZATIONAL,
			}
			response, err := r.client.CreateConnectionGroupWithResponse(ctx, r.client.Source, request)
			if err != nil {
				return "", nil, err
			}

			if response.JSON200 == nil {
				return "", nil, fmt.Errorf("could not create connection group %s", group)
			}

			currentParent = *response.JSON200.Identifier
			parents = append(parents, currentParent)
		}

		// Change group level for next loop.
		existingGroups = (*existingGroups)[idx].ChildConnectionGroups
	}

	return currentParent, parents, nil
}

// getConnectionUsers returns all users with permissions on a connection.
func (r *Reconciler) getConnectionUsers(ctx context.Context, connectionID string) ([]string, error) {
	users := []string{}

	// Query all users and their permissions. API has no ability to just return
	// users with permissions on a connection.
	//
	// TODO: Optimize getting users of connection.

	response, err := r.client.ListUsersWithResponse(ctx, r.client.Source)
	if err != nil {
		return users, err
	}

	if response.JSON200 == nil {
		return users, errors.New("could not query users")
	}

	for user := range *response.JSON200 {
		if user == r.client.Username {
			continue
		}

		response, err := r.client.GetUserPermissionsWithResponse(ctx, r.client.Source, user)
		if err != nil {
			return users, err
		}

		if response.JSON200 == nil {
			return users, fmt.Errorf("could not get permissions of user %s", user)
		}

		for id := range response.JSON200.ConnectionPermissions {
			if id == connectionID {
				users = append(users, user)
			}
		}
	}

	return users, nil
}

// addConnectionUsers adds READ permissions of users on a connection.
//
// nolint:dupl
func (r *Reconciler) addConnectionUsers(ctx context.Context, connectionID string, parentGroups []string, users []string) error {
	for _, user := range users {
		// Prepare patch entry to add user to a connection.
		var connectionPatch gen.PatchRequest_Item
		err := connectionPatch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
			Op:    gen.Add,
			Path:  fmt.Sprintf("/connectionPermissions/%s", connectionID),
			Value: string(gen.ObjectPermissionsREAD),
		})
		if err != nil {
			return err
		}

		var patch []gen.PatchRequest_Item
		patch = append(patch, connectionPatch)

		// Create additional patch entries to add permissions
		// to all parent connection groups. Guacamole does not propagate
		// permissions up the tree as of now.
		for _, groupID := range parentGroups {
			var groupPatch gen.PatchRequest_Item
			err := groupPatch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
				Op:    gen.Add,
				Path:  fmt.Sprintf("/connectionGroupPermissions/%s", groupID),
				Value: string(gen.ObjectPermissionsREAD),
			})
			if err != nil {
				return err
			}

			patch = append(patch, groupPatch)
		}

		response, err := r.client.ModifyUserPermissionsWithResponse(ctx, r.client.Source, user, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return &apierror.APIError{
				Err: fmt.Errorf("could not add permissions of user %s on connection %s", user, connectionID),
			}
		}
	}

	return nil
}

// deleteConnectionUsers removes permissions of users on a connection.
func (r *Reconciler) deleteConnectionUsers(ctx context.Context, connectionID string, users []string) error {
	for _, user := range users {
		// Prepare patch entry to remove user from a connection.
		var connectionPatch gen.PatchRequest_Item
		var permission any = string(gen.ObjectPermissionsREAD)

		err := connectionPatch.FromJSONPatchRequestRemove(gen.JSONPatchRequestRemove{
			Op:    gen.Remove,
			Path:  fmt.Sprintf("/connectionPermissions/%s", connectionID),
			Value: &permission,
		})
		if err != nil {
			return err
		}

		var patch []gen.PatchRequest_Item
		patch = append(patch, connectionPatch)

		// TODO: Create additional patch entries to remove permissions
		// from all parent connection groups. Can only be done when
		// the user has no other connection permissions in the same group(s).

		response, err := r.client.ModifyUserPermissionsWithResponse(ctx, r.client.Source, user, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not delete permissions of user %s on connection %s", user, connectionID)
		}
	}

	return nil
}

// getConnectionGroups returns all groups with permissions on a connection.
func (r *Reconciler) getConnectionGroups(ctx context.Context, connectionID string) ([]string, error) {
	groups := []string{}

	// Query all groups and their permissions. API has no ability to just return
	// groups with permissions on a connection.
	//
	// TODO: Optimize getting groups of connection.

	response, err := r.client.ListUserGroupsWithResponse(ctx, r.client.Source)
	if err != nil {
		return groups, err
	}

	if response.JSON200 == nil {
		return groups, errors.New("could not query groups")
	}

	for group := range *response.JSON200 {
		response, err := r.client.GetUserGroupPermissionsWithResponse(ctx, r.client.Source, group)
		if err != nil {
			return groups, err
		}

		if response.JSON200 == nil {
			return groups, fmt.Errorf("could not get permissions of group %s", group)
		}

		for id := range response.JSON200.ConnectionPermissions {
			if id == connectionID {
				groups = append(groups, group)
			}
		}
	}

	return groups, nil
}

// addConnectionGroups adds READ permissions of groups on a connection.
//
// nolint:dupl
func (r *Reconciler) addConnectionGroups(ctx context.Context, connectionID string, parentGroups []string, groups []string) error {
	for _, group := range groups {
		// Prepare patch entry to add a user group to a connection.
		var connectionPatch gen.PatchRequest_Item
		err := connectionPatch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
			Op:    gen.Add,
			Path:  fmt.Sprintf("/connectionPermissions/%s", connectionID),
			Value: string(gen.ObjectPermissionsREAD),
		})
		if err != nil {
			return err
		}

		var patch []gen.PatchRequest_Item
		patch = append(patch, connectionPatch)

		// Create additional patch entries to add permissions
		// to all parent connection groups. Guacamole does not propagate
		// permissions up the tree as of now.
		for _, groupID := range parentGroups {
			var groupPatch gen.PatchRequest_Item
			err := groupPatch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
				Op:    gen.Add,
				Path:  fmt.Sprintf("/connectionGroupPermissions/%s", groupID),
				Value: string(gen.ObjectPermissionsREAD),
			})
			if err != nil {
				return err
			}

			patch = append(patch, groupPatch)
		}

		response, err := r.client.ModifyUserGroupPermissionsWithResponse(ctx, r.client.Source, group, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not add permissions of group %s on connection %s", group, connectionID)
		}
	}

	return nil
}

// deleteConnectionGroups removes permissions of groups on a connection.
func (r *Reconciler) deleteConnectionGroups(ctx context.Context, connectionID string, groups []string) error {
	for _, group := range groups {
		// Prepare patch entry to remove user group from a connection.
		var connectionPatch gen.PatchRequest_Item
		var permission any = string(gen.ObjectPermissionsREAD)

		err := connectionPatch.FromJSONPatchRequestRemove(gen.JSONPatchRequestRemove{
			Op:    gen.Remove,
			Path:  fmt.Sprintf("/connectionPermissions/%s", connectionID),
			Value: &permission,
		})
		if err != nil {
			return err
		}

		var patch []gen.PatchRequest_Item
		patch = append(patch, connectionPatch)

		// TODO: Create additional patch entries to remove permissions
		// from all parent connection groups. Can only be done when
		// the user has no other connection permissions in the same group(s).

		response, err := r.client.ModifyUserGroupPermissionsWithResponse(ctx, r.client.Source, group, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not delete permissions of group %s on connection %s", group, connectionID)
		}
	}

	return nil
}
