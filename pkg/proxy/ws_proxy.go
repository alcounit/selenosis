package proxy

import (
	"net/http"
	"net/url"
	"sync"
	"time"

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

type WSProxy struct {
	Upgrader websocket.Upgrader
	Dialer   websocket.Dialer
	Resolve  TargetResolver

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
	if p.Resolve == nil {
		http.Error(w, "resolver not configured", http.StatusInternalServerError)
		return
	}

	targetURL, err := p.Resolve(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	upstreamHeaders := cloneHeaders(r)
	addForwardedHeaders(upstreamHeaders, r)
	upstreamHeaders.Set("Host", targetURL.Host)
	upstreamHeaders.Del("Origin")

	upstreamConn, resp, err := p.Dialer.Dial(targetURL.String(), upstreamHeaders)
	if err != nil {
		http.Error(w, "upstream dial failed", http.StatusBadGateway)
		return
	}

	upgradeHeaders := filterUpgradeResponseHeaders(resp.Header)

	if proto := resp.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		upgradeHeaders.Set("Sec-WebSocket-Protocol", proto)
	}

	clientConn, err := p.Upgrader.Upgrade(w, r, upgradeHeaders)
	if err != nil {
		http.Error(w, "Upgrade client connection failed", http.StatusInternalServerError)
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
	h.Set("X-Forwarded-For", r.RemoteAddr)
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
