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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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
	rp := NewHTTPReverseProxy(WithRequestModifier(modifier), WithTransport(&fakeTransport{}))

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
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("original-response")),
			Header:     make(http.Header),
		}, nil
	})

	modifier := func(r *http.Request) {
		u, _ := url.Parse("http://example.com/")
		r.URL = u
		r.Host = u.Host
	}
	rp := NewHTTPReverseProxy(WithResponseModifier(respMod), WithRequestModifier(modifier), WithTransport(rt))

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

func TestServeHTTPWithDefaultRewrite(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/test" {
			t.Errorf("expected path /test, got %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("ok")),
			Header:     make(http.Header),
		}, nil
	})

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	w := httptest.NewRecorder()

	rp := NewHTTPReverseProxy(WithTransport(rt))
	rp.ServeHTTP(w, req)

	if w.Body.String() != "ok" {
		t.Errorf("expected ok, got %s", w.Body.String())
	}
}

func TestServeHTTPWithCustomRequestModifier(t *testing.T) {
	backendURL, _ := url.Parse("http://example.com")
	customModifier := func(r *http.Request) {
		r.URL = backendURL
		r.Host = backendURL.Host
	}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("custom-director")),
			Header:     make(http.Header),
		}, nil
	})
	rp := NewHTTPReverseProxy(WithRequestModifier(customModifier), WithTransport(rt))
	req := httptest.NewRequest("GET", "http://original/", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Body.String() != "custom-director" {
		t.Errorf("expected custom-director, got %s", w.Body.String())
	}
}

func TestServeHTTPSetsXForwardedFor(t *testing.T) {
	var gotFor string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotFor = req.Header.Get("X-Forwarded-For")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("ok")),
			Header:     make(http.Header),
		}, nil
	})

	rp := NewHTTPReverseProxy(WithTransport(rt))
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "203.0.113.7:4321"
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if gotFor != "203.0.113.7" {
		t.Errorf("expected X-Forwarded-For 203.0.113.7, got %q", gotFor)
	}
}

func TestNewHTTPReverseProxyWithNoOpts(t *testing.T) {
	rp := NewHTTPReverseProxy()
	if rp.rp.Transport != DefaultTransport {
		t.Error("expected default transport")
	}
}
