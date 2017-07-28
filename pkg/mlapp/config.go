package mlapp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/kuberlab/lib/pkg/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	KUBELAB_WS_LABEL    = "kuberlab.io/workspace"
	KUBELAB_WS_ID_LABEL = "kuberlab.io/workspace-id"
)

type Config struct {
	Kind      string `json:"kind"`
	Meta      `json:"metadata"`
	Spec      `json:"spec,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

type Meta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

type Spec struct {
	Tasks          []Task    `json:"tasks,omitempty"`
	Uix            []Uix     `json:"uix,omitempty"`
	Serving        []Serving `json:"serving,omitempty"`
	Volumes        []Volume  `json:"volumes,omitempty"`
	PythonPackages []string  `json:"pythonPackages,omitempty"`
}
type Resource struct {
	Resources  *ResourceRequest `json:"resources,omitempty"`
	Images     Images           `json:"images"`
	Command    string           `json:"command"`
	WorkDir    string           `json:"workDir"`
	RawArgs    string           `json:"args,omitempty"`
	Env        []Env            `json:"env"`
	Volumes    []VolumeMount    `json:"volumes"`
	NodesLabel string           `json:"nodes"`
}

func (r Resource) Image() string {
	if r.Resources != nil && r.Resources.Accelerators.GPU > 0 {
		if len(r.Images.GPU) == 0 {
			return r.Images.CPU
		}
		return r.Images.GPU
	}
	return r.Images.CPU
}

type Uix struct {
	Meta        `json:",inline"`
	DisplayName string `json:"displayName,omitempty"`
	Ports       []Port `json:"ports,omitempty"`
	Resource    `json:",inline"`
	FrontAPI    string `json:"front_api,omitempty"`
}

type Serving struct {
	Uix
	TaskName string `json:"taskName,inline"`
	Build    string `json:"build,inline"`
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       int32  `json:"port,omitempty"`
	TargetPort int32  `targetPort:"name,omitempty"`
}

type Task struct {
	Meta           `json:",inline"`
	Version        string         `json:"version,omitempty"`
	TimeoutMinutes uint           `json:"timeoutMinutes,omitempty"`
	Resources      []TaskResource `json:"resources"`
}

type TaskResource struct {
	Meta            `json:",inline"`
	Replicas        uint   `json:"replicas"`
	RestartPolicy   string `json:"restartPolicy"`
	MaxRestartCount int    `json:"maxRestartCount"`
	AllowFail       bool   `json:"allowFail"`
	Port            int32  `json:"port,omitempty"`
	DoneCondition   string `json:"doneCondition,omitempty"`
	Resource        `json:",inline"`
}

type Images struct {
	CPU string `json:"cpu,omitempty"`
	GPU string `json:"gpu,omitempty"`
}

type Env struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type TaskResourceSpec struct {
	PodsNumber    int
	DoneCondition string
	AllowFail     bool
	TaskName      string
	ResourceName  string
	Resource      *kubernetes.KubeResource
}

func (c *Config) SetClusterStorage(mapping func(name string) (*VolumeSource, error)) error {
	for i, v := range c.Spec.Volumes {
		if len(v.ClusterStorage) > 0 {
			if s, err := mapping(v.ClusterStorage); err == nil {
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

func (c *Config) LibVolume() *Volume {
	for _, v := range c.Volumes {
		if v.IsLibDir {
			res := v
			return &res
		}
	}
	return nil
}

func (c *Config) KubeVolumesSpec(mounts []VolumeMount) ([]v1.Volume, []v1.VolumeMount, error) {
	added := make(map[string]bool)
	kvolumes := make([]v1.Volume, 0)
	kvolumesMount := make([]v1.VolumeMount, 0)
	for _, m := range mounts {
		v := c.VolumeByName(m.Name)
		if v == nil {
			return nil, nil, fmt.Errorf("Source '%s' not found", m.Name)
		}
		if _, ok := added[v.Name]; !ok {
			kvolumes = append(kvolumes, v.V1Volume())
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
			subPath = filepath.Join(subPath, m.SubPath)
		}
		kvolumesMount = append(kvolumesMount, v1.VolumeMount{
			Name:      m.Name,
			SubPath:   subPath,
			MountPath: mountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	return kvolumes, kvolumesMount, nil
}

type ConfigOption func(*Config) (*Config, error)

func NewConfig(data []byte, options ...ConfigOption) (*Config, error) {
	var c Config
	err := yaml.Unmarshal(data, &c)
	if err != nil {
		return nil, err
	}
	return ApplyConfigOptions(&c, options...)
}
func ApplyConfigOptions(c *Config, options ...ConfigOption) (res *Config, err error) {
	res = c
	for _, o := range options {
		res, err = o(res)
		if err != nil {
			return
		}
	}
	return
}

func SetClusterStorageOption(mapping func(name string) (*VolumeSource, error)) ConfigOption {
	return func(c *Config) (*Config, error) {
		err := c.SetClusterStorage(mapping)
		return c, err
	}
}

func BuildOption(workspaceID, workspaceName, appName string) func(c *Config) (res *Config, err error) {
	return func(c *Config) (res *Config, err error) {
		res = c
		res.Name = appName
		res.Workspace = workspaceName
		if res.Labels == nil{
			res.Labels = make(map[string]string)
		}
		res.Labels[KUBELAB_WS_LABEL] = workspaceName
		res.Labels[KUBELAB_WS_ID_LABEL] = workspaceID
		for i := range res.Uix{
			res.Uix[i].FrontAPI = fmt.Sprintf("/api/v1/ml2-proxy/%s/%s/%s/",
				workspaceName,appName,res.Uix[i].Name)
		}
		return
	}
}

func joinMaps(dest map[string]string, srcs ...map[string]string) {
	for _, src := range srcs {
		for k, v := range src {
			dest[k] = v
		}
	}
}

func (c Config) ToYaml() ([]byte, error) {
	return yaml.Marshal(c)
}
