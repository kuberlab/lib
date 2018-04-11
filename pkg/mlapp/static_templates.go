package mlapp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
      {{- if.NodeSelector }}
      nodeSelector:
        kuberlab.io/ml-node: .NodeSelector
      {{- end }}
      {{- if gt (len .InitContainers) 0 }}
      initContainers:
      {{- range $i, $value := .InitContainers }}
      - name: {{ $value.Name }}
        image: {{ $value.Image }}
        command: {{ $value.Command }}
{{ toYaml $value.Mounts | indent 8 }}
      {{- end }}
      {{- end }}
      tolerations:
      - key: role.kuberlab.io/cpu-compute
        effect: PreferNoSchedule
      {{- if gt .ResourcesSpec.Accelerators.GPU 0 }}
      - key: role.kuberlab.io/gpu-compute
        effect: PreferNoSchedule
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
        - name: RESOURCE_NAME
          value: '{{ .ComponentName }}'
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
            {{- if .ResourcesSpec.Requests.CPUQuantity }}
            cpu: "{{ .ResourcesSpec.Requests.CPUQuantity }}"
            {{- end }}
            {{- if .ResourcesSpec.Requests.MemoryQuantity }}
            memory: "{{ .ResourcesSpec.Requests.MemoryQuantity }}"
            {{- end }}
          limits:
            {{- if gt .ResourcesSpec.Accelerators.GPU 0 }}
            alpha.kubernetes.io/nvidia-gpu: "{{ .ResourcesSpec.Accelerators.GPU }}"
            {{- end }}
            {{- if .ResourcesSpec.Limits.CPUQuantity }}
            cpu: "{{ .ResourcesSpec.Limits.CPUQuantity }}"
            {{- end }}
            {{- if .ResourcesSpec.Limits.MemoryQuantity }}
            memory: "{{ .ResourcesSpec.Limits.MemoryQuantity }}"
            {{- end }}
{{ toYaml .Mounts | indent 8 }}
{{ toYaml .Volumes | indent 6 }}
`

type UIXResourceGenerator struct {
	c *BoardConfig
	Uix
	volumes        []v1.Volume
	mounts         []v1.VolumeMount
	InitContainers []InitContainers
}

func (ui UIXResourceGenerator) NodeSelector() string {
	if ui.ResourcesSpec().Accelerators.GPU > 0 && utils.GetDefaultGPUNodeSelector() != "" {
		return utils.GetDefaultGPUNodeSelector()
	}
	return utils.GetDefaultCPUNodeSelector()
}

func (ui UIXResourceGenerator) ResourcesSpec() ResourceRequest {
	return ResourceSpec(
		ui.Resources,
		ui.c.BoardMetadata.Limits,
		dealerclient.ResourceLimit{CPUMi: 50, MemoryMB: 128},
	)
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
	return ui.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     ui.Uix.Name,
		types.ComponentTypeLabel: "ui",
	})
}

func (ui UIXResourceGenerator) Args() string {
	return ui.Resource.RawArgs
}

func (c *BoardConfig) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		if uix.Disabled {
			continue
		}

		if err := c.CheckResourceLimit(uix.Resource, uix.Name); err != nil {
			return nil, err
		}

		volumes, mounts, err := c.KubeVolumesSpec(uix.VolumeMounts(c.VolumesData, c.DefaultMountPath))
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes '%s': %v", uix.Name, err)
		}
		initContainers, err := c.KubeInits(uix.VolumeMounts(c.VolumesData, c.DefaultMountPath), nil, nil)
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
	resources = append([]*kubernetes.KubeResource{c.generateKuberlabConfig()}, resources...)
	return resources, nil
}

type ServingResourceGenerator struct {
	UIXResourceGenerator
	TaskName  string
	Build     string
	BuildInfo map[string]interface{}
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
	if serving.BuildInfo != nil {
		for k, v := range serving.BuildInfo {
			if k == "checkpoint_path" || k == "model_path" {
				envs = append(envs,
					Env{
						Name:  k,
						Value: fmt.Sprintf("%v", v),
					},
				)
			}
		}
	}
	return envs
}
func (serving ServingResourceGenerator) Labels() map[string]string {
	return serving.c.ResourceLabels(map[string]string{
		types.ComponentLabel:     serving.Uix.Name,
		types.ComponentTypeLabel: "serving",
		types.ServingIDLabel:     serving.Name(),
	})
}

func (serving ServingResourceGenerator) Name() string {
	return fmt.Sprintf("%s-%s-%s", serving.Uix.Name, serving.TaskName, serving.Build)
}

func (serving ServingResourceGenerator) ComponentName() string {
	return utils.KubeDeploymentEncode(fmt.Sprintf("%s-%s", serving.c.Name, serving.Name()))
}

func (c *BoardConfig) GenerateServingResources(serving Serving) ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	volumes, mounts, err := c.KubeVolumesSpec(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath))
	if err != nil {
		return nil, fmt.Errorf("Failed get volumes '%s': %v", serving.Name, err)
	}
	initContainers, err := c.KubeInits(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed generate init spec '%s': %v", serving.Name, err)
	}

	if err := c.CheckResourceLimit(serving.Uix.Resource, serving.Name); err != nil {
		return nil, err
	}

	g := ServingResourceGenerator{
		TaskName:  serving.TaskName,
		Build:     serving.Build,
		BuildInfo: serving.BuildInfo,
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

func baseEnv(c *BoardConfig, r Resource) []Env {
	envs := make([]Env, 0, len(r.Env))
	path := make([]string, 0)
	for _, e := range r.Env {
		if e.Name == "PYTHONPATH" {
			path = strings.Split(e.Value, ":")
		} else {
			envs = append(envs, e)
		}
	}
	for _, m := range r.VolumeMounts(c.VolumesData, c.DefaultMountPath) {
		v := c.volumeByName(m.Name)
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
		if c.BoardMetadata.Limits != nil && c.BoardMetadata.Limits.GPU > 0 {
			count = uint(c.BoardMetadata.Limits.GPU)
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
	for _, v := range r.VolumeMounts(c.VolumesData, c.DefaultMountPath) {
		mountPath := v.MountPath
		if len(mountPath) == 0 {
			if v := c.volumeByName(v.Name); v != nil {
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

	envs = append(envs, Env{Name: "CLOUD_DEALER_URL", Value: c.DealerAPI})
	envs = append(envs, Env{Name: "PROJECT_NAME", Value: c.Name})
	envs = append(envs, Env{Name: "PROJECT_ID", Value: c.ProjectID})
	envs = append(envs, Env{Name: "WORKSPACE_NAME", Value: c.Workspace})
	envs = append(envs, Env{Name: "WORKSPACE_ID", Value: c.WorkspaceID})
	if c.Kind != KindServing {
		// Add for all resources except for serving from model.
		envs = append(envs, Env{
			Name:            "WORKSPACE_SECRET",
			ValueFromSecret: fmt.Sprintf("%v-ws-key-%v", c.Name, c.WorkspaceID),
			SecretKey:       "token",
		})
	}

	return envs
}
