package mlapp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
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
  terminationGracePeriodSeconds: 10
  hostname: "{{ .BuildName }}"
  subdomain: "{{ .BuildName }}"
  restartPolicy: Never
  {{- if gt (len .InitContainers) 0 }}
  initContainers:
  {{- range $i, $value := .InitContainers }}
  - name: {{ $value.Name }}
    image: {{ $value.Image }}
    command: {{ $value.Command }}
{{ toYaml $value.Mounts | indent 4 }}
  {{- end }}
  {{- end }}
  containers:
  - command: ["/bin/sh", "-c"]
    args:
    - >
      cd {{ .WorkDir }};
      {{ .Command }} {{ .Args }};
      code=$?;
      exit $code
    image: {{ .Image }}
    imagePullPolicy: Always
    name: "{{ .BuildName }}"
    env:
    - name: POD_NAME
      valueFrom:
        fieldRef:
          fieldPath: metadata.name
    {{- range .Env }}
    - name: {{ .Name }}
      value: '{{ .Value }}'
    {{- end }}
    {{- if gt .Port 0 }}
    ports:
    - containerPort: {{ .Port }}
      name: cluster-port
      protocol: TCP
    {{- end }}
    resources:
      requests:
        {{- if .ResourcesSpec.Requests.CPU }}
        cpu: "{{ .ResourcesSpec.Requests.CPU }}"
        {{- end }}
        {{- if .ResourcesSpec.Requests.Memory }}
        memory: "{{ .ResourcesSpec.Requests.Memory }}"
        {{- end }}
      limits:
        {{- if gt .ResourcesSpec.Accelerators.GPU 0 }}
         alpha.kubernetes.io/nvidia-gpu: "{{ .ResourcesSpec.Accelerators.GPU }}"
         {{- end }}
         {{- if .ResourcesSpec.Limits.CPU }}
         cpu: "{{ .ResourcesSpec.Limits.CPU }}"
         {{- end }}
         {{- if .ResourcesSpec.Limits.Memory }}
         memory: "{{ .ResourcesSpec.Limits.Memory }}"
         {{- end }}
{{ toYaml .Mounts | indent 4 }}
{{ toYaml .Volumes | indent 2 }}
`

type TaskResourceGenerator struct {
	JobID    string
	Callback string
	c        *Config
	task     Task
	TaskResource
	once           sync.Once
	volumes        []v1.Volume
	mounts         []v1.VolumeMount
	InitContainers []InitContainers
}

func (t TaskResourceGenerator) ResourcesSpec() ResourceRequest {
	return ResourceSpec(t.Resources, t.c.ClusterLimits, ResourceReqLim{CPU: "50m", Memory: "128Mi"})
}

func (t TaskResourceGenerator) Env() []Env {
	envs := baseEnv(t.c, t.TaskResource.Resource)
	for _, r := range t.task.Resources {
		hosts := make([]string, r.Replicas)
		for i := range hosts {
			serviceName := utils.KubePodNameEncode(fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, r.Name, t.JobID))
			hosts[i] = fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", serviceName, i, serviceName, t.Namespace())
			if r.Port > 0 {
				hosts[i] = hosts[i] + ":" + strconv.Itoa(int(r.Port))
			}
		}
		envs = append(envs, Env{
			Name:  strings.ToUpper(utils.EnvConvert(r.Name) + "_NODES"),
			Value: strings.Join(hosts, ","),
		})
	}
	envs = append(envs, Env{
		Name:  strings.ToUpper("BUILD_ID"),
		Value: t.JobID,
	})
	envs = append(envs, Env{
		Name:  strings.ToUpper("TASK_NAME"),
		Value: t.task.Name,
	})
	return envs
}
func (t TaskResourceGenerator) BuildName() string {
	return utils.KubePodNameEncode(fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, t.TaskResource.Name, t.JobID))
}
func (t TaskResourceGenerator) Mounts() interface{} {
	return map[string]interface{}{
		"volumeMounts": t.mounts,
	}
}
func (t TaskResourceGenerator) Volumes() interface{} {
	return map[string]interface{}{
		"volumes": t.volumes,
	}
}
func (t TaskResourceGenerator) Namespace() string {
	return t.c.GetNamespace()
}

func (t TaskResourceGenerator) Labels() map[string]string {
	labels := t.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     t.task.Name + "-" + t.TaskResource.Name,
		types.TASK_ID_LABEL:      t.JobID,
		types.TASK_NAME_LABEL:    t.task.Name,
		types.ComponentTypeLabel: "task",
	})
	return utils.JoinMaps(labels, t.c.Labels, t.task.Labels, t.TaskResource.Labels)
}

func (t TaskResourceGenerator) Args() string {
	//return strings.Replace(t.RawArgs, "\"", "\\\"", -1)
	return t.RawArgs
}

func (c *Config) GenerateTaskResources(task Task, jobID string, gitRefs map[string]string) ([]TaskResourceSpec, error) {
	taskSpec := make([]TaskResourceSpec, 0)
	for _, r := range task.Resources {
		volumes, mounts, err := c.KubeVolumesSpec(r.VolumeMounts(c.Volumes))
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes for '%s-%s': %v", task.Name, r.Name, err)
		}

		if gitRefs != nil {
			c.setGitRefs(volumes, gitRefs)
		}

		initContainers, err := c.KubeInits(r.VolumeMounts(c.Volumes), &task.Name, &jobID)
		if err != nil {
			return nil, fmt.Errorf("Failed generate init spec %s-%s': %v", task.Name, r.Name, err)
		}
		g := TaskResourceGenerator{
			c:              c,
			task:           task,
			TaskResource:   r,
			mounts:         mounts,
			volumes:        volumes,
			JobID:          jobID,
			InitContainers: initContainers,
		}
		res, err := kubernetes.GetTemplatedResource(ResourceTpl, g.BuildName()+":resource", g)
		if err != nil {
			return nil, err
		}
		allowFail := true
		if r.AllowFail != nil {
			allowFail = *r.AllowFail
		}
		res.Object = &kubernetes.WorkerSet{
			PodTemplate:  res.Object.(*v1.Pod),
			ResourceName: r.Name,
			TaskName:     task.Name,
			ProjectName:  c.Name,
			Namespace:    c.GetNamespace(),
			JobID:        jobID,
			AllowFail:    allowFail,
			MaxRestarts:  r.MaxRestartCount,
			Replicas:     int(r.Replicas),
			Selector: c.ResourceSelector(map[string]string{
				types.TASK_ID_LABEL:  jobID,
				types.ComponentLabel: task.Name + "-" + r.Name,
			}),
		}
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", g.BuildName(), err)
		}
		if g.Port > 0 {
			res.Deps = []*kubernetes.KubeResource{generateHeadlessService(g)}
		}
		taskSpec = append(taskSpec, TaskResourceSpec{
			DoneCondition: r.DoneCondition,
			TaskName:      task.Name,
			ResourceName:  r.Name,
			PodsNumber:    int(r.Replicas),
			Resource:      res,
			NodeAllocator: r.NodesLabel,
		})
	}
	return taskSpec, nil
}

func generateHeadlessService(g TaskResourceGenerator) *kubernetes.KubeResource {
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
	return &kubernetes.KubeResource{
		Name:   g.BuildName() + ":service",
		Object: svc,
		Kind:   &groupKind,
	}
}
