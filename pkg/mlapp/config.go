package mlapp

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/errors"
	kuberlab "github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	KUBERLAB_WS_LABEL     = "kuberlab.io/workspace"
	KUBERLAB_WS_ID_LABEL  = "kuberlab.io/workspace-id"
	KUBERLAB_PROJECT_ID   = "kuberlab.io/project-id"
	KUBERLAB_PROJECT_NAME = "kuberlab.io/project"
	KUBERLAB_STORAGE_NAME = "kuberlab.io/storage-name"

	KindMlApp   = "MLApp"
	KindServing = "Serving"
	KindTask    = "Task"
)

var validNames = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")
var validVolumes = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")

type Config struct {
	Kind        string `json:"kind"`
	Meta        `json:"metadata"`
	Spec        `json:"spec,omitempty"`
	Workspace   string    `json:"workspace,omitempty"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	ProjectID   string    `json:"project_id,omitempty"`
	Revision    *Revision `json:"revision,omitempty"`
}

type BoardConfig struct {
	DealerAPI     string   `json:"dealer_api,omitempty"`
	VolumesData   []Volume `json:"volumes_data,omitempty"`
	Secrets       []Secret `json:"secrets,omitempty"`
	BoardMetadata Metadata `json:"board_metadata,omitempty"`
	Config        `json:",inline"`
}

type Metadata struct {
	Limits *dealerclient.ResourceLimit `json:"limits,omitempty"`
}

func (c *BoardConfig) CheckResourceLimit(res Resource, resName string) error {
	if c.BoardMetadata.Limits == nil {
		return nil
	}
	if c.BoardMetadata.Limits.Replicas > 0 {
		if int64(res.Replicas) > c.BoardMetadata.Limits.Replicas {
			return fmt.Errorf(
				"Invalid replicas %v for resource %v: maximum allowed: %v",
				res.Replicas, resName, c.BoardMetadata.Limits.Replicas,
			)
		}
	}
	return nil
}

func (c *BoardConfig) Type() string {
	return KindMlApp
}

func (c *BoardConfig) GPURequests() int64 {
	var gpus int64 = 0
	for _, uix := range c.Spec.Uix {
		if uix.Resources != nil {
			gpus += int64(uix.Resources.Accelerators.GPU)
		}
	}
	return gpus
}

func (c Config) ValidateConfig() error {
	resNameErr := func(n, r string) error {
		return errors.NewStatusReason(
			http.StatusBadRequest,
			fmt.Sprintf("Invalid %s name: '%s'. ", r, n),
			"Valid name must be 63 characters or less "+
				"and must begin and end with an lower case alphanumeric character ([a-z0-9]) "+
				"with dashes (-) and lower case alphanumerics between",
		)
	}
	for _, u := range c.Uix {
		if !validNames.MatchString(u.Name) {
			return resNameErr(u.Name, "uix component")
		}
	}
	for _, t := range c.Tasks {
		if !validNames.MatchString(t.Name) {
			return resNameErr(t.Name, "task")
		}
		for _, r := range t.Resources {
			if !validNames.MatchString(r.Name) {
				return resNameErr(r.Name, "task resource")
			}
		}
	}
	resVolumeErr := func(n string) error {
		return errors.NewStatusReason(
			http.StatusBadRequest,
			fmt.Sprintf("Invalid volume name: '%s'. ", n),
			"Valid name must be 63 characters or less "+
				"and must begin and end with an lower case alphanumeric character ([a-z0-9]) "+
				"with dashes (-) and lower case alphanumerics between",
		)
	}
	for _, v := range c.Volumes {
		if !validVolumes.MatchString(v.Name) {
			return resVolumeErr(v.Name)
		}
	}
	return nil
}

func NamespaceName(workspaceID, workspaceName string) string {
	return utils.KubeNamespaceEncode(workspaceID + "-" + workspaceName)
}

func (c Config) GetNamespace() string {
	return NamespaceName(c.WorkspaceID, c.Workspace)
}
func (c Config) GetAppID() string {
	return c.WorkspaceID + "-" + c.Name
}

func (c Config) GetAppName() string {
	return utils.KubeNamespaceEncode(c.Name)
}

type Meta struct {
	Name   string            `json:"name,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

type DeploymentBasedResource interface {
	Type() string
	GetName() string
	Deployment(client *kubernetes.Clientset, namespace, appName string) (*extv1beta1.Deployment, error)
}

type Revision struct {
	Branch      string `json:"branch,omitempty"`
	NewBranch   string `json:"new_branch,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Author      string `json:"author,omitempty"`
	AuthorName  string `json:"author_name,omitempty"`
	AuthorEmail string `json:"author_email,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

type Spec struct {
	Tasks                 []Task     `json:"tasks,omitempty"`
	Uix                   []Uix      `json:"uix,omitempty"`
	Serving               []Serving  `json:"serving,omitempty"`
	Volumes               []Volume   `json:"volumes,omitempty"`
	Packages              []Packages `json:"packages,omitempty"`
	DefaultPackageManager string     `json:"package_manager,omitempty"`
	DefaultMountPath      string     `json:"default_mount_path,omitempty"`
	DefaultReadOnly       bool       `json:"default_read_only"`
	DockerAccountIDs      []string   `json:"docker_account_ids,omitempty"`
}

type Secret struct {
	Name   string            `json:"name,omitempty"`
	Data   map[string]string `json:"data,omitempty"`
	Type   string            `json:"type,omitempty"`
	Mounts map[string]string `json:"mounts,omitempty"`
	Path   string            `json:"path,omitempty"`
}
type Packages struct {
	Names   []string `json:"names,omitempty"`
	Manager string   `json:"manager,omitempty"`
}

type Resource struct {
	Replicas                int              `json:"replicas,omitempty"`
	Resources               *ResourceRequest `json:"resources,omitempty"`
	Images                  Images           `json:"images,omitempty"`
	Command                 string           `json:"command,omitempty"`
	WorkDir                 string           `json:"workDir,omitempty"`
	RawArgs                 string           `json:"args,omitempty"`
	Env                     []Env            `json:"env,omitempty"`
	Volumes                 []VolumeMount    `json:"volumes,omitempty"`
	NodesLabel              string           `json:"nodes,omitempty"`
	UseDefaultVolumeMapping bool             `json:"default_volume_mapping,omitempty"`
	DefaultMountPath        string           `json:"default_mount_path,omitempty"`
}

func (r Resource) VolumeMounts(volumes []Volume, defaultMountPath string, defaultReadOnly bool) []VolumeMount {
	if r.DefaultMountPath != "" {
		defaultMountPath = r.DefaultMountPath
	}
	defaultMountPath = strings.TrimSuffix(defaultMountPath, "/")
	var mounts []VolumeMount
	if r.UseDefaultVolumeMapping {
		for _, v := range volumes {
			rOnly := false
			rOnly = rOnly || v.ReadOnly || defaultReadOnly
			var rev *string = nil
			if v.GitRepo != nil {
				rev = &v.GitRepo.Revision
			}
			mounts = append(mounts, VolumeMount{
				Name: v.Name, ReadOnly: rOnly, MountPath: v.MountPath, GitRevision: rev,
			})
		}
	} else {
		mounts = r.Volumes
		for i := range mounts {
			for _, v := range volumes {
				if v.Name == mounts[i].Name {
					rOnly := mounts[i].ReadOnly || v.ReadOnly || defaultReadOnly
					mounts[i].ReadOnly = rOnly
					if mounts[i].MountPath == "" {
						mounts[i].MountPath = v.MountPath
					}
				}
			}
		}
	}
	for i := range mounts {
		mpath := mounts[i].MountPath
		if mpath == "" {
			mpath = mounts[i].Name
		}
		if !strings.HasPrefix(mpath, "/") {
			mpath = defaultMountPath + "/" + mpath
		}
		mounts[i].MountPath = mpath
	}
	return mounts
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
	Disabled    bool   `json:"disabled"`
}

func (uix *Uix) Type() string {
	return "Ui"
}

func (uix *Uix) GetName() string {
	return uix.Name
}

func (uix *Uix) Deployment(client *kubernetes.Clientset, namespace, appName string) (*extv1beta1.Deployment, error) {
	return client.ExtensionsV1beta1().Deployments(namespace).Get(
		utils.KubeDeploymentEncode(appName+"-"+uix.Name), meta_v1.GetOptions{},
	)
}

type Serving struct {
	Uix       `json:",inline"`
	Spec      ServingSpec            `json:"spec"`
	TaskName  string                 `json:"taskName"`
	Build     string                 `json:"build"`
	BuildInfo map[string]interface{} `json:"build_info,omitempty"`
}

func (s *Serving) GPURequests() int64 {
	var gpus int64 = 0
	if s.Uix.Resources != nil {
		gpus += int64(s.Uix.Resources.Accelerators.GPU)
	}
	return gpus
}

type ServingSpec struct {
	Params      []ServingSpecParam `json:"params,omitempty"`
	OutFilter   []string           `json:"outFilter,omitempty"`
	OutMimeType string             `json:"outMimeType,omitempty"`
	RawInput    bool               `json:"rawInput"`
	Signature   string             `json:"signature,omitempty"`
	Model       string             `json:"model,omitempty"`
}

type ServingSpecParam struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

func (s *Serving) Type() string {
	return KindServing
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       int32  `json:"port,omitempty"`
	TargetPort int32  `json:"targetPort,omitempty"`
}

type Task struct {
	Meta             `json:",inline"`
	Version          string         `json:"version,omitempty"`
	TimeoutMinutes   uint           `json:"timeoutMinutes,omitempty"`
	Resources        []TaskResource `json:"resources"`
	Revision         *Revision      `json:"revision,omitempty"`
	GitRevisions     []TaskRevision `json:"gitRevisions,omitempty"`
	DatasetRevisions []TaskRevision `json:"datasetRevisions,omitempty"`
}

func (t *Task) Type() string {
	return KindTask
}

func (t *Task) GPURequests() int64 {
	var gpus int64 = 0
	for _, r := range t.Resources {
		if r.Resources != nil {
			gpus += int64(r.Resources.Accelerators.GPU)
		}
	}
	return gpus
}

type TaskRevision struct {
	VolumeName string `json:"volumeName,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

type TaskResource struct {
	Meta            `json:",inline"`
	RestartPolicy   string `json:"restartPolicy"`
	MaxRestartCount int    `json:"maxRestartCount"`
	IsPermanent     bool   `json:"is_permanent"`
	Port            int32  `json:"port,omitempty"`
	Resource        `json:",inline"`
}

type Images struct {
	CPU string `json:"cpu"`
	GPU string `json:"gpu"`
}

type Env struct {
	Name            string `json:"name"`
	Value           string `json:"value"`
	ValueFromSecret string `json:"valueFromSecret"`
	SecretKey       string `json:"secretKey"`
}

type TaskResourceSpec struct {
	PodsNumber    int
	TaskName      string
	ResourceName  string
	NodeAllocator string
	Resource      *kuberlab.KubeResource
}

func (c Config) GetBoardConfig(mapping func(v Volume) (*VolumeSource, error)) (*BoardConfig, error) {
	b := BoardConfig{
		Config: c,
	}
	b.VolumesData = make([]Volume, len(c.Spec.Volumes))
	for i, v := range c.Spec.Volumes {
		if s, err := mapping(v); err == nil {
			v := c.Spec.Volumes[i]
			v.VolumeSource = *s
			b.VolumesData[i] = v
		} else {
			return nil, fmt.Errorf("Failed setup cluster storage '%s': %v", v.ClusterStorage, err)
		}
	}
	return &b, nil
}

func (c *BoardConfig) VolumeByName(name string) *Volume {
	return c.volumeByName(name)
}
func (c *BoardConfig) volumeByName(name string) *Volume {
	for _, v := range c.VolumesData {
		if v.Name == name {
			res := v
			return &res
		}
	}
	return nil
}

func (c *BoardConfig) volumeByID(commonID string) *Volume {
	for _, v := range c.VolumesData {
		if v.CommonID() == commonID {
			res := v
			return &res
		}
	}
	return nil
}

func (c *BoardConfig) LibVolume() (*v1.Volume, *v1.VolumeMount) {
	for _, v := range c.VolumesData {
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

func (c *BoardConfig) InjectRevisions(client *kubernetes.Clientset, task *Task) error {
	if err := c.InjectGitRevisions(client, task); err != nil {
		return err
	}
	c.InjectDatasetRevisions(task)
	return nil
}

type InitContainers struct {
	Image   string
	Command string
	Name    string
	Mounts  map[string]interface{}
}

func (c *BoardConfig) KubeInits(mounts []VolumeMount, taskName, build *string) ([]InitContainers, error) {
	var inits []InitContainers
	added := map[string]bool{}
	_, vmounts, err := c.getSecretVolumes(c.Secrets)
	if err != nil {
		return nil, err
	}
	for j, m := range mounts {
		if _, ok := added[m.Name]; ok {
			continue
		}
		added[m.Name] = true
		v := c.volumeByName(m.Name)
		id := v.CommonID()
		if v == nil {
			return nil, fmt.Errorf("Source '%s' not found", m.Name)
		}
		if v.GitRepo != nil && v.GitRepo.AccountId != "" {
			// Skip for UIX and already cloned repos.
			if v.GitRepo.AccountId == "" && taskName == nil && build == nil {
				return []InitContainers{}, nil
			}
			var cmd []string
			repoName := getGitRepoName(v.GitRepo.Repository)
			baseDir := fmt.Sprintf("/gitdata/%d", j)
			repoDir := fmt.Sprintf("%v/%v", baseDir, repoName)
			if v.GitRepo.AccountId == "" {
				// If already cloned.
				cmd = append(cmd, fmt.Sprintf("cd %v", repoDir))
			} else {
				apnd := []string{
					fmt.Sprintf("cd %v", baseDir),
					fmt.Sprintf("git clone %v", v.GitRepo.Repository),
					fmt.Sprintf("cd %v", repoDir),
				}
				cmd = append(cmd, apnd...)
			}

			if v.GitRepo.Revision != "" {
				cmd = append(cmd, fmt.Sprintf("git checkout %s", v.GitRepo.Revision))
			}
			cmd = append(cmd, "git config --local user.name kuberlab-robot")
			cmd = append(cmd, "git config --local user.email robot@kuberlab.com")

			cmdStr := strings.Join(cmd, " && ")

			vmounts = append(vmounts, v1.VolumeMount{
				Name:      id,
				MountPath: baseDir,
				ReadOnly:  false,
			})
			// Raise 39 exit code for further analysis.
			cmdStr += "; if [ $? -ne 0 ]; then exit 39; fi"

			inits = append(inits, InitContainers{
				Mounts: map[string]interface{}{
					"volumeMounts": vmounts,
				},
				Name:    m.Name,
				Image:   "kuberlab/board-init",
				Command: fmt.Sprintf(`['sh', '-c', '%v']`, cmdStr),
			})
		}

	}
	return inits, nil
}
func getGitRepoName(repo string) string {
	p := strings.Split(repo, "/")
	return strings.TrimSuffix(p[len(p)-1], ".git")
}
func (c *BoardConfig) KubeVolumesSpec(mounts []VolumeMount) ([]v1.Volume, []v1.VolumeMount, error) {
	added := make(map[string]bool)
	kVolumes := make([]v1.Volume, 0)
	kVolumesMount := make([]v1.VolumeMount, 0)
	for _, m := range mounts {
		v := c.volumeByName(m.Name)
		if v == nil {
			return nil, nil, fmt.Errorf("Source '%s' not found", m.Name)
		}
		if v.FlexVolume != nil {
			if v.FlexVolume.SecretRef != nil && v.FlexVolume.SecretRef.Name != "" && !strings.HasPrefix(v.FlexVolume.SecretRef.Name, c.Name) {
				v.FlexVolume.SecretRef.Name = fmt.Sprintf("%v-%v", c.Name, v.FlexVolume.SecretRef.Name)
			}
		}
		id := v.CommonID()
		if _, ok := added[id]; !ok {
			added[id] = true
			kVolumes = append(kVolumes, v.V1Volume())
		}
		mountPath := v.MountPath
		if len(m.MountPath) > 0 {
			mountPath = m.MountPath
		}
		subPath := v.SubPath
		if v.ClusterStorage != "" {
			if !v.IsWorkspaceLocal && strings.HasPrefix(subPath, "/shared/") {
				subPath = strings.TrimPrefix(subPath, "/")
			} else if strings.HasPrefix(subPath, "/") {
				subPath = strings.TrimPrefix(subPath, "/")
				if len(subPath) > 0 {
					subPath = c.Workspace + "/" + c.WorkspaceID + "/" + subPath
				} else {
					subPath = c.Workspace + "/" + c.WorkspaceID + "/" + c.Name
				}
			} else if len(subPath) > 0 {
				subPath = c.Workspace + "/" + c.WorkspaceID + "/" + c.Name + "/" + subPath
			} else {
				subPath = c.Workspace + "/" + c.WorkspaceID + "/" + c.Name + "/" + v.Name
			}
		} else {
			subPath = strings.TrimPrefix(subPath, "/")
		}
		if len(m.SubPath) > 0 {
			subPath = filepath.Join(subPath, m.SubPath)
		}

		subPath = strings.TrimPrefix(subPath, "/")
		kVolumesMount = append(kVolumesMount, v1.VolumeMount{
			Name:      id,
			SubPath:   subPath,
			MountPath: mountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	if len(c.Secrets) > 0 {
		vol, vom, err := c.getSecretVolumes(c.Secrets)
		if err != nil {
			return nil, nil, err
		}
		if len(vol) > 0 {
			kVolumes = append(kVolumes, vol...)
		}
		if len(vom) > 0 {
			kVolumesMount = append(kVolumesMount, vom...)
		}
	}
	return kVolumes, kVolumesMount, nil
}

func (c *BoardConfig) CleanUPVolumes() ([]v1.Volume, []v1.VolumeMount) {
	added := make(map[string]bool)
	kVolumes := make([]v1.Volume, 0)
	kVolumesMount := make([]v1.VolumeMount, 0)
	for _, v := range c.VolumesData {
		subPath := v.SubPath
		if v.ClusterStorage != "" {
			if !strings.HasPrefix(subPath, "/") {
				id := v.CommonID()
				if _, ok := added[id]; !ok {
					added[id] = true
					kVolumes = append(kVolumes, v.V1Volume())
					kVolumesMount = append(kVolumesMount, v1.VolumeMount{
						Name:      id,
						SubPath:   c.Workspace + "/" + c.WorkspaceID,
						MountPath: "/kuberlab/" + id,
						ReadOnly:  false,
					})
				}
			}
		}
	}
	return kVolumes, kVolumesMount
}

func (c *BoardConfig) getSecretVolumes(secrets []Secret) ([]v1.Volume, []v1.VolumeMount, error) {
	kvolumes := make([]v1.Volume, 0)
	kvolumesMount := make([]v1.VolumeMount, 0)
	for _, s := range secrets {
		if len(s.Mounts) > 0 {
			items := make([]v1.KeyToPath, len(s.Mounts))
			i := 0
			var mode int32 = 0600
			for k, m := range s.Mounts {
				items[i] = v1.KeyToPath{
					Key:  k,
					Path: m,
					Mode: &mode,
				}
				i += 1
			}
			v := v1.Volume{
				Name: s.Name,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: c.GetSecretName(s),
						Items:      items,
					},
				},
			}
			kvolumes = append(kvolumes, v)
			kvolumesMount = append(kvolumesMount, v1.VolumeMount{
				Name:      s.Name,
				MountPath: s.Path,
				ReadOnly:  false,
			})
		}
	}
	// Need curl https://storage.googleapis.com/pluk/kdataset-linux -o /usr/bin/kdataset on all nodes
	if c.Kind == KindServing {
		// Ignore additional volumes for serving from model.
		return kvolumes, kvolumesMount, nil
	}
	kvolumesMount = append(kvolumesMount, v1.VolumeMount{
		Name:      "kdataset",
		MountPath: "/usr/bin/kdataset",
		ReadOnly:  true,
		SubPath:   "kdataset",
	})
	kvolumes = append(kvolumes, v1.Volume{
		Name: "kdataset",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/usr/bin/",
			},
		},
	})
	kvolumesMount = append(kvolumesMount, v1.VolumeMount{
		Name:      "kuberlab-config",
		MountPath: "/root/.kuberlab/config",
		SubPath:   "config",
	})
	kvolumes = append(kvolumes, v1.Volume{
		Name: "kuberlab-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: fmt.Sprintf("%v-kuberlab-config", c.Name),
				},
			},
		},
	})
	return kvolumes, kvolumesMount, nil
}

func (c *BoardConfig) DockerSecretNames() []string {
	secrets := make([]string, 0)
	for _, s := range c.Secrets {
		if s.Type == string(v1.SecretTypeDockerConfigJson) {
			if s.Name != "mlboard-docker-config" {
				secrets = append(secrets, c.GetSecretName(s))
			} else {
				secrets = append(secrets, s.Name)
			}
		}
	}
	return secrets
}

func (c *BoardConfig) generateKuberlabConfig() *kuberlab.KubeResource {
	config := &v1.ConfigMap{
		Data: map[string]string{
			"config": "pluk_url: 'http://pluk.kuberlab.svc.cluster.local:8082'\n",
		},
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      fmt.Sprintf("%v-kuberlab-config", c.Name),
			Namespace: c.GetNamespace(),
			Labels:    c.ResourceLabels(),
		},
	}
	gv := config.GroupVersionKind()
	return &kuberlab.KubeResource{
		Name:   "kuberlab-config",
		Kind:   &gv,
		Object: config,
	}
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

func BuildOption(workspaceID, workspaceName, projectID, projectName string) func(c *Config) (res *Config, err error) {
	return func(c *Config) (res *Config, err error) {
		res = c
		res.Name = projectName
		res.Workspace = workspaceName
		res.WorkspaceID = workspaceID
		res.ProjectID = projectID
		if res.Labels == nil {
			res.Labels = make(map[string]string)
		}
		for i := range res.Uix {
			res.Uix[i].FrontAPI = res.ProxyURL(res.Uix[i].Name)
		}
		return
	}
}

func (c *BoardConfig) GetSecretName(secret Secret) string {
	return utils.KubeLabelEncode(c.Name + "-" + secret.Name)
}

func (c *BoardConfig) GetWorkspaceSecret() string {
	name := fmt.Sprintf("ws-key-%v", c.WorkspaceID)
	for _, s := range c.Secrets {
		if strings.Contains(s.Name, name) {
			for _, v := range s.Data {
				// Return 1st value.
				return v
			}
		}
	}
	return ""
}

func (c *BoardConfig) ResourceLabels(l ...map[string]string) map[string]string {
	l1 := map[string]string{
		KUBERLAB_WS_LABEL:     utils.KubeLabelEncode(c.Workspace),
		KUBERLAB_WS_ID_LABEL:  c.WorkspaceID,
		KUBERLAB_PROJECT_NAME: utils.KubeLabelEncode(c.Name),
		KUBERLAB_PROJECT_ID:   c.ProjectID,
	}
	for _, m := range l {
		for k, v := range m {
			l1[k] = utils.KubeLabelEncode(v)
		}
	}
	return l1
}

func (c *BoardConfig) ResourceSelector(l ...map[string]string) meta_v1.ListOptions {
	l1 := []map[string]string{{
		KUBERLAB_WS_ID_LABEL: c.WorkspaceID,
		KUBERLAB_PROJECT_ID:  c.ProjectID,
	},
	}
	l1 = append(l1, l...)
	return resourceSelector(l1...)
}

func resourceSelector(l ...map[string]string) meta_v1.ListOptions {
	var labelSelector = make([]string, 0)
	for _, m := range l {
		for k, v := range m {
			labelSelector = append(labelSelector, fmt.Sprintf("%v=%v", k, utils.KubeLabelEncode(v)))
		}
	}
	return meta_v1.ListOptions{LabelSelector: strings.Join(labelSelector, ",")}
}

func (c BoardConfig) ToMiniYaml() ([]byte, error) {
	c.Revision = nil
	for i := range c.Tasks {
		for j := range c.Tasks[i].Resources {
			c.Tasks[i].Resources[j].Command = strings.TrimSpace(c.Tasks[i].Resources[j].Command)
		}
		c.Tasks[i].Revision = nil
	}
	return c.ToYaml()
}
func (c Config) ToYaml() ([]byte, error) {
	return yaml.Marshal(c)
}
func (c BoardConfig) ToYaml() ([]byte, error) {
	return yaml.Marshal(c)
}
func (c Config) ProxyURL(path string) string {
	return ProxyURL([]string{c.Workspace, c.Name, path})
}

func ProxyURL(path []string) string {
	for i, p := range path {
		path[i] = url.PathEscape(p)
	}
	return fmt.Sprintf("/api/v1/ml2-proxy/%s/",
		strings.Join(path, "/"))
}
