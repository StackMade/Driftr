package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitError_Error(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{1, "exit status 1"},
		{0, "exit status 0"},
		{130, "exit status 130"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := (&ExitError{Code: tt.code}).Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExitError_SurvivesWrapping(t *testing.T) {
	// Execute() unwraps the error chain to find the exit code, so an ExitError
	// wrapped by a command must still be recognised and still carry its code.
	wrapped := fmt.Errorf("shim failed: %w", &ExitError{Code: 42})

	var exitErr *ExitError
	if !errors.As(wrapped, &exitErr) {
		t.Fatalf("errors.As did not find an *ExitError in %v", wrapped)
	}
	if exitErr.Code != 42 {
		t.Errorf("Code = %d, want 42", exitErr.Code)
	}
}
