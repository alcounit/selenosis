package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	browserv1 "github.com/alcounit/browser-controller/apis/browser/v1"
	logctx "github.com/alcounit/browser-controller/pkg/log"
	browserclient "github.com/alcounit/browser-service/pkg/client/browser"
	"github.com/alcounit/selenosis/v2/pkg/auth"
	"github.com/alcounit/selenosis/v2/pkg/ipuuid"
	"github.com/alcounit/selenosis/v2/pkg/jsonrpc"
	"github.com/alcounit/selenosis/v2/pkg/proxy"
	"github.com/alcounit/selenosis/v2/pkg/selenium"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrMissingCapabilities = errors.New("missing request capabilities")
	ErrReadRequestBody     = errors.New("failed to read request body")
	ErrDecodeRequestBody   = errors.New("failed to decode request body")
	ErrCapabilityMatch     = errors.New("cannot match request capabilities")
	ErrInternal            = errors.New("internal server error")
)

const (
	maxRequestBodySize = 1 << 20 // 1 MB
	wdHubPrefix        = "/wd/hub"
)

type Service struct {
	client browserclient.Client
	config ServiceConfig
}

type ServiceConfig struct {
	Namespace           string
	SidecarPort         string
	BrowserStartTimeout time.Duration
}

type errorKind int

const (
	browserCreate errorKind = iota
	browserEventsStart
	browserStreamClosed
	browserFailed
	browserStreamError
	browserContextDone
)

func NewService(client browserclient.Client, config ServiceConfig) *Service {
	return &Service{
		client: client,
		config: config,
	}
}

func (s *Service) sidecarHost(ip string) string {
	return net.JoinHostPort(ip, s.config.SidecarPort)
}

func (s *Service) CreateSession(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	if req.Body == nil {
		log.Err(ErrMissingCapabilities).Msg("empty request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrMissingCapabilities))
		return
	}
	defer req.Body.Close()

	body, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySize))
	if err != nil {
		log.Err(err).Msg("failed to read request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrReadRequestBody))
		return
	}

	var caps selenium.Capabilities
	if err := json.Unmarshal(body, &caps); err != nil {
		log.Err(err).Msg("failed to decode request body")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrDecodeRequestBody))
		return
	}

	processed, err := caps.ProcessCapabilities()
	if err != nil {
		log.Err(err).Msg("failed to process request capabilities")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(ErrCapabilityMatch))
		return
	}

	opts := processed.GetSelenosisOptions()
	podIP, _, ok := s.createBrowser(rw, req, processed.GetBrowserName(), processed.GetBrowserVersion(), opts, writeCreateSessionWaitError)
	if !ok {
		return
	}

	log.Info().Str("ip", podIP).Msg("proxying session create request")

	reqModifier := func(r *http.Request) {
		r.Header.Set("X-Selenosis-External-URL", externalBaseURL(req).String())
		r.URL = &url.URL{
			Scheme: "http",
			Host:   s.sidecarHost(podIP),
			Path:   strings.TrimPrefix(req.URL.Path, wdHubPrefix),
		}
		r.Method = req.Method
		r.Host = r.URL.Host
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))

		log.Info().
			Str("ip", podIP).
			Msg("session create request modified")
	}

	rp := proxy.NewHTTPReverseProxy(
		proxy.WithRequestModifier(reqModifier),
		proxy.WithErrorHandler(createSessionProxyErrorHandler(log, podIP)),
	)
	rp.ServeHTTP(rw, req)
}

