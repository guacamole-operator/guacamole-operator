package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Client implements a WebSocket client.
type Client struct {
	url     string
	conn    *websocket.Conn
	backoff wait.Backoff
	mutex   sync.Mutex
}

// New returns a WebSocket Client.
func New(url string) *Client {
	return &Client{
		url:     url,
		backoff: initBackoff(),
	}
}

// Read WebSocket messages from the wire. Has to be executed in a goroutine.
func (c *Client) Read(msgCh chan<- []byte, errCh chan<- error) {
	for {
		if !c.isConnected() {
			err := c.Connect()
			if err != nil {
				errCh <- err
			}

			time.Sleep(c.backoff.Step())
			continue
		}

		// Reset backoff until next reconnect is necessary.
		// TODO: Refactor to avoid unnecessary resets on each iteration.
		c.backoff = initBackoff()

		_, b, err := c.conn.Read(context.Background())
		if err != nil {
			errCh <- fmt.Errorf("error reading from websocket connection: %w", err)

			err := c.Connect()
			if err != nil {
				errCh <- err
			}

			continue
		}

		msgCh <- b
	}
}

// Close closes the WebSocket connection.
func (c *Client) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		err := c.conn.Close(websocket.StatusNormalClosure, "")

		c.conn = nil

		if err != nil {
			return err
		}
	}
	return nil
}

// Connect creates the WebSocket connection.
func (c *Client) Connect() error {
	err := c.Close()
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, c.url, nil)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.conn = conn
	return nil
}

// isConnected checks if a WebSocket connection is established.
func (c *Client) isConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.conn != nil
}

// initBackoff initializes backoff values.
func initBackoff() wait.Backoff {
	// Values are chosen to get a maximum timeout of roughly one minute.
	//nolint: mnd
	return wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.5,
		Steps:    10,
		Jitter:   0.1,
	}
}
