package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	logctx "github.com/alcounit/browser-controller/pkg/log"
	"github.com/gorilla/websocket"
)

type TargetResolver func(r *http.Request) (*url.URL, error)

type WSProxyOption func(*WSProxy)

func WithOnConnect(f func()) WSProxyOption {
	return func(p *WSProxy) { p.onConnect = f }
}

func WithOnMessage(f func()) WSProxyOption {
	return func(p *WSProxy) { p.onMessage = f }
}

func WithOnClose(f func()) WSProxyOption {
	return func(p *WSProxy) { p.onClose = f }
}

func WithRetryTimeout(timeout time.Duration) WSProxyOption {
	return func(p *WSProxy) {
		p.dialRetryEnabled = true
		p.timeout = timeout
	}
}

type WSProxy struct {
	Upgrader websocket.Upgrader
	Dialer   websocket.Dialer
	Resolve  TargetResolver

	dialRetryEnabled bool
	timeout          time.Duration

	onConnect func()
	onMessage func()
	onClose   func()
}

func NewWebSocketReverseProxy(resolver TargetResolver, opts ...WSProxyOption) *WSProxy {
	proxy := &WSProxy{
		Resolve: resolver,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		dialRetryEnabled: false,
		Dialer: websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 10 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(proxy)
	}

	return proxy
}

func (p *WSProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := logctx.FromContext(r.Context())

	if p.Resolve == nil {
		log.Error().Msg("no resolver configured for websocket proxy")
		http.Error(w, "resolver not configured", http.StatusInternalServerError)
		return
	}

	var (
		upstreamConn *websocket.Conn
		resp         *http.Response
		err          error
	)

	targetURL, err := p.Resolve(r)
	if err != nil {
		log.Error().Err(err).Msg("resolve target URL failed")
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	upstreamHeaders := cloneHeaders(r)
	addForwardedHeaders(upstreamHeaders, r)
	upstreamHeaders.Set("Host", targetURL.Host)
	upstreamHeaders.Del("Origin")

	if p.dialRetryEnabled {
		upstreamConn, resp, err = dialWithWait(r.Context(), targetURL.String(), &p.Dialer, upstreamHeaders, p.timeout)
	} else {
		upstreamConn, resp, err = p.Dialer.DialContext(r.Context(), targetURL.String(), upstreamHeaders)
	}

	if err != nil {
		if resp != nil {
			log.Error().Err(err).
				Str("target", targetURL.String()).
				Int("status", resp.StatusCode).
				Msg("upstream websocket dial failed")
		} else {
			log.Error().Err(err).
				Str("target", targetURL.String()).
				Msg("upstream websocket dial failed")
		}
		http.Error(w, "upstream dial failed", http.StatusBadGateway)
		return
	}

	upgradeHeaders := filterUpgradeResponseHeaders(resp.Header)

	if proto := resp.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		upgradeHeaders.Set("Sec-WebSocket-Protocol", proto)
	}

	clientConn, err := p.Upgrader.Upgrade(w, r, upgradeHeaders)
	if err != nil {
		log.Error().Err(err).Msg("client websocket upgrade failed")
		http.Error(w, fmt.Sprintf("Upgrade client connection failed: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if p.onConnect != nil {
		p.onConnect()
	}

	pipeWebSocket(clientConn, upstreamConn, p.onMessage, p.onClose)
}

func pipeWebSocket(a, b *websocket.Conn, onMessage, onClose func()) {
	var wg sync.WaitGroup
	wg.Add(2)

	var once sync.Once

	shutdown := func() {
		once.Do(func() {
			if onClose != nil {
				onClose()
			}

			deadline := time.Now().Add(1 * time.Second)

			_ = a.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "closing"),
				deadline,
			)
			_ = b.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "closing"),
				deadline,
			)
			_ = a.Close()
			_ = b.Close()
		})
	}

	pump := func(src, dst *websocket.Conn) {
		defer wg.Done()

		for {
			msgType, data, err := src.ReadMessage()
			if err != nil {
				shutdown()
				return
			}

			if onMessage != nil {
				onMessage()
			}

			if err := dst.WriteMessage(msgType, data); err != nil {
				shutdown()
				return
			}
		}
	}

	go pump(a, b)
	go pump(b, a)

	wg.Wait()
}

func cloneHeaders(r *http.Request) http.Header {
	h := http.Header{}

	for k, vv := range r.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Connection",
			"Upgrade",
			"Sec-Websocket-Key",
			"Sec-Websocket-Version",
			"Sec-Websocket-Accept",
			"Sec-Websocket-Extensions",
			"Content-Length":
			continue
		default:
			for _, v := range vv {
				h.Add(k, v)
			}
		}
	}
	return h
}

func addForwardedHeaders(h http.Header, r *http.Request) {
	h.Set("X-Forwarded-Host", r.Host)
	h.Set("X-Forwarded-Proto", schemeFromRequest(r))

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		host = r.RemoteAddr
	}
	h.Set("X-Forwarded-For", host)
}

func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "wss"
	}
	return "ws"
}

func filterUpgradeResponseHeaders(src http.Header) http.Header {
	dst := http.Header{}

	for k, vv := range src {
		switch http.CanonicalHeaderKey(k) {
		case "Connection",
			"Upgrade",
			"Sec-Websocket-Accept",
			"Content-Length":
			continue
		default:
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
	return dst
}

func dialWithWait(ctx context.Context, target string, dialer *websocket.Dialer, headers http.Header, timeout time.Duration) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, resp, err := dialer.DialContext(ctx, target, headers)
		if err == nil {
			return conn, resp, nil
		}

		select {
		case <-ctx.Done():
			return nil, resp, fmt.Errorf("could not connect to %s after %v: %w", target, timeout, err)
		case <-ticker.C:
			continue
		}
	}
}
