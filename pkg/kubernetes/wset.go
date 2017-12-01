package kubernetes

import (
	"fmt"
	"strconv"

	"github.com/kuberlab/lib/pkg/utils"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api/v1"
)

type WorkerSet struct {
	AppName      string
	Namespace    string
	TaskName     string
	ResourceName string
	JobID        string
	Replicas     int
	MaxRestarts  int
	AllowFail    bool
	PodTemplate  *v1.Pod
}

func (ws WorkerSet) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (ws *WorkerSet) LabelSelector() meta_v1.ListOptions {
	return meta_v1.ListOptions{LabelSelector: fmt.Sprintf(
		"kuberlab.io/job-id==%s,kuberlab.io/component==%s-%s",
		ws.JobID,
		utils.KubeNamespaceEncode(ws.TaskName),
		utils.KubeNamespaceEncode(ws.ResourceName),
	)}
}

func (ws *WorkerSet) GetWorker(i int, node string, restart int) *v1.Pod {
	p := *ws.PodTemplate
	p.Name = fmt.Sprintf("%s-%d", p.Name, i)
	p.Spec.Hostname = fmt.Sprintf("%s-%d", p.Spec.Hostname, i)
	annotations := make(map[string]string)
	utils.JoinMaps(annotations, p.Annotations)
	annotations["restart"] = strconv.Itoa(restart)
	p.Annotations = annotations
	containers := make([]v1.Container, len(p.Spec.Containers))
	if node != "" {
		labels := make(map[string]string)
		utils.JoinMaps(labels, p.Labels)
		labels["kuberlab.io/ml-node"] = node
		p.Labels = labels
		p.Spec.NodeSelector = map[string]string{"kuberlab.io/ml-node": node}
	}
	for j, c := range p.Spec.Containers {
		env := make([]v1.EnvVar, 0, len(c.Env))
		for _, e := range c.Env {
			if e.Name != "REPLICA_INDEX" {
				env = append(env, e)
			}
		}
		env = append(env, v1.EnvVar{
			Name:  "REPLICA_INDEX",
			Value: strconv.Itoa(i),
		})
		c.Env = env
		containers[j] = c
	}
	p.Spec.Containers = containers
	return &p
}
