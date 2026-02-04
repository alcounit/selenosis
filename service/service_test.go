package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	browserv1 "github.com/alcounit/browser-controller/apis/browser/v1"
	"github.com/alcounit/browser-service/pkg/client"
	"github.com/alcounit/browser-service/pkg/event"
	"github.com/alcounit/selenosis/v2/pkg/auth"
	"github.com/alcounit/selenosis/v2/pkg/ipuuid"
	"github.com/alcounit/selenosis/v2/pkg/proxy"
	"github.com/alcounit/selenosis/v2/pkg/selenium"
	"github.com/go-chi/chi/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateSessionNilBody(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", nil, nil)
	req.Body = nil
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrMissingCapabilities))
}

func TestCreateSessionReadBodyError(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", io.NopCloser(errorReader{}), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrorReadRequestBody))
}

func TestCreateSessionDecodeError(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString("{"), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrDecodeRequestBody))
}

func TestCreateSessionCapabilitiesError(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(`{}`), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrCapabilityMatch))
}

func TestCreateSessionCreateBrowserError(t *testing.T) {
	err := errors.New("create failed")
	fc := &fakeClient{createErr: err}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", BrowserStartTimeout: time.Second})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.Error("failed to create browser", err))
}

func TestCreateSessionEventsError(t *testing.T) {
	err := errors.New("stream")
	fc := &fakeClient{
		streamErr: err,
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", BrowserStartTimeout: time.Second})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.Error("failed to start browser event stream", err))
}

func TestCreateSessionStreamClosed(t *testing.T) {
	stream := newFakeStream()
	stream.Close()

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: browserv1.Browser{}.ObjectMeta,
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", BrowserStartTimeout: time.Second})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
}

func TestCreateSessionFailedEvent(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{
				Phase:  "Failed",
				Reason: "nope",
			},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: browserv1.Browser{}.ObjectMeta,
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", BrowserStartTimeout: time.Second})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.Error("browser failed to start", ErrInternal))
}

func TestCreateSessionEventError(t *testing.T) {
	stream := newFakeStream()
	err := errors.New("event error")
	stream.errs <- err

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: browserv1.Browser{}.ObjectMeta,
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", BrowserStartTimeout: time.Second})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.ErrUnknown(err))
}

func TestCreateSessionContextDone(t *testing.T) {
	stream := newFakeStream()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: browserv1.Browser{}.ObjectMeta,
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns"})
	req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil).WithContext(ctx)
	rw := httptest.NewRecorder()

	svc.CreateSession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
}

func TestCreateSessionSuccess(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{Browser: nil}
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{
				Phase: "Running",
				PodIP: "127.0.0.1",
			},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: browserv1.Browser{}.ObjectMeta,
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SidecarPort: "4444", BrowserStartTimeout: time.Second})

	var gotReq *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return response(http.StatusOK, `{"value":{"sessionId":"orig"}}`), nil
	})

	withProxyTransport(t, rt, func() {
		req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
		req.Host = "example.com"
		rw := httptest.NewRecorder()

		svc.CreateSession(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		if gotReq == nil {
			t.Fatal("expected transport to be called")
		}
		if gotReq.URL.Host != "127.0.0.1:4444" {
			t.Fatalf("unexpected host: %s", gotReq.URL.Host)
		}
		if gotReq.Header.Get("X-Selenosis-External-URL") == "" {
			t.Fatal("expected external url header")
		}
	})
}

