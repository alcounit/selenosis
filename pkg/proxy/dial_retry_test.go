package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func refusedErr() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
}

func okResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("ok")),
		Header:     make(http.Header),
	}
}

func upstreamModifier(r *http.Request) {
	r.URL.Scheme = "http"
	r.URL.Host = "upstream.test"
}

func bodyModifier(body []byte) RequestModifier {
	return func(r *http.Request) {
		upstreamModifier(r)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		r.ContentLength = int64(len(body))
	}
}

func TestDialRetrySucceedsAfterRefusals(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return nil, refusedErr()
		}
		return okResponse(), nil
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 5 * time.Second}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rw := httptest.NewRecorder()
	rp.ServeHTTP(rw, req)

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
}

func TestDialRetryKeepsTryingWithinBudget(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 500 * time.Millisecond, Interval: 10 * time.Millisecond}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got < 5 {
		t.Fatalf("expected the budget to allow several attempts, got %d", got)
	}
}

func TestDialRetryIntervalDefaultsWhenUnset(t *testing.T) {
	rp := NewHTTPReverseProxy(WithHTTPDialRetry(DialRetry{Timeout: time.Minute}))

	rt, ok := rp.rp.Transport.(*dialRetryTransport)
	if !ok {
		t.Fatalf("expected the transport to be wrapped, got %T", rp.rp.Transport)
	}
	if rt.interval != DefaultDialRetryInterval {
		t.Fatalf("interval = %v, want %v", rt.interval, DefaultDialRetryInterval)
	}
}

func TestDialRetryIntervalOverride(t *testing.T) {
	rp := NewHTTPReverseProxy(
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute, Interval: 25 * time.Millisecond}),
	)

	rt, ok := rp.rp.Transport.(*dialRetryTransport)
	if !ok {
		t.Fatalf("expected the transport to be wrapped, got %T", rp.rp.Transport)
	}
	if rt.interval != 25*time.Millisecond {
		t.Fatalf("interval = %v, want 25ms", rt.interval)
	}
}

func TestDialRetryIntervalFallsBackOnNonPositive(t *testing.T) {
	rp := NewHTTPReverseProxy(
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute, Interval: 0}),
	)

	rt, ok := rp.rp.Transport.(*dialRetryTransport)
	if !ok {
		t.Fatalf("expected the transport to be wrapped, got %T", rp.rp.Transport)
	}
	if rt.interval != DefaultDialRetryInterval {
		t.Fatalf("interval = %v, want %v", rt.interval, DefaultDialRetryInterval)
	}
}

func TestDialRetryIntervalWithoutTimeoutLeavesTransportUnwrapped(t *testing.T) {
	rp := NewHTTPReverseProxy(WithHTTPDialRetry(DialRetry{Interval: 25 * time.Millisecond}))

	if _, ok := rp.rp.Transport.(*dialRetryTransport); ok {
		t.Fatal("expected no retry wrapper when the budget is not set")
	}
}

func TestDialRetryStopsAtDeadline(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 300 * time.Millisecond}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rw := httptest.NewRecorder()

	start := time.Now()
	rp.ServeHTTP(rw, req)
	elapsed := time.Since(start)

	if got := atomic.LoadInt32(&attempts); got < 2 {
		t.Fatalf("expected the dial to be retried, got %d attempts", got)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("deadline did not stop the loop: took %v", elapsed)
	}
}

func TestDialRetryIgnoresOtherErrors(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("boom")
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected no retry for a non-refused error, got %d attempts", got)
	}
}

func TestDialRetryStopsOnCancelledContext(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 10 * time.Second}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(ctx)

	start := time.Now()
	rp.ServeHTTP(httptest.NewRecorder(), req)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("cancelled context did not stop the loop: took %v", elapsed)
	}
	if got := atomic.LoadInt32(&attempts); got > 2 {
		t.Fatalf("expected at most 2 attempts on a cancelled context, got %d", got)
	}
}

