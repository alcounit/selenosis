package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
)

var (
	browserPorts = struct {
		selenium, fileserver, clipboard, vnc, devtools intstr.IntOrString
	}{
		selenium:   intstr.FromString("4444"),
		fileserver: intstr.FromString("8080"),
		clipboard:  intstr.FromString("9090"),
		vnc:        intstr.FromString("5900"),
		devtools:   intstr.FromString("7070"),
	}

	defaults = struct {
		serviceType, testName, browserName, browserVersion, screenResolution, enableVNC, timeZone, session string
	}{
		serviceType:      "type",
		testName:         "testName",
		browserName:      "browserName",
		browserVersion:   "browserVersion",
		screenResolution: "SCREEN_RESOLUTION",
		enableVNC:        "ENABLE_VNC",
		timeZone:         "TZ",
		session:          "session",
	}
)

//ClientConfig ...
type ClientConfig struct {
	Namespace        string
	Service          string
	ReadinessTimeout time.Duration
	IddleTimeout     time.Duration
	ServicePort      string
}

//Client ...
type Client struct {
	ns               string
	svc              string
	svcPort          intstr.IntOrString
	readinessTimeout time.Duration
	iddleTimeout     time.Duration
	clientset        v1.CoreV1Interface
}

//NewClient ...
func NewClient(c ClientConfig) (Platform, error) {

	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to build client: %v", err)
	}

	return &Client{
		ns:               c.Namespace,
		clientset:        clientset.CoreV1(),
		svc:              c.Service,
		svcPort:          intstr.FromString(c.ServicePort),
		readinessTimeout: c.ReadinessTimeout,
		iddleTimeout:     c.IddleTimeout,
	}, nil

}

//NewDefaultClient ...
func NewDefaultClient(namespace string) (Platform, error) {

	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to build client: %v", err)
	}

	return &Client{
		ns:        namespace,
		clientset: clientset.CoreV1(),
	}, nil

}

