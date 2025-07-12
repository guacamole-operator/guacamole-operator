package ws

import (
	"context"
	"fmt"

	"github.com/coder/websocket"
)

// Client implements a WebSocket client.
type Client struct {
	url  string
	conn *websocket.Conn
}

// New returns a new Client.
func New(url string) *Client {
	return &Client{
		url: url,
	}
}

// Read CloudEvents from the wire. Has to be executed in a goroutine.
func (c *Client) Read(ctx context.Context, msgCh chan<- []byte, errCh chan<- error) {
	for {
		_, b, err := c.conn.Read(ctx)
		if err != nil {
			errCh <- fmt.Errorf("error reading from websocket connection: %w", err)
			err := c.Connect(ctx)
			if err != nil {
				errCh <- err
			}
			continue
		}
		msgCh <- b
	}
}

func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Client) Connect(ctx context.Context) error {
	err := c.Close()
	if err != nil {
		return err
	}

	return c.Dial(ctx)
}

func (c *Client) Dial(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.url, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}