func (s *Service) ProxySession(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())
	sessionId := chi.URLParam(req, "sessionId")
	if sessionId == "" {
		log.Error().Msg("missing required url param: sessionId")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(errors.ErrUnsupported))
		return
	}

	ip, err := parseSessionID(sessionId)
	if err != nil {
		log.Error().Msg("invalid url param: sessionId")
		writeErrorResponse(rw, http.StatusBadRequest, selenium.ErrInvalidArgument(errors.ErrUnsupported))
		return
	}

	log.Info().Str("sessionId", sessionId).Str("ip", ip.String()).Msg("proxying session request")

	host := s.sidecarHost(ip.String())
	if proxy.IsWebSocketRequest(req) {
		resolver := func(r *http.Request) (*url.URL, error) {
			url := &url.URL{
				Scheme: "ws",
				Host:   host,
				Path:   strings.TrimPrefix(r.URL.Path, wdHubPrefix),
			}

			log.Info().Str("ws_url", url.String()).Msg("resolved websocket target url")
			return url, nil
		}

		log.Info().
			Str("sessionId", sessionId).
			Str("ip", ip.String()).
			Msg("proxying websocket request")

		rp := proxy.NewWebSocketReverseProxy(resolver)
		rp.ServeHTTP(rw, req)
		return
	}

	reqModifier := func(r *http.Request) {
		r.Header.Set("X-Selenosis-External-URL", externalBaseURL(req).String())
		r.URL = &url.URL{
			Scheme: "http",
			Host:   host,
			Path:   strings.TrimPrefix(req.URL.Path, wdHubPrefix),
		}
		r.Host = req.Host

		log.Info().Str("sessionId", sessionId).Str("ip", ip.String()).Msg("session proxy request modified")
	}

	rp := proxy.NewHTTPReverseProxy(
		proxy.WithRequestModifier(reqModifier),
		proxy.WithErrorHandler(sessionProxyErrorHandler(log, sessionId)),
	)
	rp.ServeHTTP(rw, req)
}

