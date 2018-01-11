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
	var name string
	var pods = make([]api_v1.Pod, 0)
	switch v := obj.(type) {
	case *api_v1.Pod:
		pods = append(pods, *v)
		name = v.Name
	case *extv1beta1.Deployment:
		ps, err := client.CoreV1().Pods(v.Namespace).List(labelSelector(v.Spec.Template.Labels))
		if err != nil {
			return nil, err
		}
		pods = append(pods, ps.Items...)
		name = v.Name
	case []*WorkerSet:
		for _, wset := range v {
			logrus.Debugf("Using labelselector = %v", labelSelector(wset.PodTemplate.Labels).LabelSelector)
			ps, err := client.CoreV1().Pods(wset.Namespace).List(labelSelector(wset.PodTemplate.Labels))
			if err != nil {
				return nil, err
			}
			pods = append(pods, ps.Items...)
			name = wset.TaskName + "-" + wset.JobID
		}
	}

	state := &ComponentState{Type: type_, Name: name, ResourceStates: make([]*ResourceState, 0)}

	for _, pod := range pods {
		reason, resState, err := DetermineResourceState(pod, client)
		if err != nil {
			return nil, err
		}
		state.Reason = reason
		state.ResourceStates = append(state.ResourceStates, resState)
	}

	return state, nil
}

func SetOverallStatus(state *ComponentState) {
	statusMap := make(map[string]int, 0)
	for _, resState := range state.ResourceStates {
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
}

func DetermineResourceState(pod api_v1.Pod, client *kubernetes.Clientset) (reason string, resourceState *ResourceState, err error) {
	resourceState = &ResourceState{
		Name:      pod.Name,
		Status:    string(pod.Status.Phase),
		Events:    []api_v1.Event{},
		Resources: sumResourceRequests(pod),
	}
	events, err := client.CoreV1().Events(pod.Namespace).Search(scheme.Scheme, &pod)
	if err != nil {
		return "", nil, err
	}

	for _, e := range events.Items {
		if e.Type == "Warning" || pod.Status.Phase != api_v1.PodRunning {
			if len(resourceState.Events) < 1 {
				resourceState.Events = events.Items
			}
		}
		if insufficientPattern.MatchString(e.Message) {
			groups := insufficientPattern.FindStringSubmatch(e.Message)
			if len(groups) > 1 {
				reason = groups[1]
			}
		}
	}
	return
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
