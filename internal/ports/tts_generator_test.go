// internal/ports/tts_generator_test.go
package ports

import (
	"errors"
	"testing"
)

func TestPermanentError_Error(t *testing.T) {
	err := &PermanentError{StatusCode: 401, Body: "invalid api key"}
	got := err.Error()
	if got == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestPermanentError_IsDetectableWithErrorsAs(t *testing.T) {
	var wrapped error = &PermanentError{StatusCode: 400, Body: "bad text"}

	var permErr *PermanentError
	if !errors.As(wrapped, &permErr) {
		t.Fatal("expected errors.As to unwrap a *PermanentError")
	}
	if permErr.StatusCode != 400 {
		t.Fatalf("got status %d, want 400", permErr.StatusCode)
	}
}
