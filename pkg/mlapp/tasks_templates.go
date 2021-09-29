package mlapp

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"strconv"
	"strings"
	"sync"

	kuberlab "github.com/kuberlab/lib/pkg/kubernetes"
	//"github.com/kuberlab/lib/pkg/mlapp/ssh"
	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
)

const ResourceTpl = `
apiVersion: v1
kind: Pod
metadata:
  name: "{{ .BuildName }}"
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
spec:
  {{- if .PrivilegedMode }}
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
  {{- end }}
  terminationGracePeriodSeconds: 10
  hostname: "{{ .BuildName }}"
  subdomain: "{{ .BuildName }}"
  restartPolicy: Never
  tolerations:
  - key: role.kuberlab.io/cpu-compute
    effect: PreferNoSchedule
  {{- if gt .ResourcesSpec.Accelerators.GPU 0 }}
  - key: role.kuberlab.io/gpu-compute
    effect: PreferNoSchedule
  {{- end }}
  {{- if .DeployResourceLabel }}
  - key: kuberlab.io/private-resource
    value: {{ .DeployResourceLabel }}
    effect: NoSchedule
  {{- end }}
  {{- if gt (len .InitContainers) 0 }}
  initContainers:
  {{- range $i, $value := .InitContainers }}
  - name: {{ $value.Name }}
    image: {{ $value.Image }}
    command: {{ $value.Command }}
{{ toYaml $value.Mounts | indent 4 }}
  {{- end }}
  {{- end }}
  {{- if gt (len .DockerSecretNames) 0 }}
  imagePullSecrets:
  {{- range $i, $value := .DockerSecretNames }}
  - name: {{ $value }}
  {{- end }}
  {{- end }}
  containers:
  - command: ["/bin/bash", "-c"]
    args:
    - >
      {{- if .Conda }}
      source activate {{ .Conda }};
      {{- end }}
      export PYTHONPATH=$PYTHONPATH:{{ .PythonPath }};
      cd {{ .WorkDir }};
      {{ .Command | indent 6 }} {{ .Args }};
      code=$?;
      exit $code
    image: {{ .Image }}
    imagePullPolicy: Always
    name: "{{ .BuildName }}"
    {{- if .PrivilegedMode }}
    securityContext:
      privileged: true
      capabilities:
        add: ["SYS_ADMIN"]
    {{- end }}
    env:
    - name: POD_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    {{- range .Env }}
    - name: {{ .Name }}
    {{- if gt (len .ValueFromSecret) 0 }}
      valueFrom:
        secretKeyRef: 
          name: '{{ .ValueFromSecret }}'
          key: '{{ .SecretKey }}'
    {{- else }}
      value: '{{ .Value }}'
    {{- end }}
    {{- end }}
    {{- if gt .Port 0 }}
    ports:
    - containerPort: {{ .Port }}
      name: cluster-port
      protocol: TCP
    {{- end }}
    resources:
      requests:
        {{- if .ResourcesSpec.Requests.CPUQuantity }}
        cpu: "{{ .ResourcesSpec.Requests.CPUQuantity }}"
        {{- end }}
        {{- if .ResourcesSpec.Requests.MemoryQuantity }}
        memory: "{{ .ResourcesSpec.Requests.MemoryQuantity }}"
        {{- end }}
      limits:
        {{- if gt .ResourcesSpec.Accelerators.GPU 0 }}
        {{- if and (eq .KubeVersionMajor 1) (lt .KubeVersionMinor 9) }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .ResourcesSpec.Accelerators.GPU }}"
        {{- else }}
        nvidia.com/gpu: {{ .ResourcesSpec.Accelerators.GPU }}
        {{- end }}
        {{- end }}
        {{- if .ResourcesSpec.Limits.CPUQuantity }}
        cpu: "{{ .ResourcesSpec.Limits.CPUQuantity }}"
        {{- end }}
        {{- if .ResourcesSpec.Limits.MemoryQuantity }}
        memory: "{{ .ResourcesSpec.Limits.MemoryQuantity }}"
        {{- end }}
{{ toYaml .Mounts | indent 4 }}
{{ toYaml .Volumes | indent 2 }}
`

