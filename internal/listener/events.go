package listener

// userData defines the content of user related CloudEvent.
type userData struct {
	// Username of user.
	Username string `json:"username"`
}

// event implements controllers.GuacamoleEvent.
type event struct {
	name      string
	namespace string
	user      string
}

func (e *event) Name() string {
	return e.name
}

func (e *event) Namespace() string {
	return e.namespace
}

func (e *event) Username() string {
	return e.user
}
