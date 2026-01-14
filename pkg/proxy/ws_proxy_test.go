package proxy

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBufferPoolNew(t *testing.T) {
	pool := NewBufferPool()
	buf := pool.Get()
	if len(buf) != 8192 {
		t.Fatalf("expected default buffer size 8192, got %d", len(buf))
	}
	pool.Put(buf)
	if got := pool.Get(); len(got) != 8192 {
		t.Fatalf("expected buffer size 8192, got %d", len(got))
	}
}

func TestIsWebSocketRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	if IsWebSocketRequest(req) {
		t.Fatal("expected false for missing upgrade headers")
	}

	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	if !IsWebSocketRequest(req) {
		t.Fatal("expected true for websocket upgrade headers")
	}
}

func TestCloneHeadersAndForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Test", "a")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Host = "example.com"
	req.RemoteAddr = "1.2.3.4:1234"

	cloned := cloneHeaders(req)
	if cloned.Get("X-Test") != "a" {
		t.Fatalf("expected X-Test header, got %q", cloned.Get("X-Test"))
	}
	if cloned.Get("Connection") != "" {
		t.Fatal("expected Connection to be filtered")
	}

	addForwardedHeaders(cloned, req)
	if cloned.Get("X-Forwarded-Host") != "example.com" {
		t.Fatalf("unexpected X-Forwarded-Host: %s", cloned.Get("X-Forwarded-Host"))
	}
	if cloned.Get("X-Forwarded-Proto") != "ws" {
		t.Fatalf("unexpected X-Forwarded-Proto: %s", cloned.Get("X-Forwarded-Proto"))
	}
	if cloned.Get("X-Forwarded-For") != "1.2.3.4:1234" {
		t.Fatalf("unexpected X-Forwarded-For: %s", cloned.Get("X-Forwarded-For"))
	}
}

func TestSchemeFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	if schemeFromRequest(req) != "ws" {
		t.Fatalf("expected ws scheme")
	}

	req.TLS = &tls.ConnectionState{}
	if schemeFromRequest(req) != "wss" {
		t.Fatalf("expected wss scheme")
	}
}

func TestFilterUpgradeResponseHeaders(t *testing.T) {
	src := http.Header{
		"Connection": {"Upgrade"},
		"Upgrade":    {"websocket"},
		"X-Test":     {"a"},
	}
	dst := filterUpgradeResponseHeaders(src)
	if dst.Get("X-Test") != "a" {
		t.Fatalf("expected X-Test header, got %q", dst.Get("X-Test"))
	}
	if dst.Get("Connection") != "" {
		t.Fatal("expected Connection to be filtered")
	}
}

func TestWSProxyServeHTTPNilResolver(t *testing.T) {
	p := &WSProxy{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := httptest.NewRecorder()

	p.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rw.Code)
	}
}

func TestWSProxyServeHTTPResolveError(t *testing.T) {
	p := NewWebSocketReverseProxy(func(r *http.Request) (*url.URL, error) {
		return nil, errors.New("boom")
	})
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := httptest.NewRecorder()

	p.ServeHTTP(rw, req)

	if rw.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rw.Code)
	}
}

func TestWSProxyServeHTTPDialError(t *testing.T) {
	p := NewWebSocketReverseProxy(func(r *http.Request) (*url.URL, error) {
		return url.Parse("ws://example.com")
	})
	p.Dialer.Proxy = func(r *http.Request) (*url.URL, error) {
		return nil, errors.New("proxy err")
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := httptest.NewRecorder()

	p.ServeHTTP(rw, req)

	if rw.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rw.Code)
	}
}

func TestWSProxyServeHTTPSuccess(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })

	upClient, upServer := net.Pipe()
	t.Cleanup(func() { _ = upClient.Close() })
	t.Cleanup(func() { _ = upServer.Close() })

	resolver := func(r *http.Request) (*url.URL, error) {
		return url.Parse("ws://upstream.test")
	}

	var onConnect, onMessage, onClose int
	p := NewWebSocketReverseProxy(
		resolver,
		WithOnConnect(func() { onConnect++ }),
		WithOnMessage(func() { onMessage++ }),
		WithOnClose(func() { onClose++ }),
	)
	p.Dialer.NetDial = func(network, addr string) (net.Conn, error) {
		return upClient, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		req.RemoteAddr = "1.2.3.4:5678"
		rw := &hijackResponseWriter{conn: serverConn, header: make(http.Header)}
		p.ServeHTTP(rw, req)
	}()

	upstreamDone := make(chan struct{})
	go func() {
		defer close(upstreamDone)
		if err := writeHandshakeResponse(upServer); err != nil {
			return
		}
		payload, _, err := readFramePayload(upServer)
		if err != nil {
			return
		}
		if string(payload) != "ping" {
			return
		}
		_ = writeFrame(upServer, 0x2, []byte("pong"))
	}()

	ready := make(chan struct{})
	go func() {
		defer close(ready)
		_ = readHTTPResponse(clientConn)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for handshake response")
	}

	if err := writeMaskedFrame(clientConn, 0x2, []byte("ping")); err != nil {
		t.Fatalf("failed to write client frame: %v", err)
	}

	payload, _, err := readFramePayload(clientConn)
	if err != nil {
		t.Fatalf("failed to read client frame: %v", err)
	}
	if string(payload) != "pong" {
		t.Fatalf("unexpected client payload: %s", string(payload))
	}

	_ = clientConn.Close()
	_ = upServer.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ServeHTTP to exit")
	}

	if onConnect == 0 || onMessage == 0 || onClose == 0 {
		t.Fatalf("expected callbacks, got connect=%d message=%d close=%d", onConnect, onMessage, onClose)
	}

	<-upstreamDone
}

type hijackResponseWriter struct {
	conn   net.Conn
	header http.Header
}

func (h *hijackResponseWriter) Header() http.Header {
	return h.header
}

func (h *hijackResponseWriter) Write(p []byte) (int, error) {
	return h.conn.Write(p)
}

func (h *hijackResponseWriter) WriteHeader(statusCode int) {}

func (h *hijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}

func writeHandshakeResponse(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	var key string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
			key = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	if key == "" {
		return errors.New("missing websocket key")
	}

	accept := computeAcceptKey(key)
	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	_, err := conn.Write([]byte(resp))
	return err
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func readHTTPResponse(r io.Reader) error {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" {
			return nil
		}
	}
}

func writeMaskedFrame(w io.Writer, opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return errors.New("payload too large")
	}
	header := []byte{0x80 | opcode, 0x80 | byte(len(payload))}
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(mask); err != nil {
		return err
	}
	_, err := w.Write(masked)
	return err
}

func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return errors.New("payload too large")
	}
	header := []byte{0x80 | opcode, byte(len(payload))}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFramePayload(r io.Reader) ([]byte, byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, 0, err
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)
	if length > 125 {
		return nil, 0, errors.New("unsupported length")
	}
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return nil, 0, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, 0, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return payload, opcode, nil
}
