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
func (s *WorkerSet) GetWorker(i int) *v1.Pod {
	p := *s.PodTemplate
	p.Name = fmt.Sprintf("%s-%d", p.Name, i)
	p.Spec.Hostname = fmt.Sprintf("%s-%d", p.Spec.Hostname, i)
	containers := make([]v1.Container, len(p.Spec.Containers))
	for i, c := range p.Spec.Containers {
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
		containers[i] = c
	}
	p.Spec.Containers = containers
	return &p
}
