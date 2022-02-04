package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/alcounit/selenosis/tools"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	label        = "selenosis.app.type"
	quotaName    = "selenosis-pod-limit"
	browserPorts = struct {
		selenium, vnc, video intstr.IntOrString
	}{
		selenium: intstr.FromString("4444"),
		vnc:      intstr.FromString("5900"),
		video:    intstr.FromString("6099"),
	}

	defaultsAnnotations = struct {
		testName, browserName, browserVersion, screenResolution, enableVNC, enableVideo, videoName, timeZone string
	}{
		testName:         "testName",
		browserName:      "browserName",
		browserVersion:   "browserVersion",
		screenResolution: "SCREEN_RESOLUTION",
		enableVNC:        "ENABLE_VNC",
		enableVideo:      "ENABLE_VIDEO",
		timeZone:         "TZ",
		videoName:        "VIDEO_NAME",
	}
	defaultLabels = struct {
		serviceType, appType, session string
	}{
		serviceType: "type",
		appType:     label,
		session:     "session",
	}
)

//ClientConfig ...
type ClientConfig struct {
	Namespace           string
	Service             string
	ServicePort         string
	ImagePullSecretName string
	ProxyImage          string
	VideoImage			string
	ReadinessTimeout    time.Duration
	IdleTimeout         time.Duration
}

//Client ...
type Client struct {
	ns        string
	svc       string
	svcPort   intstr.IntOrString
	clientset kubernetes.Interface
	service   ServiceInterface
	quota     QuotaInterface
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

	service := &service{
		ns:                  c.Namespace,
		clientset:           clientset,
		svc:                 c.Service,
		svcPort:             intstr.FromString(c.ServicePort),
		imagePullSecretName: c.ImagePullSecretName,
		proxyImage:          c.ProxyImage,
		videoImage:			 c.VideoImage,
		readinessTimeout:    c.ReadinessTimeout,
		idleTimeout:         c.IdleTimeout,
	}

	quota := &quota{
		ns:        c.Namespace,
		clientset: clientset,
	}

	return &Client{
		ns:        c.Namespace,
		clientset: clientset,
		svc:       c.Service,
		svcPort:   intstr.FromString(c.ServicePort),
		service:   service,
		quota:     quota,
	}, nil

}

func (cl *Client) Service() ServiceInterface {
	return cl.service
}

func (cl *Client) Quota() QuotaInterface {
	return cl.quota
}

//List ...
func (cl *Client) State() (PlatformState, error) {
	context := context.Background()
	pods, err := cl.clientset.CoreV1().Pods(cl.ns).List(context, metav1.ListOptions{})

	if err != nil {
		return PlatformState{}, fmt.Errorf("failed to get pods: %v", err)
	}

	var services []Service
	var workers []Worker

	for _, pod := range pods.Items {
		podName := pod.GetName()
		creationTime := pod.CreationTimestamp.Time

		var status ServiceStatus
		switch pod.Status.Phase {
		case apiv1.PodRunning:
			status = Running
		case apiv1.PodPending:
			status = Pending
		default:
			status = Unknown
		}

		if application, ok := pod.GetLabels()[label]; ok {
			switch application {
			case "worker":
				workers = append(workers, Worker{
					Name:    podName,
					Labels:  pod.Labels,
					Status:  status,
					Started: creationTime,
				})

			case "browser":
				services = append(services, Service{
					SessionID: podName,
					URL: &url.URL{
						Scheme: "http",
						Host:   tools.BuildHostPort(podName, cl.svc, cl.svcPort.StrVal),
					},
					Labels: getRequestedCapabilities(pod.GetAnnotations()),
					CancelFunc: func() {
						deletePod(cl.clientset, cl.ns, podName)
					},
					Status:  status,
					Started: creationTime,
				})
			}
		}
	}

	return PlatformState{
		Services: services,
		Workers:  workers,
	}, nil

}

