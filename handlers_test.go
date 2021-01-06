package selenosis

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alcounit/selenosis/config"
	"github.com/alcounit/selenosis/platform"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"gotest.tools/assert"
)

var (
	srv *httptest.Server
)

const (
	session = "/wd/hub/session"
)

func TestNewSessionRequestErrors(t *testing.T) {
	tests := map[string]struct {
		body     io.Reader
		respCode int
		respBody string
	}{
		"Verify new session call with body error request": {
			body:     errReader(0),
			respCode: http.StatusBadRequest,
			respBody: `{"code":400,"value":{"message":"test error"}}`,
		},
		"Verify new session call with empty body request": {
			body:     bytes.NewReader([]byte("")),
			respCode: http.StatusBadRequest,
			respBody: `{"code":400,"value":{"message":"unexpected end of JSON input"}}`,
		},
		"Verify new session call with empty json body request": {
			body:     bytes.NewReader([]byte("{}")),
			respCode: http.StatusBadRequest,
			respBody: `{"code":400,"value":{"message":"unknown browser name "}}`,
		},
		"Verify new session call with wrong json body request": {
			body:     bytes.NewReader([]byte("{{}")),
			respCode: http.StatusBadRequest,
			respBody: `{"code":400,"value":{"message":"invalid character '{' looking for beginning of object key string"}}`,
		},
		"Verify new session call with unknown browser name in request": {
			body:     bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"amigo", "browserVersion":"9.0"}]}}`)),
			respCode: http.StatusBadRequest,
			respBody: `{"code":400,"value":{"message":"unknown browser name amigo"}}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		app := initApp(nil)
		req, err := http.NewRequest(http.MethodPost, session, test.body)

		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		app.HandleSession(rr, req)

		res := rr.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionOnPlatformError(t *testing.T) {

	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session call when browser not started": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       true,
			platformFailureReason: errors.New("failed to create pod"),
			respCode:              http.StatusBadRequest,
			respBody:              `{"code":400,"value":{"message":"failed to create pod"}}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		client := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
		}
		app := initApp(client)
		req, err := http.NewRequest(http.MethodPost, session, test.reqBody)

		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		app.HandleSession(rr, req)

		res := rr.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionOnBrowserNetworkError(t *testing.T) {

	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session call to browser is not responding": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusInternalServerError,
			respBody:              `{"code":500,"value":{"message":"New session attempts retry count exceeded"}}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		client := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL: &url.URL{
					Scheme: "http",
					Host:   "",
				},
			},
		}
		app := initApp(client)
		req, err := http.NewRequest(http.MethodPost, session, test.reqBody)

		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		app.HandleSession(rr, req)

		res := rr.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionOnCancelRequest(t *testing.T) {
	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session on cancel request": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusOK,
			respBody:              "",
		},
	}
	for name, test := range tests {

		t.Logf("TC: %s", name)

		r := mux.NewRouter()
		r.HandleFunc("/wd/hub/session", func(w http.ResponseWriter, r *http.Request) {
		})

		s := httptest.NewServer(r)
		defer s.Close()

		u, _ := url.Parse(s.URL)

		platform := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL:        u,
			},
		}
		app := initApp(platform)
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		resp, err := http.NewRequestWithContext(ctx, http.MethodPost, session, test.reqBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		app.HandleSession(rr, resp)

		res := rr.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionOnRequestTimeout(t *testing.T) {
	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session on cancel request": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusInternalServerError,
			respBody:              `{"code":500,"value":{"message":"New session attempts retry count exceeded"}}`,
		},
	}
	for name, test := range tests {

		t.Logf("TC: %s", name)

		r := mux.NewRouter()
		r.HandleFunc("/wd/hub/session", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1 * time.Second)
		})

		s := httptest.NewServer(r)
		defer s.Close()

		u, _ := url.Parse(s.URL)

		platform := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL:        u,
			},
		}

		app := initApp(platform)

		ctx := context.Background()
		resp, err := http.NewRequestWithContext(ctx, http.MethodPost, session, test.reqBody)
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		app.HandleSession(rr, resp)

		res := rr.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionResponseCodeError(t *testing.T) {

	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session call to browser response code error": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusInternalServerError,
			respBody:              `{"code":500,"value":{"message":"Failed to read service response"}}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		mux := http.NewServeMux()
		mux.HandleFunc("/wd/hub/session", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		s := httptest.NewServer(mux)
		defer s.Close()

		u, _ := url.Parse(s.URL)

		platform := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL:        u,
			},
		}
		app := initApp(platform)
		req, err := http.NewRequest(http.MethodPost, session, test.reqBody)
		if err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		app.HandleSession(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionResponseBodyError(t *testing.T) {

	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session call to browser response error": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusInternalServerError,
			respBody:              `{"code":500,"value":{"message":"Failed to read service response"}}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		mux := http.NewServeMux()
		mux.HandleFunc("/wd/hub/session", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("`"))
		})
		s := httptest.NewServer(mux)
		defer s.Close()

		u, _ := url.Parse(s.URL)

		platform := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL:        u,
			},
		}
		app := initApp(platform)
		req, err := http.NewRequest(http.MethodPost, session, test.reqBody)
		if err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		app.HandleSession(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func TestNewSessionCreated(t *testing.T) {

	tests := map[string]struct {
		reqBody               io.Reader
		platformFailure       bool
		platformFailureReason error
		respCode              int
		respBody              string
	}{
		"Verify new session created": {
			reqBody:               bytes.NewReader([]byte(`{"capabilities":{"firstMatch":[{"browserName":"chrome", "browserVersion":"68.0"}]}}`)),
			platformFailure:       false,
			platformFailureReason: nil,
			respCode:              http.StatusOK,
			respBody:              `{"sessionID":"223a259c-50e9-4d18-82bc-26a0cc8cb85f"}`,
		},
	}

	for name, test := range tests {
		t.Logf("TC: %s", name)

		mux := http.NewServeMux()
		mux.HandleFunc("/wd/hub/session", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sessionID":"223a259c-50e9-4d18-82bc-26a0cc8cb85f"}`))
		})
		s := httptest.NewServer(mux)
		defer s.Close()

		u, _ := url.Parse(s.URL)

		platform := &PlatformMock{
			shouldFail:    test.platformFailure,
			failureReason: test.platformFailureReason,
			service: &platform.Service{
				SessionID:  "sessionID",
				CancelFunc: func() {},
				URL:        u,
			},
		}
		app := initApp(platform)
		req, err := http.NewRequest(http.MethodPost, session, test.reqBody)
		if err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		app.HandleSession(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		body := string(bytes.TrimSpace(b))

		assert.Equal(t, test.respCode, res.StatusCode)
		assert.Equal(t, test.respBody, body)
	}

}

func initApp(p *PlatformMock) *App {
	logger := &logrus.Logger{}
	client := NewPlatformMock(p)
	conf := Configuration{
		SelenosisHost:       "hostname",
		ServiceName:         "selenosis",
		SidecarPort:         "4445",
		BrowserWaitTimeout:  300 * time.Millisecond,
		SessionIddleTimeout: 600 * time.Millisecond,
		SessionRetryCount:   2,
	}
	browsersConfig, _ := config.NewBrowsersConfig("config/browsers.yaml")

	return New(logger, client, browsersConfig, conf)
}

type PlatformMock struct {
	shouldFail    bool
	failureReason error
	service       *platform.Service
}

func NewPlatformMock(f *PlatformMock) platform.Platform {
	return f
}

func (p *PlatformMock) Create(*platform.ServiceSpec) (*platform.Service, error) {
	if !p.shouldFail {
		return p.service, nil
	}
	return nil, p.failureReason

}
func (p *PlatformMock) Delete(string) error {
	if !p.shouldFail {
		return nil
	}
	return p.failureReason
}
func (p *PlatformMock) List() ([]*platform.Service, error) {
	return nil, nil
}

func (p *PlatformMock) Watch() <-chan platform.Event {
	ch := make(chan platform.Event)
	return ch
}

func (p *PlatformMock) Logs(ctx context.Context, name string) (io.ReadCloser, error) {
	return nil, nil
}

type errReader int

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("test error")
}