type TaskResourceGenerator struct {
	JobID    string
	Callback string
	c        *BoardConfig
	task     Task
	TaskResource
	once           sync.Once
	volumes        []v1.Volume
	mounts         []v1.VolumeMount
	InitContainers []InitContainers
}

func (t *TaskResourceGenerator) KubeVersion() *version.Info {
	return kuberlab.MlBoardKubeVersion
}

func (t *TaskResourceGenerator) KubeVersionMajor() int {
	major, _ := strconv.ParseInt(kuberlab.MlBoardKubeVersion.Major, 10, 32)
	if major == 0 {
		return 1
	}
	return int(major)
}

func (t *TaskResourceGenerator) KubeVersionMinor() int {
	minor, _ := strconv.ParseInt(kuberlab.MlBoardKubeVersion.Minor, 10, 32)
	if minor == 0 {
		return 8
	}
	return int(minor)
}

func (t *TaskResourceGenerator) ResourcesSpec() ResourceRequest {
	cpu, _ := resource.ParseQuantity("50m")
	mem, _ := resource.ParseQuantity("128Mi")
	return ResourceSpec(t.Resources, t.c.BoardMetadata.Limits, dealerclient.ResourceLimit{CPU: &cpu, Memory: &mem})
}

func (t *TaskResourceGenerator) DockerSecretNames() []string {
	return t.c.DockerSecretNames()
}

func (t *TaskResourceGenerator) Conda() string {
	for _, e := range t.Env() {
		if e.Name == "CONDA_ENV" {
			return e.Value
		}
	}
	return ""
}

func (t *TaskResourceGenerator) PythonPath() string {
	_, pythonPath := baseEnv(t.c, t.TaskResource.Resource)
	return pythonPath
}
func (t *TaskResourceGenerator) DeployResourceLabel() string {
	return t.c.DeployResourceLabel
}
func (t *TaskResourceGenerator) Env() []Env {
	envs, _ := baseEnv(t.c, t.TaskResource.Resource)
	for _, r := range t.task.Resources {
		hosts := make([]string, r.Replicas)
		for i := range hosts {
			serviceName := utils.KubePodNameEncode(fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, t.JobID, r.Name))
			hosts[i] = fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", serviceName, i, serviceName, t.Namespace())
		}
		nodes := make([]string, len(hosts))
		if r.Port > 0 {
			sp := strconv.Itoa(int(r.Port))
			for i, h := range hosts {
				nodes[i] = h + ":" + sp
			}
		}
		envs = append(envs, Env{
			Name:  strings.ToUpper(utils.EnvConvert(r.Name) + "_NODES"),
			Value: strings.Join(nodes, ","),
		}, Env{
			Name:  strings.ToUpper(utils.EnvConvert(r.Name) + "_HOSTS"),
			Value: strings.Join(hosts, ","),
		})
	}
	envs = append(envs, Env{
		Name:  "BUILD_ID",
		Value: t.JobID,
	})
	envs = append(envs, Env{
		Name:  "TASK_NAME",
		Value: t.task.Name,
	})
	return ResolveEnv(envs)
}
func (t *TaskResourceGenerator) BuildName() string {
	return utils.KubePodNameEncode(fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, t.JobID, t.TaskResource.Name))
}
func (t *TaskResourceGenerator) Mounts() interface{} {
	return map[string]interface{}{
		"volumeMounts": t.mounts,
	}
}
func (t *TaskResourceGenerator) Volumes() interface{} {
	return map[string]interface{}{
		"volumes": t.volumes,
	}
}
func (t *TaskResourceGenerator) Namespace() string {
	return t.c.GetNamespace()
}

