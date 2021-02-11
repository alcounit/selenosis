package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/alcounit/selenosis/tools"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/utils/pointer"
)

var (
	browserPorts = struct {
		selenium, vnc intstr.IntOrString
	}{
		selenium: intstr.FromString("4444"),
		vnc:      intstr.FromString("5900"),
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
	Namespace           string
	Service             string
	ServicePort         string
	ImagePullSecretName string
	ProxyImage          string
	ReadinessTimeout    time.Duration
	IdleTimeout         time.Duration
}

//Client ...
type Client struct {
	ns                  string
	svc                 string
	svcPort             intstr.IntOrString
	imagePullSecretName string
	proxyImage          string
	readinessTimeout    time.Duration
	idleTimeout         time.Duration
	clientset           *kubernetes.Clientset
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
		ns:                  c.Namespace,
		clientset:           clientset,
		svc:                 c.Service,
		svcPort:             intstr.FromString(c.ServicePort),
		imagePullSecretName: c.ImagePullSecretName,
		proxyImage:          c.ProxyImage,
		readinessTimeout:    c.ReadinessTimeout,
		idleTimeout:         c.IdleTimeout,
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
		clientset: clientset,
	}, nil

}

//Create ...
func (cl *Client) Create(layout *ServiceSpec) (*Service, error) {

	labels := map[string]string{
		defaults.serviceType:    "browser",
		defaults.browserName:    layout.Template.BrowserName,
		defaults.browserVersion: layout.Template.BrowserVersion,
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
					Name:  "browser",
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
					ImagePullPolicy: apiv1.PullIfNotPresent,
				},
				{
					Name:  "seleniferous",
					Image: cl.proxyImage,
					SecurityContext: &apiv1.SecurityContext{
						Privileged: pointer.BoolPtr(true),
					},
					Ports: getSidecarPorts(cl.svcPort),
					Command: []string{
						"/seleniferous", "--listhen-port", cl.svcPort.StrVal, "--proxy-default-path", path.Join(layout.Template.Path, "session"), "--idle-timeout", cl.idleTimeout.String(), "--namespace", cl.ns,
					},
					ImagePullPolicy: apiv1.PullIfNotPresent,
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
			NodeSelector:     layout.Template.Spec.NodeSelector,
			HostAliases:      layout.Template.Spec.HostAliases,
			RestartPolicy:    apiv1.RestartPolicyNever,
			Affinity:         &layout.Template.Spec.Affinity,
			DNSConfig:        &layout.Template.Spec.DNSConfig,
			ImagePullSecrets: getImagePullSecretList(cl.imagePullSecretName),
		},
	}

	context := context.Background()
	pod, err := cl.clientset.CoreV1().Pods(cl.ns).Create(context, pod, metav1.CreateOptions{})

	if err != nil {
		return nil, fmt.Errorf("failed to create pod %v", err)
	}

	podName := pod.GetName()
	cancel := func() {
		cl.Delete(podName)
	}

	w, err := cl.clientset.CoreV1().Pods(cl.ns).Watch(context, metav1.ListOptions{
		FieldSelector:  fields.OneTermEqualSelector("metadata.name", podName).String(),
		TimeoutSeconds: pointer.Int64Ptr(cl.readinessTimeout.Milliseconds()),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to watch pod status: %v", err)
	}

	statusFn := func() error {
		defer w.Stop()
		var watchedPod *apiv1.Pod

		for event := range w.ResultChan() {
			switch event.Type {
			case watch.Error:
				return fmt.Errorf("received error while watching pod: %s",
					event.Object.GetObjectKind().GroupVersionKind().String())
			case watch.Deleted, watch.Added, watch.Modified:
				watchedPod = event.Object.(*apiv1.Pod)
			default:
				return fmt.Errorf("received unknown event type %s while watching pod", event.Type)
			}
			if event.Type == watch.Deleted {
				return errors.New("pod was deleted before becoming available")
			}
			switch watchedPod.Status.Phase {
			case apiv1.PodPending:
				continue
			case apiv1.PodSucceeded, apiv1.PodFailed:
				return fmt.Errorf("pod exited early with status %s", watchedPod.Status.Phase)
			case apiv1.PodRunning:
				return nil
			case apiv1.PodUnknown:
				return errors.New("couldn't obtain pod state")
			default:
				return errors.New("pod has unknown status")
			}
		}
		return fmt.Errorf("pod wasn't running")
	}

	if statusFn() != nil {
		cancel()
		return nil, fmt.Errorf("failed to create pod: %v", err)
	}

	host := fmt.Sprintf("%s.%s", podName, cl.svc)
	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, browserPorts.selenium.StrVal),
	}

	if err := waitForService(*u, cl.readinessTimeout); err != nil {
		cancel()
		return nil, fmt.Errorf("container service is not ready %v", u.String())
	}

	u.Host = net.JoinHostPort(host, cl.svcPort.StrVal)
	svc := &Service{
		SessionID: podName,
		URL:       u,
		Labels:    layout.Template.Meta.Labels,
		CancelFunc: func() {
			cancel()
		},
		Started: pod.CreationTimestamp.Time,
		Uptime:  tools.TimeElapsed(pod.CreationTimestamp.Time),
	}

	return svc, nil
}

