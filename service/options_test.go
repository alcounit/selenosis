package service

import (
	"net/url"
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
