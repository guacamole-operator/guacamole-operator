package apierror

// APIError defines error for Guacamole API.
type APIError struct {
	Err error
}

// Error implements the Error interface.
func (e *APIError) Error() string {
	return e.Err.Error()
}

// Unwrap implements error unwrapping.
func (e *APIError) Unwrap() error {
	return e.Err
}
