package kubernetes

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"fmt"
	"strings"
	"github.com/Sirupsen/logrus"
)

type ComponentState struct {
	Type string
	Name string
	Status string
	ResourceStates []*ResourceState
}

type ResourceState struct {
	Name string
	Replica string
	Reason string
}

func GetComponentState(client *kubernetes.Clientset, obj runtime.Object, type_ string) (*ComponentState, error) {
	var namespace, name string
	var pods = make([]api_v1.Pod, 0)
	switch v := obj.(type) {
	case *api_v1.Pod:
		pods = append(pods, *v)
		namespace = v.Namespace
		name = v.Name
	case *extv1beta1.Deployment:
		ps, err := client.Pods(v.Namespace).List(labelSelector(v.Spec.Template.Labels))
		if err != nil {
			return nil, err
		}
		pods = append(pods, ps.Items...)
		namespace = v.Namespace
		name = v.Name
	case *WorkerSet:
		ps, err := client.Pods(v.Namespace).List(labelSelector(v.PodTemplate.Labels))
		if err != nil {
			return nil, err
		}
		pods = append(pods, ps.Items...)
		namespace = v.Namespace
		name = v.TaskName + "-" + v.JobID
	}

	state := &ComponentState{Type: type_, Name: name}
	for _, pod := range pods {
		events, err := client.Events(namespace).Search(api.Scheme, &pod)
		if err != nil {
			return nil, err
		}
		for _, e := range events.Items {
			if e.Type == "Warning" {
				logrus.Warningf("Reason: %v, Message: %v", e.Reason, e.Message)
			}
		}
	}
	return state, nil
}

func labelSelector(labels map[string]string) meta_v1.ListOptions {
	var labelSelector = make([]string, 0)
	for k, v := range labels {
		labelSelector = append(labelSelector, fmt.Sprintf("%v=%v", k, v))
	}
	return meta_v1.ListOptions{LabelSelector: strings.Join(labelSelector, ",")}
}

