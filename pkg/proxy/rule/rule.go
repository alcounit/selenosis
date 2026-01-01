package rule

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

type Rule struct {
	PathRegex   string `json:"pathRegex"`
	Target      string `json:"target"`
	RewritePath string `json:"rewritePath,omitempty"`
	re          *regexp.Regexp
}

func (r Rule) RuleMatch(requestPath string) bool {
	return r.re.MatchString(requestPath)
}

func (r Rule) IsEmpty() bool {
	return r.PathRegex == "" &&
		r.Target == "" &&
		r.RewritePath == "" &&
		r.re == nil
}

func SafeRewrite(rule Rule, original string) string {
	match := rule.re.FindStringSubmatch(original)
	if match == nil {
		return original
	}

	result := rule.RewritePath
	names := rule.re.SubexpNames()

	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}

		clean := path.Clean(match[i])
		cleanEscaped := url.PathEscape(clean)
		cleanEscaped = strings.ReplaceAll(cleanEscaped, "%2F", "/")

		result = strings.ReplaceAll(result, "{"+name+"}", cleanEscaped)
	}

	return result
}

func LoadRulesFromEnv(envVarName string) ([]Rule, error) {
	env := os.Getenv(envVarName)
	if env == "" {
		return nil, fmt.Errorf("%s env var is empty", envVarName)
	}

	var rules []Rule
	if err := json.Unmarshal([]byte(env), &rules); err != nil {
		return nil, fmt.Errorf("cannot parse JSON from %s: %w", envVarName, err)
	}

	for i, r := range rules {
		compiled, err := regexp.Compile(r.PathRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in rule %d: %w", i, err)
		}
		rules[i].re = compiled
	}

	return rules, nil
}
