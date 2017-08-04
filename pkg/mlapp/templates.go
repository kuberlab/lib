package mlapp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/kuberlab/lib/pkg/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
)

const DeploymentTpl = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: "{{ .Name }}"
  namespace: "{{ .AppName }}"
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
    workspace: "{{ .AppName }}"
    component: "{{ .Name }}"
spec:
  replicas: {{ .Replicas }}
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: "{{ $value }}"
        {{- end }}
        workspace: "{{ .AppName }}"
        component: "{{ .Name }}"
      {{- if .Resources }}
      {{- if gt .Resources.Accelerators.GPU 0 }}
      annotations:
        experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
      {{- end }}
      {{- end }}
    spec:
      containers:
      - name: {{ .AppName }}-{{ .Name }}
        {{- if .Command }}
        command: ["/bin/sh", "-c"]
        args:
        - >
          {{- if .WorkDir }}
          cd {{ .WorkDir }};
          {{- end }}
          {{ .Command }} {{ .Args }};
          code=$?;
          exit $code
        {{- end }}
        image: "{{ .Image }}"
        env:
        {{- range .Env }}
        - name: {{ .Name }}
          value: '{{ .Value }}'
        {{- end }}
        - name: URL_PREFIX
          value: "/api/v1/ml2-proxy/{{ .Workspace }}/{{ .AppName }}/{{ .Name }}/"
        {{- if .Ports }}
        ports:
        {{- range .Ports }}
        - name: {{ .Name }}
          {{- if .Protocol }}
          protocol: {{ .Protocol }}
          {{- end }}
          {{- if .TargetPort }}
          containerPort: {{ .TargetPort }}
          {{- end }}
        {{- end }}
        {{- end }}
        {{- if .Resources }}
        resources:
          requests:
            {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
            {{- end }}
            {{- if .Resources.Requests.CPU }}
            cpu: "{{ .Resources.Requests.CPU }}"
            {{- end }}
            {{- if .Resources.Requests.Memory }}
            memory: "{{ .Resources.Requests.Memory }}"
            {{- end }}
          limits:
            {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
            {{- end }}
            {{- if .Resources.Limits.CPU}}
            cpu: "{{ .Resources.Limits.CPU }}"
            {{- end }}
            {{- if .Resources.Limits.Memory }}
            memory: "{{ .Resources.Limits.Memory }}"
            {{- end }}
         {{- end }}
{{ toYaml .Mounts | indent 8 }}
{{ toYaml .Volumes | indent 6 }}
`

const ResourceTpl = `
apiVersion: v1
kind: Pod
metadata:
  name: "{{ .BuildName }}"
  namespace: {{ .AppName }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
    workspace: "{{ .AppName }}"
    component: "{{ .Task }}-{{ .Name }}"
    kuberlab.io/job-id: "{{ .JobID }}"
    kuberlab.io/task: "{{ .Task }}"
  {{- if .Resources }}
  {{- if gt .Resources.Accelerators.GPU 0 }}
  annotations:
    experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
  {{- end }}
  {{- end }}
spec:
  terminationGracePeriodSeconds: 10
  {{- if .NodesLabel }}
  nodeSelector:
    kuberlab.io/mljob: {{ .NodesLabel }}
  {{- end }}
  hostname: "{{ .BuildName }}"
  subdomain: "{{ .BuildName }}"
  restartPolicy: Never
  containers:
  - command: ["/bin/sh", "-c"]
    args:
    - >
      cd {{ .WorkDir }};
      {{ .Command }} {{ .Args }};
      code=$?;
      exit $code
    image: {{ .Image }}
    name: {{ .Task }}-{{ .JobID }}
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
    {{- if .Resources }}
    resources:
      requests:
         {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
        {{- end }}
        {{- if .Resources.Requests.CPU }}
        cpu: "{{ .Resources.Requests.CPU }}"
        {{- end }}
        {{- if .Resources.Requests.Memory }}
        memory: "{{ .Resources.Requests.Memory }}"
        {{- end }}
      limits:
        {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
        {{- end }}
        {{- if .Resources.Limits.CPU}}
        cpu: "{{ .Resources.Limits.CPU }}"
        {{- end }}
        {{- if .Resources.Limits.Memory }}
        memory: "{{ .Resources.Limits.Memory }}"
        {{- end }}
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
	once    sync.Once
	volumes []v1.Volume
	mounts  []v1.VolumeMount
}

func (t TaskResourceGenerator) Task() string {
	return t.task.Name
}

func (t TaskResourceGenerator) Env() []Env {
	envs := baseEnv(t.c, t.TaskResource.Resource)
	for _, r := range t.task.Resources {
		hosts := make([]string, r.Replicas)
		for i := range hosts {
			serviceName := fmt.Sprintf("%s-%s-%s", t.task.Name, r.Name, t.JobID)
			hosts[i] = fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", serviceName, i, serviceName, t.AppName())
			if r.Port > 0 {
				hosts[i] = hosts[i] + ":" + strconv.Itoa(int(r.Port))
			}
		}
		envs = append(envs, Env{
			Name:  strings.ToUpper(r.Name + "_NODES"),
			Value: strings.Join(hosts, ","),
		})
	}
	envs = append(envs, Env{
		Name:  strings.ToUpper("BUILD_ID"),
		Value: t.JobID,
	})
	return envs
}
func (t TaskResourceGenerator) BuildName() string {
	return fmt.Sprintf("%s-%s-%s", t.task.Name, t.TaskResource.Name, t.JobID)
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
func (t TaskResourceGenerator) AppName() string {
	return t.c.Name
}
func (t TaskResourceGenerator) Workspace() string {
	return t.c.Workspace
}
func (t TaskResourceGenerator) Labels() map[string]string {
	labels := make(map[string]string, 0)
	joinMaps(labels, t.c.Labels, t.task.Labels, t.TaskResource.Labels)
	return labels
}

func (t TaskResourceGenerator) Args() string {
	//return strings.Replace(t.RawArgs, "\"", "\\\"", -1)
	return t.RawArgs
}

func (c *Config) GenerateTaskResources(task Task, jobID string) ([]TaskResourceSpec, error) {
	taskSpec := make([]TaskResourceSpec, 0)
	for _, r := range task.Resources {
		volumes, mounts, err := c.KubeVolumesSpec(r.Volumes)
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes for '%s-%s': %v", task.Name, r.Name, err)
		}

		g := TaskResourceGenerator{
			c:            c,
			task:         task,
			TaskResource: r,
			mounts:       mounts,
			volumes:      volumes,
			JobID:        jobID,
		}
		res, err := kubernetes.GetTemplatedResource(ResourceTpl, g.BuildName()+":resource", g)
		if err != nil {
			return nil, err
		}
		res.Object = &WorkerSet{
			PodTemplate:  res.Object.(*v1.Pod),
			ResourceName: r.Name,
			TaskName:     task.Name,
			AppName:      c.Name,
			JobID:        jobID,
			AllowFail:    r.AllowFail,
			MaxRestarts:  r.MaxRestartCount,
			Replicas:     int(r.Replicas),
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
			AllowFail:     r.AllowFail,
			PodsNumber:    int(r.Replicas),
			Resource:      res,
			NodeAllocator: r.NodesLabel,
		})
	}
	return taskSpec, nil
}

func generateHeadlessService(g TaskResourceGenerator) *kubernetes.KubeResource {
	labels := map[string]string{
		"kuberlab.io/job-id": g.JobID,
		"component":          g.task.Name + "-" + g.TaskResource.Name,
		"kuberlab.io/task":   g.task.Name,
	}
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      g.BuildName(),
			Namespace: g.c.Name,
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

type UIXResourceGenerator struct {
	c *Config
	Uix
	volumes []v1.Volume
	mounts  []v1.VolumeMount
}

func (ui UIXResourceGenerator) Replicas() int {
	if ui.Resource.Replicas > 0 {
		return ui.Resource.Replicas
	}
	return 1
}
func (ui UIXResourceGenerator) Env() []Env {
	return baseEnv(ui.c, ui.Resource)
}
func (ui UIXResourceGenerator) Mounts() interface{} {
	return map[string]interface{}{
		"volumeMounts": ui.mounts,
	}
}
func (ui UIXResourceGenerator) Volumes() interface{} {
	return map[string]interface{}{
		"volumes": ui.volumes,
	}
}
func (ui UIXResourceGenerator) AppName() string {
	return ui.c.Name
}
func (ui UIXResourceGenerator) Workspace() string {
	return ui.c.Workspace
}
func (ui UIXResourceGenerator) Labels() map[string]string {
	labels := make(map[string]string, 0)
	joinMaps(labels, ui.c.Labels, ui.Uix.Labels)
	return labels
}

func (ui UIXResourceGenerator) Args() string {
	return ui.Resource.RawArgs
}

func (c *Config) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		volumes, mounts, err := c.KubeVolumesSpec(uix.Volumes)
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes '%s': %v", uix.Name, err)
		}
		g := UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes}
		res, err := kubernetes.GetTemplatedResource(DeploymentTpl, uix.Name+":resource", g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", uix.Name, err)
		}

		res.Deps = []*kubernetes.KubeResource{generateUIService(g)}
		resources = append(resources, res)
	}
	return resources, nil
}

