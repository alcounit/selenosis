package proxy

import (
	"net/http"
	"net/textproto"
	"strings"
)

func IsWebSocketRequest(r *http.Request) bool {
	return headerContainsToken(r.Header, "Connection", "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func headerContainsToken(h http.Header, key, token string) bool {
	for _, v := range h[textproto.CanonicalMIMEHeaderKey(key)] {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}
