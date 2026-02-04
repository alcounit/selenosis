package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	browserv1 "github.com/alcounit/browser-controller/apis/browser/v1"
)

var (
	reContainer = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	reEnvName   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	reLabelKey  = regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)
)

type parseLimits struct {
	MaxLabels     int
	MaxContainers int
	MaxEnvPerCont int
	MaxValueLen   int
}

func defaultParseLimits() parseLimits {
	return parseLimits{
		MaxLabels:     64,
		MaxContainers: 16,
		MaxEnvPerCont: 64,
		MaxValueLen:   512,
	}
}

func parseSelenosisOptions(q url.Values, limits parseLimits) (map[string]any, error) {
	var (
		labels     map[string]string
		containers map[string]map[string]map[string]string
	)

	last := func(vs []string) string {
		if len(vs) == 0 {
			return ""
		}
		return vs[len(vs)-1]
	}

	for key, vals := range q {
		if key == "" {
			continue
		}

		val := strings.TrimSpace(last(vals))
		if limits.MaxValueLen > 0 && len(val) > limits.MaxValueLen {
			return nil, fmt.Errorf("value too long for key %q (%d)", key, len(val))
		}

		parts := strings.Split(key, ".")
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {

		case "labels":
			if len(parts) != 2 {
				continue
			}

			k := strings.TrimSpace(parts[1])
			if k == "" || !reLabelKey.MatchString(k) {
				return nil, fmt.Errorf("invalid label key %q", k)
			}

			if labels == nil {
				labels = make(map[string]string)
			}

			labels[k] = val
			if limits.MaxLabels > 0 && len(labels) > limits.MaxLabels {
				return nil, fmt.Errorf("too many labels (>%d)", limits.MaxLabels)
			}

		case "containers":
			if len(parts) != 4 || parts[2] != "env" {
				continue
			}

			container := strings.TrimSpace(parts[1])
			if container == "" || !reContainer.MatchString(container) {
				return nil, fmt.Errorf("invalid container name %q", container)
			}

			envName := strings.TrimSpace(parts[3])
			if envName == "" || !reEnvName.MatchString(envName) {
				return nil, fmt.Errorf("invalid env name %q for container %q", envName, container)
			}

			if containers == nil {
				containers = make(map[string]map[string]map[string]string)
			}
			if _, ok := containers[container]; !ok {
				if limits.MaxContainers > 0 && len(containers) >= limits.MaxContainers {
					return nil, fmt.Errorf("too many containers (>%d)", limits.MaxContainers)
				}
				containers[container] = map[string]map[string]string{
					"env": {},
				}
			}

			envMap := containers[container]["env"]
			envMap[envName] = val

			if limits.MaxEnvPerCont > 0 && len(envMap) > limits.MaxEnvPerCont {
				return nil, fmt.Errorf(
					"too many env vars for container %q (>%d)",
					container,
					limits.MaxEnvPerCont,
				)
			}
		}
	}

	out := make(map[string]any)

	if len(labels) > 0 {
		out["labels"] = labels
	}

	if len(containers) > 0 {
		containersOut := map[string]any{}
		for name, cfg := range containers {
			if len(cfg["env"]) > 0 {
				containersOut[name] = map[string]any{
					"env": cfg["env"],
				}
			}
		}
		if len(containersOut) > 0 {
			out["containers"] = containersOut
		}
	}

	return out, nil
}

func setSelenosisOptions(ann map[string]string, opts map[string]any) (map[string]string, error) {
	if len(opts) == 0 {
		return ann, nil
	}

	b, err := json.Marshal(opts)
	if err != nil {
		return ann, fmt.Errorf("marshal selenosis options: %w", err)
	}

	if ann == nil {
		ann = map[string]string{}
	}

	ann[browserv1.SelenosisOptionsAnnotationKey] = string(b)
	return ann, nil
}