func TestCreateSessionUsesBrowserNameFilter(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{
				Phase: "Running",
				PodIP: "127.0.0.1",
			},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SidecarPort: "4444", BrowserStartTimeout: time.Second})

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{"value":{"sessionId":"orig"}}`), nil
	})

	withProxyTransport(t, rt, func() {
		req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBody()), nil)
		rw := httptest.NewRecorder()

		svc.CreateSession(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
	})

	if len(fc.eventsOpts) != 1 {
		t.Fatalf("expected 1 events option, got %d", len(fc.eventsOpts))
	}

	values := url.Values{}
	fc.eventsOpts[0](values)
	if values.Get("name") != "br" {
		t.Fatalf("expected browser name filter, got %q", values.Get("name"))
	}
}

func TestCreateSessionSelenosisOptionsAnnotation(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{Browser: nil}
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{
				Phase: "Running",
				PodIP: "127.0.0.1",
			},
		},
	}

	fc := &captureClient{fakeClient: fakeClient{stream: stream}}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SidecarPort: "4444", BrowserStartTimeout: time.Second})

	withProxyTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{"value":{"sessionId":"orig"}}`), nil
	}), func() {
		req := newRequestWithParams(http.MethodPost, "/wd/hub/session", bytes.NewBufferString(validCapsBodyWithOptions()), nil)
		rw := httptest.NewRecorder()

		svc.CreateSession(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		if fc.created == nil {
			t.Fatal("expected browser to be created")
		}
		if got := fc.created.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey]; got == "" {
			t.Fatalf("expected %s annotation to be set", browserv1.SelenosisOptionsAnnotationKey)
		}
	})
}

func TestProxySessionMissingId(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/wd/hub/session", nil, nil)
	rw := httptest.NewRecorder()

	svc.ProxySession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.ErrUnknown(errors.ErrUnsupported))
}

func TestProxySessionInvalidUUID(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/wd/hub/session/bad", nil, map[string]string{"sessionId": "bad"})
	rw := httptest.NewRecorder()

	svc.ProxySession(rw, req)

	verifyResponseError(t, rw, http.StatusInternalServerError, selenium.ErrUnknown(errors.ErrUnsupported))
}

func TestProxySessionWebSocket(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	uid, _ := ipuuid.IPToUUID(ip)
	sessionId := uid.String()

	svc := NewService(&fakeClient{}, ServiceConfig{SidecarPort: "4444"})

	req := newRequestWithParams(http.MethodGet, "/wd/hub/session/"+sessionId, nil, map[string]string{"sessionId": sessionId})
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rw := httptest.NewRecorder()

	svc.ProxySession(rw, req)

	if rw.Code != http.StatusBadGateway && rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 502 or 500, got %d", rw.Code)
	}
}

func TestProxySessionHTTP(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	uid, _ := ipuuid.IPToUUID(ip)
	sessionId := uid.String()

	svc := NewService(&fakeClient{}, ServiceConfig{SidecarPort: "4444"})

	var gotReq *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return response(http.StatusOK, "{}"), nil
	})

	withProxyTransport(t, rt, func() {
		req := newRequestWithParams(http.MethodGet, "/wd/hub/session/"+sessionId+"/url", nil, map[string]string{"sessionId": sessionId})
		req.Host = "example.com"
		rw := httptest.NewRecorder()

		svc.ProxySession(rw, req)

		if gotReq == nil {
			t.Fatal("expected transport to be called")
		}
		if gotReq.URL.Host != "example.com" {
			t.Fatalf("unexpected host: %s", gotReq.URL.Host)
		}
		if gotReq.Header.Get("X-Selenosis-External-URL") == "" {
			t.Fatal("expected external url header")
		}
	})
}

func TestSessionStatus(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/status", nil, nil)
	rw := httptest.NewRecorder()

	svc.SessionStatus(rw, req)

	if rw.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", rw.Header().Get("Content-Type"))
	}
	var status selenium.Status
	if err := json.Unmarshal(rw.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	if status.Value["ready"] != true {
		t.Fatalf("expected ready true, got %v", status.Value["ready"])
	}
}

func TestPlaywrightMissingParams(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/playwright", nil, nil)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rw.Code)
	}
}

func TestPlaywrightParseOptionsError(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright?labels.bad!=x",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "invalid label key") {
		t.Fatalf("expected parse error, got %q", rw.Body.String())
	}
}

