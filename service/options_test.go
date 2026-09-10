package service

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseSelenosisOptionsSuccess(t *testing.T) {
	q := url.Values{
		"labels.env":                    {"dev", "prod"},
		"containers.browser.env.DEBUG":  {"0", " 1 "},
		"containers.browser.env.LOG_LV": {"info"},
	}

	opts, err := parseSelenosisOptions(q, defaultParseLimits())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	labels, ok := opts["labels"].(map[string]string)
	if !ok {
		t.Fatalf("expected labels map, got %#v", opts["labels"])
	}
	if labels["env"] != "prod" {
		t.Fatalf("expected last label value, got %q", labels["env"])
	}

	containers, ok := opts["containers"].(map[string]any)
	if !ok {
		t.Fatalf("expected containers map, got %#v", opts["containers"])
	}
	browser, ok := containers["browser"].(map[string]any)
	if !ok {
		t.Fatalf("expected browser config, got %#v", containers["browser"])
	}
	env, ok := browser["env"].(map[string]string)
	if !ok {
		t.Fatalf("expected env map, got %#v", browser["env"])
	}
	if env["DEBUG"] != "1" {
		t.Fatalf("expected trimmed env value, got %q", env["DEBUG"])
	}
}

func TestParseSelenosisOptionsIgnoresUnknownShape(t *testing.T) {
	q := url.Values{
		"":                             {"x"},
		"justkey":                      {"x"},
		"labels.a.b":                   {"x"},
		"containers.browser.env":       {"x"},
		"containers.browser.bad.DEBUG": {"x"},
		"other.foo":                    {"x"},
	}

	opts, err := parseSelenosisOptions(q, defaultParseLimits())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected empty opts, got %#v", opts)
	}
}

