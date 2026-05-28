package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/guacamole-operator/guacamole-operator/internal/apierror"
	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
	"github.com/guacamole-operator/guacamole-operator/internal/set"
)

// Client for the Guacamole API.
type Client struct {
	*gen.ClientWithResponses
	Source   string
	Username string
}

// Config for client instantiation.
type Config struct {
	Endpoint string
	Username string
	Password string
	Insecure bool
	Source   string
}

type loginClient struct {
	*gen.ClientWithResponses
	username string
	password string
	token    string
}

// New instantiates a client.
func New(config *Config) (*Client, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: config.Insecure,
			},
		},
	}

	cl, err := gen.NewClientWithResponses(config.Endpoint, gen.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}

	loginClient := loginClient{
		ClientWithResponses: cl,
		username:            config.Username,
		password:            config.Password,
	}

	c, err := gen.NewClientWithResponses(config.Endpoint, gen.WithHTTPClient(httpClient), gen.WithRequestEditorFn(authenticate(loginClient)))
	if err != nil {
		return nil, err
	}

	return &Client{
		ClientWithResponses: c,
		Source:              config.Source,
		Username:            config.Username,
	}, nil
}

// authenticate is a request mutation function adding the Guacamole
// credentials to a request. It will renew the token if required.
func authenticate(client loginClient) gen.RequestEditorFn {
	const guacamoleToken string = "Guacamole-Token"

	return func(ctx context.Context, req *http.Request) error {
		// Generate or validate token.
		// Guacamole will not issue a new token if the old one in the payload is still valid.
		response, err := client.CreateOrValidateTokenWithFormdataBodyWithResponse(ctx, gen.TokenRequest{
			Username: client.username,
			Password: client.password,
			Token:    client.token,
		})
		if err != nil {
			return err
		}

		if response.JSON200 == nil {
			return errors.New("error creating or validating session token")
		}

		client.token = response.JSON200.AuthToken
		req.Header.Add(guacamoleToken, client.token)
		return nil
	}
}

// resolveConnectionGroup resolves a connection group path to the internal identifier.
// Missing groups will be created automatically. Returns the direct parent identifier
// and a list of all parent connection groups.
func (c *Client) ResolveConnectionGroup(ctx context.Context, p string) (parent string, parents []string, err error) {
	path := p
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
	response, err := c.GetConnectionGroupTreeWithResponse(ctx, c.Source, "ROOT")
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
			response, err := c.CreateConnectionGroupWithResponse(ctx, c.Source, request)
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
			response, err := c.CreateConnectionGroupWithResponse(ctx, c.Source, request)
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

// ConnectionExistsInGroup checks if the connection exists in a parent group and returns parent ID in that case.
func (c *Client) ConnectionExistsInGroup(ctx context.Context, parent string, connection string) (bool, string, error) {
	exists := false
	identifier := ""

	response, err := c.GetConnectionGroupTreeWithResponse(ctx, c.Source, parent)
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
		if c.Name == connection {
			exists = true
			identifier = c.Identifier
		}
	}

	return exists, identifier, nil
}

// SyncUserGroup synchronizes a user group. If the group already exists, keep it unchanged.
func (c Client) SyncUserGroupAndMembers(ctx context.Context, group string, users []string) error {
	// Get current users within group if exists.
	response, err := c.ListUserGroupMembersWithResponse(ctx, c.Source, group)
	if err != nil {
		return err
	}

	status := response.StatusCode()

	if status != http.StatusOK && status != http.StatusNotFound {
		return errors.New("error checking a user group and members")
	}

	// Get list of the user group members, if the group already exists.
	currentMembers := set.New()
	if status == http.StatusOK {
		if response.JSON200 == nil {
			return fmt.Errorf("could not fetch members of the group %s", group)
		}

		for _, u := range *response.JSON200 {
			currentMembers.Add(u)
		}
	}

	// Group doesn't exist. Create a group.
	if response.StatusCode() == http.StatusNotFound {
		resp, err := c.CreateUserGroupWithResponse(ctx, c.Source, gen.CreateUserGroupJSONRequestBody{
			Identifier: group,
			Disabled:   false,
		})
		if err != nil {
			return err
		}

		if resp.StatusCode() != http.StatusOK {
			return errors.New("error creating a group")
		}
	}

	requestedMembers := set.FromSlice(users)

	usersToAdd := set.Difference(requestedMembers, currentMembers)
	usersToDelete := set.Difference(currentMembers, requestedMembers)

	// If the list of the users to add/delete is empty, keep group unchanged.
	if usersToAdd.Len() == 0 && usersToDelete.Len() == 0 {
		return nil
	}

	var membersPatch gen.ModifyUserGroupMembersJSONRequestBody
	for _, user := range usersToAdd.ToSlice() {
		// Prepare patch entry to add a group member.
		var patch gen.PatchRequest_Item

		err := patch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
			Op:    gen.Add,
			Path:  "/",
			Value: user,
		})
		if err != nil {
			return err
		}

		membersPatch = append(membersPatch, patch)
	}

	for _, user := range usersToDelete.ToSlice() {
		// Prepare patch entry to delete a group member.
		var patch gen.PatchRequest_Item

		var u any = user
		err := patch.FromJSONPatchRequestRemove(gen.JSONPatchRequestRemove{
			Op:    gen.Remove,
			Path:  "/",
			Value: &u,
		})
		if err != nil {
			return err
		}

		membersPatch = append(membersPatch, patch)
	}

	membersResp, err := c.ModifyUserGroupMembersWithResponse(ctx, c.Source, group, membersPatch)
	if err != nil {
		return err
	}

	if membersResp.StatusCode() != http.StatusNoContent {
		return fmt.Errorf("could not update members of the group %s", group)
	}

	return nil
}

