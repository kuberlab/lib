package mlapp

import (
	"fmt"
	kapi_v1 "k8s.io/client-go/pkg/api/v1"
	"path/filepath"
	"strings"
)

type Config struct {
	Kind      string `json:"kind"`
	Meta      `json:"metadata"`
	Spec      `json:"spec,omitempty"`
	Workspace string `json:"workspace"`
}

type Meta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type Spec struct {
	Tasks   []Task   `json:"tasks,omitempty"`
	Uix     []Uix    `json:"uix,omitempty"`
	Volumes []Volume `json:"volumes"`
}

type Uix struct {
	Meta        `json:",inline"`
	DisplayName string          `json:"displayName,omitempty"`
	Resources   ResourceRequest `json:"resources,omitempty"`
	Ports       []Port          `json:"ports,omitempty"`
	Volumes     []VolumeMount   `json:"volumes"`
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       uint   `json:"port,omitempty"`
	TargetPort uint   `targetPort:"name,omitempty"`
}

type Task struct {
	Meta      `json:",inline"`
	Resources []Resource `json:"resources"`
}

type Resource struct {
	Meta            `json:",inline"`
	Replicas        uint            `json:"replicas"`
	MinAvailable    uint            `json:"minAvailable"`
	RestartPolicy   string          `json:"restartPolicy"`
	MaxRestartCount uint            `json:"maxRestartCount"`
	Images          Images          `json:"images"`
	Command         string          `json:"command"`
	WorkDir         string          `json:"workDir"`
	Args            string          `json:"args,omitempty"`
	Env             []Env           `json:"env"`
	Resources       ResourceRequest `json:"resources"`
	Volumes         []VolumeMount   `json:"volumes"`
}

type Images struct {
	CPU string `json:"cpu,omitempty"`
	GPU string `json:"gpu,omitempty"`
}

type Env struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

func (c *Config) SetClusterStorage(mapping func(name string) (*VolumeSource, error)) error {
	for i, v := range c.Spec.Volumes {
		if len(v.ClusterStorage) > 0 {
			if s, err := mapping(v.ClusterStorage); err != nil {
				c.Spec.Volumes[i].VolumeSource = *s
			} else {
				return fmt.Errorf("Failed setup cluster storage '%s': %v", v.ClusterStorage, err)
			}
		}
	}
	return nil
}

func (c *Config) VolumeByName(name string) *Volume {
	for _, v := range c.Volumes {
		if v.Name == name {
			res := v
			return &res
		}
	}
	return nil
}
func (c *Config) KubeVolumesSpec(mounts []VolumeMount) ([]kapi_v1.Volume, []kapi_v1.VolumeMount, error) {
	added := make(map[string]string)
	names := make(map[string]string)
	kvolumes := make([]kapi_v1.Volume, 0)
	kvolumesMount := make([]kapi_v1.VolumeMount, 0)
	for _, m := range mounts {
		v := c.VolumeByName(m.Name)
		if v == nil {
			return nil, nil, fmt.Errorf("Source '%s' not found", m.Name)
		}
		id := v.GetBoundID()
		if duplicate, ok := added[id]; ok {
			if duplicate == m.Name {
				continue
			}
			names[m.Name] = duplicate
		} else {
			names[m.Name] = v.Name
			added[id] = m.Name
			kvolumes = append(kvolumes, v.v1Volume())
		}
		mountPath := v.MountPath
		if len(m.MountPath) > 0 {
			mountPath = m.MountPath
		}
		subPath := v.SubPath
		if strings.HasPrefix(subPath, "/") {
			subPath = strings.TrimPrefix(subPath, "/")
		} else if len(subPath) > 0 {
			subPath = c.Workspace + "/" + c.Name + "/" + subPath
		}
		if len(m.SubPath) > 0 {
			filepath.Join(subPath, m.SubPath)
		}
		kvolumesMount = append(kvolumesMount, kapi_v1.VolumeMount{
			Name:      names[m.Name],
			SubPath:   subPath,
			MountPath: mountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	return kvolumes, kvolumesMount, nil
}
