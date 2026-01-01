package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	browserv1 "github.com/alcounit/browser-controller/apis/browser/v1"
	logctx "github.com/alcounit/browser-controller/pkg/log"
	"github.com/alcounit/browser-service/pkg/client"
	"github.com/alcounit/selenosis/pkg/ipuuid"
	"github.com/alcounit/selenosis/pkg/proxy"
	"github.com/alcounit/selenosis/pkg/selenium"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrMissingCapabilities = errors.New("missing request capabilities")
	ErrorReadRequestBody   = errors.New("failed to read request body")
	ErrDecodeRequestBody   = errors.New("failed to decode request body")
	ErrCapabilityMatch     = errors.New("cannot match request capabilities")
	ErrInternal            = errors.New("internal server error")
)

type Service struct {
	client client.Client
	config ServiceConfig
}

type ServiceConfig struct {
	Namespace             string
	SidecarPort           string
	SessionCreateAttempts int
	SessionCreateTimeout  time.Duration
}

func NewService(client client.Client, config ServiceConfig) *Service {
	return &Service{
		client: client,
		config: config,
	}
}

func (s *Service) CreateSession(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	if req.Body == nil {
		log.Err(ErrMissingCapabilities).Msg("empty request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrMissingCapabilities))
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Err(err).Msg("failed to read request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrorReadRequestBody))
		return
	}
	defer req.Body.Close()

	var caps selenium.Capabilities
	if err := json.Unmarshal(body, &caps); err != nil {
		log.Err(err).Msg("failed to decode request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrDecodeRequestBody))
		return
	}

	processed, err := caps.ProcessCapabilities()
	if err != nil {
		log.Err(err).Msg("failed process request capabilities")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrCapabilityMatch))
		return
	}

	template := &browserv1.Browser{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: browserv1.BrowserSpec{
			BrowserName:    processed.GetBrowserName(),
			BrowserVersion: processed.GetBrowserVersion(),
		},
	}

	log.Info().
		Str("browser", template.Spec.BrowserName).
		Str("version", template.Spec.BrowserVersion).
		Msg("creating browser resource")

	result, err := s.client.CreateBrowser(req.Context(), s.config.Namespace, template)
	if err != nil {
		log.Err(err).Msg("failed to create browser resource")
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.Error("failed to create browser", err))
		return
	}

	browserName := result.GetName()

	log.Info().Str("name", browserName).Msg("waiting for browser to become ready")

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	stream, err := s.client.Events(ctx, s.config.Namespace)
	if err != nil {
		log.Err(err).Str("browserId", browserName).Msg("failed to start browser event stream")
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.Error("failed to start browser event stream", err))
		return
	}
	defer stream.Close()

	var podIP string

waitLoop:
	for {
		select {
		case event, ok := <-stream.Events():
			if !ok {
				log.Error().Str("browserId", browserName).Msg("browser event stream closed unexpectedly")
				writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
				return
			}

			if event.Browser == nil {
				log.Warn().Str("browserId", browserName).Msg("received browser event with nil browser")
				continue
			}

			if event.Browser.GetName() != browserName {
				continue
			}

			if event.Browser.Status.PodIP == "" {
				continue
			}

			switch event.Browser.Status.Phase {
			case "Failed":
				log.Error().Str("browserId", browserName).Msg("browser failed to start")
				writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(err))
				return

			case "Running":
				podIP = event.Browser.Status.PodIP
				log.Info().Str("browserId", browserName).Msg("browser successfully started")
				break waitLoop
			}

		case err, ok := <-stream.Errors():
			if ok && err != nil {
				log.Error().Str("browserId", browserName).Msg("browser event stream error")
				writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(err))
				return
			}

		case <-ctx.Done():
			log.Info().Str("browserId", browserName).Msg("context cancelled, stopping browser event stream")
			writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(ErrInternal))
			return
		}
	}

	reqModifier := func(r *http.Request) {
		base := externalBaseURL(req)
		r.Header.Set("X-Selenosis-External-URL", base.String())

		r.URL = &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(podIP, s.config.SidecarPort),
			Path:   strings.TrimPrefix(req.URL.Path, "/wd/hub"),
		}
		r.Method = req.Method
		r.Host = r.URL.Host
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))

		log.Info().Str("browserId", browserName).Msg("request modified")
	}

	rp := proxy.NewHTTPReverseProxy(proxy.WithRequestModifier(reqModifier))
	rp.ServeHTTP(rw, req)

}