type SyncUserGroupPermissionsParams struct {
	ConnID  string
	Parents []string
	Groups  []string
}

// SyncUserGroupPermissions synchronizes the permissions of user groups on a connection.
// Furthermore synchronizes the permissions of user groups on all parent connection groups.
func (c *Client) SyncUserGroupPermissions(ctx context.Context, params SyncUserGroupPermissionsParams, filters ...Filter) error {
	requestedGroups := set.FromSlice(params.Groups)

	connGroups, err := c.getConnectionGroups(ctx, params.ConnID, filters...)
	if err != nil {
		return err
	}

	currentGroups := set.FromSlice(connGroups)

	groupsToAdd := set.Difference(requestedGroups, currentGroups)
	if err := c.addConnectionGroups(ctx, params.ConnID, params.Parents, groupsToAdd.ToSlice()); err != nil {
		return err
	}

	groupsToDelete := set.Difference(currentGroups, requestedGroups)
	if err := c.deleteConnectionGroups(ctx, params.ConnID, groupsToDelete.ToSlice()); err != nil {
		return err
	}

	return nil
}

type SyncUserPermissionsParams struct {
	ConnID      string
	Users       []string
	Parents     []string
	Concurrency int
}

// SyncUserPermissions synchronizes the permissions of a user on a connection.
// Furthermore synchronizes the permissions of a user on all parent connection groups.
func (c *Client) SyncUserPermissions(ctx context.Context, params SyncUserPermissionsParams) error {
	requestedUsers := set.FromSlice(params.Users)

	connUsers, err := c.getConnectionUsers(ctx, params.ConnID, params.Concurrency)
	if err != nil {
		return err
	}

	currentUsers := set.FromSlice(connUsers)

	usersToAdd := set.Difference(requestedUsers, currentUsers)
	if err := c.addConnectionUsers(ctx, params.ConnID, params.Parents, usersToAdd.ToSlice()); err != nil {
		return err
	}

	usersToDelete := set.Difference(currentUsers, requestedUsers)
	if err := c.deleteConnectionUsers(ctx, params.ConnID, usersToDelete.ToSlice()); err != nil {
		return err
	}

	return nil
}

// RemoveUserGroup removes user group.
func (c Client) RemoveUserGroup(ctx context.Context, name string) error {
	response, err := c.DeleteUserGroup(ctx, c.Source, name)
	if err != nil {
		return err
	}

	// Assumption that resource is already deleted.
	if response.StatusCode == http.StatusNotFound {
		return nil
	}

	if response.StatusCode != http.StatusNoContent {
		return errors.New("could not delete user group")
	}

	return nil
}

