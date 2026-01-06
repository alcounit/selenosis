package selenium

import (
	"net/url"
	"strings"
)

type Payload map[string]any

type Value map[string]any

func (s Payload) UpdateSessionId(sessionId string) bool {
	if _, ok := s["sessionId"].(string); !ok {
		value, ok := s["value"]
		if ok {
			valueMap, ok := value.(map[string]any)
			if ok {
				_, ok = valueMap["sessionId"].(string)
				if ok {
					s["value"].(map[string]any)["sessionId"] = sessionId
					return true
				}
			}
		}
	}
	return false
}

const empty string = ""

func (s Payload) GetSessionId() (string, bool) {
	if sessionId, ok := s["sessionId"].(string); ok {
		return sessionId, true
	}

	if value, ok := s["value"]; ok {
		if vm, ok := value.(map[string]any); ok {
			if sessionId, ok := vm["sessionId"].(string); ok {
				return sessionId, true
			}
		}
	}

	return empty, false
}

func UpdateBiDiURL(scheme, host, oldSessionId, newSessionId string, payload Payload) {
	updateCapsPropURL("webSocketUrl", scheme, host, oldSessionId, newSessionId, payload)
}

func UpdateChromeCDPURL(scheme, host, oldSessionId, newSessionId string, payload Payload) {
	updateCapsPropURL("se:cdp", scheme, host, oldSessionId, newSessionId, payload)
}

func updateCapsPropURL(propName, scheme, host, oldSessionId, newSessionId string, payload Payload) {
	rawValue, ok := payload["value"]
	if !ok {
		return
	}

	value, ok := rawValue.(map[string]any)
	if !ok {
		return
	}

	rawCaps, ok := value["capabilities"]
	if !ok {
		return
	}

	caps, ok := rawCaps.(map[string]any)
	if !ok {
		return
	}

	rawWSURL, ok := caps[propName]
	if !ok {
		return
	}

	wsURLStr, ok := rawWSURL.(string)
	if !ok || wsURLStr == "" {
		return
	}

	parsedURL, err := url.Parse(wsURLStr)
	if err != nil {
		return
	}

	parsedURL.Scheme = scheme
	parsedURL.Host = host
	parsedURL.Path = strings.Replace(parsedURL.Path, oldSessionId, newSessionId, 1)

	caps[propName] = parsedURL.String()
}
