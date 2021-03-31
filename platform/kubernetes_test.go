package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	testcore "k8s.io/client-go/testing"

	"github.com/alcounit/selenosis/selenium"
	"gotest.tools/assert"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
)

func TestErrorsOnServiceCreate(t *testing.T) {
	tests := map[string]struct {
		ns        string
		podName   string
		layout    *ServiceSpec
		eventType watch.EventType
		podPhase  apiv1.PodPhase
		err       error
	}{
		"Verify platform error on pod startup phase PodSucceeded": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Added,
			podPhase:  apiv1.PodSucceeded,
			err:       errors.New("pod is not ready after creation: pod exited early with status Succeeded"),
		},
		"Verify platform error on pod startup phase PodFailed": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Added,
			podPhase:  apiv1.PodFailed,
			err:       errors.New("pod is not ready after creation: pod exited early with status Failed"),
		},
		"Verify platform error on pod startup phase PodUnknown": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Added,
			podPhase:  apiv1.PodUnknown,
			err:       errors.New("pod is not ready after creation: couldn't obtain pod state"),
		},
		"Verify platform error on pod startup phase Unknown": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Added,
			err:       errors.New("pod is not ready after creation: pod has unknown status"),
		},
		"Verify platform error on pod startup event Error": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Error,
			podPhase:  apiv1.PodUnknown,
			err:       errors.New("pod is not ready after creation: received error while watching pod: /, Kind="),
		},
		"Verify platform error on pod startup event Deleted": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Deleted,
			podPhase:  apiv1.PodUnknown,
			err:       errors.New("pod is not ready after creation: pod was deleted before becoming available"),
		},
		"Verify platform error on pod startup event Unknown": {
			ns:      "selenosis",
			podName: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			layout: &ServiceSpec{
				SessionID: "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
				RequestedCapabilities: selenium.Capabilities{
					VNC: true,
				},
				Template: &BrowserSpec{
					BrowserName:    "chrome",
					BrowserVersion: "85.0",
					Image:          "selenoid/vnc:chrome_85.0",
					Path:           "/",
				},
			},
			eventType: watch.Bookmark,
			podPhase:  apiv1.PodSucceeded,
			err:       errors.New("pod is not ready after creation: received unknown event type BOOKMARK while watching pod"),
		},
	}

	for name, test := range tests {

		t.Logf("TC: %s", name)

		mock := fake.NewSimpleClientset()
		watcher := watch.NewFakeWithChanSize(1, false)
		mock.PrependWatchReactor("pods", testcore.DefaultWatchReactor(watcher, nil))
		watcher.Action(test.eventType, &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: test.podName,
			},
			Status: apiv1.PodStatus{
				Phase: test.podPhase,
			},
		})

		client := &Client{
			ns:        test.ns,
			clientset: mock,
			service: &service{
				ns:        test.ns,
				clientset: mock,
			},
		}

		_, err := client.Service().Create(test.layout)

		assert.Equal(t, test.err.Error(), err.Error())
	}
}

func TestPodDelete(t *testing.T) {
	tests := map[string]struct {
		ns           string
		createPod    string
		deletePod    string
		browserImage string
		proxyImage   string
		err          error
	}{
		"Verify platform deletes running pod": {
			ns:           "selenosis",
			createPod:    "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			deletePod:    "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			browserImage: "selenoid/vnc:chrome_85.0",
			proxyImage:   "alcounit/seleniferous:latest",
		},
		"Verify platform delete return error": {
			ns:           "selenosis",
			createPod:    "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911",
			deletePod:    "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144912",
			browserImage: "selenoid/vnc:chrome_85.0",
			proxyImage:   "alcounit/seleniferous:latest",
			err:          errors.New(`pods "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144912" not found`),
		},
	}

	for name, test := range tests {

		t.Logf("TC: %s", name)

		mock := fake.NewSimpleClientset()

		client := &Client{
			ns:        test.ns,
			clientset: mock,
			service: &service{
				ns:        test.ns,
				clientset: mock,
			},
		}

		ctx := context.Background()
		_, err := mock.CoreV1().Pods(test.ns).Create(ctx, &apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: test.createPod,
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Name:  "browser",
						Image: test.browserImage,
					},
					{
						Name:  "seleniferous",
						Image: test.proxyImage,
					},
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create fake pod: %v", err)
		}

		err = client.Service().Delete(test.deletePod)

		if err != nil {
			assert.Equal(t, test.err.Error(), err.Error())
		} else {
			assert.Equal(t, test.err, err)
		}
	}

}

func TestListPods(t *testing.T) {
	tests := map[string]struct {
		ns       string
		svc      string
		podNames []string
		podData  []struct {
			podName   string
			podPhase  apiv1.PodPhase
			podStatus ServiceStatus
		}
		podPhase     []apiv1.PodPhase
		podStatus    []ServiceStatus
		labels       map[string]string
		browserImage string
		proxyImage   string
		err          error
	}{
		"Verify platform returns running pods that match selector": {
			ns:           "selenosis",
			svc:          "selenosis",
			podNames:     []string{"chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144911", "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144912", "chrome-85-0-de44c3c4-1a35-412b-b526-f5da802144913"},
			podPhase:     []apiv1.PodPhase{apiv1.PodRunning, apiv1.PodPending, apiv1.PodFailed},
			podStatus:    []ServiceStatus{Running, Pending, Unknown},
			labels:       map[string]string{"selenosis.app.type": "browser"},
			browserImage: "selenoid/vnc:chrome_85.0",
			proxyImage:   "alcounit/seleniferous:latest",
		},
	}

	for name, test := range tests {

		t.Logf("TC: %s", name)

		mock := fake.NewSimpleClientset()
		client := &Client{
			ns:        test.ns,
			svc:       test.svc,
			svcPort:   intstr.FromString("4445"),
			clientset: mock,
		}

		for i, name := range test.podNames {
			ctx := context.Background()
			_, err := mock.CoreV1().Pods(test.ns).Create(ctx, &apiv1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   name,
					Labels: test.labels,
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "browser",
							Image: test.browserImage,
						},
						{
							Name:  "seleniferous",
							Image: test.proxyImage,
						},
					},
				},
				Status: apiv1.PodStatus{
					Phase: test.podPhase[i],
				},
			}, metav1.CreateOptions{})

			if err != nil {
				t.Logf("failed to create fake pod: %v", err)
			}
		}

		state, err := client.State()
		if err != nil {
			t.Fatalf("Failed to list pods %v", err)
		}

		for i, pod := range state.Services {
			assert.Equal(t, pod.SessionID, test.podNames[i])

			u := &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(fmt.Sprintf("%s.%s", test.podNames[i], test.svc), "4445"),
			}

			assert.Equal(t, pod.URL.String(), u.String())

			assert.Equal(t, pod.Status, test.podStatus[i])
		}
	}
}
