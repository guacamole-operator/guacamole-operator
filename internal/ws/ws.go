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
func (c *Client) Read(ctx context.Context, msgCh chan<- []byte, errCh chan<- error) {
	for {
		if err := ctx.Err(); err != nil {
			break
		}

		if !c.isConnected() {
			if err := c.Connect(); err != nil {
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
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				break
			}

			errCh <- fmt.Errorf("error reading from websocket connection: %w", err)

			if err := c.Connect(); err != nil {
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
	if err := c.Close(); err != nil {
		return err
	}

	conn, _, err := websocket.Dial(context.Background(), c.url, nil)
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