//Watch ...
func (cl *Client) Watch() <-chan Event {
	ch := make(chan Event)
	namespace := informers.WithNamespace(cl.ns)
	labels := informers.WithTweakListOptions(func(list *metav1.ListOptions) {
		list.LabelSelector = label
	})

	sharedIformer := informers.NewSharedInformerFactoryWithOptions(cl.clientset, 30*time.Second, namespace, labels)

	podEventFunc := func(obj interface{}, eventType EventType) {
		if pod, ok := obj.(*apiv1.Pod); ok {
			if application, ok := pod.GetLabels()[label]; ok {
				podName := pod.GetName()
				creationTime := pod.CreationTimestamp.Time

				var status ServiceStatus
				switch pod.Status.Phase {
				case apiv1.PodRunning:
					status = Running
				case apiv1.PodPending:
					status = Pending
				default:
					status = Unknown
				}

				switch application {
				case "worker":
					ch <- Event{
						Type: eventType,
						PlatformObject: Worker{
							Name:    podName,
							Labels:  pod.Labels,
							Status:  status,
							Started: creationTime,
						},
					}

				case "browser":
					ch <- Event{
						Type: eventType,
						PlatformObject: Service{
							SessionID: podName,
							URL: &url.URL{
								Scheme: "http",
								Host:   tools.BuildHostPort(podName, cl.svc, cl.svcPort.StrVal),
							},
							Labels: getRequestedCapabilities(pod.GetAnnotations()),
							CancelFunc: func() {
								deletePod(cl.clientset, cl.ns, podName)
							},
							Status:  status,
							Started: creationTime,
						},
					}
				}
			}
		}
	}
	sharedIformer.Core().V1().Pods().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				podEventFunc(obj, Added)
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				podEventFunc(new, Updated)
			},
			DeleteFunc: func(obj interface{}) {
				podEventFunc(obj, Deleted)
			},
		},
	)

	quotaEventFunc := func(obj interface{}, eventType EventType) {
		if rq, ok := obj.(*apiv1.ResourceQuota); ok {
			if _, ok := rq.GetLabels()[label]; ok {
				rqName := rq.GetName()
				ch <- Event{
					Type: eventType,
					PlatformObject: Quota{
						Name:            rqName,
						CurrentMaxLimit: rq.Spec.Hard.Pods().Value(),
					},
				}
			}
		}
	}
	sharedIformer.Core().V1().ResourceQuotas().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				quotaEventFunc(obj, Added)
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				quotaEventFunc(new, Updated)
			},
			DeleteFunc: func(obj interface{}) {
				quotaEventFunc(obj, Deleted)
			},
		},
	)

	var neverStop <-chan struct{} = make(chan struct{})
	sharedIformer.Start(neverStop)
	return ch
}

type service struct {
	ns                  string
	svc                 string
	svcPort             intstr.IntOrString
	imagePullSecretName string
	proxyImage          string
	videoImage          string
	readinessTimeout    time.Duration
	idleTimeout         time.Duration
	clientset           kubernetes.Interface
}