func TestPlaywrightCreateBrowserError(t *testing.T) {
	fc := &fakeClient{createErr: errors.New("boom")}
	svc := NewService(fc, ServiceConfig{Namespace: "ns"})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "failed to create browser resource") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightEventsError(t *testing.T) {
	fc := &fakeClient{streamErr: errors.New("stream failed")}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "failed to start browser event stream") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightStreamClosed(t *testing.T) {
	stream := newFakeStream()
	stream.Close()

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "browser event stream closed unexpectedly") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightFailedEvent(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{Phase: "Failed", Reason: "oops"},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "browser failed to start") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightEventError(t *testing.T) {
	stream := newFakeStream()
	stream.errs <- errors.New("event failed")

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "browser event stream error") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightContextDone(t *testing.T) {
	stream := newFakeStream()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/playwright", nil).WithContext(ctx)
	req = setParams(req, map[string]string{"name": "chromium", "version": "123"})
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "context cancelled") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightInvalidPodIP(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{Phase: "Running", PodIP: ""},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "failed to convert IP to UUID") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightNilBrowserEventIsIgnored(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{Browser: nil}
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{Phase: "Running", PodIP: ""},
		},
	}

	fc := &fakeClient{
		stream: stream,
		createResult: &browserv1.Browser{
			ObjectMeta: metav1.ObjectMeta{Name: "br"},
		},
	}
	svc := NewService(fc, ServiceConfig{Namespace: "ns", SessionCreateTimeout: time.Second})
	req := newRequestWithParams(
		http.MethodGet,
		"/playwright",
		nil,
		map[string]string{"name": "chromium", "version": "123"},
	)
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "failed to convert IP to UUID") {
		t.Fatalf("unexpected body: %q", rw.Body.String())
	}
}

func TestPlaywrightProxyAttemptAndOwnerLabel(t *testing.T) {
	stream := newFakeStream()
	stream.events <- &event.BrowserEvent{
		Browser: &browserv1.Browser{
			Status: browserv1.BrowserStatus{Phase: "Running", PodIP: "127.0.0.1"},
		},
	}

	fc := &captureClient{
		fakeClient: fakeClient{
			stream: stream,
			createResult: &browserv1.Browser{
				ObjectMeta: metav1.ObjectMeta{Name: "br"},
			},
		},
	}
	svc := NewService(fc, ServiceConfig{
		Namespace:            "ns",
		SidecarPort:          "4444",
		SessionCreateTimeout: time.Second,
	})

	ctx := auth.WithOwner(context.Background(), auth.Owner{Name: "qa-owner"})
	req := httptest.NewRequest(http.MethodGet, "/playwright?labels.env=test&containers.browser.env.DEBUG=1", nil).WithContext(ctx)
	req = setParams(req, map[string]string{"name": "chromium", "version": "123"})
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	rw := httptest.NewRecorder()

	svc.Playwright(rw, req)

	if rw.Code != http.StatusBadGateway && rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 502 or 500, got %d", rw.Code)
	}
	if fc.created == nil {
		t.Fatal("expected browser to be created")
	}
	if fc.created.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey] == "" {
		t.Fatalf("expected %s annotation", browserv1.SelenosisOptionsAnnotationKey)
	}
	if fc.created.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey] != "qa-owner" {
		t.Fatalf("unexpected owner label: %q", fc.created.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey])
	}
}

func TestSetOwnerReference(t *testing.T) {
	template := &browserv1.Browser{}
	setOwnerReference(template, context.Background())
	if template.ObjectMeta.Labels != nil {
		t.Fatalf("expected nil labels without owner, got %+v", template.ObjectMeta.Labels)
	}

	ctx := auth.WithOwner(context.Background(), auth.Owner{Name: "alice"})
	setOwnerReference(template, ctx)
	if template.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey] != "alice" {
		t.Fatalf("unexpected owner label: %q", template.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey])
	}

	template.ObjectMeta.Labels = map[string]string{"existing": "x"}
	setOwnerReference(template, auth.WithOwner(context.Background(), auth.Owner{Name: "bob"}))
	if template.ObjectMeta.Labels["existing"] != "x" {
		t.Fatalf("expected existing label to stay unchanged")
	}
	if template.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey] != "bob" {
		t.Fatalf("unexpected owner label: %q", template.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey])
	}
}

