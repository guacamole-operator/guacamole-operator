package ws

type EventType string

// Defines values for EventType.
const (
	UserCreate   EventType = "io.github.guacamole_operator.user.success.create"
	UserGet      EventType = "io.github.guacamole_operator.user.success.get"
	UserUpdate   EventType = "io.github.guacamole_operator.user.success.update"
	UserDelete   EventType = "io.github.guacamole_operator.user.success.delete"
	Authenticate EventType = "io.github.guacamole_operator.authentication.success"
)

// UserData defines the content of user event.
type UserData struct {
	// Username of user.
	Username string `json:"username"`
}
