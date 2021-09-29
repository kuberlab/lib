package mlapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/errors"
	kuberlab "github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	KUBERLAB_WS_LABEL     = "kuberlab.io/workspace"
	KUBERLAB_WS_ID_LABEL  = "kuberlab.io/workspace-id"
	KUBERLAB_PROJECT_ID   = "kuberlab.io/project-id"
	KUBERLAB_PROJECT_NAME = "kuberlab.io/project"
	KUBERLAB_STORAGE_NAME = "kuberlab.io/storage-name"

	KindMlApp             = "MLApp"
	KindServing           = "modelServing"
	KindTask              = "Task"
	kibernetikaPythonLibs = "/kibernetika-python-libs"

	GPUDisabledMessage = "GPU is disabled. Please wait till GPU is available (if you have one) or see and update your billing plans."
)

var validNames = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")
var validVolumes = regexp.MustCompile("^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$")

// swagger:model
type Config struct {
	// Resource kind
	Kind string `json:"kind" example:"MLApp"`
	Meta `json:"metadata"`
	Spec `json:"spec,omitempty"`
	// User workspace name
	Workspace string `json:"workspace,omitempty"`
	// User workspace id
	WorkspaceID string `json:"workspace_id,omitempty"`
	// Project id
	ProjectID string `json:"project_id,omitempty"`
	// Information to commit new configuration
	Revision *Revision `json:"revision,omitempty"`
}

type BoardConfig struct {
	DealerAPI           string   `json:"dealer_api,omitempty"`
	VolumesData         []Volume `json:"volumes_data,omitempty"`
	Secrets             []Secret `json:"secrets,omitempty"`
	BoardMetadata       Metadata `json:"board_metadata,omitempty"`
	Config              `json:",inline"`
	DeployResourceLabel string `json:"-"`
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
		if uix.Disabled {
			continue
		}
		if uix.Resources != nil {
			gpus += int64(uix.Resources.Accelerators.GPU)
		}
	}
	return gpus
}

func (c *BoardConfig) DisableGPU(num int) int {
	disabled := 0
	remain := num
	for i, ui := range c.Uix {
		reqs := 0
		if ui.Resources != nil {
			reqs = int(ui.Resources.Accelerators.GPU)
		}
		// Disable component which has GPU request and fit to disable num.
		if reqs >= remain && reqs > 0 {
			// Disable it.
			c.Uix[i].DisabledReason = GPUDisabledMessage
			c.Uix[i].Disabled = true
			remain -= reqs
			disabled += reqs
		}
		if remain <= 0 {
			break
		}
	}
	return disabled
}

func (c *BoardConfig) CPUMiLimits() map[string]int64 {
	cpuMap := make(map[string]int64)
	for _, uix := range c.Uix {
		if uix.Disabled {
			continue
		}
		if uix.Resources != nil {
			cpu, _ := uix.Resources.CPUMemLimits()
			cpuMap[uix.Name] = cpu
		} else {
			cpuMap[uix.Name] = 0
		}
	}
	return cpuMap
}

func (c *BoardConfig) MemoryMBLimits() map[string]int64 {
	memoryMap := make(map[string]int64)
	for _, uix := range c.Uix {
		if uix.Disabled {
			continue
		}
		if uix.Resources != nil {
			_, mem := uix.Resources.CPUMemLimits()
			memoryMap[uix.Name] = mem
		} else {
			memoryMap[uix.Name] = 0
		}
	}
	return memoryMap
}

