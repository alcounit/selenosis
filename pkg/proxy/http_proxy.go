package proxy

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	logctx "github.com/alcounit/browser-controller/pkg/log"
)

var DefaultTransport http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	IdleConnTimeout:     30 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

const DefaultDialRetryInterval = 100 * time.Millisecond

type dialRetryTransport struct {
	base        http.RoundTripper
	timeout     time.Duration
	interval    time.Duration
	maxInterval time.Duration
}

func isDialError(err error) bool {
	var oe *net.OpError
	return errors.As(err, &oe) && oe.Op == "dial"
}

func replayable(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

func (t *dialRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil || !isDialError(err) || !replayable(req) {
		return resp, err
	}

	log := logctx.FromContext(req.Context())
	start := time.Now()
	deadline := start.Add(t.timeout)
	ctx := req.Context()

	delay := t.interval
	timer := time.NewTimer(delay)
	defer timer.Stop()

	attempt := 1

	giveUp := func(reason string) (*http.Response, error) {
		log.Warn().Err(err).
			Str("reason", reason).
			Int("attempts", attempt).
			Dur("elapsed", time.Since(start)).
			Dur("timeout", t.timeout).
			Dur("interval", t.interval).
			Dur("maxInterval", t.maxInterval).
			Str("host", req.URL.Host).
			Msg("upstream dial keeps failing, giving up")
		return resp, err
	}

	for ; ; attempt++ {
		if !time.Now().Before(deadline) {
			return giveUp("deadline")
		}

		select {
		case <-ctx.Done():
			return giveUp("client-cancelled")
		case <-timer.C:
		}

		if t.maxInterval > delay {
			delay = min(delay*2, t.maxInterval)
		}
		timer.Reset(delay)

		retry := req.Clone(ctx)
		if req.GetBody != nil {
			body, berr := req.GetBody()
			if berr != nil {
				return resp, err
			}
			retry.Body = body
		}

		resp, err = t.base.RoundTrip(retry)
		if err == nil {
			log.Info().
				Int("attempts", attempt+1).
				Dur("elapsed", time.Since(start)).
				Dur("timeout", t.timeout).
				Dur("interval", t.interval).
				Dur("maxInterval", t.maxInterval).
				Str("host", req.URL.Host).
				Msg("upstream reachable after dial retry")
			return resp, nil
		}

		if !isDialError(err) {
			return resp, err
		}
	}
}

type RequestModifier func(*http.Request)

type ResponseModifier func(*http.Response) error

type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

type HTTPReverseProxy struct {
	rp              *httputil.ReverseProxy
	requestModifier RequestModifier

	dialRetry DialRetry
}

type HTTPReverseProxyOptions func(*HTTPReverseProxy)

func WithTransport(transport http.RoundTripper) HTTPReverseProxyOptions {
	return func(p *HTTPReverseProxy) {
		p.rp.Transport = transport
	}
}

func WithRequestModifier(modifier RequestModifier) HTTPReverseProxyOptions {
	return func(p *HTTPReverseProxy) {
		p.requestModifier = modifier
	}
}

func WithResponseModifier(modifier ResponseModifier) HTTPReverseProxyOptions {
	return func(p *HTTPReverseProxy) {
		p.rp.ModifyResponse = modifier
	}
}

func WithErrorHandler(errHandler ErrorHandler) HTTPReverseProxyOptions {
	return func(p *HTTPReverseProxy) {
		p.rp.ErrorHandler = errHandler
	}
}

type DialRetry struct {
	Timeout     time.Duration
	Interval    time.Duration
	MaxInterval time.Duration
}

func WithHTTPDialRetry(retry DialRetry) HTTPReverseProxyOptions {
	return func(p *HTTPReverseProxy) {
		p.dialRetry = retry
	}
}

func NewHTTPReverseProxy(opts ...HTTPReverseProxyOptions) *HTTPReverseProxy {
	proxy := HTTPReverseProxy{
		rp: &httputil.ReverseProxy{
			FlushInterval: time.Millisecond * 200,
			BufferPool:    bufPool,
			Transport:     DefaultTransport,
		},
	}

	for _, opt := range opts {
		opt(&proxy)
	}

	if proxy.dialRetry.Timeout > 0 {
		interval := proxy.dialRetry.Interval
		if interval <= 0 {
			interval = DefaultDialRetryInterval
		}

		proxy.rp.Transport = &dialRetryTransport{
			base:        proxy.rp.Transport,
			timeout:     proxy.dialRetry.Timeout,
			interval:    interval,
			maxInterval: proxy.dialRetry.MaxInterval,
		}
	}

	proxy.rp.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetXForwarded()

		if proxy.requestModifier != nil {
			proxy.requestModifier(pr.Out)
			return
		}

		pr.Out.URL.Scheme = pr.In.URL.Scheme
		pr.Out.URL.Host = pr.In.URL.Host
		pr.Out.URL.Path = pr.In.URL.Path
	}

	return &proxy
}

func (p *HTTPReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.rp.ServeHTTP(rw, req)
}
