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
  namespace: {{ .AppName }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
    {{- end }}
    workspace: {{ .AppName }}
    component: {{ .Name }}
spec:
  replicas: 1
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: {{ $value }}
        {{- end }}
        workspace: {{ .AppName }}
        component: {{ .Name }}
      {{- if .Resources }}
      {{- if and (gt .Resources.Accelerators.GPU 0) (not .Resources.Accelerators.DedicatedGPU) }}
      annotations:
        experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
      {{- end }}
      {{- end }}
    spec:
      containers:
      - name: {{ .AppName }}-{{ .Name }}
        {{- if .Command }}
        command: ["{{ .Command }}"]
        {{- end }}
        {{- if .RawArgs }}
          {{- if gt (len .RawArgs) 0 }}
        args:
          {{- range .Args }}
          - {{ . }}
          {{- end }}
          {{- end }}
        {{- end }}
        image: "{{ .Image }}"
        env:
        {{- range .Env }}
        - name: {{ .Name }}
          value: '{{ .Value }}'
        {{- end }}
        - name: URL_PREFIX
          value: "/api/v1/ml-proxy/{{ .Workspace }}/{{ .AppName }}/{{ .Name }}/"
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

const StatefulSetTpl = `
apiVersion: apps/v1beta1
kind: StatefulSet
metadata:
  name: "{{ .BuildName }}"
  namespace: {{ .AppName }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
    {{- end }}
    workspace: {{ .AppName }}
    component: "{{ .Task }}-{{ .Name }}"
    kuberlab.io/job-id: "{{ .JobID }}"
    kuberlab.io/task: "{{ .Task }}"
spec:
  replicas: {{ .Replicas }}
  serviceName: "{{ .BuildName }}"
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: {{ $value }}
        {{- end }}
        workspace: {{ .AppName }}
        component: "{{ .Task }}-{{ .Name }}"
        kuberlab.io/job-id: "{{ .JobID }}"
        service: "{{ .BuildName }}"
      {{- if .Resources }}
      {{- if and (gt .Resources.Accelerators.GPU 0) (not .Resources.Accelerators.DedicatedGPU) }}
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
      containers:
      - command: ["/bin/sh", "-c"]
        args:
        - >
          task_id=$(hostname | rev | cut -d ''-'' -f 1 | rev);
          echo "Start with task-id=$task_id";
          cd {{ .WorkDir }};
          {{ .Command }} {{ .ExtraArgs }};
          code=$?;
          echo "Script exit code: ${code}";
          while true; do  echo "waiting..."; curl -H "X-Source: $task_id" -H "X-Result: ${code}" {{ .Callback }}; sleep 60; done;
          echo 'Wait deletion...';
          sleep 86400
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
			serviceName := t.BuildName()
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

func (t TaskResourceGenerator) ExtraArgs() string {
	return t.RawArgs
}

func (c *Config) GenerateTaskResources(task Task, submitURL string, jobID string) ([]TaskResourceSpec, error) {
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
			Callback:     fmt.Sprintf("%s/%s/%s/%s/%s", submitURL, c.Name, task.Name, r.Name, jobID),
		}
		data, err := kubernetes.GetTemplate(StatefulSetTpl, g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", g.BuildName(), err)
		}
		res, err := kubernetes.GetKubeResource(g.BuildName()+":resource", data, nil)
		if err != nil {
			return nil, fmt.Errorf("Failed get kube resource '%s': %v", g.BuildName(), err)
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

func (ui UIXResourceGenerator) Args() []string {
	return strings.Split(ui.RawArgs, " ")
}
func (c *Config) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		volumes, mounts, err := c.KubeVolumesSpec(uix.Volumes)
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes '%s': %v", uix.Name, err)
		}
		g := UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes}
		data, err := kubernetes.GetTemplate(DeploymentTpl, g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", uix.Name, err)
		}

		res, err := kubernetes.GetKubeResource(uix.Name+":resource", data, nil)
		if err != nil {
			return nil, fmt.Errorf("Failed get kube resource '%s': %v", uix.Name, err)
		}
		res.Deps = []*kubernetes.KubeResource{generateUIService(g)}
		resources = append(resources, res)
	}
	return resources, nil
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
