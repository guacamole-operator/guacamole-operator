package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/guacamole-operator/guacamole-operator/controllers"
	"github.com/guacamole-operator/guacamole-operator/internal/ws"
)

// id identifies a Guacamole instance by its name and namespace.
type id struct {
	Namespace string
	Name      string
}

// client encapsulates a WebSocket client and holds its relevant
// channels and context cancel functions.
type client struct {
	*ws.Client
	dataCh <-chan []byte
	errCh  <-chan error
	cancel context.CancelFunc
}

// Listener implements an event listener for CloudEvents produced by the
// custom Guacamole `cloudevents` extension.
type Listener struct {
	// WebSocket clients per Guacamole instance.
	clients map[id]*client
	mutex   sync.RWMutex
}

// Add a WebSocket Client for a Guacamole Instance.
func (l *Listener) Add(namespace, name, URL string) {
	if l.clients == nil {
		l.clients = make(map[id]*client)
	}

	id := id{
		Namespace: namespace,
		Name:      name,
	}

	if _, exists := l.clients[id]; exists {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	dataCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	client := client{
		Client: ws.New(URL),
		dataCh: dataCh,
		errCh:  errCh,
		cancel: cancel,
	}

	go client.Read(ctx, dataCh, errCh)
	l.clients[id] = &client
}

// Remove a WebSocket client for a Guacamole instance
// and close the connection.
func (l *Listener) Remove(namespace, name string) {
	id := id{
		Namespace: namespace,
		Name:      name,
	}

	client, exists := l.clients[id]
	if !exists {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	client.cancel()
	client.Close()

	delete(l.clients, id)
}

// Listen for events from all clients.
func (l *Listener) Listen(ctx context.Context, eventCh chan<- controllers.GuacamoleWrappedEvent, errCh chan<- error, doneCh chan<- struct{}) {
	for {
		if err := ctx.Err(); err != nil {
			break
		}

		for id, client := range l.clients {
			func() {
				l.mutex.RLock()
				defer l.mutex.RUnlock()

				select {
				case msg := <-client.dataCh:
					user, ok, err := getEventUser(msg)
					if err != nil {
						errCh <- fmt.Errorf("%s in %s: %w", id.Name, id.Namespace, err)
					}

					if !ok {
						break
					}

					e := controllers.GuacamoleWrappedEvent{
						Object: &event{
							namespace: id.Namespace,
							name:      id.Name,
							user:      user,
						},
					}

					eventCh <- e

				case err := <-client.errCh:
					errCh <- fmt.Errorf("%s in %s: %w", id.Name, id.Namespace, err)

				default:
				}
			}()
		}
	}

	// Cancel all `Read` goroutines and close the client's connection.
	for _, client := range l.clients {
		client.cancel()
		client.Close()
	}

	close(doneCh)
}

// getEventUser checks if a CloudEvent is a user related event
// and extracts the username. Only successful create, update
// or delete events are considered.
func getEventUser(msg []byte) (string, bool, error) {
	// Read CloudEvent.
	ce := cloudevents.NewEvent()
	err := json.Unmarshal(msg, &ce)
	if err != nil {
		return "", false, err
	}

	// Filter all valid user success events.
	validEventTypes := []string{
		"io.github.guacamole_operator.user.success.create",
		"io.github.guacamole_operator.user.success.update",
		"io.github.guacamole_operator.user.success.delete",
	}

	if !slices.Contains(validEventTypes, ce.Context.GetType()) {
		return "", false, nil
	}

	// Extract username from event.
	var user userData
	err = json.Unmarshal(ce.Data(), &user)
	if err != nil {
		return "", false, err
	}

	return user.Username, true, nil
}