//Create ...
func (cl *service) Create(layout ServiceSpec) (Service, error) {
	annontations := map[string]string{
		defaultsAnnotations.browserName:    layout.Template.BrowserName,
		defaultsAnnotations.browserVersion: layout.Template.BrowserVersion,
		defaultsAnnotations.testName:       layout.RequestedCapabilities.TestName,
		defaultsAnnotations.videoName:      layout.RequestedCapabilities.VideoName,
	}

	labels := map[string]string{
		defaultLabels.serviceType: "browser",
		defaultLabels.appType:     "browser",
		defaultLabels.session:     layout.SessionID,
	}

	envVar := func(name string) (i int, b bool) {
		for i, slice := range layout.Template.Spec.EnvVars {
			if slice.Name == name {
				return i, true
			}
		}
		return -1, false
	}

	i, b := envVar(defaultsAnnotations.screenResolution)
	if layout.RequestedCapabilities.ScreenResolution != "" {
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars,
				apiv1.EnvVar{Name: defaultsAnnotations.screenResolution,
					Value: layout.RequestedCapabilities.ScreenResolution})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaultsAnnotations.screenResolution, Value: layout.RequestedCapabilities.ScreenResolution}
		}
		annontations[defaultsAnnotations.screenResolution] = layout.RequestedCapabilities.ScreenResolution
	} else {
		if b {
			annontations[defaultsAnnotations.screenResolution] = layout.Template.Spec.EnvVars[i].Value
		}
	}

	i, b = envVar(defaultsAnnotations.enableVNC)
	if layout.RequestedCapabilities.VNC {
		vnc := fmt.Sprintf("%v", layout.RequestedCapabilities.VNC)
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaultsAnnotations.enableVNC, Value: vnc})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaultsAnnotations.enableVNC, Value: vnc}
		}
		annontations[defaultsAnnotations.enableVNC] = vnc
	} else {
		if b {
			annontations[defaultsAnnotations.enableVNC] = layout.Template.Spec.EnvVars[i].Value
		}
	}

	i, b = envVar(defaultsAnnotations.enableVideo)
	if layout.RequestedCapabilities.Video {
		video := fmt.Sprintf("%v", layout.RequestedCapabilities.Video)
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaultsAnnotations.enableVideo, Value: video})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaultsAnnotations.enableVideo, Value: video}
		}
		i, b = envVar(defaultsAnnotations.videoName)
		videoName := fmt.Sprintf("%v", layout.RequestedCapabilities.VideoName)
		if videoName == "" {
			videoName = fmt.Sprintf("%v.mp4", layout.SessionID)
		}
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaultsAnnotations.videoName, Value: videoName})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaultsAnnotations.videoName, Value: video}
		}
		annontations[defaultsAnnotations.enableVideo] = video
		annontations[defaultsAnnotations.videoName] = videoName
	} else {
		if b {
			annontations[defaultsAnnotations.enableVideo] = layout.Template.Spec.EnvVars[i].Value
		}
	}

	i, b = envVar(defaultsAnnotations.timeZone)
	if layout.RequestedCapabilities.TimeZone != "" {
		if !b {
			layout.Template.Spec.EnvVars = append(layout.Template.Spec.EnvVars, apiv1.EnvVar{Name: defaultsAnnotations.timeZone, Value: layout.RequestedCapabilities.TimeZone})
		} else {
			layout.Template.Spec.EnvVars[i] = apiv1.EnvVar{Name: defaultsAnnotations.timeZone, Value: layout.RequestedCapabilities.TimeZone}
		}
		annontations[defaultsAnnotations.timeZone] = layout.RequestedCapabilities.TimeZone
	} else {
		if b {
			annontations[defaultsAnnotations.timeZone] = layout.Template.Spec.EnvVars[i].Value
		}
	}
	

	if layout.Template.Meta.Labels == nil {
		layout.Template.Meta.Labels = make(map[string]string)
	}

	for k, v := range labels {
		layout.Template.Meta.Labels[k] = v
	}

	if layout.Template.Meta.Annotations == nil {
		layout.Template.Meta.Annotations = make(map[string]string)
	}

	if caps, err := json.Marshal(annontations); err == nil {
		layout.Template.Meta.Annotations["capabilities"] = string(caps)
	}

	pod := cl.BuildPod(layout)

	context := context.Background()
	pod, err := cl.clientset.CoreV1().Pods(cl.ns).Create(context, pod, metav1.CreateOptions{})

	if err != nil {
		return Service{}, fmt.Errorf("failed to create pod %v", err)
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
		return Service{}, fmt.Errorf("failed to watch pod status: %v", err)
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

	err = statusFn()
	if err != nil {
		cancel()
		return Service{}, fmt.Errorf("pod is not ready after creation: %v", err)
	}

	u := &url.URL{
		Scheme: "http",
		Host:   podName + "." + cl.svc + ":" + browserPorts.selenium.StrVal,
	}

	if err := waitForService(*u, cl.readinessTimeout); err != nil {
		cancel()
		return Service{}, fmt.Errorf("container service is not ready %v", u.String())
	}

	u.Host = podName + "." + cl.svc + ":" + cl.svcPort.StrVal

	return Service{
		SessionID: podName,
		URL:       u,
		Labels:    getRequestedCapabilities(pod.GetAnnotations()),
		CancelFunc: func() {
			cancel()
		},
		Status:  Running,
		Started: pod.CreationTimestamp.Time,
	}, nil
}

