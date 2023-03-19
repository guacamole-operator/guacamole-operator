package connection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

	// Normalize parameters
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

	// Update connection.
	if identifier != nil {
		request := gen.ConnectionRequest{
			Name:             obj.Name,
			Protocol:         obj.Spec.Protocol,
			ParentIdentifier: *obj.Status.Parent,
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
	parent, err := r.resolveConnectionGroup(ctx, obj)
	if err != nil {
		return err
	}

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

// resolveConnectionGroup resolves a connection group path to the internal identifier.
// Missing groups will be created automatically.
func (r *Reconciler) resolveConnectionGroup(ctx context.Context, obj *v1alpha1.Connection) (string, error) {
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
		return "ROOT", nil
	}

	// Retrieve current connection groups.
	response, err := r.client.GetConnectionGroupTreeWithResponse(ctx, r.client.Source, "ROOT")
	if err != nil {
		return "", err
	}

	if response.JSON200 == nil {
		return "", errors.New("could not get connection group tree")
	}

	tree := response.JSON200

	// Iterator over all other groups and create them if necessary.
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
				return "", err
			}

			if response.JSON200 == nil {
				return "", fmt.Errorf("could not create connection group %s", group)
			}

			currentParent = *response.JSON200.Identifier
			continue
		}

		// Check sub groups of this level.
		found := false
		idx := 0
		for i, subGroup := range *existingGroups {
			// Group exists.
			if subGroup.Name == group {
				currentParent = *subGroup.Identifier
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
				return "", err
			}

			if response.JSON200 == nil {
				return "", fmt.Errorf("could not create connection group %s", group)
			}

			currentParent = *response.JSON200.Identifier
		}

		// Change group level for next loop.
		existingGroups = (*existingGroups)[idx].ChildConnectionGroups
	}

	return currentParent, nil
}