func TestRouteHTTPMissingSessionId(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/session/", nil, nil)
	rw := httptest.NewRecorder()

	svc.RouteHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
}

func TestRouteHTTPMissingRestPath(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/session/abc", nil, map[string]string{"sessionId": "abc"})
	setRoutePath(req, "/")
	rw := httptest.NewRecorder()

	svc.RouteHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
}

func TestRouteHTTPInvalidUUID(t *testing.T) {
	svc := NewService(&fakeClient{}, ServiceConfig{})
	req := newRequestWithParams(http.MethodGet, "/session/abc", nil, map[string]string{"sessionId": "abc"})
	setRoutePath(req, "/foo")
	rw := httptest.NewRecorder()

	svc.RouteHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}
}

func TestRouteHTTPSuccess(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	uid, _ := ipuuid.IPToUUID(ip)
	sessionId := uid.String()

	svc := NewService(&fakeClient{}, ServiceConfig{SidecarPort: "4444"})

	var gotReq *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return response(http.StatusOK, "{}"), nil
	})

	withProxyTransport(t, rt, func() {
		req := newRequestWithParams(http.MethodGet, "/session/"+sessionId+"/path", nil, map[string]string{"sessionId": sessionId})
		setRoutePath(req, "/path")
		rw := httptest.NewRecorder()

		svc.RouteHTTP(rw, req)

		if gotReq == nil {
			t.Fatal("expected transport to be called")
		}
		if gotReq.URL.Host != "example.com" {
			t.Fatalf("unexpected host: %s", gotReq.URL.Host)
		}
	})
}

func TestExternalBaseURL(t *testing.T) {
	req := newRequestWithParams(http.MethodGet, "/", nil, nil)
	req.Host = "example.com"

	base := externalBaseURL(req)
	if base.String() != "http://example.com" {
		t.Fatalf("unexpected base: %s", base.String())
	}

	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "proxy.example.com")
	base = externalBaseURL(req)
	if base.String() != "https://proxy.example.com" {
		t.Fatalf("unexpected base: %s", base.String())
	}

	req.Header.Del("X-Forwarded-Proto")
	req.TLS = &tls.ConnectionState{}
	base = externalBaseURL(req)
	if base.Scheme != "https" {
		t.Fatalf("expected https scheme, got %s", base.Scheme)
	}
}

func verifyResponseError(t *testing.T, rw *httptest.ResponseRecorder, expectedCode int, expected *selenium.SeleniumError) {

	if rw.Code != expectedCode {
		t.Fatalf("expected status 500, got %d", rw.Code)
	}

	var actual selenium.SeleniumError
	if err := json.NewDecoder(rw.Body).Decode(&actual); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if expected.Value.Name != actual.Value.Name {
		t.Fatalf("expected error name %q, got %q", expected.Value.Name, actual.Value.Name)
	}

	if expected.Value.Message != actual.Value.Message {
		t.Fatalf("expected error message %q, got %q", expected.Value.Message, actual.Value.Message)
	}
}

type fakeClient struct {
	createErr    error
	createResult *browserv1.Browser
	stream       *fakeStream
	streamErr    error
	eventsOpts   []client.EventsOption
}

func (f *fakeClient) CreateBrowser(ctx context.Context, namespace string, browser *browserv1.Browser) (*browserv1.Browser, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResult != nil {
		return f.createResult, nil
	}
	return browser, nil
}

func (f *fakeClient) GetBrowser(ctx context.Context, namespace, name string) (*browserv1.Browser, error) {
	return nil, nil
}

