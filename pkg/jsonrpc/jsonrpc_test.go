package jsonrpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    int
		message string
	}{
		{"invalid params", http.StatusBadRequest, InvalidParams, "Bad Request: missing params"},
		{"invalid request", http.StatusBadRequest, InvalidRequest, "Invalid Request: already initialized"},
		{"session not found", http.StatusNotFound, SessionNotFound, "Session not found"},
		{"internal error", http.StatusInternalServerError, InternalError, "Internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			WriteError(rw, tt.status, tt.code, tt.message)

			if rw.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, rw.Code)
			}
			if ct := rw.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("expected Content-Type application/json, got %q", ct)
			}

			var body struct {
				JSONRPC string `json:"jsonrpc"`
				ID      any    `json:"id"`
				Error   struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rw.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode body %q: %v", rw.Body.String(), err)
			}
			if body.JSONRPC != "2.0" {
				t.Fatalf("expected jsonrpc 2.0, got %q", body.JSONRPC)
			}
			if body.ID != nil {
				t.Fatalf("expected id null, got %v", body.ID)
			}
			if body.Error.Code != tt.code {
				t.Fatalf("expected code %d, got %d", tt.code, body.Error.Code)
			}
			if body.Error.Message != tt.message {
				t.Fatalf("expected message %q, got %q", tt.message, body.Error.Message)
			}
		})
	}
}