func (s *Service) SessionStatus(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	var status selenium.Status
	status.Set("service started", true)

	log.Info().Msg("service status")

	raw, err := json.Marshal(&status)
	if err != nil {
		log.Err(err).Msg("failed to encode response body")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.Write(raw)
}

func (s *Service) Playwright(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	name := chi.URLParam(req, "name")
	version := chi.URLParam(req, "version")
	if name == "" || version == "" {
		log.Error().Str("name", name).Str("version", version).Msg("missing required url params: name or version")
		http.Error(rw, fmt.Sprintf("missing required url param: name=%s version=%s", name, version), http.StatusNotFound)
		return
	}

	opts, err := parseSelenosisOptions(req.URL.Query(), defaultParseLimits())
	if err != nil {
		log.Err(err).Msg("failed to parse selenosis options from query parameters")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	podIP, sessionUUID, ok := s.createBrowser(rw, req, name, version, opts, writePlaywrightWaitError)
	if !ok {
		return
	}

	resolver := func(r *http.Request) (*url.URL, error) {
		url := &url.URL{
			Scheme: "ws",
			Host:   net.JoinHostPort(podIP, s.config.SidecarPort),
			Path:   "/playwright",
		}

		query := url.Query()
		query.Add("ipuuid", sessionUUID.String())

		url.RawQuery = query.Encode()
		return url, nil
	}

	log.Info().Str("ip", podIP).Msg("proxying playwright request")

	rp := proxy.NewWebSocketReverseProxy(resolver)
	rp.ServeHTTP(rw, req)
}

func (s *Service) RouteHTTP(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	sessionId := chi.URLParam(req, "sessionId")
	if sessionId == "" {
		log.Error().Msg("missing required url param: sessionId")
		http.Error(rw, "missing required url param: sessionId", http.StatusBadRequest)
		return
	}

	rest := chi.RouteContext(req.Context()).RoutePath
	if rest == "" || rest == "/" {
		log.Error().Msg("missing required url param: path after sessionId is required")
		http.Error(rw, "missing required url param: path after sessionId is required", http.StatusBadRequest)
		return
	}

	ip, err := parseSessionID(sessionId)
	if err != nil {
		log.Error().Msg("invalid url param: sessionId")
		http.Error(rw, "invalid url param: sessionId", http.StatusBadRequest)
		return
	}

	reqModifier := func(r *http.Request) {
		r.URL.Scheme = "http"
		r.URL.Host = s.sidecarHost(ip.String())
		r.URL.Path = path.Clean(req.URL.Path)

		log.Info().Str("sessionId", sessionId).Str("ip", ip.String()).Msg("http proxy request modified")
	}

	log.Info().Msg("proxying http proxy request")

	rp := proxy.NewHTTPReverseProxy(
		proxy.WithRequestModifier(reqModifier),
		proxy.WithErrorHandler(routeHTTPProxyErrorHandler(log, sessionId)),
	)
	rp.ServeHTTP(rw, req)
}

func (s *Service) McpHandler(rw http.ResponseWriter, req *http.Request) {
	log := logctx.FromContext(req.Context())

	sessionId := req.Header.Get("Mcp-Session-Id")
	if sessionId == "" && req.Method == http.MethodPost {

		name := req.URL.Query().Get("browser")
		version := req.URL.Query().Get("version")
		if name == "" || version == "" {
			log.Error().Str("browser", name).Str("version", version).Msg("missing required query params: browser or version")
			jsonrpc.WriteError(rw, http.StatusBadRequest, jsonrpc.InvalidParams, "Bad Request: browser and version query params are required")
			return
		}

		selenosisOpts, err := parseSelenosisOptions(req.URL.Query(), defaultParseLimits())
		if err != nil {
			log.Err(err).Msg("failed to parse selenosis options from query parameters")
			jsonrpc.WriteError(rw, http.StatusBadRequest, jsonrpc.InvalidParams, "Bad Request: "+err.Error())
			return
		}

		podIP, _, ok := s.createBrowser(rw, req, name, version, selenosisOpts, writeMcpWaitError)
		if !ok {
			return
		}

		host := s.sidecarHost(podIP)

		log.Info().Str("ip", podIP).Msg("proxying mcp initialize request")

		reqModifier := func(r *http.Request) {
			r.URL = &url.URL{
				Scheme:   "http",
				Host:     host,
				Path:     "/mcp",
				RawQuery: req.URL.RawQuery,
			}
			r.Host = host
		}

		rp := proxy.NewHTTPReverseProxy(
			proxy.WithRequestModifier(reqModifier),
			proxy.WithErrorHandler(mcpInitProxyErrorHandler(log, podIP)),
		)
		rp.ServeHTTP(rw, req)
		return
	}

	if sessionId == "" {
		log.Error().Msg("missing Mcp-Session-Id header")
		jsonrpc.WriteError(rw, http.StatusBadRequest, jsonrpc.InvalidParams, "Bad Request: Mcp-Session-Id header is required")
		return
	}

	ip, err := parseSessionID(sessionId)
	if err != nil {
		log.Error().Str("mcpSessionId", sessionId).Msg("invalid Mcp-Session-Id")
		jsonrpc.WriteError(rw, http.StatusBadRequest, jsonrpc.InvalidParams, "Bad Request: invalid Mcp-Session-Id")
		return
	}

	host := s.sidecarHost(ip.String())

	log.Info().Str("ip", ip.String()).Msg("proxying mcp request")

	reqModifier := func(r *http.Request) {
		r.URL = &url.URL{
			Scheme:   "http",
			Host:     host,
			Path:     "/mcp",
			RawQuery: req.URL.RawQuery,
		}
		r.Host = host
	}

	rp := proxy.NewHTTPReverseProxy(
		proxy.WithRequestModifier(reqModifier),
		proxy.WithErrorHandler(mcpProxyErrorHandler(log, ip.String())),
	)
	rp.ServeHTTP(rw, req)
}

func (s *Service) createBrowser(rw http.ResponseWriter, req *http.Request, name, version string, opts map[string]any, writeWaitError func(http.ResponseWriter, *browserError)) (string, uuid.UUID, bool) {
	log := logctx.FromContext(req.Context())

	template := &browserv1.Browser{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: browserv1.BrowserSpec{
			BrowserName:    name,
			BrowserVersion: version,
		},
	}

	var err error
	if len(opts) > 0 {
		template.ObjectMeta.Annotations, err = setSelenosisOptions(template.ObjectMeta.Annotations, opts)
		if err != nil {
			log.Err(err).Msg("failed to set selenosis options annotation")
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return "", uuid.UUID{}, false
		}
	}

	setOwnerReference(req.Context(), template)

	log = log.With().
		Str("browserName", template.Spec.BrowserName).
		Str("browserVersion", template.Spec.BrowserVersion).
		Str("namespace", s.config.Namespace).
		Logger()

	ctx, cancel := context.WithTimeout(req.Context(), s.config.BrowserStartTimeout)
	defer cancel()

	_, podIP, waitErr := s.createBrowserAndWait(ctx, log, template)
	if waitErr != nil {
		writeWaitError(rw, waitErr)
		return "", uuid.UUID{}, false
	}

	ip := net.ParseIP(podIP)
	if ip == nil {
		log.Err(fmt.Errorf("invalid pod IP: %s", podIP)).Msg("failed to parse pod IP")
		http.Error(rw, "failed to get browser IP", http.StatusInternalServerError)
		return "", uuid.UUID{}, false
	}

	sessionUUID, err := ipuuid.IPToUUID(ip)
	if err != nil {
		log.Err(err).Str("podIP", podIP).Msg("failed to convert IP to UUID")
		http.Error(rw, "failed to convert IP to UUID", http.StatusInternalServerError)
		return "", uuid.UUID{}, false
	}

	return podIP, sessionUUID, true
}

func (s *Service) createBrowserAndWait(ctx context.Context, logger zerolog.Logger, template *browserv1.Browser) (string, string, *browserError) {
	logger.Info().Msg("creating browser resource")

	stream, err := s.client.Events(ctx, s.config.Namespace, browserclient.WithName(template.GetName()))
	if err != nil {
		logger.Err(err).Str("name", template.GetName()).Msg("failed to start browser event stream")
		return template.GetName(), "", &browserError{kind: browserEventsStart, err: err}
	}
	defer stream.Close()

	result, err := s.client.Create(ctx, s.config.Namespace, template)
	if err != nil {
		logger.Err(err).Msg("failed to create browser resource")
		return "", "", &browserError{kind: browserCreate, err: err}
	}

	browserName := result.GetName()
	logger.Info().Str("name", browserName).Msg("waiting for browser to become ready")

	for {
		select {
		case event, ok := <-stream.Events():
			if !ok {
				logger.Error().Str("name", browserName).Msg("browser event stream closed unexpectedly")
				return browserName, "", &browserError{kind: browserStreamClosed}
			}

			if event.Browser == nil {
				logger.Warn().Str("name", browserName).Msg("received browser event with nil browser")
				continue
			}

			switch event.Browser.Status.Phase {
			case "Failed":
				logger.Error().Str("name", browserName).Str("statusReason", event.Browser.Status.Reason).Msg("browser failed to start")
				return browserName, "", &browserError{kind: browserFailed}

			case "Running":
				podIP := event.Browser.Status.PodIP
				logger.Info().Str("name", browserName).Msg("browser successfully started")
				return browserName, podIP, nil
			}

		case err, ok := <-stream.Errors():
			if !ok {
				logger.Error().Str("name", browserName).Msg("browser error stream closed unexpectedly")
				return browserName, "", &browserError{kind: browserStreamClosed}
			}

			if err != nil {
				logger.Err(err).Str("name", browserName).Msg("browser event stream error")
				return browserName, "", &browserError{kind: browserStreamError, err: err}
			}

		case <-ctx.Done():
			logger.Info().Str("name", browserName).Msg("context cancelled, stopping browser event stream")
			return browserName, "", &browserError{kind: browserContextDone}
		}
	}
}

func parseSessionID(sessionId string) (net.IP, error) {
	uid, err := uuid.Parse(sessionId)
	if err != nil {
		return nil, err
	}
	return ipuuid.UUIDToIP(uid), nil
}

func setOwnerReference(ctx context.Context, template *browserv1.Browser) {
	owner, ok := auth.OwnerFrom(ctx)
	if ok {
		if template.ObjectMeta.Labels == nil {
			template.ObjectMeta.Labels = map[string]string{}
		}
		template.ObjectMeta.Labels[browserv1.SelenosisOwnerLabelKey] = owner.Name
	}
}

func writeErrorResponse(rw http.ResponseWriter, status int, err *selenium.SeleniumError) {
	selenium.WriteError(rw, status, err)
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
