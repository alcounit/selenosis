package selenosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
	"time"

	"github.com/alcounit/selenosis/platform"
	"github.com/alcounit/selenosis/selenium"
	"github.com/alcounit/selenosis/tools"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/imdario/mergo"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

var (
	httpClient = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

//HandleSession ...
func (s *App) HandleSession(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	logger := s.logger.WithFields(logrus.Fields{
		"request_method": r.Method,
		"req_path":       r.URL.Path,
		"request_id":     uuid.New(),
	})
	logger.WithField("time_elapsed", tools.TimeElapsed(start)).Info("session")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("Failed to read request body: %v", err)
		tools.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	type request struct {
		DesiredCapabilities selenium.Capabilities `json:"desiredCapabilities"`
		Capabilities        struct {
			AlwaysMatch selenium.Capabilities    `json:"alwaysMatch"`
			FirstMatch  []*selenium.Capabilities `json:"firstMatch"`
		} `json:"capabilities"`
	}

	caps := request{}
	err = json.Unmarshal(body, &caps)
	if err != nil {
		logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("Failed to parse request: %v", err)
		tools.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	caps.DesiredCapabilities.ValidateCapabilities()
	caps.Capabilities.AlwaysMatch.ValidateCapabilities()

	if caps.DesiredCapabilities.BrowserName != "" && caps.Capabilities.AlwaysMatch.BrowserName != "" {
		caps.DesiredCapabilities = caps.Capabilities.AlwaysMatch
	}

	firstMatchCaps := caps.Capabilities.FirstMatch
	if len(firstMatchCaps) == 0 {
		firstMatchCaps = append(firstMatchCaps, &selenium.Capabilities{})
	}

	var browser *platform.BrowserSpec
	var capabilities selenium.Capabilities

	for _, first := range firstMatchCaps {
		capabilities = caps.DesiredCapabilities
		mergo.Merge(&capabilities, first)
		capabilities.ValidateCapabilities()

		browser, err = s.browsers.Find(capabilities.BrowserName, capabilities.BrowserVersion)
		if err == nil {
			break
		}
	}

	if err != nil {
		logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("Requested browser config not found: %v", err)
		tools.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	image := parseImage(browser.Image)
	template := &platform.ServiceSpec{
		SessionID:             fmt.Sprintf("%s-%s", image, uuid.New()),
		RequestedCapabilities: capabilities,
		Template:              browser,
	}

	logger.WithField("time_elapsed", tools.TimeElapsed(start)).Infof("Creating pod from image: %s", template.Template.Image)

	service, err := s.client.Create(template)
	if err != nil {
		logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("Failed to start pod: %v", err)
		tools.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	cancel := func() {
		service.CancelFunc()
	}

	logger.WithField("time_elapsed", tools.TimeElapsed(start)).Infof("Pod started: %s", service.SessionID)

	var resp *http.Response

	service.URL.Path = r.URL.Path

	i := 1
	for ; ; i++ {
		req, _ := http.NewRequest(http.MethodPost, service.URL.String(), bytes.NewReader(body))
		req.Header.Set("X-Forwarded-Selenosis", s.selenosisHost)
		ctx, done := context.WithTimeout(r.Context(), s.browserWaitTimeout)
		rsp, err := httpClient.Do(req.WithContext(ctx))
		defer done()
		select {
		case <-ctx.Done():
			if rsp != nil {
				rsp.Body.Close()
			}
			switch ctx.Err() {
			case context.DeadlineExceeded:
				logger.WithField("time_elapsed", tools.TimeElapsed(start)).Warn("Session attempt timeout")
				if i < s.sessionRetryCount {
					continue
				}
				logger.WithField("time_elapsed", tools.TimeElapsed(start)).Warn("Service is not ready")
				tools.JSONError(w, "New session attempts retry count exceeded", http.StatusInternalServerError)
			case context.Canceled:
				logger.WithField("time_elapsed", tools.TimeElapsed(start)).Warn("Client disconnected")
			}
			cancel()
			return
		default:
		}
		if err != nil {
			logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("session failed: %v", err)
			tools.JSONError(w, "New session attempts retry count exceeded", http.StatusInternalServerError)
			cancel()
			return
		}
		if rsp.StatusCode == http.StatusNotFound {
			continue
		}
		resp = rsp
		break
	}

	logger.WithField("time_elapsed", tools.TimeElapsed(start)).Infof("Browser response code: %d", resp.StatusCode)

	defer resp.Body.Close()

	var msg map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&msg)
	if err != nil {
		cancel()
		logger.WithField("time_elapsed", tools.TimeElapsed(start)).Errorf("Unable to read service response: %v", err)
		tools.JSONError(w, "Failed to read service response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	json.NewEncoder(w).Encode(msg)

	logger.WithField("time_elapsed", tools.TimeElapsed(start)).Infof("Browser sessionId: %s", service.SessionID)

}

//HandleProxy ...
func (s *App) HandleProxy(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	host := tools.BuildHostPort(sessionID, s.serviceName, s.sidecarPort)

	logger := s.logger.WithFields(logrus.Fields{
		"request_method": r.Method,
		"session_id":     sessionID,
		"req_path":       r.URL.Path,
		"request_id":     uuid.New(),
	})

	logger.Info("proxy")

	(&httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "http"
			r.Host = host
			r.URL.Host = host
			r.Header.Set("X-Forwarded-Selenosis", s.selenosisHost)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Errorf("Proxy error: %s", err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}).ServeHTTP(w, r)

}

//HandleReverseProxy ...
func (s *App) HandleReverseProxy(port string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)
		sessionID := vars["sessionId"]

		fragments := strings.Split(r.URL.Path, "/")
		path := "/" + strings.Join(fragments[3:], "/")
		logger := s.logger.WithFields(logrus.Fields{
			"request_method": r.Method,
			"session_id":     sessionID,
			"req_path":       strings.TrimPrefix(r.URL.Path, "/wd/hub"),
			"request_id":     uuid.New(),
		})

		logger.Info("proxy")

		(&httputil.ReverseProxy{
			Director: func(r *http.Request) {
				r.URL.Scheme = "http"
				r.URL.Host = tools.BuildHostPort(sessionID, s.serviceName, port)
				r.URL.Path = path
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				logger.Errorf("Proxy error: %s", err)
				w.WriteHeader(http.StatusBadGateway)
			},
		}).ServeHTTP(w, r)
	}
}

//HandleVNC ...
func (s *App) HandleVNC(port string) websocket.Handler {
	return func(c *websocket.Conn) {
		defer c.Close()

		vars := mux.Vars(c.Request())
		sessionID := vars["sessionId"]

		host := tools.BuildHostPort(sessionID, s.serviceName, port)

		var dialer net.Dialer
		conn, err := dialer.DialContext(c.Request().Context(), "tcp", host)
		if err != nil {
			s.logger.Errorf("vnc connection error: %v", err)
		}
		defer conn.Close()

		go func() {
			io.Copy(c, conn)
			c.Close()
			s.logger.Errorf("vnc connection closed")
		}()
		io.Copy(conn, c)
		s.logger.Errorf("client disconnected")
	}
}

func parseImage(image string) (container string) {
	pref, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		return "selenoid-browser"
	}
	return pref.ReplaceAllString(image, "-")
}
