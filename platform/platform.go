package platform

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/alcounit/selenosis/selenium"
	apiv1 "k8s.io/api/core/v1"
)

//Meta describes standart metadata
type Meta struct {
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

//Spec describes specification for Service
type Spec struct {
	Resources    apiv1.ResourceRequirements `yaml:"resources,omitempty" json:"resources,omitempty"`
	HostAliases  []apiv1.HostAlias          `yaml:"hostAliases,omitempty" json:"hostAliases,omitempty"`
	EnvVars      []apiv1.EnvVar             `yaml:"env,omitempty" json:"env,omitempty"`
	NodeSelector map[string]string          `yaml:"nodeSelector,omitempty" json:"nodeSelector,omitempty"`
	Affinity     apiv1.Affinity             `yaml:"affinity,omitempty" json:"affinity,omitempty"`
	DNSConfig    apiv1.PodDNSConfig         `yaml:"dnsConfig,omitempty" json:"dnsConfig,omitempty"`
	Tolerations  []apiv1.Toleration         `yaml:"tolerations,omitempty" json:"tolerations,omitempty"`
	VolumeMounts []apiv1.VolumeMount        `yaml:"volumeMounts,omitempty" json:"volumeMounts,omitempty"`
}

//BrowserSpec describes settings for Service
type BrowserSpec struct {
	BrowserName    string         `yaml:"-" json:"-"`
	BrowserVersion string         `yaml:"-" json:"-"`
	Image          string         `yaml:"image" json:"image"`
	Path           string         `yaml:"path" json:"path"`
	Privileged     bool           `yaml:"privileged" json:"privileged"`
	Meta           Meta           `yaml:"meta" json:"meta"`
	Spec           Spec           `yaml:"spec" json:"spec"`
	Volumes        []apiv1.Volume `yaml:"volumes,omitempty" json:"volumes,omitempty"`
}

//ServiceSpec describes data requred for creating service
type ServiceSpec struct {
	SessionID             string
	RequestedCapabilities selenium.Capabilities
	Template              *BrowserSpec
}

//Service ...
type Service struct {
	SessionID  string            `json:"id"`
	URL        *url.URL          `json:"-"`
	Labels     map[string]string `json:"labels"`
	OnTimeout  chan struct{}     `json:"-"`
	CancelFunc func()            `json:"-"`
	Status     ServiceStatus     `json:"-"`
	Started    time.Time         `json:"started"`
	Uptime     string            `json:"uptime"`
}

//ServiceStatus ...
type ServiceStatus string

//Event ...
type Event struct {
	Type    EventType
	Service *Service
}

//EventType ...
type EventType string

const (
	Added   EventType = "Added"
	Updated EventType = "Updated"
	Deleted EventType = "Deleted"

	Pending ServiceStatus = "Pending"
	Running ServiceStatus = "Running"
	Unknown ServiceStatus = "Unknown"
)

//Platform ...
type Platform interface {
	Create(*ServiceSpec) (*Service, error)
	Delete(string) error
	List() ([]*Service, error)
	Watch() <-chan Event
	Logs(context.Context, string) (io.ReadCloser, error)
}
