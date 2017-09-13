package kubernetes

import (
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/kuberlab/lib/pkg/utils"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type ComponentState struct {
	Type           string
	Name           string
	Status         string
	ResourceStates []*ResourceState
}

type ResourceState struct {
	Name   string
	Status string
	Reason string
}

func GetComponentState(client *kubernetes.Clientset, obj interface{}, type_ string) (*ComponentState, error) {
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
	case []*WorkerSet:
		for _, wset := range v {
			logrus.Infof("Using labelselector = %v", labelSelector(wset.PodTemplate.Labels).LabelSelector)
			ps, err := client.Pods(wset.Namespace).List(labelSelector(wset.PodTemplate.Labels))
			if err != nil {
				return nil, err
			}
			pods = append(pods, ps.Items...)
			namespace = wset.Namespace
			name = wset.TaskName + "-" + wset.JobID
		}
	}

	state := &ComponentState{Type: type_, Name: name, ResourceStates: make([]*ResourceState, 0)}
	statusMap := make(map[string]int, 0)

	for _, pod := range pods {
		resState := &ResourceState{Name: pod.Name, Status: string(pod.Status.Phase)}
		events, err := client.Events(namespace).Search(api.Scheme, &pod)
		if err != nil {
			return nil, err
		}

		reason := []string{}
		for _, e := range events.Items {
			if e.Type == "Warning" || pod.Status.Phase != api_v1.PodRunning {
				reason = append(reason, fmt.Sprintf("%v: %v", e.Reason, e.Message))
			}
		}
		resState.Reason = strings.Join(reason, "\n")
		state.ResourceStates = append(state.ResourceStates, resState)

		if _, ok := statusMap[resState.Status]; ok {
			statusMap[resState.Status] = statusMap[resState.Status] + 1
		} else {
			statusMap[resState.Status] = 1
		}
	}

	// Set overall status
	overallStatus := utils.RankByWordCount(statusMap)
	if len(overallStatus) > 0 {
		state.Status = overallStatus[0].Key
	} else {
		state.Status = "Unknown"
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
