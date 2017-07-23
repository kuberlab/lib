package mlapp

import (
	"fmt"
	"github.com/kuberlab/lib/pkg/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"os"
	"strconv"
	"strings"
	"sync"
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
      {{- if and (gt .Resources.Accelerators.GPU 0) (not .Resources.Accelerators.DedicatedGPU) }}
      annotations:
        experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
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
          value "/api/v1/ml-proxy/{{ .Workspace }}/{{ .AppName }}/{{ .Name }}/"
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
        resources:
          requests:
            {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: {{ .Resources.Accelerators.GPU }}
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
{{ toYaml .Mounts | indent 8 }}
{{ toYaml .Volumes | indent 6 }}
`

const StatefulSetTpl = `
apiVersion: apps/v1beta1
kind: StatefulSet
metadata:
  name: "{{ .Task }}-{{ .Name }}-{{ .JobID }}"
  namespace: {{ .AppName }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
    {{- end }}
    workspace: {{ .AppName }}
    component: "{{ .Task }}-{{ .Name }}"
    kuberlab.io/job-id: "{{ .JobID }}"
spec:
  replicas: {{ .Replicas }}
  serviceName: "{{ .Task }}-{{ .Name }}-{{ .JobID }}"
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: {{ $value }}
        {{- end }}
        workspace: {{ .AppName }}
        component: "{{ .Task }}-{{ .Name }}"
        kuberlab.io/job-id: "{{ .JobID }}"
        service: "{{ .Task }}-{{ .Name }}-{{ .JobID }}"
      {{- if and (gt .Resources.Accelerators.GPU 0) (not .Resources.Accelerators.DedicatedGPU) }}
      annotations:
        experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
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
          cd {{.WorkDir}};
          {{.Command}} {{.ExtraArgs}};
          code=$?;
          echo "Script exit code: ${code}";
          while true; do  echo "waiting..."; curl -H "X-Source: ${POD_NAME}" -H "X-Result: ${code}" {{ .Callback }}; sleep 60; done;
          echo 'Wait deletion...';
          sleep 86400
        image: {{ .Image }}
        name: build
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          {{- range .Env }}
          - name: {{ .Name }}
            value: '{{ .Value }}'
          {{- end }}
        # Auto-deleting metric from prometheus.
        {{- if gt .Port 0 }}
        ports:
        - containerPort: {{ .Port }}
          name: cluster-port
          protocol: TCP
        {{- end }}
        resources:
          requests:
            {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: {{ .Resources.Accelerators.GPU }}
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
	envs := make([]Env, 0, len(t.TaskResource.Env))
	path := make([]string, 0)
	for _, e := range t.TaskResource.Env {
		if e.Name == "PYTHONPATH" {
			path = strings.Split(e.Value, ":")
		} else {
			envs = append(envs, e)
		}
	}
	for _, m := range t.TaskResource.Volumes {
		v := t.c.VolumeByName(m.Name)
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
	if t.TaskResource.Resources.Accelerators.GPU > 0 && !t.TaskResource.Resources.Accelerators.DedicatedGPU {
		envs = append(envs, Env{
			Name:  "KUBERLAB_GPU",
			Value: "all",
		})
	}
	for _, r := range t.task.Resources {
		//cassandra-0.cassandra.cirrus.svc.cluster.local
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
	for _, v := range t.TaskResource.Volumes {
		mountPath := v.MountPath
		if len(mountPath) == 0 {
			if v := t.c.VolumeByName(v.Name); v != nil {
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
	joinMap(labels, t.c.Labels)
	joinMap(labels, t.task.Labels)
	joinMap(labels, t.TaskResource.Labels)
	return labels
}
func (t TaskResourceGenerator) Image() string {
	if t.Resources.Accelerators.GPU > 0 {
		if len(t.Images.GPU) == 0 {
			return t.Images.CPU
		}
		return t.Images.GPU
	}
	return t.Images.CPU
}
func (t TaskResourceGenerator) ExtraArgs() string {
	return "required"
}
func (c *Config) GenerateTaskResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, task := range c.Tasks {
		for _, r := range task.Resources {
			volumes, mounts, err := c.KubeVolumesSpec(r.Volumes)
			if err != nil {
				return nil, err
			}
			data, err := kubernetes.GetTemplate(StatefulSetTpl, TaskResourceGenerator{
				c:            c,
				task:         task,
				TaskResource: r,
				mounts:       mounts,
				volumes:      volumes,
				JobID:        "1",
				Callback:     "http://test.com",
			})
			if err != nil {
				return nil, err
			}
			f, err := os.Create(fmt.Sprintf("test/%s-%s.yaml", task.Name, r.Name))
			if err != nil {
				return nil, err
			}
			f.WriteString(data)
			f.Close()
		}
	}
	return resources, nil
}

type UIXResourceGenerator struct {
	c *Config
	Uix
	volumes []v1.Volume
	mounts  []v1.VolumeMount
}

func (ui UIXResourceGenerator) Env() []Env {
	envs := make([]Env, 0, len(ui.Uix.Env))
	path := make([]string, 0)
	for _, e := range ui.Uix.Env {
		if e.Name == "PYTHONPATH" {
			path = strings.Split(e.Value, ":")
		} else {
			envs = append(envs, e)
		}
	}
	for _, m := range ui.Uix.Volumes {
		v := ui.c.VolumeByName(m.Name)
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
	if ui.Resources.Accelerators.GPU > 0 && !ui.Resources.Accelerators.DedicatedGPU {
		envs = append(envs, Env{
			Name:  "KUBERLAB_GPU",
			Value: "all",
		})
	}
	return envs
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
	joinMap(labels, ui.c.Labels)
	joinMap(labels, ui.Uix.Labels)
	return labels
}
func (ui UIXResourceGenerator) Image() string {
	if ui.Resources.Accelerators.GPU > 0 {
		if len(ui.Images.GPU) == 0 {
			return ui.Images.CPU
		}
		return ui.Images.GPU
	}
	return ui.Images.CPU
}
func (c *Config) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		volumes, mounts, err := c.KubeVolumesSpec(uix.Volumes)
		if err != nil {
			return nil, err
		}
		data, err := kubernetes.GetTemplate(DeploymentTpl, UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes})
		if err != nil {
			return nil, err
		}

		/*res, err := kubernetes.GetKubeResource(uix.Name, data, insertVolumes)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
		svc := GenerateServiceDeploy(*res.Object.(*extv1beta1.Deployment), uix.Ports)
		kind := svc.GroupVersionKind()
		resources = append(
			resources,
			&kubernetes.KubeResource{Kind: &kind, Object: svc, Name: svc.Name},
		)*/
		f, err := os.Create("test/" + uix.Name + ".yaml")
		if err != nil {
			return nil, err
		}
		f.WriteString(data)
		f.Close()
	}
	return resources, nil
}

func GenerateServiceDeploy(deploy extv1beta1.Deployment, portSpec []Port) *v1.Service {
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      deploy.Name,
			Namespace: deploy.Namespace,
			Labels:    deploy.Spec.Template.Labels,
		},
		Spec: v1.ServiceSpec{
			Selector: deploy.Spec.Template.Labels,
			Type:     v1.ServiceTypeClusterIP,
		},
	}
	portsByName := make(map[string]Port)
	for _, p := range portSpec {
		portsByName[p.Name] = p
	}
	for _, p := range deploy.Spec.Template.Spec.Containers[0].Ports {
		svc.Spec.Ports = append(
			svc.Spec.Ports,
			v1.ServicePort{
				Name:       p.Name,
				TargetPort: intstr.IntOrString{IntVal: int32(portsByName[p.Name].TargetPort)},
				Protocol:   p.Protocol,
				Port:       p.ContainerPort,
			},
		)
	}
	return svc
}

func GenerateHeadlessService(set appsv1beta1.StatefulSet, portSpec []Port) *v1.Service {
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      set.Name,
			Namespace: set.Namespace,
			Labels:    set.Spec.Template.Labels,
		},
		Spec: v1.ServiceSpec{
			Selector:  set.Spec.Template.Labels,
			ClusterIP: v1.ClusterIPNone,
		},
	}

	portsByName := make(map[string]Port)
	for _, p := range portSpec {
		portsByName[p.Name] = p
	}
	for _, p := range set.Spec.Template.Spec.Containers[0].Ports {
		svc.Spec.Ports = append(
			svc.Spec.Ports,
			v1.ServicePort{
				Name:       p.Name,
				TargetPort: intstr.IntOrString{IntVal: int32(portsByName[p.Name].TargetPort)},
				Protocol:   p.Protocol,
				Port:       p.ContainerPort,
			},
		)
	}

	return svc
}