func TestDialRetryReplaysRequestBody(t *testing.T) {
	payload := []byte(`{"capabilities":{}}`)

	var attempts int32
	var seen []string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		seen = append(seen, string(body))

		if atomic.AddInt32(&attempts, 1) < 3 {
			return nil, refusedErr()
		}
		return okResponse(), nil
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(bodyModifier(payload)),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 5 * time.Second}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/session", bytes.NewReader(payload))
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if len(seen) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(seen))
	}
	for i, got := range seen {
		if got != string(payload) {
			t.Fatalf("attempt %d proxied body %q, want %q", i+1, got, payload)
		}
	}
}

func TestDialRetrySkippedWhenBodyIsNotReplayable(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	modifier := func(r *http.Request) {
		upstreamModifier(r)
		r.Body = io.NopCloser(bytes.NewBufferString("payload"))
		r.GetBody = nil
		r.ContentLength = int64(len("payload"))
	}

	rp := NewHTTPReverseProxy(
		WithRequestModifier(modifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/session", bytes.NewBufferString("payload"))
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected no retry without GetBody, got %d attempts", got)
	}
}

func TestWithoutDialRetryDialsOnce(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	rp := NewHTTPReverseProxy(WithRequestModifier(upstreamModifier), WithTransport(rt))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected exactly 1 attempt without the option, got %d", got)
	}
}

func TestDialRetryHonoursTransportSetAfterOption(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) < 2 {
			return nil, refusedErr()
		}
		return okResponse(), nil
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithHTTPDialRetry(DialRetry{Timeout: 5 * time.Second}),
		WithTransport(rt),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rw := httptest.NewRecorder()
	rp.ServeHTTP(rw, req)

	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("expected the option to wrap a transport set after it, got %d attempts", got)
	}
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
}

func TestIsDialError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"refused", refusedErr(), true},
		{"host unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}, true},
		{"network unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH}, true},
		{"dial timeout", &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}, true},
		{"wrapped dial", fmt.Errorf("proxy: %w", refusedErr()), true},
		{"read reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, false},
		{"read timeout", &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}, false},
		{"bare errno", syscall.ECONNREFUSED, false},
		{"eof", io.EOF, false},
		{"plain", errors.New("boom"), false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDialError(tt.err); got != tt.want {
				t.Fatalf("isDialError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDialRetryRetriesUnreachableAndTimeout(t *testing.T) {
	errs := []error{
		&net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH},
		&net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded},
	}

	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if int(n) <= len(errs) {
			return nil, errs[n-1]
		}
		return okResponse(), nil
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 5 * time.Second, Interval: time.Millisecond}),
	)

	rw := httptest.NewRecorder()
	rp.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "http://example.com/", nil))

	if got := atomic.LoadInt32(&attempts); got != int32(len(errs))+1 {
		t.Fatalf("expected %d attempts, got %d", len(errs)+1, got)
	}
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
}

func TestDialRetrySkippedForNonDialErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"eof", io.EOF},
		{"read reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return nil, tt.err
			})

			rp := NewHTTPReverseProxy(
				WithRequestModifier(upstreamModifier),
				WithTransport(rt),
				WithHTTPDialRetry(DialRetry{Timeout: 5 * time.Second, Interval: time.Millisecond}),
			)

			rw := httptest.NewRecorder()
			rp.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "http://example.com/", nil))

			if got := atomic.LoadInt32(&attempts); got != 1 {
				t.Fatalf("expected a single attempt, got %d", got)
			}
		})
	}
}

