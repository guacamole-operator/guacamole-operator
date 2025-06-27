package apierror_test

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/guacamole-operator/guacamole-operator/internal/apierror"
)

func TestError(t *testing.T) {
	want := "test error"
	apiErr := apierror.APIError{
		Err: fmt.Errorf("%s", want),
	}
	if !cmp.Equal(want, apiErr.Error()) {
		t.Errorf("unexpected diff (-want +got):\n%s", cmp.Diff(want, apiErr.Error()))
	}
}

func TestUnwrap(t *testing.T) {
	want := "test error"
	err := fmt.Errorf("%s", want)
	apiErr := apierror.APIError{
		Err: fmt.Errorf("%s", err),
	}
	if err == apiErr.Unwrap() {
		t.Errorf("unexpected diff (-want +got):\n%s", cmp.Diff(err, apiErr.Unwrap()))
	}
}
