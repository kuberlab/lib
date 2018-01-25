package mlapp

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/ghodss/yaml"
	"github.com/kuberlab/lib/pkg/apputil"
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
)

var validNames = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")
var validVolumes = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")

type Config struct {
	Kind        string  `json:"kind"`
	Parent      *string `json:"parent,omitempty"`
	Meta        `json:"metadata"`
	Spec        `json:"spec,omitempty"`
	Workspace   string    `json:"workspace,omitempty"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	ProjectID   string    `json:"project_id,omitempty"`
	DealerAPI   string    `json:"dealer_api,omitempty"`
	Revision    *Revision `json:"revision,omitempty"`
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
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
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
	Replicas                int              `json:"replicas"`
	Resources               *ResourceRequest `json:"resources,omitempty"`
	Images                  Images           `json:"images"`
	Command                 string           `json:"command"`
	WorkDir                 string           `json:"workDir"`
	RawArgs                 string           `json:"args,omitempty"`
	Env                     []Env            `json:"env"`
	Volumes                 []VolumeMount    `json:"volumes"`
	NodesLabel              string           `json:"nodes"`
	UseDefaultVolumeMapping bool             `json:"default_volume_mapping"`
	DefaultMountPath        string           `json:"default_mount_path"`
}

func (r Resource) VolumeMounts(volumes []Volume) []VolumeMount {
	if r.UseDefaultVolumeMapping {
		var mounts []VolumeMount
		for _, v := range volumes {
			mpath := v.MountPath
			if strings.HasPrefix(r.DefaultMountPath, "[") {
				tmp := strings.TrimSuffix(strings.TrimPrefix(r.DefaultMountPath, "["), "]")
				mpath = execTemplate(tmp, v.MountPath)
			} else if r.DefaultMountPath != "" {
				mpath = r.DefaultMountPath + "/" + strings.TrimPrefix(v.MountPath, "/")
			}

			var rev *string = nil
			if v.GitRepo != nil {
				rev = &v.GitRepo.Revision
			}

			mounts = append(mounts, VolumeMount{
				Name: v.Name, ReadOnly: false, MountPath: mpath, GitRevision: rev,
			})
		}
		return mounts
	}
	return r.Volumes
}

func execTemplate(tmp, v string) string {
	t := template.New("gotpl")
	t = t.Funcs(apputil.FuncMap())
	t, err := t.Parse(tmp)
	if err != nil {
		return v
	}
	buffer := bytes.NewBuffer(make([]byte, 0))

	if err := t.ExecuteTemplate(buffer, "gotpl", map[string]string{"Value": v}); err != nil {
		return v
	}
	return buffer.String()
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
	return "UIX"
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
	TargetPort int32  `json:"targetPort,omitempty"`
}

type Task struct {
	Meta           `json:",inline"`
	ParentTask     *string          `json:"parentTask,omitempty"`
	Version        string           `json:"version,omitempty"`
	TimeoutMinutes uint             `json:"timeoutMinutes,omitempty"`
	Resources      []TaskResource   `json:"resources"`
	Revision       *Revision        `json:"revision,omitempty"`
	GitRevision    *TaskGitRevision `json:"gitRevision,omitempty"`
}

type TaskGitRevision struct {
	VolumeName string `json:"volumeName,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

type TaskResource struct {
	Meta            `json:",inline"`
	RestartPolicy   string `json:"restartPolicy"`
	MaxRestartCount int    `json:"maxRestartCount"`
	AllowFail       *bool  `json:"allowFail,omitempty"`
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
	TaskName      string
	ResourceName  string
	NodeAllocator string
	Resource      *kuberlab.KubeResource
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

func SetupClusterStorage(mapping func(v Volume) (*VolumeSource, error)) ConfigOption {
	return func(c *Config) (*Config, error) {
		err := c.SetupClusterStorage(mapping)
		return c, err
	}
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

func (c *Config) VolumeByID(commonID string) *Volume {
	for _, v := range c.Volumes {
		if v.CommonID() == commonID {
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
	Mounts  map[string]interface{}
}

func (c *Config) KubeInits(mounts []VolumeMount, taskName, build *string) ([]InitContainers, error) {
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
		v := c.VolumeByName(m.Name)
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
			//var submitRef = ""
			//if taskName != nil && build != nil {
			//	submitRef = fmt.Sprintf(
			//		`; curl http://mlboard-v2.kuberlab:8082/api/v2/submit/%s/%s/%s -H "X-Source: %s" -H "X-Ref: $(git rev-parse HEAD)"`,
			//		c.GetAppID(), *taskName, *build, v.Name,
			//	)
			//	cmdStr += submitRef
			//}

			vmounts = append(vmounts, v1.VolumeMount{
				Name:      id,
				MountPath: baseDir,
				ReadOnly:  false,
			})
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
func (c *Config) KubeVolumesSpec(mounts []VolumeMount) ([]v1.Volume, []v1.VolumeMount, error) {
	added := make(map[string]bool)
	kVolumes := make([]v1.Volume, 0)
	kVolumesMount := make([]v1.VolumeMount, 0)
	for _, m := range mounts {
		v := c.VolumeByName(m.Name)
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
				subPath = c.Workspace + "/" + c.WorkspaceID + "/" + c.Name
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

func (c *Config) getSecretVolumes(secrets []Secret) ([]v1.Volume, []v1.VolumeMount, error) {
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

func (c *Config) GetSecretName(secret Secret) string {
	return utils.KubeLabelEncode(c.Name + "-" + secret.Name)
}

func (c *Config) ResourceLabels(l ...map[string]string) map[string]string {
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

func (c *Config) ResourceSelector(l ...map[string]string) meta_v1.ListOptions {
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

func (c Config) ToMiniYaml() ([]byte, error) {
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