type ServingResourceGenerator struct {
	UIXResourceGenerator
	TaskName string
	Build    string
}

func (serving ServingResourceGenerator) Env() []Env {
	envs := baseEnv(serving.c, serving.Resource)
	envs = append(envs,
		Env{
			Name:  "BUILD_ID",
			Value: serving.Build,
		},
		Env{
			Name:  "TASK_ID",
			Value: serving.TaskName,
		},
	)
	return envs
}
func (serving ServingResourceGenerator) Labels() map[string]string {
	labels := serving.UIXResourceGenerator.Labels()
	labels["kuberlab.io/serving-id"] = serving.Name()
	return labels
}

func (serving ServingResourceGenerator) Name() string {
	return fmt.Sprintf("%v-%v-%v", serving.Uix.Name, serving.TaskName, serving.Build)
}

func (c *Config) GenerateServingResources(serving Serving) ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	volumes, mounts, err := c.KubeVolumesSpec(serving.Volumes)
	if err != nil {
		return nil, fmt.Errorf("Failed get volumes '%s': %v", serving.Name, err)
	}
	g := ServingResourceGenerator{
		TaskName: serving.TaskName,
		Build:    serving.Build,
		UIXResourceGenerator: UIXResourceGenerator{
			c:       c,
			Uix:     serving.Uix,
			mounts:  mounts,
			volumes: volumes,
		},
	}
	res, err := kubernetes.GetTemplatedResource(DeploymentTpl, serving.Name+":resource", g)
	if err != nil {
		return nil, fmt.Errorf("Failed parse template '%s': %v", serving.Name, err)
	}
	res.Deps = []*kubernetes.KubeResource{generateServingService(g)}
	resources = append(resources, res)
	return resources, nil
}

