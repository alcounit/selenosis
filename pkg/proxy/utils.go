package proxy

import "net/http"

func IsWebSocketRequest(r *http.Request) bool {
	return r.Header.Get("Connection") == "Upgrade" &&
		r.Header.Get("Upgrade") == "websocket"
}
