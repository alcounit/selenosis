package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/alcounit/selenosis/platform"
	"github.com/alcounit/selenosis/tools"
	"github.com/imdario/mergo"
	apiv1 "k8s.io/api/core/v1"
)

//Layout ...
type Layout struct {
	DefaultSpec    platform.Spec                    `yaml:"spec" json:"spec"`
	Meta           platform.Meta                    `yaml:"meta" json:"meta"`
	Path           string                           `yaml:"path" json:"path"`
	DefaultVersion string                           `yaml:"defaultVersion" json:"defaultVersion"`
	Versions       map[string]*platform.BrowserSpec `yaml:"versions" json:"versions"`
	Volumes        []apiv1.Volume                   `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

//BrowsersConfig ...
type BrowsersConfig struct {
	configFile string
	lock       sync.RWMutex
	containers map[string]*Layout
}

//NewBrowsersConfig returns parced browsers config from JSON or YAML file.
func NewBrowsersConfig(configFile string) (*BrowsersConfig, error) {
	layouts, err := readConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	return &BrowsersConfig{
		configFile: configFile,
		containers: layouts,
	}, nil
}

//Reload ...
func (cfg *BrowsersConfig) Reload() error {
	cfg.lock.Lock()
	defer cfg.lock.Unlock()

	layouts, err := readConfig(cfg.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	cfg.containers = layouts
	return nil
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
		if c.DefaultVersion != "" {
			v, ok = c.Versions[c.DefaultVersion]
			if !ok {
				return nil, fmt.Errorf("unknown browser version %s", version)
			}
			v.BrowserName = name
			v.BrowserVersion = c.DefaultVersion
			return v, nil
		}
		return nil, fmt.Errorf("unknown browser version %s", version)
	}
	v.BrowserName = name
	v.BrowserVersion = version
	return v, nil
}

//GetBrowserVersions ...
func (cfg *BrowsersConfig) GetBrowserVersions() map[string][]string {
	cfg.lock.Lock()
	defer cfg.lock.Unlock()

	browsers := make(map[string][]string)

	for name, layout := range cfg.containers {
		versions := make([]string, 0)
		for version := range layout.Versions {
			versions = append(versions, version)
		}
		sort.Slice(versions[:], func(i, j int) bool {
			ii := tools.StrToFloat64(versions[i])
			jj := tools.StrToFloat64(versions[j])
			return ii < jj
		})
		browsers[name] = versions
	}

	return browsers
}

func readConfig(configFile string) (map[string]*Layout, error) {
	content, err := ioutil.ReadFile(configFile)
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
			container.Volumes = layout.Volumes

			if err := mergo.Merge(&container.Spec, spec); err != nil {
				return nil, fmt.Errorf("merge error %v", err)
			}
		}
	}
	return layouts, nil
}

func merge(from, to map[string]string) map[string]string {
	for k, v := range from {
		to[k] = v
	}
	return to
}