func generateServingService(serv ServingResourceGenerator) *kubernetes.KubeResource {
	labels := map[string]string{
		"workspace":              serv.AppName(),
		"component":              serv.Name(),
		"kuberlab.io/serving-id": serv.Name(),
	}
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      serv.Name(),
			Namespace: serv.AppName(),
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	for _, p := range serv.Ports {
		svc.Spec.Ports = append(
			svc.Spec.Ports,
			v1.ServicePort{
				Name:       p.Name,
				TargetPort: intstr.FromInt(int(p.TargetPort)),
				Protocol:   v1.Protocol(p.Protocol),
				Port:       p.Port,
			},
		)
	}
	groupKind := svc.GroupVersionKind()
	return &kubernetes.KubeResource{
		Name:   serv.Name() + ":service",
		Object: svc,
		Kind:   &groupKind,
	}
}
func generateUIService(ui UIXResourceGenerator) *kubernetes.KubeResource {
	labels := map[string]string{
		"workspace": ui.AppName(),
		"component": ui.Name,
	}
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      ui.Name,
			Namespace: ui.AppName(),
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	for _, p := range ui.Ports {
		svc.Spec.Ports = append(
			svc.Spec.Ports,
			v1.ServicePort{
				Name:       p.Name,
				TargetPort: intstr.FromInt(int(p.TargetPort)),
				Protocol:   v1.Protocol(p.Protocol),
				Port:       p.Port,
			},
		)
	}
	groupKind := svc.GroupVersionKind()
	return &kubernetes.KubeResource{
		Name:   ui.Name + ":service",
		Object: svc,
		Kind:   &groupKind,
	}
}

func baseEnv(c *Config, r Resource) []Env {
	envs := make([]Env, 0, len(r.Env))
	path := make([]string, 0)
	for _, e := range r.Env {
		if e.Name == "PYTHONPATH" {
			path = strings.Split(e.Value, ":")
		} else {
			envs = append(envs, e)
		}
	}
	for _, m := range r.Volumes {
		v := c.VolumeByName(m.Name)
		if v == nil {
			continue
		}
		if !v.IsLibDir {
			continue
		}
		mount := m.MountPath
		if len(mount) < 1 {
			mount = v.MountPath
		}
		path = append(path, mount)
	}
	envs = append(envs, Env{
		Name:  "PYTHONPATH",
		Value: strings.Join(path, ":"),
	})
	if r.Resources != nil && r.Resources.Accelerators.GPU > 0 && !r.Resources.Accelerators.DedicatedGPU {
		envs = append(envs, Env{
			Name:  "KUBERLAB_GPU",
			Value: "all",
		})
	}
	if r.Resources != nil && r.Resources.Accelerators.GPU > 0 {
		envs = append(envs, Env{
			Name:  strings.ToUpper("GPU_COUNT"),
			Value: strconv.Itoa(int(r.Resources.Accelerators.GPU)),
		})
	} else {
		envs = append(envs, Env{
			Name:  strings.ToUpper("GPU_COUNT"),
			Value: "0",
		})
	}
	for _, v := range r.Volumes {
		mountPath := v.MountPath
		if len(mountPath) == 0 {
			if v := c.VolumeByName(v.Name); v != nil {
				mountPath = v.MountPath
			}
		}
		if len(mountPath) > 0 {
			envs = append(envs, Env{
				Name:  strings.ToUpper(v.Name + "_DIR"),
				Value: mountPath,
			})
		}
	}
	return envs
}