//Delete ...
func (cl *service) Delete(name string) error {
	return deletePod(cl.clientset, cl.ns, name)
}

//Logs ...
func (cl *service) Logs(ctx context.Context, name string) (io.ReadCloser, error) {
	req := cl.clientset.CoreV1().Pods(cl.ns).GetLogs(name, &apiv1.PodLogOptions{
		Container:  "browser",
		Follow:     true,
		Previous:   false,
		Timestamps: false,
	})
	return req.Stream(ctx)
}

func (cl *service) BuildPod(layout ServiceSpec) *apiv1.Pod {
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
						Privileged:   layout.Template.Privileged,
						Capabilities: getCapabilities(layout.Template.Capabilities),
					},
					Env:             layout.Template.Spec.EnvVars,
					Ports:           getBrowserPorts(),
					Resources:       layout.Template.Spec.Resources,
					VolumeMounts:    getVolumeMounts(layout.Template.Spec.VolumeMounts),
					ImagePullPolicy: apiv1.PullIfNotPresent,
				},
				{
					Name:  "seleniferous",
					Image: cl.proxyImage,
					Ports: getSidecarPorts(cl.svcPort),
					Command: []string{
						"/seleniferous", "--listhen-port", cl.svcPort.StrVal, "--proxy-default-path", path.Join(layout.Template.Path, "session"), "--idle-timeout", cl.idleTimeout.String(), "--namespace", cl.ns,
					},
					ImagePullPolicy: apiv1.PullIfNotPresent,
				},
			},
			Volumes:          getVolumes(layout.Template.Volumes),
			NodeSelector:     layout.Template.Spec.NodeSelector,
			HostAliases:      layout.Template.Spec.HostAliases,
			RestartPolicy:    apiv1.RestartPolicyNever,
			Affinity:         &layout.Template.Spec.Affinity,
			DNSConfig:        &layout.Template.Spec.DNSConfig,
			Tolerations:      layout.Template.Spec.Tolerations,
			ImagePullSecrets: getImagePullSecretList(cl.imagePullSecretName),
			SecurityContext:  getSecurityContext(layout.Template.RunAs),
		},
	}

	if layout.RequestedCapabilities.Video {
		videoContainer := apiv1.Container{
			Name: "video",
			Image: cl.videoImage,
			Ports: getVideoPorts(),
			Command: []string{},
			Env: layout.Template.Spec.EnvVars,
			VolumeMounts: getVolumeMounts(layout.Template.Spec.VolumeMounts),
			ImagePullPolicy: apiv1.PullIfNotPresent,
		}
		pod.Spec.Containers = append(pod.Spec.Containers, videoContainer)
	}
	return pod
}

type quota struct {
	ns        string
	clientset kubernetes.Interface
}

//Create ...
func (cl quota) Create(limit int64) (Quota, error) {
	context := context.Background()
	quantity, err := resource.ParseQuantity(strconv.FormatInt(limit, 10))
	if err != nil {
		return Quota{}, fmt.Errorf("failed to parse limit amount")
	}
	quota := &apiv1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:   quotaName,
			Labels: map[string]string{label: "quota"},
		},
		Spec: apiv1.ResourceQuotaSpec{
			Hard: map[apiv1.ResourceName]resource.Quantity{apiv1.ResourcePods: quantity},
		},
	}
	quota, err = cl.clientset.CoreV1().ResourceQuotas(cl.ns).Create(context, quota, metav1.CreateOptions{})
	if err != nil {
		return Quota{}, fmt.Errorf("failed to create resourceQuota")
	}
	return Quota{
		Name:            quota.GetName(),
		CurrentMaxLimit: quota.Spec.Hard.Pods().Value(),
	}, nil
}

