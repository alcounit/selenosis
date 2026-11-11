package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newWSPipe(tb testing.TB, upstream func(*websocket.Conn), opts ...WSProxyOption) *websocket.Conn {
	tb.Helper()

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := websocket.Upgrader{}
		c, err := u.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		upstream(c)
	}))
	tb.Cleanup(up.Close)

	target, err := url.Parse("ws" + strings.TrimPrefix(up.URL, "http"))
	if err != nil {
		tb.Fatal(err)
	}

	p := NewWebSocketReverseProxy(func(*http.Request) (*url.URL, error) { return target, nil }, opts...)
	p.Dialer.Proxy = nil
	front := httptest.NewServer(p)
	tb.Cleanup(front.Close)

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(front.URL, "http"), nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { c.Close() })
	return c
}

func echoUpstream(c *websocket.Conn) {
	for {
		mt, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		if err := c.WriteMessage(mt, data); err != nil {
			return
		}
	}
}

func assertCloseError(t *testing.T, err error, code int, text string) {
	t.Helper()
	var ce *websocket.CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("expected close error, got %v", err)
	}
	if ce.Code != code || ce.Text != text {
		t.Fatalf("close = %d %q, want %d %q", ce.Code, ce.Text, code, text)
	}
}

func TestPipeWebSocketForwardsMessagesIntact(t *testing.T) {
	var messages atomic.Int32
	client := newWSPipe(t, echoUpstream, WithOnMessage(func() { messages.Add(1) }))

	cases := []struct {
		msgType int
		data    []byte
	}{
		{websocket.TextMessage, []byte("hello")},
		{websocket.BinaryMessage, bytes.Repeat([]byte{0xAB}, 5<<20)},
	}

	for _, tc := range cases {
		if err := client.WriteMessage(tc.msgType, tc.data); err != nil {
			t.Fatal(err)
		}
		mt, data, err := client.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt != tc.msgType {
			t.Fatalf("message type = %d, want %d", mt, tc.msgType)
		}
		if !bytes.Equal(data, tc.data) {
			t.Fatalf("payload mismatch: got %d bytes, want %d", len(data), len(tc.data))
		}
	}

	if got := messages.Load(); got != 4 {
		t.Fatalf("onMessage calls = %d, want 4", got)
	}
}

func TestPipeWebSocketForwardsUpstreamClose(t *testing.T) {
	deadline := func() time.Time { return time.Now().Add(time.Second) }

	tests := []struct {
		name     string
		close    func(*websocket.Conn)
		wantCode int
		wantText string
	}{
		{
			name: "custom code",
			close: func(c *websocket.Conn) {
				_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4000, "custom"), deadline())
			},
			wantCode: 4000,
			wantText: "custom",
		},
		{
			name: "no status",
			close: func(c *websocket.Conn) {
				_ = c.WriteControl(websocket.CloseMessage, []byte{}, deadline())
			},
			wantCode: websocket.CloseNoStatusReceived,
		},
		{
			name: "connection dropped",
			close: func(c *websocket.Conn) {
				_ = c.UnderlyingConn().Close()
			},
			wantCode: websocket.CloseInternalServerErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newWSPipe(t, func(c *websocket.Conn) {
				tt.close(c)
				_, _, _ = c.ReadMessage()
			})

			_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _, err := client.ReadMessage()
			assertCloseError(t, err, tt.wantCode, tt.wantText)
		})
	}
}

func TestPipeWebSocketForwardsClientClose(t *testing.T) {
	var closes atomic.Int32
	got := make(chan error, 1)

	client := newWSPipe(t, func(c *websocket.Conn) {
		_, _, err := c.ReadMessage()
		got <- err
	}, WithOnClose(func() { closes.Add(1) }))

	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
	if err := client.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-got:
		assertCloseError(t, err, websocket.CloseNormalClosure, "bye")
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive close")
	}

	if n := closes.Load(); n != 1 {
		t.Fatalf("onClose calls = %d, want 1", n)
	}
}

func TestPipeWebSocketUpstreamDropsMidMessage(t *testing.T) {
	client := newWSPipe(t, func(c *websocket.Conn) {
		w, err := c.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return
		}
		_, _ = w.Write(bytes.Repeat([]byte{1}, 64<<10))
		_ = c.UnderlyingConn().Close()
	})

	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err := client.ReadMessage()
	assertCloseError(t, err, websocket.CloseInternalServerErr, "")
}

func TestCopyMessageAfterCloseSent(t *testing.T) {
	got := make(chan error, 1)
	newWSPipe(t, func(c *websocket.Conn) {
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		got <- copyMessage(c, websocket.TextMessage, strings.NewReader("late"))
	})

	select {
	case err := <-got:
		if !errors.Is(err, websocket.ErrCloseSent) {
			t.Fatalf("expected ErrCloseSent, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copyMessage did not return")
	}
}

func TestCloseStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantText string
	}{
		{"custom code", &websocket.CloseError{Code: 4000, Text: "custom"}, 4000, "custom"},
		{"normal", &websocket.CloseError{Code: websocket.CloseNormalClosure}, websocket.CloseNormalClosure, ""},
		{"no status", &websocket.CloseError{Code: websocket.CloseNoStatusReceived}, websocket.CloseNoStatusReceived, ""},
		{"abnormal", &websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "unexpected EOF"}, websocket.CloseInternalServerErr, ""},
		{"tls handshake", &websocket.CloseError{Code: websocket.CloseTLSHandshake}, websocket.CloseInternalServerErr, ""},
		{"wrapped", fmt.Errorf("read: %w", &websocket.CloseError{Code: 4001, Text: "x"}), 4001, "x"},
		{"plain error", io.EOF, websocket.CloseInternalServerErr, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, text := closeStatus(tt.err)
			if code != tt.wantCode || text != tt.wantText {
				t.Fatalf("closeStatus = %d %q, want %d %q", code, text, tt.wantCode, tt.wantText)
			}
		})
	}
}

func BenchmarkWSPipe(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1 << 10},
		{"64KB", 64 << 10},
		{"4MB", 4 << 20},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			client := newWSPipe(b, echoUpstream)
			payload := bytes.Repeat([]byte{1}, s.size)

			b.SetBytes(int64(s.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := client.WriteMessage(websocket.BinaryMessage, payload); err != nil {
					b.Fatal(err)
				}
				if _, _, err := client.ReadMessage(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
