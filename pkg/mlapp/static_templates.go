package mlapp

import (
	"fmt"
	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	"strconv"
	"strings"
)

const DeploymentTpl = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: "{{ .ComponentName }}"
  namespace: "{{ .Namespace }}"
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
spec:
  replicas: {{ .Replicas }}
  revisionHistoryLimit: 1
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: "{{ $value }}"
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
      - name: {{ .ComponentName }}
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
{{ toYaml .Mounts | indent 8 }}
{{ toYaml .Volumes | indent 6 }}
`

type UIXResourceGenerator struct {
	c *Config
	Uix
	volumes        []v1.Volume
	mounts         []v1.VolumeMount
	InitContainers []InitContainers
}

func (ui UIXResourceGenerator) ResourcesSpec() ResourceRequest {
	return ResourceSpec(ui.Resources, ui.c.ClusterLimits, ResourceReqLim{CPU: "50m", Memory: "128Mi"})
}

func (ui UIXResourceGenerator) Replicas() int {
	if ui.Resource.Replicas > 0 {
		return ui.Resource.Replicas
	}
	return 1
}
func (ui UIXResourceGenerator) Env() []Env {
	env := baseEnv(ui.c, ui.Resource)
	env = append(env,
		Env{
			Name:  "URL_PREFIX",
			Value: ui.c.ProxyURL(ui.Uix.Name),
		},
	)
	return env
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
func (ui UIXResourceGenerator) Namespace() string {
	return ui.c.GetNamespace()
}

func (ui UIXResourceGenerator) ComponentName() string {
	return utils.KubeDeploymentEncode(ui.c.Name + "-" + ui.Name)
}

func (ui UIXResourceGenerator) Labels() map[string]string {
	labels := ui.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     ui.Uix.Name,
		types.ComponentTypeLabel: "ui",
	})
	return utils.JoinMaps(labels, ui.c.Labels, ui.Uix.Labels)

}

func (ui UIXResourceGenerator) Args() string {
	return ui.Resource.RawArgs
}

func (c *Config) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		volumes, mounts, err := c.KubeVolumesSpec(uix.VolumeMounts(c.Volumes))
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes '%s': %v", uix.Name, err)
		}
		initContainers, err := c.KubeInits(uix.VolumeMounts(c.Volumes), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("Failed generate init spec '%s': %v", uix.Name, err)
		}
		g := UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes, InitContainers: initContainers}
		res, err := kubernetes.GetTemplatedResource(DeploymentTpl, g.ComponentName()+":resource", g)
		if err != nil {
			return nil, fmt.Errorf("Failed parse template '%s': %v", g.ComponentName(), err)
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
	labels := serving.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     serving.Uix.Name,
		types.ComponentTypeLabel: "serving",
		types.ServingIDLabel:     serving.Name(),
	})
	return utils.JoinMaps(labels, serving.c.Labels, serving.Uix.Labels)
}

func (serving ServingResourceGenerator) Name() string {
	return fmt.Sprintf("%s-%s-%s", serving.Uix.Name, serving.TaskName, serving.Build)
}

func (serving ServingResourceGenerator) ComponentName() string {
	return utils.KubeDeploymentEncode(fmt.Sprintf("%s-%s", serving.c.Name, serving.Name()))
}

func (c *Config) GenerateServingResources(serving Serving) ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	volumes, mounts, err := c.KubeVolumesSpec(serving.VolumeMounts(c.Volumes))
	if err != nil {
		return nil, fmt.Errorf("Failed get volumes '%s': %v", serving.Name, err)
	}
	initContainers, err := c.KubeInits(serving.VolumeMounts(c.Volumes), nil, nil)
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
	res, err := kubernetes.GetTemplatedResource(DeploymentTpl, g.ComponentName()+":resource", g)
	if err != nil {
		return nil, fmt.Errorf("Failed parse template '%s': %v", g.ComponentName(), err)
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
			Name:      serv.ComponentName(),
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
		Name:   serv.ComponentName() + ":service",
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
			Name:      ui.ComponentName(),
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
		Name:   ui.ComponentName() + ":service",
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
	for _, m := range r.VolumeMounts(c.Volumes) {
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
	for _, v := range r.VolumeMounts(c.Volumes) {
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
	envs = append(envs, Env{Name: "PROJECT_NAME", Value: c.Name})
	envs = append(envs, Env{Name: "PROJECT_ID", Value: c.ProjectID})
	envs = append(envs, Env{Name: "WORKSPACE_ID", Value: c.WorkspaceID})
	return envs
}
