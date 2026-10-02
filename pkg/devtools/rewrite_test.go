package devtools

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const sessionId = "00000000-0000-0000-0000-ffff7f000001"

func external(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse external url: %v", err)
	}

	return parsed
}

func rewriteObject(t *testing.T, body string, base *url.URL) map[string]any {
	t.Helper()

	rewritten, err := Rewrite([]byte(body), base, sessionId)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(rewritten, &result); err != nil {
		t.Fatalf("failed to decode rewritten body: %v", err)
	}

	return result
}

func TestRewriteVersion(t *testing.T) {
	body := `{"Browser":"Chrome/140.0","Protocol-Version":"1.3",
		"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/0f3c-guid"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	want := "ws://selenosis.example.com/devtools/session/" + sessionId + "/devtools/browser/0f3c-guid"
	if got := result["webSocketDebuggerUrl"]; got != want {
		t.Fatalf("webSocketDebuggerUrl = %v, want %v", got, want)
	}
	if got := result["Browser"]; got != "Chrome/140.0" {
		t.Fatalf("unrelated field was changed: %v", got)
	}
}

func TestRewriteVersionOverTLS(t *testing.T) {
	body := `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/0f3c-guid"}`

	result := rewriteObject(t, body, external(t, "https://selenosis.example.com"))

	want := "wss://selenosis.example.com/devtools/session/" + sessionId + "/devtools/browser/0f3c-guid"
	if got := result["webSocketDebuggerUrl"]; got != want {
		t.Fatalf("webSocketDebuggerUrl = %v, want %v", got, want)
	}
}

func TestRewriteListRewritesEveryTarget(t *testing.T) {
	body := `[
		{"id":"AB12","type":"page","url":"https://example.com",
		 "faviconUrl":"https://example.com/favicon.ico",
		 "webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/page/AB12"},
		{"id":"CD34","type":"worker",
		 "webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/page/CD34"}
	]`

	rewritten, err := Rewrite([]byte(body), external(t, "http://selenosis.example.com"), sessionId)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	var targets []map[string]any
	if err := json.Unmarshal(rewritten, &targets); err != nil {
		t.Fatalf("failed to decode rewritten body: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	for _, id := range []string{"AB12", "CD34"} {
		var found bool
		for _, target := range targets {
			if target["id"] != id {
				continue
			}
			found = true
			want := "ws://selenosis.example.com/devtools/session/" + sessionId + "/devtools/page/" + id
			if got := target["webSocketDebuggerUrl"]; got != want {
				t.Fatalf("target %s: webSocketDebuggerUrl = %v, want %v", id, got, want)
			}
		}
		if !found {
			t.Fatalf("target %s is missing from the rewritten body", id)
		}
	}

	if got := targets[0]["faviconUrl"]; got != "https://example.com/favicon.ico" {
		t.Fatalf("faviconUrl must be left alone, got %v", got)
	}
	if got := targets[0]["url"]; got != "https://example.com" {
		t.Fatalf("url must be left alone, got %v", got)
	}
}

func TestRewriteRelativeFrontendURL(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"/devtools/inspector.html?ws=127.0.0.1:9222/devtools/page/AB12"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	raw, ok := result["devtoolsFrontendUrl"].(string)
	if !ok {
		t.Fatalf("devtoolsFrontendUrl is missing: %#v", result)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse rewritten frontend url: %v", err)
	}

	wantPath := "/devtools/session/" + sessionId + "/devtools/inspector.html"
	if parsed.Path != wantPath {
		t.Fatalf("frontend path = %q, want %q", parsed.Path, wantPath)
	}

	wantSocket := "selenosis.example.com/devtools/session/" + sessionId + "/devtools/page/AB12"
	if got := parsed.Query().Get("ws"); got != wantSocket {
		t.Fatalf("ws param = %q, want %q", got, wantSocket)
	}
}

func TestRewriteAbsoluteFrontendURLKeepsItsHost(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"https://chrome-devtools-frontend.appspot.com/serve_rev/@abc/inspector.html?ws=127.0.0.1:9222/devtools/page/AB12"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	raw := result["devtoolsFrontendUrl"].(string)

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse rewritten frontend url: %v", err)
	}

	if parsed.Host != "chrome-devtools-frontend.appspot.com" {
		t.Fatalf("frontend host must not change, got %q", parsed.Host)
	}
	if parsed.Path != "/serve_rev/@abc/inspector.html" {
		t.Fatalf("frontend path must not change, got %q", parsed.Path)
	}

	wantSocket := "selenosis.example.com/devtools/session/" + sessionId + "/devtools/page/AB12"
	if got := parsed.Query().Get("ws"); got != wantSocket {
		t.Fatalf("ws param = %q, want %q", got, wantSocket)
	}
}

