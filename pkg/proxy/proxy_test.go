package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type fakeTransport struct {
	called bool
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.called = true
	rec := httptest.NewRecorder()
	rec.WriteString("transport-ok")
	return rec.Result(), nil
}

type fakeErrorTransport struct{}

func (fet *fakeErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("backend unavailable")
}

func TestWithTransport(t *testing.T) {
	ft := &fakeTransport{}
	rp := NewHTTPReverseProxy(WithTransport(ft))

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()

	rp.ServeHTTP(w, req)

	if !ft.called {
		t.Error("expected custom transport to be called")
	}
}

func TestWithRequestModifier(t *testing.T) {
	modCalled := false
	modifier := func(r *http.Request) {
		modCalled = true
		r.URL.Host = "modified-host"
		r.URL.Scheme = "http"
	}
	rp := NewHTTPReverseProxy(WithRequestModifier(modifier))

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if !modCalled {
		t.Error("expected request modifier to be called")
	}
}

func TestWithResponseModifier(t *testing.T) {
	respCalled := false
	respMod := func(resp *http.Response) error {
		respCalled = true
		resp.Body = io.NopCloser(bytes.NewBufferString("modified-response"))
		return nil
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("original-response"))
	}))
	defer backend.Close()

	rp := NewHTTPReverseProxy(WithResponseModifier(respMod))
	rp.rp.Director = func(r *http.Request) {
		u, _ := url.Parse(backend.URL)
		r.URL = u
		r.Host = u.Host
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if !respCalled {
		t.Error("expected response modifier to be called")
	}
	got := w.Body.String()
	if got != "modified-response" {
		t.Errorf("expected modified-response, got %s", got)
	}
}

func TestWithErrorHandler(t *testing.T) {
	errCalled := false
	errHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		errCalled = true
		w.WriteHeader(http.StatusBadGateway)
	}

	rp := NewHTTPReverseProxy(
		WithTransport(&fakeErrorTransport{}),
		WithErrorHandler(errHandler),
	)

	req := httptest.NewRequest("GET", "http://nonexistent/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if !errCalled {
		t.Error("expected error handler to be called")
	}
	if w.Result().StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 status, got %d", w.Result().StatusCode)
	}
}

func TestServeHTTPWithDefaultDirector(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/test" {
			t.Errorf("expected path /test, got %s", r.URL.Path)
		}
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	req := httptest.NewRequest("GET", backendURL.String()+"/test", nil)
	w := httptest.NewRecorder()

	rp := NewHTTPReverseProxy()
	rp.ServeHTTP(w, req)

	if w.Body.String() != "ok" {
		t.Errorf("expected ok, got %s", w.Body.String())
	}
}

func TestServeHTTPWithCustomDirector(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom-director"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	customDirector := func(r *http.Request) {
		r.URL = backendURL
		r.Host = backendURL.Host
	}

	rp := NewHTTPReverseProxy(WithRequestModifier(customDirector))
	req := httptest.NewRequest("GET", "http://original/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Body.String() != "custom-director" {
		t.Errorf("expected custom-director, got %s", w.Body.String())
	}
}

func TestNewHTTPReverseProxyWithNoOpts(t *testing.T) {
	rp := NewHTTPReverseProxy()
	if rp.rp.Transport != transport {
		t.Error("expected default transport")
	}
}

func TestServeHTTPWithDirectorAlreadySet(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("director-already-set"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	rp := NewHTTPReverseProxy()
	rp.rp.Director = func(r *http.Request) {
		r.URL = backendURL
		r.Host = backendURL.Host
	}

	req := httptest.NewRequest("GET", "http://example/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Body.String() != "director-already-set" {
		t.Errorf("expected director-already-set, got %s", w.Body.String())
	}
}