func TestParseSelenosisOptionsErrors(t *testing.T) {
	tests := []struct {
		name   string
		q      url.Values
		limits parseLimits
		want   string
	}{
		{
			name: "value too long",
			q:    url.Values{"labels.env": {"abcd"}},
			limits: parseLimits{
				MaxValueLen: 3,
			},
			want: "value too long",
		},
		{
			name:   "invalid label key",
			q:      url.Values{"labels.bad!": {"x"}},
			limits: defaultParseLimits(),
			want:   "invalid label key",
		},
		{
			name:   "too many labels",
			q:      url.Values{"labels.a": {"1"}, "labels.b": {"2"}},
			limits: parseLimits{MaxLabels: 1, MaxValueLen: 100},
			want:   "too many labels",
		},
		{
			name:   "invalid container name",
			q:      url.Values{"containers.Bad.env.DEBUG": {"1"}},
			limits: defaultParseLimits(),
			want:   "invalid container name",
		},
		{
			name:   "invalid env name",
			q:      url.Values{"containers.browser.env.bad": {"1"}},
			limits: defaultParseLimits(),
			want:   "invalid env name",
		},
		{
			name:   "too many containers",
			q:      url.Values{"containers.a.env.A": {"1"}, "containers.b.env.B": {"2"}},
			limits: parseLimits{MaxContainers: 1, MaxEnvPerCont: 10, MaxValueLen: 100},
			want:   "too many containers",
		},
		{
			name:   "too many env vars",
			q:      url.Values{"containers.browser.env.A": {"1"}, "containers.browser.env.B": {"2"}},
			limits: parseLimits{MaxContainers: 10, MaxEnvPerCont: 1, MaxValueLen: 100},
			want:   "too many env vars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSelenosisOptions(tt.q, tt.limits)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestDropSelenosisOptions(t *testing.T) {
	tests := []struct {
		name string
		q    url.Values
		want url.Values
	}{
		{
			name: "empty",
			q:    url.Values{},
			want: url.Values{},
		},
		{
			name: "drops labels and containers",
			q: url.Values{
				"labels.env":                   {"test"},
				"containers.browser.env.DEBUG": {"1"},
			},
			want: url.Values{},
		},
		{
			name: "keeps browser options",
			q: url.Values{
				"headless": {"false"},
				"timeout":  {"30000"},
				"devtools": {"true"},
			},
			want: url.Values{
				"headless": {"false"},
				"timeout":  {"30000"},
				"devtools": {"true"},
			},
		},
		{
			name: "keeps unrelated dotted keys",
			q: url.Values{
				"labelsx.env":  {"test"},
				"container.a":  {"1"},
				"other.labels": {"1"},
			},
			want: url.Values{
				"labelsx.env":  {"test"},
				"container.a":  {"1"},
				"other.labels": {"1"},
			},
		},
		{
			name: "keeps bare prefixes",
			q: url.Values{
				"labels":     {"x"},
				"containers": {"x"},
			},
			want: url.Values{
				"labels":     {"x"},
				"containers": {"x"},
			},
		},
		{
			name: "drops deep selenosis keys",
			q: url.Values{
				"labels.a.b":                   {"x"},
				"containers.browser.bad.DEBUG": {"x"},
				"headless":                     {"false"},
			},
			want: url.Values{
				"headless": {"false"},
			},
		},
		{
			name: "preserves repeated values",
			q: url.Values{
				"args":       {"--no-sandbox", "--disable-gpu"},
				"labels.env": {"a", "b"},
			},
			want: url.Values{
				"args": {"--no-sandbox", "--disable-gpu"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropSelenosisOptions(tt.q)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestDropMcpOptions(t *testing.T) {
	tests := []struct {
		name string
		q    url.Values
		want string
	}{
		{
			name: "empty",
			q:    url.Values{},
			want: "",
		},
		{
			name: "drops routing and selenosis params",
			q: url.Values{
				"browser":                      {"playwright-mcp"},
				"version":                      {"0.0.75"},
				"labels.team":                  {"qa"},
				"containers.browser.env.DEBUG": {"1"},
			},
			want: "",
		},
		{
			name: "keeps client params",
			q: url.Values{
				"browser": {"playwright-mcp"},
				"version": {"0.0.75"},
				"foo":     {"bar"},
			},
			want: "foo=bar",
		},
		{
			name: "keeps repeated values",
			q: url.Values{
				"browser": {"playwright-mcp"},
				"arg":     {"a", "b"},
			},
			want: "arg=a&arg=b",
		},
		{
			name: "keeps similar keys",
			q: url.Values{
				"browserName": {"chrome"},
				"versions":    {"1"},
			},
			want: "browserName=chrome&versions=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dropMcpOptions(tt.q); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDropMcpOptionsNilValues(t *testing.T) {
	if got := dropMcpOptions(nil); got != "" {
		t.Fatalf("expected empty encoding, got %q", got)
	}
}

func BenchmarkPlaywrightUpstreamQuery(b *testing.B) {
	raw := "headless=false&timeout=30000&args=--no-sandbox&args=--disable-gpu&labels.env=test&containers.browser.env.DEBUG=1"
	uuid := "11111111-2222-3333-4444-555555555555"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		q, _ := url.ParseQuery(raw)
		q = dropSelenosisOptions(q)
		q.Set("ipuuid", uuid)
		_ = q.Encode()
	}
}

func BenchmarkMcpUpstreamQuery(b *testing.B) {
	raw := "browser=playwright-mcp&version=0.0.75&labels.team=qa&containers.browser.env.DEBUG=1"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		q, _ := url.ParseQuery(raw)
		_ = dropMcpOptions(q)
	}
}

func TestDropSelenosisOptionsNilValues(t *testing.T) {
	got := dropSelenosisOptions(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty values, got %#v", got)
	}
	if got.Encode() != "" {
		t.Fatalf("expected empty encoding, got %q", got.Encode())
	}
}

func TestParseSelenosisOptionsEmptyValueSlice(t *testing.T) {
	q := url.Values{}
	q["labels.env"] = []string{} // key present, but empty slice

	opts, err := parseSelenosisOptions(q, defaultParseLimits())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	labels, ok := opts["labels"].(map[string]string)
	if !ok {
		t.Fatalf("expected labels map, got %#v", opts["labels"])
	}
	if v := labels["env"]; v != "" {
		t.Fatalf("expected empty string value, got %q", v)
	}
}