// RemoveConnection removes connection.
func (c Client) RemoveConnection(ctx context.Context, connectionID string) error {
	response, err := c.DeleteConnectionWithResponse(ctx, c.Source, connectionID)
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

type Filter func(gen.UserGroups) gen.UserGroups

// getConnectionGroups returns all groups with permissions on a connection.
func (c *Client) getConnectionGroups(ctx context.Context, connID string, filters ...Filter) ([]string, error) {
	groups := []string{}

	// Query all groups and their permissions. API has no ability to just return
	// groups with permissions on a connection.
	//
	// TODO: Optimize getting groups of connection.

	response, err := c.ListUserGroupsWithResponse(ctx, c.Source)
	if err != nil {
		return groups, err
	}

	if response.JSON200 == nil {
		return groups, errors.New("could not query groups")
	}

	filteredGroups := *response.JSON200

	for _, f := range filters {
		filteredGroups = f(filteredGroups)
	}

	for group := range filteredGroups {
		response, err := c.GetUserGroupPermissionsWithResponse(ctx, c.Source, group)
		if err != nil {
			return groups, err
		}

		if response.JSON200 == nil {
			return groups, fmt.Errorf("could not get permissions of group %s", group)
		}

		for id := range response.JSON200.ConnectionPermissions {
			if id == connID {
				groups = append(groups, group)
			}
		}
	}

	return groups, nil
}

// addConnectionGroups adds READ permissions of user groups on a connection.
// Furthermore adds permissions of user groups on all parent connection groups.
// nolint:dupl
func (c *Client) addConnectionGroups(ctx context.Context, connID string, parents []string, groups []string) error {
	for _, group := range groups {
		// Prepare patch entry to add a user group to a connection.
		var connectionPatch gen.PatchRequest_Item
		err := connectionPatch.FromJSONPatchRequestAdd(gen.JSONPatchRequestAdd{
			Op:    gen.Add,
			Path:  fmt.Sprintf("/connectionPermissions/%s", connID),
			Value: string(gen.ObjectPermissionsREAD),
		})
		if err != nil {
			return err
		}

		var patch []gen.PatchRequest_Item
		patch = append(patch, connectionPatch)

		// Create additional patch entries to add permissions of user group
		// to all parent connection groups. Guacamole does not propagate
		// permissions up the tree as of now.
		for _, groupID := range parents {
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

		response, err := c.ModifyUserGroupPermissionsWithResponse(ctx, c.Source, group, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not add permissions of group %s on connection %s", group, connID)
		}
	}

	return nil
}

// deleteConnectionGroups removes permissions of user groups on a connection.
func (c *Client) deleteConnectionGroups(ctx context.Context, connectionID string, groups []string) error {
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

		// TODO: Create additional patch entries to remove user group permissions
		// from all parent connection groups. Can only be done when
		// the user has no other connection permissions in the same group(s).

		response, err := c.ModifyUserGroupPermissionsWithResponse(ctx, c.Source, group, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not delete permissions of group %s on connection %s", group, connectionID)
		}
	}

	return nil
}

// getConnectionUsers returns all users with permissions on a connection.
func (c *Client) getConnectionUsers(ctx context.Context, connectionID string, concurrency int) ([]string, error) {
	// Query all users and their permissions. API has no ability to just return
	// users with permissions on a connection.
	//
	// TODO: Optimize getting users of connection.

	response, err := c.ListUsersWithResponse(ctx, c.Source)
	if err != nil {
		return nil, err
	}

	if response.JSON200 == nil {
		return nil, errors.New("could not query users")
	}

	userCount := len(*response.JSON200)
	usersCh := make(chan string, userCount)
	resultsCh := make(chan string, userCount)
	errCh := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()
			c.userPermissionWorker(ctx, connectionID, usersCh, resultsCh, errCh)
		}()
	}

	for user := range *response.JSON200 {
		if user == c.Username {
			continue
		}
		usersCh <- user
	}
	close(usersCh)

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		doneCh <- struct{}{}
	}()

	var users []string
	var errs error

L:
	for {
		select {
		case err := <-errCh:
			errs = errors.Join(errs, err)
		case user := <-resultsCh:
			users = append(users, user)
		case <-doneCh:
			break L
		}
	}

	return users, errs
}

// userPermissionWorker returns users who have the permissions on provided connection.
func (c *Client) userPermissionWorker(ctx context.Context, connectionID string,
	usersCh <-chan string, resultsCh chan<- string, errCh chan<- error,
) {
	for user := range usersCh {
		response, err := c.GetUserPermissionsWithResponse(ctx, c.Source, user)
		if err != nil {
			errCh <- err
			break
		}

		if response.JSON200 == nil {
			errCh <- fmt.Errorf("could not get permissions of user %s", user)
			continue
		}

		for id := range response.JSON200.ConnectionPermissions {
			if id == connectionID {
				resultsCh <- user
				break
			}
		}
	}
}

// addConnectionUsers adds READ permissions of users on a connection.
//
// nolint:dupl
func (c *Client) addConnectionUsers(ctx context.Context, connectionID string, parentGroups []string, users []string) error {
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

		response, err := c.ModifyUserPermissionsWithResponse(ctx, c.Source, user, patch)
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
func (c *Client) deleteConnectionUsers(ctx context.Context, connectionID string, users []string) error {
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

		response, err := c.ModifyUserPermissionsWithResponse(ctx, c.Source, user, patch)
		if err != nil {
			return err
		}

		if response.StatusCode() != http.StatusNoContent {
			return fmt.Errorf("could not delete permissions of user %s on connection %s", user, connectionID)
		}
	}

	return nil
}