func (s *Service) ProxySession(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())
	sessionId := chi.URLParam(req, "sessionId")
	if sessionId == "" {
		log.Error().Msg("missing required url param: sessionId")
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(errors.ErrUnsupported))
		return
	}

	log.Info().Str("sessionId", sessionId).Msg("proxying session request")
	uid, err := uuid.Parse(sessionId)
	if err != nil {
		log.Error().Msg("invalid url param: sessionId")
		writeErrorResponse(rw, http.StatusInternalServerError, selenium.ErrUnknown(errors.ErrUnsupported))
		return
	}

	ip := ipuuid.UUIDToIP(uid)

	if proxy.IsWebSocketRequest(req) {
		resolver := func(r *http.Request) (*url.URL, error) {
			url := &url.URL{
				Scheme: "ws",
				Host:   net.JoinHostPort(ip.String(), s.config.SidecarPort),
				Path:   strings.TrimPrefix(r.URL.Path, "/wd/hub"),
			}
			log.Info().Str("ws_url", url.String()).Send()
			return url, nil
		}

		log.Info().
			Str("sessionId", sessionId).
			Str("ip", ip.String()).
			Msg("proxying websocketrequest to browser")

		rp := proxy.NewWebSocketReverseProxy(resolver)
		rp.ServeHTTP(rw, req)
		return
	}

	log.Info().
		Str("sessionId", sessionId).
		Str("ip", ip.String()).
		Msg("proxying request to browser")

	reqModifier := func(r *http.Request) {
		base := externalBaseURL(req)
		r.Header.Set("X-Selenosis-External-URL", base.String())

		r.URL = &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(ip.String(), s.config.SidecarPort),
			Path:   strings.TrimPrefix(req.URL.Path, "/wd/hub"),
		}
		r.Host = req.Host
	}

	rp := proxy.NewHTTPReverseProxy(proxy.WithRequestModifier(reqModifier))
	rp.ServeHTTP(rw, req)

}

func (s *Service) SessionStatus(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	var status selenium.Status
	status.Set("service started", true)

	log.Info().
		Msg("service status")

	raw, err := json.Marshal(&status)
	if err != nil {
		log.Err(err).Msg("error encoding the request body")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.Write(raw)
	rw.Header().Set("Content-Type", "application/json")
}

func (s *Service) RouteHTTP(rw http.ResponseWriter, req *http.Request) {
	sessionId := chi.URLParam(req, "sessionId")
	if sessionId == "" {
		log.Error().Msg("missing required url param: sessionId")
		http.Error(rw, "missing required url param: sessionId", http.StatusInternalServerError)
		return
	}

	rest := chi.RouteContext(req.Context()).RoutePath
	if rest == "" || rest == "/" {
		log.Error().Msg("missing required url param: path after sessionId is required")
		http.Error(rw, "missing required url param: path after sessionId is required", http.StatusInternalServerError)
		return
	}

	uid, err := uuid.Parse(sessionId)
	if err != nil {
		log.Error().Msg("invalid url param: sessionId")
		http.Error(rw, "invalid url param: sessionId", http.StatusInternalServerError)
		return
	}

	ip := ipuuid.UUIDToIP(uid)
	log.Info().
		Str("sessionId", sessionId).
		Str("ip", ip.String()).
		Msg("proxying api request to browser")

	reqModifier := func(r *http.Request) {
		r.URL.Scheme = "http"
		r.URL.Host = net.JoinHostPort(ip.String(), s.config.SidecarPort)
		r.URL.Path = path.Clean(req.URL.Path)
	}

	rp := proxy.NewHTTPReverseProxy(proxy.WithRequestModifier(reqModifier))
	rp.ServeHTTP(rw, req)
}

func writeErrorResponse(rw http.ResponseWriter, status int, err *selenium.SeleniumError) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	json.NewEncoder(rw).Encode(err)
}

func externalBaseURL(r *http.Request) *url.URL {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	return &url.URL{
		Scheme: proto,
		Host:   host,
	}
}
