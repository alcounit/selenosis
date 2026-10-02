package devtools

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

const (
	webSocketDebuggerUrl = "webSocketDebuggerUrl"
	devtoolsFrontendUrl  = "devtoolsFrontendUrl"

	wsParam  = "ws"
	wssParam = "wss"

	PathPrefix          = "/devtools/session"
	browserSocketPrefix = "/devtools/browser/"

	secureScheme  = "https"
	wsScheme      = "ws"
	wssScheme     = "wss"
	VersionPath   = "/json/version"
	listPath      = "/json/list"
	shortListPath = "/json"
	newTargetPath = "/json/new"
	pathSeparator = "/"
)

func SessionPath(sessionId, rest string) string {
	return path.Join(PathPrefix, sessionId, rest)
}

func IsBrowserSocket(browserPath string) bool {
	return browserPath == pathSeparator || strings.HasPrefix(browserPath, browserSocketPrefix)
}

func RewritesBody(browserPath string) bool {
	switch browserPath {
	case VersionPath, listPath, shortListPath, newTargetPath:
		return true
	default:
		return false
	}
}

func Rewrite(body []byte, external *url.URL, sessionId string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	switch value := payload.(type) {
	case map[string]any:
		RewriteWSRequestUrls(value, external, sessionId)
	case []any:
		for _, item := range value {
			if target, ok := item.(map[string]any); ok {
				RewriteWSRequestUrls(target, external, sessionId)
			}
		}
	default:
		return body, nil
	}

	return json.Marshal(payload)
}

func RewriteWSRequestUrls(target map[string]any, external *url.URL, sessionId string) {
	if raw, ok := target[webSocketDebuggerUrl].(string); ok && raw != "" {
		if rewritten, ok := RewriteWebSocketURL(raw, external, sessionId); ok {
			target[webSocketDebuggerUrl] = rewritten
		}
	}

	if raw, ok := target[devtoolsFrontendUrl].(string); ok && raw != "" {
		if rewritten, ok := RewriteFrontendURL(raw, external, sessionId); ok {
			target[devtoolsFrontendUrl] = rewritten
		}
	}
}

func RewriteWebSocketURL(raw string, external *url.URL, sessionId string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	parsed.Scheme = socketScheme(external)
	parsed.Host = external.Host
	parsed.Path = SessionPath(sessionId, parsed.Path)

	return parsed.String(), true
}

func RewriteFrontendURL(raw string, external *url.URL, sessionId string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	query := parsed.Query()

	value := query.Get(wsParam)
	if value == "" {
		value = query.Get(wssParam)
	}

	if value == "" {
		return "", false
	}

	query.Del(wsParam)
	query.Del(wssParam)
	query.Set(socketParam(external), rewriteFrontendSocket(value, external, sessionId))

	parsed.RawQuery = query.Encode()

	if parsed.Host == "" {
		parsed.Path = SessionPath(sessionId, parsed.Path)
	}

	return parsed.String(), true
}

func rewriteFrontendSocket(value string, external *url.URL, sessionId string) string {
	rest := ""
	if index := strings.Index(value, pathSeparator); index >= 0 {
		rest = value[index:]
	}

	return external.Host + SessionPath(sessionId, rest)
}

func socketScheme(external *url.URL) string {
	if external.Scheme == secureScheme {
		return wssScheme
	}

	return wsScheme
}

func socketParam(external *url.URL) string {
	if external.Scheme == secureScheme {
		return wssParam
	}

	return wsParam
}
