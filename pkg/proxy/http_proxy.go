package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"time"
)

var transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	TLSHandshakeTimeout: 10 * time.Second,
}

type RequestModifier func(*http.Request)

type ResponseModifier func(*http.Response) error

type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

type HTTPReverseProxy struct {
	rp *httputil.ReverseProxy
}

type HTTPReverseProxyOptions func(*HTTPReverseProxy)

func WithTransport(transport http.RoundTripper) HTTPReverseProxyOptions {
	return HTTPReverseProxyOptions(func(p *HTTPReverseProxy) {
		p.rp.Transport = transport
	})
}

func WithRequestModifier(modifier RequestModifier) HTTPReverseProxyOptions {
	return HTTPReverseProxyOptions(func(p *HTTPReverseProxy) {
		p.rp.Director = modifier
	})
}

func WithResponseModifier(modifier func(*http.Response) error) HTTPReverseProxyOptions {
	return HTTPReverseProxyOptions(func(p *HTTPReverseProxy) {
		p.rp.ModifyResponse = modifier
	})
}

func WithErrorHandler(errHandler ErrorHandler) HTTPReverseProxyOptions {
	return HTTPReverseProxyOptions(func(p *HTTPReverseProxy) {
		p.rp.ErrorHandler = errHandler
	})
}

func NewHTTPReverseProxy(opts ...HTTPReverseProxyOptions) *HTTPReverseProxy {
	proxy := HTTPReverseProxy{
		rp: &httputil.ReverseProxy{
			FlushInterval: time.Millisecond * 200,
			BufferPool:    bufPool,
			Transport:     transport,
		},
	}

	for _, opt := range opts {
		opt(&proxy)
	}

	return &proxy
}

func (p *HTTPReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	if p.rp.Director == nil {
		p.rp.Director = func(r *http.Request) {
			r.URL.Scheme = req.URL.Scheme
			r.URL.Host = req.URL.Host
			r.URL.Path = req.URL.Path
		}
	}

	p.rp.ServeHTTP(rw, req)
}
