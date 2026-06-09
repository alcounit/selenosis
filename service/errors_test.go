package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func dialErr() error {
	return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
}

func assertMcpError(t *testing.T, rw *httptest.ResponseRecorder, wantStatus, wantCode int) {
	t.Helper()
	if rw.Code != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, rw.Code)
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
		t.Fatalf("failed to decode JSON-RPC error body %q: %v", rw.Body.String(), err)
	}
	if body.JSONRPC != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %q", body.JSONRPC)
	}
	if body.ID != nil {
		t.Fatalf("expected id null, got %v", body.ID)
	}
	if body.Error.Code != wantCode {
		t.Fatalf("expected error code %d, got %d", wantCode, body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestIsUpstreamUnreachable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"dial op error", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"wrapped dial op error", fmt.Errorf("proxy: %w", &net.OpError{Op: "dial"}), true},
		{"read op error", &net.OpError{Op: "read", Err: errors.New("reset")}, false},
		{"write op error", &net.OpError{Op: "write", Err: errors.New("broken pipe")}, false},
		{"generic error", errors.New("boom"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUpstreamUnreachable(tt.err); got != tt.want {
				t.Fatalf("isUpstreamUnreachable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCreateSessionProxyErrorHandler(t *testing.T) {
	h := createSessionProxyErrorHandler(zerolog.Nop(), "10.0.0.1")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodPost, "/", nil), errors.New("boom"))

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "session not created") {
		t.Fatalf("expected selenium session not created error, got %s", rw.Body.String())
	}
}

func TestSessionProxyErrorHandlerUnreachable(t *testing.T) {
	h := sessionProxyErrorHandler(zerolog.Nop(), "sid")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/", nil), dialErr())

	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "invalid session id") {
		t.Fatalf("expected invalid session id error, got %s", rw.Body.String())
	}
}

func TestSessionProxyErrorHandlerGeneric(t *testing.T) {
	h := sessionProxyErrorHandler(zerolog.Nop(), "sid")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("boom"))

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "unknown error") {
		t.Fatalf("expected unknown error, got %s", rw.Body.String())
	}
}

func TestRouteHTTPProxyErrorHandlerUnreachable(t *testing.T) {
	h := routeHTTPProxyErrorHandler(zerolog.Nop(), "sid")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/", nil), dialErr())

	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "session not found") {
		t.Fatalf("expected session not found, got %s", rw.Body.String())
	}
}

func TestRouteHTTPProxyErrorHandlerGeneric(t *testing.T) {
	h := routeHTTPProxyErrorHandler(zerolog.Nop(), "sid")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("boom"))

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rw.Code)
	}
}

func TestMcpInitProxyErrorHandler(t *testing.T) {
	h := mcpInitProxyErrorHandler(zerolog.Nop(), "10.0.0.1")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodPost, "/", nil), dialErr())

	assertMcpError(t, rw, http.StatusInternalServerError, -32603)
}

func TestMcpProxyErrorHandlerUnreachable(t *testing.T) {
	h := mcpProxyErrorHandler(zerolog.Nop(), "10.0.0.1")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodPost, "/", nil), dialErr())

	assertMcpError(t, rw, http.StatusNotFound, -32001)
}

func TestMcpProxyErrorHandlerGeneric(t *testing.T) {
	h := mcpProxyErrorHandler(zerolog.Nop(), "10.0.0.1")
	rw := httptest.NewRecorder()
	h(rw, httptest.NewRequest(http.MethodPost, "/", nil), errors.New("boom"))

	assertMcpError(t, rw, http.StatusInternalServerError, -32603)
}
