package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sync"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/alcounit/selenosis/platform"
	"github.com/imdario/mergo"
)

//Layout ...
type Layout struct {
	DefaultSpec platform.Spec                    `yaml:"spec" json:"spec"`
	Meta        platform.Meta                    `yaml:"meta" json:"meta"`
	Path        string                           `yaml:"path" json:"path"`
	Versions    map[string]*platform.BrowserSpec `yaml:"versions" json:"versions"`
}

//BrowsersConfig ...
type BrowsersConfig struct {
	lock       sync.RWMutex
	containers map[string]*Layout
}

//NewBrowsersConfig returns parced browsers config from JSON or YAML file.
func NewBrowsersConfig(cfg string) (*BrowsersConfig, error) {
	content, err := ioutil.ReadFile(cfg)
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}

	layouts := make(map[string]*Layout)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 1000)

	if err := decoder.Decode(&layouts); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	if len(layouts) == 0 {
		return nil, fmt.Errorf("empty config: %v", err)
	}

	for _, layout := range layouts {
		spec := layout.DefaultSpec
		for _, container := range layout.Versions {
			container.Path = layout.Path
			container.Meta.Annotations = merge(container.Meta.Annotations, layout.Meta.Annotations)
			container.Meta.Labels = merge(container.Meta.Labels, layout.Meta.Labels)

			if err := mergo.Merge(&container.Spec, spec); err != nil {
				return nil, fmt.Errorf("merge error %v", err)
			}
		}
	}

	return &BrowsersConfig{
		containers: layouts,
	}, nil
}

//Find return Container if it present in config
func (cfg *BrowsersConfig) Find(name, version string) (*platform.BrowserSpec, error) {
	cfg.lock.Lock()
	defer cfg.lock.Unlock()
	c, ok := cfg.containers[name]
	if !ok {
		return nil, fmt.Errorf("unknown browser name %s", name)
	}

	v, ok := c.Versions[version]
	if !ok {
		return nil, fmt.Errorf("unknown browser version %s", version)
	}

	return v, nil
}

func merge(from, to map[string]string) map[string]string {
	for k, v := range from {
		to[k] = v
	}
	return to
}
