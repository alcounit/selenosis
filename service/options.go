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
	MaxLabels      int
	MaxAnnotations int
	MaxContainers  int
	MaxEnvPerCont  int
	MaxValueLen    int
}

func defaultParseLimits() parseLimits {
	return parseLimits{
		MaxLabels:      64,
		MaxAnnotations: 64,
		MaxContainers:  16,
		MaxEnvPerCont:  64,
		MaxValueLen:    512,
	}
}

func parseSelenosisOptions(q url.Values, limits parseLimits) (map[string]any, error) {
	var (
		labels      map[string]string
		annotations map[string]string
		containers  map[string]map[string]map[string]string
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

		case "annotations":
			if len(parts) != 2 {
				continue
			}

			k := strings.TrimSpace(parts[1])
			if k == "" || !reLabelKey.MatchString(k) {
				return nil, fmt.Errorf("invalid annotation key %q", k)
			}

			if annotations == nil {
				annotations = make(map[string]string)
			}

			annotations[k] = val
			if limits.MaxAnnotations > 0 && len(annotations) > limits.MaxAnnotations {
				return nil, fmt.Errorf("too many annotations (>%d)", limits.MaxAnnotations)
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

	if len(annotations) > 0 {
		out["annotations"] = annotations
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

type containerOption struct {
	Env map[string]string `json:"env,omitempty"`
}

type selenosisOptions struct {
	Labels      map[string]string          `json:"labels,omitempty"`
	Annotations map[string]string          `json:"annotations,omitempty"`
	Containers  map[string]containerOption `json:"containers,omitempty"`
}

func parseSelenosisOptionsMap(opts map[string]any) (selenosisOptions, error) {
	var parsed selenosisOptions

	if len(opts) == 0 {
		return parsed, nil
	}

	b, err := json.Marshal(opts)
	if err != nil {
		return parsed, fmt.Errorf("marshal selenosis options: %w", err)
	}

	if err := json.Unmarshal(b, &parsed); err != nil {
		return parsed, fmt.Errorf("invalid selenosis options: %w", err)
	}

	return parsed, nil
}

func setSelenosisLabels(template *browserv1.Browser, labels map[string]string) {
	for k, v := range labels {
		if template.ObjectMeta.Labels == nil {
			template.ObjectMeta.Labels = map[string]string{}
		}
		template.ObjectMeta.Labels[k] = v
	}
}

func setSelenosisAnnotations(template *browserv1.Browser, annotations map[string]string) {
	for k, v := range annotations {
		if template.ObjectMeta.Annotations == nil {
			template.ObjectMeta.Annotations = map[string]string{}
		}
		template.ObjectMeta.Annotations[k] = v
	}
}

func setSelenosisOptions(template *browserv1.Browser, containers map[string]containerOption) {
	if len(containers) == 0 {
		return
	}

	b, _ := json.Marshal(selenosisOptions{Containers: containers})

	if template.ObjectMeta.Annotations == nil {
		template.ObjectMeta.Annotations = map[string]string{}
	}

	template.ObjectMeta.Annotations[browserv1.SelenosisOptionsAnnotationKey] = string(b)
}

func dropMcpOptions(q url.Values) string {
	q = dropSelenosisOptions(q)
	q.Del("browser")
	q.Del("version")
	return q.Encode()
}

func dropSelenosisOptions(q url.Values) url.Values {
	for k := range q {
		if strings.HasPrefix(k, "labels.") || strings.HasPrefix(k, "annotations.") || strings.HasPrefix(k, "containers.") {
			delete(q, k)
		}
	}
	return q
}