//Create ...
func (cl *Client) Create(layout *ServiceSpec) (*Service, error) {

	labels := map[string]string{
		defaults.serviceType:    "browser",
		defaults.browserName:    layout.RequestedCapabilities.BrowserName,
		defaults.browserVersion: layout.RequestedCapabilities.BrowserVersion,
		defaults.testName:       layout.RequestedCapabilities.TestName,
		defaults.session:        layout.SessionID,
	}

	envVar := func(name, value string) (i int, b bool) {
		for i, slice := range layout.Template.Spec.EnvVars {
			if slice.Name == name {
				slice.Value = value
				return i, true
			}
		}
		return -1, false
	}

	if layout.RequestedCapabilities.ScreenResolution != "" {
		i, b := envVar(defaults.screenResolution, layout.RequestedCapabilities.ScreenResolution)
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars,
				apiv1.EnvVar{Name: defaults.screenResolution,
					Value: layout.RequestedCapabilities.ScreenResolution})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaults.screenResolution, Value: layout.RequestedCapabilities.ScreenResolution}
		}
		labels[defaults.screenResolution] = layout.RequestedCapabilities.ScreenResolution
	}

	if layout.RequestedCapabilities.VNC {
		vnc := fmt.Sprintf("%v", layout.RequestedCapabilities.VNC)
		i, b := envVar(defaults.enableVNC, vnc)
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaults.enableVNC, Value: vnc})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaults.enableVNC, Value: vnc}
		}
		labels[defaults.enableVNC] = vnc
	}

	if layout.RequestedCapabilities.TimeZone != "" {
		i, b := envVar(defaults.timeZone, layout.RequestedCapabilities.TimeZone)
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaults.timeZone, Value: layout.RequestedCapabilities.TimeZone})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaults.timeZone, Value: layout.RequestedCapabilities.TimeZone}
		}
		labels[defaults.timeZone] = layout.RequestedCapabilities.TimeZone
	}

	if layout.Template.Meta.Labels == nil {
		layout.Template.Meta.Labels = make(map[string]string)
	}

	for k, v := range labels {
		layout.Template.Meta.Labels[k] = v
	}

	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        layout.SessionID,
			Labels:      layout.Template.Meta.Labels,
			Annotations: layout.Template.Meta.Annotations,
		},
		Spec: apiv1.PodSpec{
			Hostname:  layout.SessionID,
			Subdomain: cl.svc,
			Containers: []apiv1.Container{
				{
					Name:  layout.SessionID,
					Image: layout.Template.Image,
					SecurityContext: &apiv1.SecurityContext{
						Privileged: pointer.BoolPtr(false),
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								"SYS_ADMIN",
							},
						},
					},
					Env:       layout.Template.Spec.EnvVars,
					Ports:     getBrowserPorts(),
					Resources: layout.Template.Spec.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dshm",
							MountPath: "/dev/shm",
						},
					},
				},
				{
					Name:  "seleniferous",
					Image: "alcounit/seleniferous:latest",
					SecurityContext: &apiv1.SecurityContext{
						Privileged: pointer.BoolPtr(true),
					},
					Ports: getSidecarPorts(cl.svcPort),
					Command: []string{
						"/seleniferous", "--listhen-port", cl.svcPort.StrVal, "--proxy-default-path", path.Join(layout.Template.Path, "session"), "--iddle-timeout", cl.iddleTimeout.String(), "--namespace", cl.ns,
					},
				},
			},
			Volumes: []apiv1.Volume{
				{
					Name: "dshm",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{
							Medium: apiv1.StorageMediumMemory,
						},
					},
				},
			},
			NodeSelector:  layout.Template.Spec.NodeSelector,
			HostAliases:   layout.Template.Spec.HostAliases,
			RestartPolicy: apiv1.RestartPolicyNever,
			Affinity:      &layout.Template.Spec.Affinity,
			DNSConfig:     &layout.Template.Spec.DNSConfig,
		},
	}

	context := context.Background()
	pod, err := cl.clientset.Pods(cl.ns).Create(context, pod, metav1.CreateOptions{})

	if err != nil {
		return nil, fmt.Errorf("failed to create pod %v", err)
	}

	podName := pod.GetName()
	cancel := func() {
		cl.Delete(podName)
	}

	var status apiv1.PodStatus
	w, err := cl.clientset.Pods(cl.ns).Watch(context, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", podName).String(),
	})

	if err != nil {
		cancel()
		return nil, fmt.Errorf("watch pod: %v", err)
	}

	func() {
		for {
			select {
			case events, ok := <-w.ResultChan():
				if !ok {
					return
				}
				pod = events.Object.(*apiv1.Pod)
				status = pod.Status
				if pod.Status.Phase != apiv1.PodPending {
					w.Stop()
				}
			case <-time.After(cl.iddleTimeout):
				w.Stop()
			}
		}
	}()

	if status.Phase != apiv1.PodRunning {
		cancel()
		return nil, fmt.Errorf("pod status: %v", status.Phase)
	}

	host := fmt.Sprintf("%s.%s", podName, cl.svc)
	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, browserPorts.selenium.StrVal),
	}

	if err := waitForService(u, cl.readinessTimeout); err != nil {
		cancel()
		return nil, fmt.Errorf("container service is not ready %v", u.String())
	}

	u.Host = net.JoinHostPort(host, cl.svcPort.StrVal)
	svc := &Service{
		SessionID: podName,
		URL:       u,
		CancelFunc: func() {
			cancel()
		},
	}

	return svc, nil
}

//Delete ...
func (cl *Client) Delete(name string) error {
	context := context.Background()

	return cl.clientset.Pods(cl.ns).Delete(context, name, metav1.DeleteOptions{
		GracePeriodSeconds: pointer.Int64Ptr(15),
	})
}

//List ...
func (cl *Client) List() ([]*Service, error) {
	return nil, errors.New("not implemted yet")
}

func getBrowserPorts() []apiv1.ContainerPort {
	port := []apiv1.ContainerPort{}
	fn := func(name string, value int) {
		port = append(port, apiv1.ContainerPort{Name: name, ContainerPort: int32(value)})
	}

	fn("clipboard", browserPorts.clipboard.IntValue())
	fn("devtools", browserPorts.devtools.IntValue())
	fn("fileserver", browserPorts.fileserver.IntValue())
	fn("vnc", browserPorts.vnc.IntValue())
	fn("selenium", browserPorts.selenium.IntValue())

	return port
}

func getSidecarPorts(p intstr.IntOrString) []apiv1.ContainerPort {
	port := []apiv1.ContainerPort{}
	fn := func(name string, value int) {
		port = append(port, apiv1.ContainerPort{Name: name, ContainerPort: int32(value)})
	}
	fn("selenium", p.IntValue())
	return port
}

//code credits to https://github.com/aerokube/selenoid/blob/master/service/service.go#L97
func waitForService(u *url.URL, t time.Duration) error {
	up := make(chan struct{})
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			req, _ := http.NewRequest(http.MethodHead, u.String(), nil)
			req.Close = true
			resp, err := http.DefaultClient.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			if err != nil {
				<-time.After(50 * time.Millisecond)
				continue
			}
			up <- struct{}{}
			return
		}
	}()
	select {
	case <-time.After(t):
		close(done)
		return fmt.Errorf("%s does not respond in %v", u, t)
	case <-up:
	}
	return nil
}
