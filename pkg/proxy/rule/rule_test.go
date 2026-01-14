package rule

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

func TestRuleMatch(t *testing.T) {
	r := Rule{PathRegex: "^/foo/(?P<bar>.*)$"}
	r.re = regexp.MustCompile(r.PathRegex)
	if !r.RuleMatch("/foo/123") {
		t.Error("expected match for /foo/123")
	}
	if r.RuleMatch("/bar/123") {
		t.Error("expected no match for /bar/123")
	}
}

func TestSafeRewriteMatchFound(t *testing.T) {
	r := Rule{
		PathRegex:   "^/foo/(?P<bar>.*)$",
		RewritePath: "/baz/{bar}",
	}
	r.re = regexp.MustCompile(r.PathRegex)
	out := SafeRewrite(r, "/foo/hello/world")
	exp := "/baz/hello/world"
	if out != exp {
		t.Errorf("expected %s, got %s", exp, out)
	}
}

func TestSafeRewriteNoMatch(t *testing.T) {
	r := Rule{PathRegex: "^/foo/(.*)$"}
	r.re = regexp.MustCompile(r.PathRegex)
	original := "/bar/123"
	out := SafeRewrite(r, original)
	if out != original {
		t.Errorf("expected unchanged original, got %s", out)
	}
}

func TestSafeRewritePathCleaningAndEscaping(t *testing.T) {
	r := Rule{
		PathRegex:   "^/foo/(?P<bar>.*)$",
		RewritePath: "/baz/{bar}",
	}
	r.re = regexp.MustCompile(r.PathRegex)
	out := SafeRewrite(r, "/foo/../evil path")
	exp := "/baz/../evil%20path"
	if out != exp {
		t.Errorf("expected %s, got %s", exp, out)
	}
}

func TestLoadRulesFromEnvSuccess(t *testing.T) {
	rules := []Rule{
		{PathRegex: "^/foo/(.*)$", Target: "localhost:8080"},
	}
	data, _ := json.Marshal(rules)
	os.Setenv("ROUTING_RULES", string(data))
	loaded, err := LoadRulesFromEnv("ROUTING_RULES")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(loaded))
	}
	if loaded[0].Target != "localhost:8080" {
		t.Errorf("expected target localhost:8080, got %s", loaded[0].Target)
	}
	if loaded[0].re == nil {
		t.Error("expected compiled regexp not nil")
	}
	if !loaded[0].re.MatchString("/foo/bar") {
		t.Error("compiled regexp does not match /foo/bar")
	}
}

func TestLoadRulesFromEnvEmptyEnv(t *testing.T) {
	os.Unsetenv("ROUTING_RULES")
	loaded, err := LoadRulesFromEnv("ROUTING_RULES")
	if err != nil {
		t.Fatalf("unexpected error for empty env var: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty rules, got %d", len(loaded))
	}
}

func TestLoadRulesFromEnvInvalidJSON(t *testing.T) {
	os.Setenv("ROUTING_RULES", "{invalid json")
	_, err := LoadRulesFromEnv("ROUTING_RULES")
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestLoadRulesFromEnvInvalidRegex(t *testing.T) {
	rules := []Rule{
		{PathRegex: "^(foo", Target: "localhost:8080"},
	}
	data, _ := json.Marshal(rules)
	os.Setenv("ROUTING_RULES", string(data))
	_, err := LoadRulesFromEnv("ROUTING_RULES")
	if err == nil {
		t.Fatal("expected regexp compile error")
	}
}
