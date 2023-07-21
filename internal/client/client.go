package client

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"

	"github.com/guacamole-operator/guacamole-operator/internal/client/gen"
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
