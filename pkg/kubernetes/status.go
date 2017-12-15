package kubernetes

import (
	"fmt"
	"strings"

	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/kuberlab/lib/pkg/utils"
	api_v1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

type ComponentState struct {
	Type           string           `json:"type"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	Reason         string           `json:"reason"`
	ResourceStates []*ResourceState `json:"resource_states"`
}

type ResourceState struct {
	Name      string                      `json:"name"`
	Status    string                      `json:"status"`
	Resources api_v1.ResourceRequirements `json:"resources"`
	Events    []api_v1.Event              `json:"events"`
}

var insufficientPattern = regexp.MustCompile(`No nodes are available.*(Insufficient .*?\(.*?\)).*`)

func GetComponentState(client *kubernetes.Clientset, obj interface{}, type_ string) (*ComponentState, error) {
	var namespace, name string
	var pods = make([]api_v1.Pod, 0)
	switch v := obj.(type) {
	case *api_v1.Pod:
		pods = append(pods, *v)
		namespace = v.Namespace
		name = v.Name
	case *extv1beta1.Deployment:
		ps, err := client.CoreV1().Pods(v.Namespace).List(labelSelector(v.Spec.Template.Labels))
		if err != nil {
			return nil, err
		}
		pods = append(pods, ps.Items...)
		namespace = v.Namespace
		name = v.Name
	case []*WorkerSet:
		for _, wset := range v {
			logrus.Debugf("Using labelselector = %v", labelSelector(wset.PodTemplate.Labels).LabelSelector)
			ps, err := client.CoreV1().Pods(wset.Namespace).List(labelSelector(wset.PodTemplate.Labels))
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
		resState := &ResourceState{
			Name:      pod.Name,
			Status:    string(pod.Status.Phase),
			Events:    []api_v1.Event{},
			Resources: sumResourceRequests(pod),
		}
		events, err := client.CoreV1().Events(namespace).Search(scheme.Scheme, &pod)
		if err != nil {
			return nil, err
		}

		for _, e := range events.Items {
			if e.Type == "Warning" || pod.Status.Phase != api_v1.PodRunning {
				if len(resState.Events) < 1 {
					resState.Events = events.Items
				}
			}
			if insufficientPattern.MatchString(e.Message) {
				groups := insufficientPattern.FindStringSubmatch(e.Message)
				if len(groups) > 1 {
					state.Reason = groups[1]
				}
			}
		}
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

func sumResourceRequests(pod api_v1.Pod) api_v1.ResourceRequirements {
	req := api_v1.ResourceRequirements{
		Requests: make(api_v1.ResourceList),
		Limits:   make(api_v1.ResourceList),
	}

	var reqs = make(map[api_v1.ResourceName]*resource.Quantity)
	var limits = make(map[api_v1.ResourceName]*resource.Quantity)
	for _, container := range pod.Spec.Containers {
		for k, v := range container.Resources.Requests {
			if _, ok := reqs[k]; ok {
				reqs[k].Add(v)
			} else {
				vv := &v
				reqs[k] = vv.Copy()
			}
		}
		for k, v := range container.Resources.Limits {
			if _, ok := limits[k]; ok {
				limits[k].Add(v)
			} else {
				vv := &v
				limits[k] = vv.Copy()
			}
		}
	}

	for k, v := range reqs {
		req.Requests[k] = *v
	}
	for k, v := range limits {
		req.Limits[k] = *v
	}

	return req
}