func (t *TaskResourceGenerator) Labels() map[string]string {
	computeType := "cpu"
	if t.ResourcesSpec().Accelerators.GPU > 0 {
		computeType = "gpu"
	}
	return t.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     t.task.Name + "-" + t.TaskResource.Name,
		types.TASK_ID_LABEL:      t.JobID,
		types.TASK_NAME_LABEL:    t.task.Name,
		types.ComponentTypeLabel: "task",
		types.ComputeTypeLabel:   computeType,
		"scope":                  "mlboard",
	})
}

func (t *TaskResourceGenerator) Args() string {
	//return strings.Replace(t.RawArgs, "\"", "\\\"", -1)
	return t.RawArgs
}

func (t *TaskResourceGenerator) PrivilegedMode() bool {
	return t.NodesLabel == "knode:movidius"
}

func (c *BoardConfig) GenerateTaskResources(task Task, jobID string) ([]TaskResourceSpec, error) {
	taskSpec := make([]TaskResourceSpec, 0)
	for _, r := range task.Resources {
		if err := c.CheckResourceLimit(r.Resource, r.Name); err != nil {
			return nil, err
		}
		volumes, mounts, err := c.KubeVolumesSpec(r.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly))
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes for '%s-%s': %v", task.Name, r.Name, err)
		}

		c.setRevisions(volumes, task)

		initContainers, err := c.KubeInits(r.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly), &task, &jobID)
		if err != nil {
			return nil, fmt.Errorf("Failed generate init spec %s-%s': %v", task.Name, r.Name, err)
		}
		//volumes = append(volumes, sshVolumes...)
		//mounts = append(mounts, sshVolumesMount...)
		g := &TaskResourceGenerator{
			c:              c,
			task:           task,
			TaskResource:   r,
			mounts:         mounts,
			volumes:        volumes,
			JobID:          jobID,
			InitContainers: initContainers,
		}

		if g.PrivilegedMode() {
			g.volumes = append(
				g.volumes,
				v1.Volume{
					Name:         "dev",
					VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}},
				},
			)
			g.mounts = append(g.mounts, v1.VolumeMount{Name: "dev", MountPath: "/dev"})
		}

		res, err := kuberlab.GetTemplatedResource(ResourceTpl, g.BuildName()+":resource", g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", g.BuildName(), err)
		}
		res.Object = &kuberlab.WorkerSet{
			PodTemplate:         res.Object.(*v1.Pod),
			ResourceName:        r.Name,
			TaskName:            task.Name,
			ProjectName:         c.Name,
			Namespace:           c.GetNamespace(),
			JobID:               jobID,
			IsPermanent:         r.IsPermanent,
			MaxRestarts:         r.MaxRestartCount,
			Replicas:            int(r.Replicas),
			DeployResourceLabel: c.DeployResourceLabel,
			Selector: c.ResourceSelector(map[string]string{
				types.TASK_ID_LABEL:  jobID,
				types.ComponentLabel: task.Name + "-" + r.Name,
			}),
		}
		//res.Deps = []*kuberlab.KubeResource{&sshSecretResource}
		if g.Port > 0 {
			res.Deps = []*kuberlab.KubeResource{generateHeadlessService(g)}
		}
		taskSpec = append(taskSpec, TaskResourceSpec{
			TaskName:      task.Name,
			ResourceName:  r.Name,
			PodsNumber:    int(r.Replicas),
			Resource:      res,
			NodeAllocator: r.NodesLabel,
		})
	}
	return taskSpec, nil
}

func generateHeadlessService(g *TaskResourceGenerator) *kuberlab.KubeResource {
	labels := g.Labels()
	utils.JoinMaps(labels, g.c.Labels)
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      g.BuildName(),
			Namespace: g.c.GetNamespace(),
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector:  labels,
			ClusterIP: v1.ClusterIPNone,
			Ports: []v1.ServicePort{
				{
					Name:       "cluster",
					TargetPort: intstr.FromInt(int(g.Port)),
					Protocol:   v1.ProtocolTCP,
					Port:       g.Port,
				},
			},
		},
	}
	groupKind := svc.GroupVersionKind()
	return &kuberlab.KubeResource{
		Name:   g.BuildName() + ":service",
		Object: svc,
		Kind:   &groupKind,
	}
}
