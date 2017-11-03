package mlapp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
)

const DeploymentTpl = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: "{{ .AppName }}-{{ .Name }}"
  namespace: "{{ .Namespace }}"
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
spec:
  replicas: {{ .Replicas }}
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: "{{ $value }}"
        {{- end }}
      {{- if .Resources }}
      {{- if gt .Resources.Accelerators.GPU 0 }}
      annotations:
        experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
      {{- end }}
      {{- end }}
    spec:
      {{- if gt (len .InitContainers) 0 }}
      initContainers:
      {{- range $i, $value := .InitContainers }}
      - name: {{ $value.Name }}
        image: {{ $value.Image }}
        command: {{ $value.Command }}
{{ toYaml $value.Mounts | indent 8 }}
      {{- end }}
      {{- end }}
      containers:
      - name: {{ .AppName }}-{{ .Name }}
        {{- if .Command }}
        command: ["/bin/sh", "-c"]
        imagePullPolicy: Always
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
          value: "{{ .ProxyURL }}"
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
            {{- if and (and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU) (gt .Limits.GPU 0) }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Limits.GPU }}"
            {{- else }}
              {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
              {{- end }}
            {{- end }}
            {{- if .Resources.Requests.CPU }}
            cpu: "{{ .Resources.Requests.CPU }}"
            {{- end }}
            {{- if .Resources.Requests.Memory }}
            memory: "{{ .Resources.Requests.Memory }}"
            {{- end }}
          limits:
            {{- if and (and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU) (gt .Limits.GPU 0) }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Limits.GPU }}"
            {{- else }}
              {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
              {{- end }}
            {{- end }}
            {{- if gt (len .Limits.CPU) 0 }}
            cpu: "{{ .Limits.CPU }}"
            {{- end }}
            {{- if gt (len .Limits.Memory) 0 }}
            memory: "{{ .Limits.Memory }}"
            {{- end }}
        {{- else }}
        {{- if or (gt (len .Limits.CPU) 0) (gt (len .Limits.Memory) 0) }}
        resources:
          limits:
            {{- if gt (len .Limits.CPU) 0 }}
            cpu: "{{ .Limits.CPU }}"
            {{- end }}
            {{- if gt (len .Limits.Memory) 0 }}
            memory: "{{ .Limits.Memory }}"
            {{- end }}
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
  namespace: {{ .Namespace }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
  {{- if .Resources }}
  {{- if gt .Resources.Accelerators.GPU 0 }}
  annotations:
    experimental.kubernetes.io/nvidia-gpu-driver: "http://127.0.0.1:3476/v1.0/docker/cli/json"
  {{- end }}
  {{- end }}
spec:
  terminationGracePeriodSeconds: 10
  #{{- if .NodesLabel }}
  #nodeSelector:
  #  kuberlab.io/mljob: {{ .NodesLabel }}
  #{{- end }}
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
        {{- if and (and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU) (gt .Limits.GPU 0) }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Limits.GPU }}"
        {{- else }}
          {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
          {{- end }}
        {{- end }}
        {{- if .Resources.Requests.CPU }}
        cpu: "{{ .Resources.Requests.CPU }}"
        {{- end }}
        {{- if .Resources.Requests.Memory }}
        memory: "{{ .Resources.Requests.Memory }}"
        {{- end }}
      limits:
        {{- if and (and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU) (gt .Limits.GPU 0) }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Limits.GPU }}"
        {{- else }}
          {{- if and (gt .Resources.Accelerators.GPU 0) .Resources.Accelerators.DedicatedGPU }}
        alpha.kubernetes.io/nvidia-gpu: "{{ .Resources.Accelerators.GPU }}"
          {{- end }}
        {{- end }}
        {{- if gt (len .Limits.CPU) 0 }}
        cpu: "{{ .Limits.CPU }}"
		{{- end }}
		{{- if gt (len .Limits.Memory) 0 }}
        memory: "{{ .Limits.Memory }}"
		{{- end }}
    {{- else }}
    {{- if or (gt (len .Limits.CPU) 0) (gt (len .Limits.Memory) 0) }}
    resources:
      limits:
        {{- if gt (len .Limits.CPU) 0 }}
        cpu: "{{ .Limits.CPU }}"
        {{- end }}
        {{- if gt (len .Limits.Memory) 0 }}
        memory: "{{ .Limits.Memory }}"
        {{- end }}
    {{- end }}
    {{- end }}
{{ toYaml .Mounts | indent 4 }}
{{ toYaml .Volumes | indent 2 }}
`

const ComponentTypeLabel = "kuberlab.io/component-type"

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

func (t TaskResourceGenerator) Limits() ResourceReqLim {
	if t.c.ClusterLimits != nil {
		return *t.c.ClusterLimits
	}
	if t.TaskResource.Resources != nil {
		return t.TaskResource.Resources.Limits
	}
	return ResourceReqLim{}
}

func (t TaskResourceGenerator) Task() string {
	return t.task.Name
}

func (t TaskResourceGenerator) Env() []Env {
	envs := baseEnv(t.c, t.TaskResource.Resource)
	for _, r := range t.task.Resources {
		hosts := make([]string, r.Replicas)
		for i := range hosts {
			serviceName := fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, r.Name, t.JobID)
			hosts[i] = fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", serviceName, i, serviceName, t.Namespace())
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
	envs = append(envs, Env{
		Name:  strings.ToUpper("TASK_NAME"),
		Value: t.task.Name,
	})
	return envs
}
func (t TaskResourceGenerator) BuildName() string {
	return fmt.Sprintf("%s-%s-%s-%s", t.c.Name, t.task.Name, t.TaskResource.Name, t.JobID)
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
func (t TaskResourceGenerator) AppName() string {
	return t.c.Name
}
func (t TaskResourceGenerator) Workspace() string {
	return t.c.Workspace
}
func (t TaskResourceGenerator) Labels() map[string]string {
	labels := t.c.ResourceLabels(map[string]string{"workspace": t.AppName(),
		"component":          t.task.Name + "-" + t.TaskResource.Name,
		"kuberlab.io/job-id": t.JobID,
		"kuberlab.io/task":   t.task.Name,
		ComponentTypeLabel:   "task"})
	return utils.JoinMaps(labels, t.c.Labels, t.task.Labels, t.TaskResource.Labels)
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
		initContainers, err := c.KubeInits(r.Volumes)
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
		res.Object = &kubernetes.WorkerSet{
			PodTemplate:  res.Object.(*v1.Pod),
			ResourceName: r.Name,
			TaskName:     task.Name,
			AppName:      c.Name,
			Namespace:    c.GetNamespace(),
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

type UIXResourceGenerator struct {
	c *Config
	Uix
	volumes        []v1.Volume
	mounts         []v1.VolumeMount
	InitContainers []InitContainers
}

func (ui UIXResourceGenerator) ProxyURL() string {
	return fmt.Sprintf("/api/v1/ml2-proxy/%s/%s/%s/", ui.Workspace(), ui.AppName(), ui.Uix.Name)
}

func (ui UIXResourceGenerator) Name() string {
	return ui.Uix.Name
}
func (ui UIXResourceGenerator) Limits() ResourceReqLim {
	if ui.c.ClusterLimits != nil {
		return *ui.c.ClusterLimits
	}
	if ui.Uix.Resource.Resources != nil {
		return ui.Uix.Resources.Limits
	}
	return ResourceReqLim{}
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
func (t UIXResourceGenerator) Namespace() string {
	return t.c.GetNamespace()
}
func (ui UIXResourceGenerator) AppName() string {
	return ui.c.Name
}
func (ui UIXResourceGenerator) Workspace() string {
	return ui.c.Workspace
}
func (ui UIXResourceGenerator) Labels() map[string]string {
	labels := ui.c.ResourceLabels(map[string]string{"workspace": ui.AppName(),
		"component":        ui.Uix.Name,
		ComponentTypeLabel: "ui"})
	return utils.JoinMaps(labels, ui.c.Labels, ui.Uix.Labels)

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
		initContainers, err := c.KubeInits(uix.Volumes)
		if err != nil {
			return nil, fmt.Errorf("Failed generate init spec '%s': %v", uix.Name, err)
		}
		g := UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes, InitContainers: initContainers}
		res, err := kubernetes.GetTemplatedResource(DeploymentTpl, g.Name()+":resource", g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", g.Name(), err)
		}

		res.Deps = []*kubernetes.KubeResource{generateUIService(g)}
		resources = append(resources, res)
	}
	return resources, nil
}

func (c *Config) GeneratePVC() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	labels := map[string]string{
		KUBELAB_WS_LABEL:    c.Workspace,
		KUBELAB_WS_ID_LABEL: c.WorkspaceID,
	}
	for _, v := range c.Volumes {
		if v.PersistentStorage != nil {
			q, err := resource.ParseQuantity(v.PersistentStorage.Size)
			if err != nil {
				return nil, fmt.Errorf("Invalid kuberlab storage size %v", err)
			}
			storageClass := "glusterfs"
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      v.PersistentStorage.StorageName,
					Namespace: c.GetNamespace(),
					Labels:    labels,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
					StorageClassName: &storageClass,
					Resources: v1.ResourceRequirements{
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceStorage: q,
						},
					},
				},
			}
			groupKind := pvc.GroupVersionKind()
			resources = append(resources, &kubernetes.KubeResource{
				Name:   v.PersistentStorage.StorageName + ":pvc",
				Object: pvc,
				Kind:   &groupKind,
			})
		}
	}
	return resources, nil
}

type ServingResourceGenerator struct {
	UIXResourceGenerator
	TaskName string
	Build    string
}

func (serving ServingResourceGenerator) Limits() ResourceReqLim {
	return serving.UIXResourceGenerator.Limits()
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
	labels[ComponentTypeLabel] = "serving"
	return labels
}

func (serving ServingResourceGenerator) Name() string {
	return fmt.Sprintf("%v-%v-%v-%v", serving.c.Name, serving.Uix.Name, serving.TaskName, serving.Build)
}

func (c *Config) GenerateServingResources(serving Serving) ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	volumes, mounts, err := c.KubeVolumesSpec(serving.Volumes)
	if err != nil {
		return nil, fmt.Errorf("Failed get volumes '%s': %v", serving.Name, err)
	}
	initContainers, err := c.KubeInits(serving.Volumes)
	if err != nil {
		return nil, fmt.Errorf("Failed generate init spec '%s': %v", serving.Name, err)
	}
	g := ServingResourceGenerator{
		TaskName: serving.TaskName,
		Build:    serving.Build,
		UIXResourceGenerator: UIXResourceGenerator{
			c:              c,
			Uix:            serving.Uix,
			mounts:         mounts,
			volumes:        volumes,
			InitContainers: initContainers,
		},
	}
	res, err := kubernetes.GetTemplatedResource(DeploymentTpl, g.Name()+":resource", g)
	if err != nil {
		return nil, fmt.Errorf("Failed parse template '%s': %v", g.Name(), err)
	}
	res.Deps = []*kubernetes.KubeResource{generateServingService(g)}
	resources = append(resources, res)
	return resources, nil
}

func generateServingService(serv ServingResourceGenerator) *kubernetes.KubeResource {
	labels := serv.Labels()
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      serv.Name(),
			Namespace: serv.Namespace(),
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
	labels := ui.Labels()
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      ui.c.Name+"-"+ui.Name(),
			Namespace: ui.Namespace(),
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
		Name:   ui.Name() + ":service",
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
		count := r.Resources.Accelerators.GPU
		if c.ClusterLimits != nil && c.ClusterLimits.GPU > 0 {
			count = c.ClusterLimits.GPU
		}
		envs = append(envs, Env{
			Name:  "GPU_COUNT",
			Value: strconv.Itoa(int(count)),
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

	envs = append(envs, Env{Name: "WORKSPACE_NAME", Value: c.Workspace})
	envs = append(envs, Env{Name: "APP_NAME", Value: c.Name})

	return envs
}