func (c *Config) ValidateConfig() error {
	var resNameErr = func(n, r string, allowUnderscore bool) error {
		var underscoreRsn string
		if allowUnderscore {
			underscoreRsn = ", underscores (_)"
		}
		return errors.NewStatusReason(
			http.StatusBadRequest,
			fmt.Sprintf("Invalid %s name: '%s'. ", r, n),
			fmt.Sprintf("Valid name must be 63 characters or less "+
				"and must begin and end with an lower case alphanumeric character ([a-z0-9]) "+
				"with dashes (-)%s and lower case alphanumerics between", underscoreRsn),
		)
	}
	for _, u := range c.Uix {
		if !validNames.MatchString(u.Name) {
			return resNameErr(u.Name, "uix component", false)
		}
	}
	for _, t := range c.Tasks {
		if t.TaskType != TaskTypeGeneral && t.TaskType != TaskTypeInit && t.TaskType != TaskTypeExport {
			return errors.NewStatusReason(
				http.StatusBadRequest,
				fmt.Sprintf("Invalid task type '%s'", t.TaskType),
				fmt.Sprintf("Available types: '%s'. ", strings.Join([]string{
					TaskTypeGeneral,
					TaskTypeInit,
					TaskTypeExport,
				}, "', '")),
			)
		}
		if !validNames.MatchString(t.Name) {
			return resNameErr(t.Name, "task", false)
		}
		for _, r := range t.Resources {
			if !validNames.MatchString(r.Name) {
				return resNameErr(r.Name, "task resource", false)
			}
		}
	}
	for _, v := range c.Volumes {
		if !validVolumes.MatchString(v.Name) {
			return resNameErr(v.Name, "volume", false)
		}
		if v.Model != nil || v.Dataset != nil || v.DatasetFS != nil {
			v.ReadOnly = true
		}
	}
	return nil
}

func NamespaceName(workspaceID, workspaceName string) string {
	return utils.KubeNamespaceEncode(workspaceID + "-" + workspaceName)
}

func (c *Config) GetNamespace() string {
	return NamespaceName(c.WorkspaceID, c.Workspace)
}
func (c *Config) GetAppID() string {
	return c.WorkspaceID + "-" + c.Name
}

func (c *Config) GetAppName() string {
	return utils.KubeNamespaceEncode(c.Name)
}

type Meta struct {
	// Component name
	Name string `json:"name,omitempty"`
	// Map of string keys and values
	Labels map[string]string `json:"labels,omitempty"`
}

