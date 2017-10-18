package mlapp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	kuberlab "github.com/kuberlab/lib/pkg/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	KUBELAB_WS_LABEL    = "kuberlab.io/workspace"
	KUBELAB_WS_ID_LABEL = "kuberlab.io/workspace-id"
)

type Config struct {
	Kind        string `json:"kind"`
	Meta        `json:"metadata"`
	Spec        `json:"spec,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

func (c Config) GetAppID() string {
	return c.WorkspaceID + "-" + c.Name
}

type Meta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

type DeploymentBasedResource interface {
	Type() string
	GetName() string
	Deployment(client *kubernetes.Clientset, namespace string) (*extv1beta1.Deployment, error)
}

type Spec struct {
	Tasks                 []Task          `json:"tasks,omitempty"`
	Uix                   []Uix           `json:"uix,omitempty"`
	Serving               []Serving       `json:"serving,omitempty"`
	Volumes               []Volume        `json:"volumes,omitempty"`
	Packages              []Packages      `json:"packages,omitempty"`
	DefaultPackageManager string          `json:"package_manager,omitempty"`
	ClusterLimits         *ResourceReqLim `json:"cluster_limits,omitempty"`
	Secrets               []Secret        `json:"secrets,omitempty"`
}

type Secret struct {
	Name   string            `json:"name,omitempty"`
	Data   map[string]string `json:"data,omitempty"`
	Type   string            `json:"type,omitempty"`
	Mounts map[string]string `json:"mounts,omitempty"`
	Path   string            `json:"path,omitempty"`
}
type Packages struct {
	Names   []string `json:"names"`
	Manager string   `json:"manager"`
}

type Resource struct {
	Replicas   int              `json:"replicas"`
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

func (uix *Uix) Type() string {
	return "UIX"
}

func (uix *Uix) GetName() string {
	return uix.Name
}

func (uix *Uix) Deployment(client *kubernetes.Clientset, namespace string) (*extv1beta1.Deployment, error) {
	return client.ExtensionsV1beta1().Deployments(namespace).Get(uix.Name, meta_v1.GetOptions{})
}

type Serving struct {
	Uix      `json:",inline"`
	TaskName string `json:"taskName"`
	Build    string `json:"build"`
}

func (s *Serving) Type() string {
	return "Serving"
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
	RestartPolicy   string `json:"restartPolicy"`
	MaxRestartCount int    `json:"maxRestartCount"`
	AllowFail       bool   `json:"allowFail"`
	Port            int32  `json:"port,omitempty"`
	DoneCondition   string `json:"doneCondition,omitempty"`
	Resource        `json:",inline"`
}

type Images struct {
	CPU string `json:"cpu"`
	GPU string `json:"gpu"`
}

type Env struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TaskResourceSpec struct {
	PodsNumber    int
	DoneCondition string
	AllowFail     bool
	TaskName      string
	ResourceName  string
	NodeAllocator string
	Resource      *kuberlab.KubeResource
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

func (c *Config) SetupClusterStorage(mapping func(v Volume) (*VolumeSource, error)) error {
	for i, v := range c.Spec.Volumes {
		if s, err := mapping(v); err == nil {
			c.Spec.Volumes[i].VolumeSource = *s
		} else {
			return fmt.Errorf("Failed setup cluster storage '%s': %v", v.ClusterStorage, err)
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

func (c *Config) LibVolume() (*v1.Volume, *v1.VolumeMount) {
	for _, v := range c.Volumes {
		if v.IsLibDir {
			vols, mounts, err := c.KubeVolumesSpec(
				[]VolumeMount{
					{Name: v.Name, ReadOnly: false, MountPath: v.MountPath},
				},
			)
			if err != nil {
				return nil, nil
			}
			return &vols[0], &mounts[0]
		}
	}
	return nil, nil
}

type InitContainers struct {
	Image   string
	Command string
	Name    string
	Mounts  []v1.VolumeMount
}

func (c *Config) KubeInits(mounts []VolumeMount) ([]InitContainers, error) {
	inits := []InitContainers{}
	added := map[string]bool{}
	_, vmounts, err := getSecretVolumes(c.Secrets)
	if err != nil {
		return nil, err
	}
	for _, m := range mounts {
		added[m.Name] = true
		if _, ok := added[m.Name]; ok {
			continue
		}
		v := c.VolumeByName(m.Name)
		if v == nil {
			return nil, fmt.Errorf("Source '%s' not found", m.Name)
		}
		if v.GitRepo != nil && v.GitRepo.AccountId != "" {
			checkout := ""
			if v.GitRepo.Revision != "" {
				repo := getGitRepoName(v.GitRepo.Repository)
				checkout = fmt.Sprintf(" && cd /gitdata/%s && git checkout %", repo, v.GitRepo.Revision)
			}
			vmounts = append(vmounts, v1.VolumeMount{
				Name:      v.Name,
				MountPath: "/gitdata",
				ReadOnly:  false,
			})
			inits = append(inits, InitContainers{
				Mounts:  vmounts,
				Name:    m.Name,
				Image:   "kuberlab/board-init",
				Command: fmt.Sprintf(`['sh', '-c', 'cd /gitdata && git clone %s%s']`, v.GitRepo.Repository, checkout),
			})
		}

	}
	return inits, nil
}
func getGitRepoName(repo string) string {
	p := strings.Split(repo, "/")
	return strings.TrimSuffix(p[len(p)-1], ".git")
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
		if v.NFS != nil {
			if strings.HasPrefix(subPath, "/shared/") {
				subPath = strings.TrimPrefix(subPath, "/")
			} else if strings.HasPrefix(subPath, "/") {
				subPath = c.Workspace + "/" + c.WorkspaceID + "/" + strings.TrimPrefix(subPath, "/")
			} else if len(subPath) > 0 {
				subPath = c.Workspace + "/" + c.WorkspaceID + "/" + c.Name + "/" + subPath
			}
		}
		if len(m.SubPath) > 0 {
			subPath = filepath.Join(subPath, m.SubPath)
		}
		subPath = strings.TrimPrefix(subPath, "/")
		kvolumesMount = append(kvolumesMount, v1.VolumeMount{
			Name:      m.Name,
			SubPath:   subPath,
			MountPath: mountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	for len(c.Secrets) > 0 {
		v1, v2, err := getSecretVolumes(c.Secrets)
		if err != nil {
			return nil, nil, err
		}
		if len(v1) > 0 {
			kvolumes = append(kvolumes, v1...)
		}
		if len(v2) > 0 {
			kvolumesMount = append(kvolumesMount, v2...)
		}
	}
	return kvolumes, kvolumesMount, nil
}

func getSecretVolumes(secrets []Secret) ([]v1.Volume, []v1.VolumeMount, error) {
	kvolumes := make([]v1.Volume, 0)
	kvolumesMount := make([]v1.VolumeMount, 0)
	for _, s := range secrets {
		if len(s.Mounts) > 0 {
			items := make([]v1.KeyToPath, len(s.Mounts))
			i := 0
			for k, m := range s.Mounts {
				items[i] = v1.KeyToPath{
					Key:  k,
					Path: m,
				}
				i += 1
			}
			v := v1.Volume{
				Name: s.Name,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: s.Name,
						Items:      items,
					},
				},
			}
			kvolumes = append(kvolumes, v)
			kvolumesMount = append(kvolumesMount, v1.VolumeMount{
				Name:      s.Name,
				MountPath: s.Name,
				ReadOnly:  false,
			})
		}
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
	// init empty arrays
	if c.Volumes == nil {
		c.Volumes = []Volume{}
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	// init empty arrays for tasks
	if c.Spec.Tasks != nil {
		for i := range c.Spec.Tasks {
			if c.Spec.Tasks[i].Resources == nil {
				c.Spec.Tasks[i].Resources = []TaskResource{}
			}
			for j := range c.Spec.Tasks[i].Resources {
				if c.Spec.Tasks[i].Resources[j].Env == nil {
					c.Spec.Tasks[i].Resources[j].Env = []Env{}
				}
				if c.Spec.Tasks[i].Resources[j].Labels == nil {
					c.Spec.Tasks[i].Resources[j].Labels = map[string]string{}
				}
				if c.Spec.Tasks[i].Resources[j].Volumes == nil {
					c.Spec.Tasks[i].Resources[j].Volumes = []VolumeMount{}
				}
			}
		}
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

func LimitsOption(limits *ResourceReqLim) func(c *Config) (res *Config, err error) {
	return func(c *Config) (res *Config, err error) {
		res = c
		res.ClusterLimits = limits
		return
	}
}

func SetClusterStorageOption(mapping func(name string) (*VolumeSource, error)) ConfigOption {
	return func(c *Config) (*Config, error) {
		err := c.SetClusterStorage(mapping)
		return c, err
	}
}
func SetupClusterStorage(mapping func(v Volume) (*VolumeSource, error)) ConfigOption {
	return func(c *Config) (*Config, error) {
		err := c.SetupClusterStorage(mapping)
		return c, err
	}
}

func BuildOption(workspaceID, workspaceName, appName string) func(c *Config) (res *Config, err error) {
	return func(c *Config) (res *Config, err error) {
		res = c
		res.Name = appName
		res.Workspace = workspaceName
		res.WorkspaceID = workspaceID
		if res.Labels == nil {
			res.Labels = make(map[string]string)
		}
		res.Labels[KUBELAB_WS_LABEL] = workspaceName
		res.Labels[KUBELAB_WS_ID_LABEL] = workspaceID
		for i := range res.Uix {
			res.Uix[i].FrontAPI = fmt.Sprintf("/api/v1/ml2-proxy/%s/%s/%s/",
				workspaceName, appName, res.Uix[i].Name)
		}
		return
	}
}

func (c Config) ToYaml() ([]byte, error) {
	return yaml.Marshal(c)
}
