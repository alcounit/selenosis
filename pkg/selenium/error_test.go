package selenium

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorConstructors(t *testing.T) {
	rootErr := errors.New("root cause")

	tests := []struct {
		fn       func(error) *SeleniumError
		expected string
	}{
		{ErrSessionNotCreated, "session not created"},
		{ErrInvalidSessionId, "invalid session id"},
		{ErrInvalidArgument, "invalid argument"},
		{ErrBadRequest, "bad request"},
		{ErrUnknown, "unknown error"},
	}

	for _, tt := range tests {
		se := tt.fn(rootErr)
		if se.Value.Name != tt.expected {
			t.Errorf("expected Name=%s, got %s", tt.expected, se.Value.Name)
		}
		if !strings.HasPrefix(se.Value.Message, tt.expected) {
			t.Errorf("message should start with %s, got %s", tt.expected, se.Value.Message)
		}
		if !strings.Contains(se.Value.Message, "root cause") {
			t.Errorf("message should contain root cause, got %s", se.Value.Message)
		}
	}
}

func TestErrorDirect(t *testing.T) {
	rootErr := errors.New("direct cause")
	se := Error("custom-name", rootErr)
	if se.Value.Name != "custom-name" {
		t.Errorf("expected Name=custom-name, got %s", se.Value.Name)
	}
	if !strings.HasPrefix(se.Value.Message, "custom-name") {
		t.Errorf("message should start with custom-name, got %s", se.Value.Message)
	}
	if !strings.Contains(se.Value.Message, "direct cause") {
		t.Errorf("message should contain direct cause, got %s", se.Value.Message)
	}
}

func TestWriteError(t *testing.T) {
	rw := httptest.NewRecorder()
	WriteError(rw, http.StatusBadRequest, ErrSessionNotCreated(errors.New("boom")))

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rw.Code)
	}
	if ct := rw.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body SeleniumError
	if err := json.Unmarshal(rw.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode body %q: %v", rw.Body.String(), err)
	}
	if body.Value.Name != "session not created" {
		t.Fatalf("expected error name 'session not created', got %q", body.Value.Name)
	}
	if !strings.Contains(body.Value.Message, "boom") {
		t.Fatalf("expected message to contain root cause, got %q", body.Value.Message)
	}
}
