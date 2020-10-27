package platform

import (
	"net/url"

	"github.com/alcounit/selenosis/selenium"
	apiv1 "k8s.io/api/core/v1"
)

//Spec describes specification for Service
type Spec struct {
	Resources    apiv1.ResourceRequirements `yaml:"resources,omitempty" json:"resources,omitempty"`
	HostAliases  []apiv1.HostAlias          `yaml:"hostAliases,omitempty" json:"hostAliases,omitempty"`
	EnvVars      []apiv1.EnvVar             `yaml:"envVars,omitempty" json:"envVars,omitempty"`
	NodeSelector map[string]string          `yaml:"nodeSelector,omitempty" json:"nodeSelector,omitempty"`
	Affinity     apiv1.Affinity             `yaml:"affinity,omitempty" json:"affinity,omitempty"`
	DNSConfig    apiv1.PodDNSConfig         `yaml:"dnsConfig,omitempty" json:"dnsConfig,omitempty"`
}

//BrowserSpec describes settings for Service
type BrowserSpec struct {
	Image       string            `yaml:"image" json:"image"`
	Path        string            `yaml:"path" json:"path"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Spec        Spec              `yaml:"spec" json:"spec"`
}

//ServiceSpec describes data requred for creating service
type ServiceSpec struct {
	SessionID             string
	RequestedCapabilities selenium.Capabilities
	Template              *BrowserSpec
}

//Service ...
type Service struct {
	SessionID  string
	URL        *url.URL
	OnTimeout  chan struct{}
	CancelFunc func()
}

//Platform ...
type Platform interface {
	Create(*ServiceSpec) (*Service, error)
	Delete(string) error
	List() ([]*Service, error)
}
