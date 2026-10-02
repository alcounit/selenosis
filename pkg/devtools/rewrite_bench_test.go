package devtools

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func benchmarkExternal(b *testing.B) *url.URL {
	b.Helper()

	parsed, err := url.Parse("https://selenosis.example.com")
	if err != nil {
		b.Fatalf("failed to parse external url: %v", err)
	}
	return parsed
}

func BenchmarkRewriteVersion(b *testing.B) {
	body := []byte(`{"Browser":"Chrome/151.0.7922.109","Protocol-Version":"1.3","User-Agent":"Mozilla/5.0 (X11; Linux x86_64) Chrome/151.0","V8-Version":"15.1.206.16","WebKit-Version":"537.36","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/7e55898b-6b24-412b-9962-5d2846654221"}`)
	external := benchmarkExternal(b)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Rewrite(body, external, sessionId); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRewriteList(b *testing.B) {
	targets := make([]string, 0, 20)
	for i := range 20 {
		targets = append(targets, fmt.Sprintf(`{"description":"","devtoolsFrontendUrl":"/devtools/inspector.html?ws=127.0.0.1:9222/devtools/page/T%02d","id":"T%02d","title":"page","type":"page","url":"https://example.com/%d","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/page/T%02d"}`, i, i, i, i))
	}
	body := []byte("[" + strings.Join(targets, ",") + "]")
	external := benchmarkExternal(b)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Rewrite(body, external, sessionId); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSessionPath(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = SessionPath(sessionId, "/devtools/browser/7e55898b-6b24-412b-9962-5d2846654221")
	}
}
