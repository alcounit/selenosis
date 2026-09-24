package service

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	browserv1 "github.com/alcounit/browser-controller/apis/browser/v1"
)

func TestParseSelenosisOptionsSuccess(t *testing.T) {
	q := url.Values{
		"labels.env":                    {"dev", "prod"},
		"annotations.startedManually":   {"true"},
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

	annotations, ok := opts["annotations"].(map[string]string)
	if !ok {
		t.Fatalf("expected annotations map, got %#v", opts["annotations"])
	}
	if annotations["startedManually"] != "true" {
		t.Fatalf("expected annotation value, got %q", annotations["startedManually"])
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
		"annotations.a.b":              {"x"},
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
			name:   "invalid annotation key",
			q:      url.Values{"annotations.bad!": {"x"}},
			limits: defaultParseLimits(),
			want:   "invalid annotation key",
		},
		{
			name:   "too many annotations",
			q:      url.Values{"annotations.a": {"1"}, "annotations.b": {"2"}},
			limits: parseLimits{MaxAnnotations: 1, MaxValueLen: 100},
			want:   "too many annotations",
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
			name: "drops labels, annotations and containers",
			q: url.Values{
				"labels.env":                   {"test"},
				"annotations.startedManually":  {"true"},
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

func newTemplate() *browserv1.Browser {
	return &browserv1.Browser{}
}

func TestParseSelenosisOptionsMap(t *testing.T) {
	opts, err := parseSelenosisOptionsMap(map[string]any{
		"labels":      map[string]any{"team": "qa"},
		"annotations": map[string]any{"startedManually": "true"},
		"containers":  map[string]any{"browser": map[string]any{"env": map[string]any{"LOG_LEVEL": "debug"}}},
		"unknown":     map[string]any{"ignored": true},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if opts.Labels["team"] != "qa" {
		t.Fatalf("labels = %#v", opts.Labels)
	}
	if opts.Annotations["startedManually"] != "true" {
		t.Fatalf("annotations = %#v", opts.Annotations)
	}
	if opts.Containers["browser"].Env["LOG_LEVEL"] != "debug" {
		t.Fatalf("containers = %#v", opts.Containers)
	}
}

func TestParseSelenosisOptionsMapEmpty(t *testing.T) {
	opts, err := parseSelenosisOptionsMap(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if opts.Labels != nil || opts.Annotations != nil || opts.Containers != nil {
		t.Fatalf("expected zero options, got %#v", opts)
	}
}

func TestParseSelenosisOptionsMapInvalid(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]any
	}{
		{name: "labels not an object", opts: map[string]any{"labels": "nope"}},
		{name: "label value not a string", opts: map[string]any{"labels": map[string]any{"team": 1}}},
		{name: "annotations not an object", opts: map[string]any{"annotations": "nope"}},
		{name: "annotation value not a string", opts: map[string]any{"annotations": map[string]any{"a": true}}},
		{name: "containers not an object", opts: map[string]any{"containers": "nope"}},
		{name: "env value not a string", opts: map[string]any{
			"containers": map[string]any{"browser": map[string]any{"env": map[string]any{"A": 1}}},
		}},
		{name: "unmarshalable input", opts: map[string]any{"labels": func() {}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSelenosisOptionsMap(tt.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSetSelenosisLabels(t *testing.T) {
	template := newTemplate()

	setSelenosisLabels(template, map[string]string{"team": "qa", "env": "test"})

	if template.ObjectMeta.Labels["team"] != "qa" || template.ObjectMeta.Labels["env"] != "test" {
		t.Fatalf("labels = %#v", template.ObjectMeta.Labels)
	}
}

func TestSetSelenosisLabelsKeepsExisting(t *testing.T) {
	template := &browserv1.Browser{}
	template.ObjectMeta.Labels = map[string]string{"keep": "me"}

	setSelenosisLabels(template, map[string]string{"team": "qa"})

	if template.ObjectMeta.Labels["keep"] != "me" || template.ObjectMeta.Labels["team"] != "qa" {
		t.Fatalf("labels = %#v", template.ObjectMeta.Labels)
	}
}

func TestSetSelenosisLabelsNoop(t *testing.T) {
	template := newTemplate()

	setSelenosisLabels(template, nil)

	if template.ObjectMeta.Labels != nil {
		t.Fatalf("expected no labels, got %#v", template.ObjectMeta.Labels)
	}
}

func TestSetSelenosisAnnotations(t *testing.T) {
	template := newTemplate()

	setSelenosisAnnotations(template, map[string]string{"startedManually": "true", "team": "qa"})

	if template.ObjectMeta.Annotations["startedManually"] != "true" || template.ObjectMeta.Annotations["team"] != "qa" {
		t.Fatalf("annotations = %#v", template.ObjectMeta.Annotations)
	}
}

func TestSetSelenosisAnnotationsKeepsExisting(t *testing.T) {
	template := &browserv1.Browser{}
	template.ObjectMeta.Annotations = map[string]string{"keep": "me"}

	setSelenosisAnnotations(template, map[string]string{"startedManually": "true"})

	if template.ObjectMeta.Annotations["keep"] != "me" || template.ObjectMeta.Annotations["startedManually"] != "true" {
		t.Fatalf("annotations = %#v", template.ObjectMeta.Annotations)
	}
}

func TestSetSelenosisAnnotationsNoop(t *testing.T) {
	template := newTemplate()

	setSelenosisAnnotations(template, nil)

	if template.ObjectMeta.Annotations != nil {
		t.Fatalf("expected no annotations, got %#v", template.ObjectMeta.Annotations)
	}
}

func TestSetSelenosisOptionsKeepsOnlyContainers(t *testing.T) {
	template := newTemplate()
	setSelenosisAnnotations(template, map[string]string{"startedManually": "true"})
	setSelenosisLabels(template, map[string]string{"team": "qa"})

	setSelenosisOptions(template, map[string]containerOption{
		"browser": {Env: map[string]string{"LOG_LEVEL": "debug"}},
	})

	raw := template.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey]
	if raw == "" {
		t.Fatalf("expected %s to be set", browserv1.SelenosisOptionsAnnotationKey)
	}

	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("failed to decode stored options: %v", err)
	}

	if _, ok := stored["labels"]; ok {
		t.Fatalf("labels must not be stored in %s: %s", browserv1.SelenosisOptionsAnnotationKey, raw)
	}
	if _, ok := stored["annotations"]; ok {
		t.Fatalf("annotations must not be stored in %s: %s", browserv1.SelenosisOptionsAnnotationKey, raw)
	}
	if _, ok := stored["containers"]; !ok {
		t.Fatalf("expected containers to be stored, got %s", raw)
	}
}

func TestSetSelenosisOptionsNoContainers(t *testing.T) {
	template := newTemplate()

	setSelenosisOptions(template, nil)

	if _, ok := template.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey]; ok {
		t.Fatalf("expected no %s annotation, got %#v", browserv1.SelenosisOptionsAnnotationKey, template.ObjectMeta.Annotations)
	}
}

func TestSetSelenosisOptionsClientCannotOverrideOptionsKey(t *testing.T) {
	template := newTemplate()

	setSelenosisAnnotations(template, map[string]string{browserv1.SelenosisOptionsAnnotationKey: "hijacked"})
	setSelenosisOptions(template, map[string]containerOption{
		"browser": {Env: map[string]string{"A": "1"}},
	})

	if template.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey] == "hijacked" {
		t.Fatal("client must not be able to override the selenosis options annotation")
	}
}