//Delete ...
func (cl *Client) Delete(name string) error {
	context := context.Background()

	return cl.clientset.CoreV1().Pods(cl.ns).Delete(context, name, metav1.DeleteOptions{
		GracePeriodSeconds: pointer.Int64Ptr(15),
	})
}

//List ...
func (cl *Client) List() ([]*Service, error) {
	context := context.Background()
	pods, err := cl.clientset.CoreV1().Pods(cl.ns).List(context, metav1.ListOptions{
		LabelSelector: "type=browser",
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %v", err)
	}

	var services []*Service

	for _, pod := range pods.Items {
		podName := pod.GetName()
		host := fmt.Sprintf("%s.%s", podName, cl.svc)

		var status ServiceStatus
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			status = Running
		case apiv1.PodPending:
			status = Pending
		default:
			status = Unknown
		}

		service := &Service{
			SessionID: podName,
			URL: &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(host, cl.svcPort.StrVal),
			},
			Labels: pod.GetLabels(),
			CancelFunc: func() {
				cl.Delete(podName)
			},
			Status:  status,
			Started: pod.CreationTimestamp.Time,
			Uptime:  tools.TimeElapsed(pod.CreationTimestamp.Time),
		}
		services = append(services, service)
	}

	return services, nil

}

//Watch ...
func (cl Client) Watch() <-chan Event {
	ch := make(chan Event)

	convert := func(obj interface{}) *Service {
		pod := obj.(*apiv1.Pod)
		podName := pod.GetName()
		host := fmt.Sprintf("%s.%s", podName, cl.svc)

		var status ServiceStatus
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			status = Running
		case apiv1.PodPending:
			status = Pending
		default:
			status = Unknown
		}

		return &Service{
			SessionID: podName,
			URL: &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(host, cl.svcPort.StrVal),
			},
			Labels: pod.GetLabels(),
			CancelFunc: func() {
				cl.Delete(podName)
			},
			Status:  status,
			Started: pod.CreationTimestamp.Time,
			Uptime:  tools.TimeElapsed(pod.CreationTimestamp.Time),
		}
	}

	namespace := informers.WithNamespace(cl.ns)
	labels := informers.WithTweakListOptions(func(list *metav1.ListOptions) {
		list.LabelSelector = "type=browser"
	})

	sharedIformer := informers.NewSharedInformerFactoryWithOptions(cl.clientset, 30*time.Second, namespace, labels)
	sharedIformer.Core().V1().Pods().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				ch <- Event{
					Type:    Added,
					Service: convert(obj),
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				ch <- Event{
					Type:    Updated,
					Service: convert(new),
				}
			},
			DeleteFunc: func(obj interface{}) {
				ch <- Event{
					Type:    Deleted,
					Service: convert(obj),
				}
			},
		},
	)

	var neverStop <-chan struct{} = make(chan struct{})
	sharedIformer.Start(neverStop)
	return ch
}

//Logs ...
func (cl *Client) Logs(ctx context.Context, name string) (io.ReadCloser, error) {
	req := cl.clientset.CoreV1().Pods(cl.ns).GetLogs(name, &apiv1.PodLogOptions{
		Container:  "browser",
		Follow:     true,
		Previous:   false,
		Timestamps: false,
	})
	return req.Stream(ctx)
}

func getBrowserPorts() []apiv1.ContainerPort {
	port := []apiv1.ContainerPort{}
	fn := func(name string, value int) {
		port = append(port, apiv1.ContainerPort{Name: name, ContainerPort: int32(value)})
	}

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

func getImagePullSecretList(secret string) []apiv1.LocalObjectReference {
	refList := make([]apiv1.LocalObjectReference, 0)
	if secret != "" {
		ref := apiv1.LocalObjectReference{
			Name: secret,
		}
		refList = append(refList, ref)
	}
	return refList
}

func waitForService(u url.URL, t time.Duration) error {
	up := make(chan struct{})
	done := make(chan struct{})
	u.Path = "/status"
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
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
		return fmt.Errorf("no responce after %v", t)
	case <-up:
	}
	return nil
}
