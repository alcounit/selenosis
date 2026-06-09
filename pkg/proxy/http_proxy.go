package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"time"
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

type RequestModifier func(*http.Request)

type ResponseModifier func(*http.Response) error

type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

type HTTPReverseProxy struct {
	rp              *httputil.ReverseProxy
	requestModifier RequestModifier
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
