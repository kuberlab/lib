package mlapp

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api/v1"
)

type WorkerSet struct {
	AppName      string
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
func (s *WorkerSet) GetWorker(i int,restart int) *v1.Pod {
	p := *s.PodTemplate
	p.Name = fmt.Sprintf("%s-%d", p.Name, i)
	p.Spec.Hostname = fmt.Sprintf("%s-%d", p.Spec.Hostname, i)
	annotations := make(map[string]string)
	joinMaps(annotations,p.Annotations)
	annotations["restart"] = strconv.Itoa(restart)
	p.Annotations = annotations
	containers := make([]v1.Container, len(p.Spec.Containers))
	for j, c := range p.Spec.Containers {
		env := make([]v1.EnvVar, 0, len(c.Env))
		for _, e := range c.Env {
			if e.Name != "REPLICA_INDEX" {
				env = append(env, e)
			}
		}
		env = append(env, v1.EnvVar{
			Name:  "REPLICA_INDEX",
			Value: strconv.Itoa(j),
		})
		c.Env = env
		containers[j] = c
	}
	p.Spec.Containers = containers
	return &p
}