func (f *fakeClient) DeleteBrowser(ctx context.Context, namespace, name string) error {
	return nil
}

func (f *fakeClient) ListBrowsers(ctx context.Context, namespace string) ([]*browserv1.Browser, error) {
	return nil, nil
}

func (f *fakeClient) Events(ctx context.Context, namespace string, opts ...client.EventsOption) (client.BrowserEventStream, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	f.eventsOpts = opts
	if f.stream != nil {
		return f.stream, nil
	}
	return newFakeStream(), nil
}

type captureClient struct {
	fakeClient
	created *browserv1.Browser
}

func (c *captureClient) CreateBrowser(ctx context.Context, namespace string, browser *browserv1.Browser) (*browserv1.Browser, error) {
	c.created = browser
	return c.fakeClient.CreateBrowser(ctx, namespace, browser)
}

type fakeStream struct {
	events    chan *event.BrowserEvent
	errs      chan error
	closeOnce sync.Once
}

func newFakeStream() *fakeStream {
	return &fakeStream{
		events: make(chan *event.BrowserEvent, 4),
		errs:   make(chan error, 2),
	}
}

func (s *fakeStream) Events() <-chan *event.BrowserEvent {
	return s.events
}

func (s *fakeStream) Errors() <-chan error {
	return s.errs
}

func (s *fakeStream) Close() {
	s.closeOnce.Do(func() {
		close(s.events)
		close(s.errs)
	})
}

func validCapsBody() string {
	payload := map[string]any{
		"desiredCapabilities": map[string]any{
			"browserName":    "chrome",
			"browserVersion": "120",
		},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func validCapsBodyWithOptions() string {
	payload := map[string]any{
		"desiredCapabilities": map[string]any{
			"browserName":    "chrome",
			"browserVersion": "120",
			"selenosis:options": map[string]any{
				"labels": map[string]any{
					"env": "test",
				},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (errorReader) Close() error {
	return nil
}

func TestSetSelenosisOptions(t *testing.T) {
	ann := map[string]string{"k": "v"}
	opts := map[string]any{"a": "b"}
	out, err := setSelenosisOptions(ann, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out[browserv1.SelenosisOptionsAnnotationKey] == "" {
		t.Fatalf("expected %s to be set", browserv1.SelenosisOptionsAnnotationKey)
	}

	out, err = setSelenosisOptions(nil, map[string]any{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != nil && out[browserv1.SelenosisOptionsAnnotationKey] != "" {
		t.Fatalf("expected no options annotation for empty opts")
	}
}

func TestSetSelenosisOptionsMarshalError(t *testing.T) {
	_, err := setSelenosisOptions(nil, map[string]any{"bad": func() {}})
	if err == nil {
		t.Fatalf("expected error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

var defaultClientMu sync.Mutex

func serveRoundTrip(conn net.Conn, rt http.RoundTripper) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		resp = &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader(err.Error())),
			Header:     make(http.Header),
		}
	}
	_ = resp.Write(conn)
	if resp.Body != nil {
		resp.Body.Close()
	}
}

func withProxyTransport(t *testing.T, rt http.RoundTripper, fn func()) {
	t.Helper()
	defaultClientMu.Lock()
	prev := proxy.DefaultTransport
	proxy.DefaultTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go serveRoundTrip(serverConn, rt)
			return clientConn, nil
		},
		DisableKeepAlives: true,
	}
	defer func() {
		proxy.DefaultTransport = prev
		defaultClientMu.Unlock()
	}()
	fn()
}

func newRequestWithParams(method, path string, body io.Reader, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if params == nil {
		return req
	}
	return setParams(req, params)
}

func setParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, val := range params {
		rctx.URLParams.Add(key, val)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

func setRoutePath(req *http.Request, routePath string) {
	rctx := chi.RouteContext(req.Context())
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.RoutePath = routePath
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	*req = *req.WithContext(ctx)
}