func TestRewriteFrontendURLRenamesParamOverTLS(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"/devtools/inspector.html?ws=127.0.0.1:9222/devtools/page/AB12"}`

	result := rewriteObject(t, body, external(t, "https://selenosis.example.com"))

	parsed, err := url.Parse(result["devtoolsFrontendUrl"].(string))
	if err != nil {
		t.Fatalf("failed to parse rewritten frontend url: %v", err)
	}

	query := parsed.Query()
	if _, ok := query["ws"]; ok {
		t.Fatal("expected the ws param to be renamed to wss over TLS")
	}

	wantSocket := "selenosis.example.com/devtools/session/" + sessionId + "/devtools/page/AB12"
	if got := query.Get("wss"); got != wantSocket {
		t.Fatalf("wss param = %q, want %q", got, wantSocket)
	}
}

func TestRewriteFrontendURLAcceptsWSSParam(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"/devtools/inspector.html?wss=127.0.0.1:9222/devtools/page/AB12"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	parsed, err := url.Parse(result["devtoolsFrontendUrl"].(string))
	if err != nil {
		t.Fatalf("failed to parse rewritten frontend url: %v", err)
	}

	wantSocket := "selenosis.example.com/devtools/session/" + sessionId + "/devtools/page/AB12"
	if got := parsed.Query().Get("ws"); got != wantSocket {
		t.Fatalf("ws param = %q, want %q", got, wantSocket)
	}
}

func TestRewriteKeepsUnknownFields(t *testing.T) {
	body := `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/guid",
		"someFutureField":"keep me","nested":{"deep":["a","b"]},"count":42,"big":10000000000}`

	rewritten, err := Rewrite([]byte(body), external(t, "http://selenosis.example.com"), sessionId)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	raw := string(rewritten)
	for _, fragment := range []string{`"someFutureField":"keep me"`, `"deep":["a","b"]`, `"count":42`, `"big":10000000000`} {
		if !strings.Contains(raw, fragment) {
			t.Fatalf("expected %s to survive, got %s", fragment, raw)
		}
	}
}

func TestRewriteWithoutAddressFields(t *testing.T) {
	body := `{"Browser":"Chrome/140.0"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	if got := result["Browser"]; got != "Chrome/140.0" {
		t.Fatalf("unexpected body: %#v", result)
	}
}

func TestRewriteFrontendURLWithoutSocketParam(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"/devtools/inspector.html?panel=console"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	if got := result["devtoolsFrontendUrl"]; got != "/devtools/inspector.html?panel=console" {
		t.Fatalf("expected the frontend url to be left alone, got %v", got)
	}
}

func TestRewriteFrontendSocketWithoutPath(t *testing.T) {
	body := `{"devtoolsFrontendUrl":"/devtools/inspector.html?ws=127.0.0.1:9222"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	parsed, err := url.Parse(result["devtoolsFrontendUrl"].(string))
	if err != nil {
		t.Fatalf("failed to parse rewritten frontend url: %v", err)
	}

	want := "selenosis.example.com/devtools/session/" + sessionId
	if got := parsed.Query().Get("ws"); got != want {
		t.Fatalf("ws param = %q, want %q", got, want)
	}
}

func TestRewriteScalarBodyIsUntouched(t *testing.T) {
	body := `"just a string"`

	rewritten, err := Rewrite([]byte(body), external(t, "http://selenosis.example.com"), sessionId)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	if string(rewritten) != body {
		t.Fatalf("expected the body to pass through, got %s", rewritten)
	}
}

func TestRewriteMalformedJSONFails(t *testing.T) {
	if _, err := Rewrite([]byte("{"), external(t, "http://selenosis.example.com"), sessionId); err == nil {
		t.Fatal("expected an error on malformed json")
	}
}

func TestRewriteIgnoresNonStringFields(t *testing.T) {
	body := `{"webSocketDebuggerUrl":42,"devtoolsFrontendUrl":null}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	if got := result["webSocketDebuggerUrl"]; got != float64(42) {
		t.Fatalf("expected the non-string value to survive, got %#v", got)
	}
	if got, ok := result["devtoolsFrontendUrl"]; !ok || got != nil {
		t.Fatalf("expected null to survive, got %#v", got)
	}
}

func TestRewriteUnparseableAddressesAreLeftAlone(t *testing.T) {
	body := `{"webSocketDebuggerUrl":"ws://[::1/devtools/browser/guid",
		"devtoolsFrontendUrl":"http://[::1/inspector.html?ws=127.0.0.1:9222/devtools/page/AB12"}`

	result := rewriteObject(t, body, external(t, "http://selenosis.example.com"))

	if got := result["webSocketDebuggerUrl"]; got != "ws://[::1/devtools/browser/guid" {
		t.Fatalf("expected the unparseable socket url to survive, got %v", got)
	}
	if got := result["devtoolsFrontendUrl"]; got != "http://[::1/inspector.html?ws=127.0.0.1:9222/devtools/page/AB12" {
		t.Fatalf("expected the unparseable frontend url to survive, got %v", got)
	}
}

func TestSessionPath(t *testing.T) {
	if got := SessionPath(sessionId, "/devtools/page/AB12"); got != "/devtools/session/"+sessionId+"/devtools/page/AB12" {
		t.Fatalf("unexpected session path: %q", got)
	}
	if got := SessionPath(sessionId, ""); got != "/devtools/session/"+sessionId {
		t.Fatalf("unexpected session path: %q", got)
	}
}

func TestIsBrowserSocket(t *testing.T) {
	browser := []string{"/devtools/browser/0f3c-guid", "/devtools/browser/x", "/"}
	for _, browserPath := range browser {
		if !IsBrowserSocket(browserPath) {
			t.Fatalf("expected %s to be the browser socket", browserPath)
		}
	}

	other := []string{"/devtools/page/AB12", "/devtools/browser", "/json/version", ""}
	for _, browserPath := range other {
		if IsBrowserSocket(browserPath) {
			t.Fatalf("expected %s not to be the browser socket", browserPath)
		}
	}
}

func TestRewritesBody(t *testing.T) {
	rewritten := []string{"/json/version", "/json/list", "/json", "/json/new"}
	for _, browserPath := range rewritten {
		if !RewritesBody(browserPath) {
			t.Fatalf("expected %s to be rewritten", browserPath)
		}
	}

	untouched := []string{"/json/protocol", "/json/activate/AB12", "/json/close/AB12", "/devtools/inspector.html", "/"}
	for _, browserPath := range untouched {
		if RewritesBody(browserPath) {
			t.Fatalf("expected %s to be left alone", browserPath)
		}
	}
}