type DeploymentBasedResource interface {
	Type() string
	GetName() string
	Deployment(client *kubernetes.Clientset, namespace, appName string) (*appsv1.Deployment, error)
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

type ExportQuery struct {
	Revision
	BaseTask   string `json:"base_task"`
	ExportTask string `json:"export_task"`
}

type Spec struct {
	// Project tasks description
	Tasks []Task `json:"tasks,omitempty"`
	// Additional Project UI tabs description
	Uix []Uix `json:"uix,omitempty"`
	// Serving Description
	Serving []UniversalServing `json:"serving,omitempty"`
	// Project sources description
	Volumes []Volume `json:"volumes,omitempty"`
	// Packages required for project
	Packages []Packages `json:"packages,omitempty"`
	// Default package manager for project. pip, pip3, conda.
	DefaultPackageManager string `json:"package_manager,omitempty"`
	// Default path prefixes for mount sources
	DefaultMountPath string `json:"default_mount_path,omitempty"`
	// Image used for package manage
	DefaultInstallerImage string   `json:"default_installer_image,omitempty"`
	DockerAccountIDs      []string `json:"docker_account_ids,omitempty"`
	// Readonly project. User can't change configuration.
	DefaultReadOnly bool `json:"default_read_only"`
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
	// Number of process replica for component
	Replicas int `json:"replicas,omitempty"`
	// Resources required for component
	Resources *ResourceRequest `json:"resources,omitempty"`
	// Docker images used to start component
	Images Images `json:"images,omitempty"`
	// Execution command
	Command string `json:"command,omitempty"`
	// Work directory inside component
	WorkDir string `json:"workDir,omitempty"`
	// for internal usage
	RawArgs string `json:"args,omitempty"`
	// Environment variables for container
	Env []Env `json:"env,omitempty"`
	// Volumes (sources) to attach to component during execution
	Volumes []VolumeMount `json:"volumes,omitempty"`
	// Execute component on specific node type
	NodesLabel string `json:"nodes,omitempty"`
	// Attach all project volumes (sources)
	UseDefaultVolumeMapping bool `json:"default_volume_mapping,omitempty"`
	// Default mount prefix for volumes inside component
	DefaultMountPath string `json:"default_mount_path,omitempty"`
	// Resource autoscaling settings
	Autoscale *Autoscale `json:"autoscale,omitempty"`
}

type Autoscale struct {
	Enabled                  bool  `json:"enabled,omitempty"`
	MinReplicas              int32 `json:"min_replicas,omitempty"`
	MaxReplicas              int32 `json:"max_replicas,omitempty"`
	TargetAverageUtilization int32 `json:"target_average_utilization,omitempty"`
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
	Meta           `json:",inline"`
	DisplayName    string `json:"displayName,omitempty"`
	Ports          []Port `json:"ports,omitempty"`
	Resource       `json:",inline"`
	FrontAPI       string `json:"front_api,omitempty"`
	Disabled       bool   `json:"disabled"`
	DisabledReason string `json:"disabledReason,omitempty"`
	SkipPrefix     bool   `json:"skipPrefix"`
}

func (uix *Uix) Type() string {
	return "Ui"
}

func (uix *Uix) GetName() string {
	return uix.Name
}

func (uix *Uix) Deployment(client *kubernetes.Clientset, namespace, appName string) (*appsv1.Deployment, error) {
	return client.AppsV1().Deployments(namespace).Get(
		context.TODO(),
		utils.KubeDeploymentEncode(appName+"-"+uix.Name),
		meta_v1.GetOptions{},
	)
}

type Serving struct {
	Uix       `json:",inline"`
	Spec      ServingSpec            `json:"spec,omitempty"`
	TaskName  string                 `json:"taskName,omitempty"`
	Build     string                 `json:"build,omitempty"`
	BuildInfo map[string]interface{} `json:"build_info,omitempty"`
}

func (s *Serving) GPURequests() int64 {
	var gpus int64 = 0
	if s.Uix.Resources != nil {
		gpus += int64(s.Uix.Resources.Accelerators.GPU)
	}
	return gpus
}

func (s *Serving) CPUMiLimits() map[string]int64 {
	cpuMap := make(map[string]int64)
	if s.Uix.Resources != nil {
		cpu, _ := s.Uix.Resources.CPUMemLimits()
		cpuMap[s.Uix.Name] = cpu
	} else {
		cpuMap[s.Uix.Name] = 0
	}
	return cpuMap
}

func (s *Serving) MemoryMBLimits() map[string]int64 {
	memoryMap := make(map[string]int64)
	if s.Uix.Resources != nil {
		_, memory := s.Uix.Resources.CPUMemLimits()
		memoryMap[s.Uix.Name] = memory
	} else {
		memoryMap[s.Uix.Name] = 0
	}
	return memoryMap
}

func (s *Serving) DisableGPU(num int) int {
	s.Disabled = true
	s.DisabledReason = GPUDisabledMessage
	return int(s.GPURequests())
}

type ServingModelSpec struct {
	Driver  string            `json:"driver"`
	Path    string            `json:"path"`
	Options map[string]string `json:"options"`
	Inputs  InputOutSpec      `json:"inputs"`
	Outputs InputOutSpec      `json:"outputs"`
}

type InputOutSpec struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ServingSpecOptions struct {
	NoCache            bool `json:"noCache,omitempty"`
	SaveStreamPreviews bool `json:"saveStreamPreviews,omitempty"`
	// edge serving
	EdgeHost     string `json:"edgeHost,omitempty"`
	AudienceHost string `json:"audienceHost,omitempty"`
}

type ServingSpec struct {
	Params           []ServingSpecParam     `json:"params,omitempty"`
	Response         []ServingResponseParam `json:"response,omitempty"`
	ResponseTemplate string                 `json:"responseTemplate,omitempty"`
	OutFilter        []string               `json:"outFilter,omitempty"`
	ModelSpec        ServingModelSpec       `json:"model_spec,omitempty"`
	Options          ServingSpecOptions     `json:"options,omitempty"`
	// deprecated, todo remove soon
	OutMimeType string `json:"outMimeType,omitempty"`
	RawInput    bool   `json:"rawInput,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Model       string `json:"model,omitempty"`
	Template    string `json:"template,omitempty"`
}

type ServingSpecParam struct {
	Name    string      `json:"name,omitempty"`
	Type    string      `json:"type,omitempty"`
	Label   string      `json:"label,omitempty"`
	Value   interface{} `json:"value,omitempty"`
	Options []string    `json:"options,omitempty"`
}

type ServingResponseParam struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Shape       []int  `json:"shape,omitempty"`
	Description string `json:"description,omitempty"`
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
	Meta `json:",inline"`
	// Deprecated
	Version string `json:"version,omitempty"`
	// Deprecated
	TimeoutMinutes uint `json:"timeoutMinutes,omitempty"`
	// Task type, can be init, export or general (by default)
	TaskType string `json:"type,omitempty"`
	// Components that should be started during task execution
	Resources []TaskResource `json:"resources,omitempty"`
	// Information to commit new configuration
	Revision *Revision `json:"revision,omitempty"`
	// Revisions of source code used for execution
	GitRevisions []TaskRevision `json:"gitRevisions,omitempty"`
	// Revisions of datasets used for execution
	DatasetRevisions []TaskRevision `json:"datasetRevisions,omitempty"`
	// Revisions of models used for execution
	ModelRevisions []TaskRevision `json:"modelRevisions,omitempty"`
}

const (
	TaskTypeGeneral = "general"
	TaskTypeInit    = "init"
	TaskTypeExport  = "export"
)

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

func (t *Task) CPUMiLimits() map[string]int64 {
	cpuMap := make(map[string]int64)
	for _, resource := range t.Resources {
		if resource.Resources != nil {
			cpu, _ := resource.Resources.CPUMemLimits()
			cpuMap[resource.Name] = cpu
		} else {
			cpuMap[resource.Name] = 0
		}
	}
	return cpuMap
}

func (t *Task) MemoryMBLimits() map[string]int64 {
	memoryMap := make(map[string]int64)
	for _, resource := range t.Resources {
		if resource.Resources != nil {
			_, mem := resource.Resources.CPUMemLimits()
			memoryMap[resource.Name] = mem
		} else {
			memoryMap[resource.Name] = 0
		}
	}
	return memoryMap
}

func (t *Task) DisableGPU(num int) int {
	return 0
}

type TaskRevision struct {
	// Name of data source that support versioning
	VolumeName string `json:"volumeName,omitempty"`
	// Revision id or branch or tag
	Revision string `json:"revision,omitempty"`
}

type TaskResource struct {
	Meta          `json:",inline"`
	RestartPolicy string `json:"restartPolicy"`
	// How many times component may fail during task exection
	MaxRestartCount int `json:"maxRestartCount,omitempty"`
	// Is it permanent component that should not stop execution until all other component will be finished
	IsPermanent bool `json:"is_permanent,omitempty"`
	// Port used for communication with other component inside tasks.
	Port     int32 `json:"port,omitempty"`
	Resource `json:",inline"`
}

type Images struct {
	// Docker image used for component execution on CPU
	CPU string `json:"cpu"`
	// Docker image used for component execution on GPU. It it empty cpu image will be used.
	GPU string `json:"gpu"`
}

type Env struct {
	// Component environment variable name
	Name string `json:"name"`
	// Component environment variable value
	Value string `json:"value,omitempty"`
	// Only for internal usage
	ValueFromSecret string `json:"valueFromSecret,omitempty"`
	// Only for internal usage
	SecretKey string `json:"secretKey,omitempty"`
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

func (c *BoardConfig) InstallerImage() string {
	if c.DefaultInstallerImage != "" {
		return c.DefaultInstallerImage
	}
	if len(c.Uix) > 0 {
		if c.Uix[0].Images.CPU != "" {
			return c.Uix[0].Images.CPU
		}
	}
	if len(c.Tasks) > 0 {
		if len(c.Tasks[0].Resources) > 0 && c.Tasks[0].Resources[0].Images.CPU != "" {
			return c.Tasks[0].Resources[0].Images.CPU
		}
	}
	return ""
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
			mountPath := c.DefaultMountPath
			if v.MountPath != "" {
				mountPath = v.MountPath
			}

			vols, mounts, err := c.KubeVolumesSpec(
				[]VolumeMount{
					{Name: v.Name, ReadOnly: false, MountPath: mountPath},
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
	c.InjectModelRevisions(task)
	return nil
}

type InitContainers struct {
	Image   string
	Command string
	Name    string
	Mounts  map[string]interface{}
}

func (c *BoardConfig) KubeInits(mounts []VolumeMount, task *Task, build *string) ([]InitContainers, error) {
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
			if v.GitRepo.AccountId == "" && task == nil && build == nil {
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

			findRevision := func(volume string) string {
				for _, rev := range task.GitRevisions {
					if rev.VolumeName == volume {
						return rev.Revision
					}
				}
				return ""
			}

			if task != nil && findRevision(v.Name) != "" {
				cmd = append(cmd, fmt.Sprintf("git checkout %s", findRevision(v.Name)))
			} else if v.GitRepo.Revision != "" {
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
		if v.Model != nil {
			baseDir := fmt.Sprintf("/model/%d", j)
			vmounts = append(vmounts, v1.VolumeMount{
				Name:      id,
				MountPath: baseDir,
				ReadOnly:  false,
			})
			inits = append(inits, InitContainers{
				Name:  m.Name,
				Image: "kuberlab/board-init",
				Command: fmt.Sprintf(
					`["/bin/sh", "-c", "mkdir -p %v; curl -L -o m.tar %v && tar -xvf m.tar -C %v"]`,
					baseDir, v.Model.DownloadURL, baseDir,
				),
				Mounts: map[string]interface{}{"volumeMounts": vmounts},
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
			if v.FlexVolume.SecretRef != nil && v.FlexVolume.SecretRef.Name != "" &&
				!strings.HasPrefix(v.FlexVolume.SecretRef.Name, utils.KubeDeploymentEncode(c.Name)) {
				//v.FlexVolume.SecretRef.Name = fmt.Sprintf("%v-%v", c.Name, v.FlexVolume.SecretRef.Name)
				v.FlexVolume.SecretRef.Name = c.GetSecretName(Secret{Name: v.FlexVolume.SecretRef.Name})
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

			// Sort items by key.
			sort.Sort(keyPathSorted(items))

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
					Name: utils.KubePodNameEncode(fmt.Sprintf("%v-kuberlab-config", c.Name)),
				},
			},
		},
	})

	kvolumesMount = append(kvolumesMount, v1.VolumeMount{
		Name:      "tf-conf",
		MountPath: "/usr/bin/tf_conf",
		ReadOnly:  true,
		SubPath:   "tf_conf",
	})
	kvolumes = append(kvolumes, v1.Volume{
		Name: "tf-conf",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/usr/bin/",
			},
		},
	})

	kvolumesMount = append(kvolumesMount, v1.VolumeMount{
		Name:      "mlboardclient",
		MountPath: kibernetikaPythonLibs,
		ReadOnly:  true,
	})
	dirOrCreate := v1.HostPathDirectoryOrCreate
	kvolumes = append(kvolumes, v1.Volume{
		Name: "mlboardclient",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: kibernetikaPythonLibs,
				Type: &dirOrCreate,
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
			Name:      utils.KubePodNameEncode(fmt.Sprintf("%v-kuberlab-config", c.Name)),
			Namespace: c.GetNamespace(),
			Labels:    c.ResourceLabels(),
		},
	}
	gv := config.GroupVersionKind()
	return &kuberlab.KubeResource{
		Name:   "kuberlab-config:configmap",
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
	return utils.KubePodNameEncode(c.Name + "-" + secret.Name)
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

func (c *BoardConfig) resourceLabels() map[string]string {
	l := map[string]string{
		KUBERLAB_WS_LABEL:    utils.KubeLabelEncode(c.Workspace),
		KUBERLAB_WS_ID_LABEL: c.WorkspaceID,
	}
	if c.ProjectID != "" {
		l[KUBERLAB_PROJECT_NAME] = utils.KubeLabelEncode(c.Name)
		l[KUBERLAB_PROJECT_ID] = c.ProjectID
	}
	return l
}

func (c *BoardConfig) ResourceLabels(l ...map[string]string) map[string]string {
	l1 := c.resourceLabels()
	defautTemplate := utils.GetDefaultCPUNodeSelector()
	for _, m := range l {
		for k, v := range m {
			l1[k] = utils.KubeLabelEncode(v)
			if k == types.ComputeTypeLabel && v == "gpu" {
				if gtemplate := utils.GetDefaultGPUNodeSelector(); gtemplate != "" {
					defautTemplate = gtemplate
				}
			}
		}
	}
	if defautTemplate != "" {
		l1[types.KuberlabMLNodeLabel] = defautTemplate
	}
	return l1
}

func (c *BoardConfig) GenericResourceLabels(l ...map[string]string) map[string]string {
	l1 := c.resourceLabels()
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
