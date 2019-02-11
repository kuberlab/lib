package kubernetes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/kuberlab/lib/pkg/utils"
	api_v1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	ReasonInsufficient = "insufficient"
	ReasonError        = "error"
)

const (
	ResourceNvidiaGPU = "nvidia.com/gpu"
)

type ComponentState struct {
	Type           string `json:"type"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	ReasonCode     string
	ResourceStates []*ResourceState `json:"resource_states"`
}

type ResourceState struct {
	Name      string                      `json:"name"`
	Status    string                      `json:"status"`
	Resources api_v1.ResourceRequirements `json:"resources"`
	Events    []api_v1.Event              `json:"events"`
}

var insufficientPattern = regexp.MustCompile(`nodes are available.*(Insufficient .*?\(.*?\)).*`)
var mountFailedPattern = regexp.MustCompile(`.*(MountVolume.*failed.*)`)
var gitRepoPattern = regexp.MustCompile(`([\w]+@)+([\w\d-\.]+)[:/]([\w\d-_\./]+)|(\w+://)(.+@)*([\w\d\.]+)(:[\d]+){0,1}/*(.*)`)

func NvidiaGPU(reqs *api_v1.ResourceList) *resource.Quantity {
	if reqs != nil {
		if val, ok := (*reqs)[ResourceNvidiaGPU]; ok {
			return &val
		}
		if val, ok := (*reqs)[api_v1.ResourceNvidiaGPU]; ok {
			return &val
		}
	}
	return &resource.Quantity{}
}

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
		reason, resState, code, err := DetermineResourceState(pod, client)
		if err != nil {
			return nil, err
		}
		state.Reason = reason
		state.ReasonCode = code
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

func DetermineResourceState(pod api_v1.Pod, client *kubernetes.Clientset) (reason string, resourceState *ResourceState, code string, err error) {
	resourceState = &ResourceState{
		Name:      pod.Name,
		Status:    GetPodState(pod),
		Events:    []api_v1.Event{},
		Resources: sumResourceRequests(pod),
	}

	if pod.Status.Phase == api_v1.PodRunning {
		return
	}

	events, err := client.CoreV1().Events(pod.Namespace).Search(scheme.Scheme, &pod)
	if err != nil {
		return "", nil, "", err
	}

	resourceState.Events = events.Items

	if pod.Status.Phase == api_v1.PodRunning {
		return
	}

	for _, e := range events.Items {
		if insufficientPattern.MatchString(e.Message) {
			groups := insufficientPattern.FindStringSubmatch(e.Message)
			if len(groups) > 1 {
				reason = groups[1]
				code = ReasonInsufficient
			}
		}
		if mountFailedPattern.MatchString(e.Message) {
			groups := mountFailedPattern.FindStringSubmatch(e.Message)
			if len(groups) > 1 {
				reason = groups[1]
				code = ReasonError
			}
		}
	}

	for i, init := range pod.Status.InitContainerStatuses {
		if init.State.Waiting != nil {
			event := api_v1.Event{
				Message:        init.State.Waiting.Message,
				Reason:         init.State.Waiting.Reason,
				Count:          1,
				Type:           "Warning",
				FirstTimestamp: meta_v1.Now(),
				LastTimestamp:  meta_v1.Now(),
			}
			resourceState.Events = append(resourceState.Events, event)
		}
		if init.State.Terminated != nil || init.LastTerminationState.Terminated != nil {
			var terminated *api_v1.ContainerStateTerminated
			if init.LastTerminationState.Terminated != nil {
				terminated = init.LastTerminationState.Terminated
			} else {
				terminated = init.State.Terminated
			}
			if terminated.ExitCode != 0 {
				event := api_v1.Event{
					Count:          1,
					Type:           "Warning",
					FirstTimestamp: meta_v1.Now(),
					LastTimestamp:  meta_v1.Now(),
					Source: api_v1.EventSource{
						Component: "mlboard",
					},
				}
				if terminated.ExitCode == 39 {
					event.Reason = fmt.Sprintf("%v: %v", terminated.Message, terminated.Reason)
					matches := gitRepoPattern.FindAllStringSubmatch(strings.Join(pod.Spec.InitContainers[i].Command, " "), -1)
					repos := make([]string, 0)
					for _, groups := range matches {
						if len(groups) > 1 {
							repos = append(repos, groups[0])
						}
					}
					msg := "Failed get access to the repo(s): [" + strings.Join(repos, ",") + "]"
					event.Message = msg
				} else {
					event.Reason = terminated.Reason
					event.Message = terminated.Message
				}
				resourceState.Events = append(resourceState.Events, event)
				reason = event.Message
				code = ReasonError
			}

		}
	}
	return
}

func GetPodState(pod api_v1.Pod) string {
	// Pod may be in Running phase even if the termination began already.
	// So first check for terminating.
	if isTerminating(pod) {
		return "Terminating"
	}

	if len(pod.Status.ContainerStatuses) < 1 {
		return string(pod.Status.Phase)
	}

	containerState := pod.Status.ContainerStatuses[0].State
	if pod.Status.Phase == api_v1.PodRunning && containerState.Terminated == nil && containerState.Waiting == nil {
		return string(pod.Status.Phase)
	}

	if containerState.Terminated != nil {
		return containerState.Terminated.Reason
	}
	if containerState.Waiting != nil {
		return containerState.Waiting.Reason
	}

	return string(pod.Status.Phase)
}

// isTerminating returns true if pod's DeletionTimestamp has been set
func isTerminating(pod api_v1.Pod) bool {
	return pod.DeletionTimestamp != nil
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