func (cl quota) Get() (Quota, error) {
	context := context.Background()
	quota, err := cl.clientset.CoreV1().ResourceQuotas(cl.ns).Get(context, quotaName, metav1.GetOptions{})
	if err != nil {
		return Quota{}, fmt.Errorf("quota not found")
	}

	return Quota{
		Name:            quota.GetName(),
		CurrentMaxLimit: quota.Spec.Hard.Pods().Value(),
	}, nil
}

//Update ...
func (cl quota) Update(limit int64) (Quota, error) {
	context := context.Background()
	quantity, err := resource.ParseQuantity(strconv.FormatInt(limit, 10))
	if err != nil {
		return Quota{}, fmt.Errorf("failed to parse limit amount")
	}
	rq := &apiv1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:   quotaName,
			Labels: map[string]string{label: "quota"},
		},
		Spec: apiv1.ResourceQuotaSpec{
			Hard: map[apiv1.ResourceName]resource.Quantity{apiv1.ResourcePods: quantity},
		},
	}
	quota, err := cl.clientset.CoreV1().ResourceQuotas(cl.ns).Update(context, rq, metav1.UpdateOptions{})
	if err != nil {
		return Quota{}, fmt.Errorf("resourse quota update error: %v", err)
	}

	return Quota{
		Name:            quota.GetName(),
		CurrentMaxLimit: rq.Spec.Hard.Pods().Value(),
	}, err
}

func deletePod(clientset kubernetes.Interface, namespace, name string) error {
	context := context.Background()

	return clientset.CoreV1().Pods(namespace).Delete(context, name, metav1.DeleteOptions{
		GracePeriodSeconds: pointer.Int64Ptr(15),
	})
}

func getBrowserPorts() []apiv1.ContainerPort {
	port := []apiv1.ContainerPort{}
	fn := func(name string, value int) {
		port = append(port, apiv1.ContainerPort{Name: name, ContainerPort: int32(value)})
	}

	fn("vnc", browserPorts.vnc.IntValue())
	fn("selenium", browserPorts.selenium.IntValue())
	fn("video", browserPorts.video.IntValue())

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

func getVideoPorts() []apiv1.ContainerPort {
	port := []apiv1.ContainerPort{}
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

func getRequestedCapabilities(annotations map[string]string) map[string]string {
	if k, ok := annotations["capabilities"]; ok {
		capabilities := make(map[string]string)
		err := json.Unmarshal([]byte(k), &capabilities)
		if err == nil {
			return capabilities
		}
	}
	return nil
}

func getVolumeMounts(mounts []apiv1.VolumeMount) []apiv1.VolumeMount {
	vm := []apiv1.VolumeMount{
		{
			Name:      "dshm",
			MountPath: "/dev/shm",
		},
	}
	if mounts != nil {
		vm = append(vm, mounts...)
	}
	return vm
}

func getVolumes(volumes []apiv1.Volume) []apiv1.Volume {
	v := []apiv1.Volume{
		{
			Name: "dshm",
			VolumeSource: apiv1.VolumeSource{
				EmptyDir: &apiv1.EmptyDirVolumeSource{
					Medium: apiv1.StorageMediumMemory,
				},
			},
		},
	}
	if volumes != nil {
		v = append(v, volumes...)
	}
	return v
}

func getCapabilities(caps []apiv1.Capability) *apiv1.Capabilities {
	if len(caps) > 0 {
		return &apiv1.Capabilities{Add: caps}
	}
	return nil
}

func getSecurityContext(runAsOptions RunAsOptions) *apiv1.PodSecurityContext {
	secContext := &apiv1.PodSecurityContext{}
	if runAsOptions.RunAsUser != nil {
		secContext.RunAsUser = runAsOptions.RunAsUser
	}
	if runAsOptions.RunAsGroup != nil {
		secContext.RunAsGroup = runAsOptions.RunAsGroup
	}
	return secContext
}

func waitForService(u url.URL, t time.Duration) error {
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
		return fmt.Errorf("no responce after %v", t)
	case <-up:
	}
	return nil
}
