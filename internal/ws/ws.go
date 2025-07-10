package ws

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/coder/websocket"
)

// Client implements a WebSocket client.
type Client struct {
	conn *websocket.Conn
}

// New returns a new Client.
func New(conn *websocket.Conn) *Client {
	return &Client{
		conn: conn,
	}
}

// Read CloudEvents from the wire. Has to be executed in a goroutine.
func (c *Client) Read(ctx context.Context, errCh chan<- error) {
	for {
		msgType, b, err := c.conn.Read(ctx)
		if err != nil {
			errCh <- fmt.Errorf("error reading from websocket connection: %w", err)
			continue
		}

		// JSON expected.
		if msgType != websocket.MessageText {
			continue
		}

		// Read CloudEvent.
		ce := cloudevents.NewEvent()

		err = json.Unmarshal(b, &ce)
		if err != nil {
			errCh <- fmt.Errorf("error converting to CloudEvent: %w", err)
			continue
		}
	}
}