func TestReplayable(t *testing.T) {
	withBody := httptest.NewRequest(http.MethodPost, "http://example.com/", bytes.NewBufferString("x"))
	withBody.GetBody = nil

	replayableBody := httptest.NewRequest(http.MethodPost, "http://example.com/", bytes.NewBufferString("x"))
	replayableBody.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewBufferString("x")), nil
	}

	noBody := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	noBody.Body = nil

	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"nil body", noBody, true},
		{"body with GetBody", replayableBody, true},
		{"body without GetBody", withBody, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replayable(tt.req); got != tt.want {
				t.Fatalf("replayable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDialRetryReturnsErrorWhenGetBodyFails(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	modifier := func(r *http.Request) {
		upstreamModifier(r)
		r.Body = io.NopCloser(bytes.NewBufferString("payload"))
		r.GetBody = func() (io.ReadCloser, error) {
			return nil, errors.New("rewind failed")
		}
		r.ContentLength = int64(len("payload"))
	}

	rp := NewHTTPReverseProxy(
		WithRequestModifier(modifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/session", bytes.NewBufferString("payload"))
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected the loop to stop when GetBody fails, got %d attempts", got)
	}
}

func BenchmarkHTTPReverseProxy(b *testing.B) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return okResponse(), nil
	})
	rp := NewHTTPReverseProxy(WithRequestModifier(upstreamModifier), WithTransport(rt))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rp.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkHTTPReverseProxyWithHTTPDialRetry(b *testing.B) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return okResponse(), nil
	})
	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute}),
	)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rp.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func TestDialRetryStopsWhenRetryHitsAnotherError(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			return nil, refusedErr()
		}
		return nil, errors.New("boom")
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("expected the loop to stop on a non-refused error, got %d attempts", got)
	}
}

func TestDialRetryMaxIntervalWiring(t *testing.T) {
	rp := NewHTTPReverseProxy(
		WithHTTPDialRetry(DialRetry{Timeout: time.Minute, Interval: 50 * time.Millisecond, MaxInterval: 500 * time.Millisecond}),
	)

	rt, ok := rp.rp.Transport.(*dialRetryTransport)
	if !ok {
		t.Fatalf("expected the transport to be wrapped, got %T", rp.rp.Transport)
	}
	if rt.interval != 50*time.Millisecond {
		t.Fatalf("interval = %v, want 50ms", rt.interval)
	}
	if rt.maxInterval != 500*time.Millisecond {
		t.Fatalf("maxInterval = %v, want 500ms", rt.maxInterval)
	}
}

func TestDialRetryBackoffSlowsDownRetries(t *testing.T) {
	count := func(opts ...HTTPReverseProxyOptions) int32 {
		var attempts int32
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attempts, 1)
			return nil, refusedErr()
		})

		opts = append(opts, WithRequestModifier(upstreamModifier), WithTransport(rt))
		rp := NewHTTPReverseProxy(opts...)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		rp.ServeHTTP(httptest.NewRecorder(), req)

		return atomic.LoadInt32(&attempts)
	}

	flat := count(
		WithHTTPDialRetry(DialRetry{Timeout: 400 * time.Millisecond, Interval: 10 * time.Millisecond}),
	)
	backoff := count(
		WithHTTPDialRetry(DialRetry{Timeout: 400 * time.Millisecond, Interval: 10 * time.Millisecond, MaxInterval: 160 * time.Millisecond}),
	)

	if backoff >= flat {
		t.Fatalf("expected backoff to make fewer attempts than a flat interval, got %d vs %d", backoff, flat)
	}
}

func TestDialRetryMaxIntervalBelowIntervalStaysFlat(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, refusedErr()
	})

	rp := NewHTTPReverseProxy(
		WithRequestModifier(upstreamModifier),
		WithTransport(rt),
		WithHTTPDialRetry(DialRetry{Timeout: 300 * time.Millisecond, Interval: 20 * time.Millisecond, MaxInterval: 5 * time.Millisecond}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rp.ServeHTTP(httptest.NewRecorder(), req)

	if got := atomic.LoadInt32(&attempts); got < 5 {
		t.Fatalf("expected a flat interval to keep retrying, got %d attempts", got)
	}
}
