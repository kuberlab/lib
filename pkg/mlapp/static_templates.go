package mlapp

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kuberlab/lib/pkg/apputil"
	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
)

const DeploymentTpl = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: "{{ .ComponentName }}"
  namespace: "{{ .Namespace }}"
  labels:
    {{- range $key, $value := .DLabels }}
    {{ $key }}: "{{ $value }}"
    {{- end }}
spec:
  replicas: {{ .Replicas }}
  revisionHistoryLimit: 1
  selector:
    matchLabels:
      {{- range $key, $value := .DLabels }}
      {{ $key }}: "{{ $value }}"
      {{- end }}
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: "{{ $value }}"
        {{- end }}
      {{- if .ExportMetrics }}
      annotations:
        prometheus.io/scrape: 'true'
        prometheus.io/port: '{{ .MetricsPort }}'
      {{- end }}
    spec:
      {{- if .PrivilegedMode }}
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      {{- end }}
      {{- if gt (len .NodeSelectors ) 0 }}
      nodeSelector:
        {{- range $key, $value := .NodeSelectors }}
        {{ $key }}: "{{ $value }}"
        {{- end }}
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
      {{- if .DeployResourceLabel }}
      - key: kuberlab.io/private-resource
        value: {{ .DeployResourceLabel }}
        effect: NoSchedule
      {{- end }}
      {{- if gt (len .DockerSecretNames) 0 }}
      imagePullSecrets:
      {{- range $i, $value := .DockerSecretNames }}
      - name: {{ $value }}
      {{- end }}
      {{- end }}
      containers:
      - name: {{ .ComponentName }}
        {{- if .Command }}
        command: ["/bin/bash", "-c"]
        args:
        - >
          {{- if .Conda }}
          source activate {{ .Conda }};
          {{- end }}
          export PYTHONPATH=$PYTHONPATH:{{ .PythonPath }};
          {{- if .WorkDir }}
          cd {{ .WorkDir }};
          {{- end }}
          {{ .Command | indent 10 }} {{ .Args }};
          code=$?;
          exit $code
        {{- end }}
        image: "{{ .Image }}"
        imagePullPolicy: Always
        {{- if .PrivilegedMode }}
        securityContext:
          privileged: true
          capabilities:
            add: ["SYS_ADMIN"]
        {{- end }}
        env:
        {{- if not .Command }}
        - name: PYTHONPATH
          value: '{{ .PythonPath }}'
        {{- end }}
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
        {{- if .AllPorts }}
        ports:
        {{- range .AllPorts }}
        - name: {{ .Name }}
          {{- if .Protocol }}
          protocol: {{ .Protocol }}
          {{- end }}
          {{- if .TargetPort }}
          containerPort: {{ .TargetPort }}
          {{- end }}
        {{- end }}
        {{- end }}
        {{- if gt .LivenessPort 0 }}
        livenessProbe:
          tcpSocket:
            port: {{ .LivenessPort }}
          initialDelaySeconds: 2400
          periodSeconds: 60
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

func (ui UIXResourceGenerator) ExportMetrics() bool {
	return false
}

func (ui UIXResourceGenerator) MetricsPort() int32 {
	return 9090
}

func (ui UIXResourceGenerator) LivenessPort() int32 {
	if len(ui.Ports) == 0 {
		return 0
	}
	return ui.Ports[0].TargetPort
}

func (ui UIXResourceGenerator) AllPorts() []Port {
	if !ui.ExportMetrics() {
		return ui.Ports
	}
	ports := ui.Ports
	for i, p := range ports {
		if p.Name == "" {
			ports[i].Name = fmt.Sprintf("port%v", i)
		}
	}
	metricPort := Port{Name: "metric", Port: ui.MetricsPort(), Protocol: "TCP", TargetPort: ui.MetricsPort()}
	ports = append(ports, metricPort)
	return ports
}

func (ui UIXResourceGenerator) KubeVersion() *version.Info {
	return kubernetes.MlBoardKubeVersion
}

func (ui UIXResourceGenerator) KubeVersionMajor() int {
	major, _ := strconv.ParseInt(kubernetes.MlBoardKubeVersion.Major, 10, 32)
	if major == 0 {
		return 1
	}
	return int(major)
}

func (ui UIXResourceGenerator) KubeVersionMinor() int {
	minor, _ := strconv.ParseInt(kubernetes.MlBoardKubeVersion.Minor, 10, 32)
	if minor == 0 {
		return 8
	}
	return int(minor)
}

func (ui UIXResourceGenerator) NodeSelectors() map[string]string {
	nSlector := map[string]string{}
	if ui.NodesLabel != "" {
		nSlector[types.KuberlabMLNodeLabel] = strings.TrimPrefix(ui.NodesLabel, "knode:")
	} else {
		if ui.ResourcesSpec().Accelerators.GPU > 0 && utils.GetDefaultGPUNodeSelector() != "" {
			nSlector[types.KuberlabMLNodeLabel] = utils.GetDefaultGPUNodeSelector()
		} else if v := utils.GetDefaultCPUNodeSelector(); v != "" {
			nSlector[types.KuberlabMLNodeLabel] = v
		}
	}
	if ui.c.DeployResourceLabel != "" {
		nSlector[types.KuberlabPrivateNodeLabel] = ui.c.DeployResourceLabel
	}
	return nSlector
}

func (ui UIXResourceGenerator) DeployResourceLabel() string {
	return ui.c.DeployResourceLabel
}
func (ui UIXResourceGenerator) DockerSecretNames() []string {
	return ui.c.DockerSecretNames()
}

func (ui UIXResourceGenerator) PrivilegedMode() bool {
	return ui.NodesLabel == "knode:movidius"
}

func (ui UIXResourceGenerator) Conda() string {
	for _, e := range ui.Env() {
		if e.Name == "CONDA_ENV" {
			return e.Value
		}
	}
	return ""
}

func (ui UIXResourceGenerator) ResourcesSpec() ResourceRequest {
	cpu, _ := resource.ParseQuantity("50m")
	mem, _ := resource.ParseQuantity("128Mi")
	return ResourceSpec(
		ui.Resources,
		ui.c.BoardMetadata.Limits,
		dealerclient.ResourceLimit{CPU: &cpu, Memory: &mem},
	)
}

func (ui UIXResourceGenerator) Replicas() int {
	if ui.Disabled {
		return 0
	}
	if ui.Resource.Replicas > 0 {
		return ui.Resource.Replicas
	}
	return 1
}
func (ui UIXResourceGenerator) Env() []Env {
	env, _ := baseEnv(ui.c, ui.Resource)
	env = append(env,
		Env{
			Name:  "URL_PREFIX",
			Value: ui.c.ProxyURL(ui.Uix.Name),
		},
	)
	return ResolveEnv(env)
}

func (ui UIXResourceGenerator) PythonPath() string {
	_, pythonPath := baseEnv(ui.c, ui.Resource)
	return pythonPath
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

func (ui UIXResourceGenerator) SLabels() map[string]string {
	labels := map[string]string{
		types.ComponentLabel:     ui.Uix.Name,
		types.ComponentTypeLabel: "ui",
		"scope":                  "mlboard",
	}
	if ui.NodesLabel != "" {
		labels[types.KuberlabMLNodeLabel] = ui.NodesLabel
	}
	return labels
}

func (ui UIXResourceGenerator) DLabels() map[string]string {
	return ui.c.GenericResourceLabels(ui.SLabels())
}

func (ui UIXResourceGenerator) Labels() map[string]string {
	labels := ui.SLabels()
	computeType := "cpu"
	if ui.ResourcesSpec().Accelerators.GPU > 0 {
		computeType = "gpu"

	}
	labels[types.ComputeTypeLabel] = computeType
	return ui.c.ResourceLabels(labels)
}

func (ui UIXResourceGenerator) Args() string {
	return ui.Resource.RawArgs
}

func (c *BoardConfig) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		//if uix.Disabled {
		//	continue
		//}

		if err := c.CheckResourceLimit(uix.Resource, uix.Name); err != nil {
			return nil, err
		}

		volumes, mounts, err := c.KubeVolumesSpec(uix.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly))
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes '%s': %v", uix.Name, err)
		}
		initContainers, err := c.KubeInits(uix.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("Failed generate init spec '%s': %v", uix.Name, err)
		}
		g := UIXResourceGenerator{c: c, Uix: uix, mounts: mounts, volumes: volumes, InitContainers: initContainers}

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

func (serving ServingResourceGenerator) ExportMetrics() bool {
	return true
}

func (serving ServingResourceGenerator) MetricsPort() int32 {
	return 9090
}

func (serving ServingResourceGenerator) LivenessPort() int32 {
	if len(serving.Ports) == 0 {
		return 0
	}
	return serving.Ports[0].TargetPort
}

func (serving ServingResourceGenerator) AllPorts() []Port {
	if !serving.ExportMetrics() {
		return serving.Ports
	}
	ports := serving.Ports
	for i, p := range ports {
		if p.Name == "" {
			ports[i].Name = fmt.Sprintf("port%v", i)
		}
	}
	metricPort := Port{Name: "metric", Port: serving.MetricsPort(), Protocol: "TCP", TargetPort: serving.MetricsPort()}
	ports = append(ports, metricPort)
	return ports
}

func (serving ServingResourceGenerator) Env() []Env {
	envs, _ := baseEnv(serving.c, serving.Resource)
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
	return ResolveEnv(envs)
}

func (serving ServingResourceGenerator) SLabels() map[string]string {
	labels := map[string]string{
		types.ComponentLabel:     serving.Uix.Name,
		types.ComponentTypeLabel: "serving",
		types.ServingIDLabel:     serving.Name(),
		"scope":                  "mlboard",
	}
	return labels
}

func (serving ServingResourceGenerator) DLabels() map[string]string {
	return serving.UIXResourceGenerator.c.GenericResourceLabels(serving.SLabels())
}

func (serving ServingResourceGenerator) Labels() map[string]string {
	labels := serving.SLabels()
	computeType := "cpu"
	if serving.UIXResourceGenerator.ResourcesSpec().Accelerators.GPU > 0 {
		computeType = "gpu"

	}
	labels[types.ComputeTypeLabel] = computeType
	return serving.UIXResourceGenerator.c.ResourceLabels(labels)
}

func (serving ServingResourceGenerator) Name() string {
	return fmt.Sprintf("%s-%s-%s", serving.Uix.Name, serving.TaskName, serving.Build)
}

func (serving ServingResourceGenerator) ComponentName() string {
	return utils.KubeDeploymentEncode(fmt.Sprintf("%s-%s", serving.c.Name, serving.Name()))
}

func (c *BoardConfig) GenerateServingResources(serving Serving) ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	volumes, mounts, err := c.KubeVolumesSpec(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly))
	if err != nil {
		return nil, fmt.Errorf("Failed get volumes '%s': %v", serving.Name, err)
	}
	initContainers, err := c.KubeInits(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly), nil, nil)
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
				Name:       utils.KubeNamespaceEncode(p.Name),
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
				Name:       utils.KubeNamespaceEncode(p.Name),
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

func baseEnv(c *BoardConfig, r Resource) ([]Env, string) {
	envs := make([]Env, 0, len(r.Env))
	pythonPath := make([]string, 0)
	for _, e := range r.Env {
		if e.Name == "PYTHONPATH" {
			pythonPath = strings.Split(e.Value, ":")
		} else {
			envs = append(envs, e)
		}
	}
	for _, m := range r.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly) {
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
		pythonPath = append(pythonPath, mount)
	}
	//envs = append(envs, Env{
	//	Name:  "PYTHONPATH",
	//	Value: strings.Join(pythonPath, ":"),
	//})
	if r.Resources != nil && r.Resources.Accelerators.GPU > 0 {
		count := r.Resources.Accelerators.GPU
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
	for _, v := range r.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly) {
		mountPath := v.MountPath
		if len(mountPath) == 0 {
			if v := c.volumeByName(v.Name); v != nil {
				mountPath = v.MountPath
			}
		}
		if len(mountPath) > 0 {
			envs = append(envs, Env{
				Name:  strings.ToUpper(utils.EnvConvert(v.Name) + "_DIR"),
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
			ValueFromSecret: utils.KubePodNameEncode(fmt.Sprintf("%v-ws-key-%v", c.Name, c.WorkspaceID)),
			SecretKey:       "token",
		})
	}
	pythonPath = append(pythonPath, kibernetikaPythonLibs)

	return envs, strings.Join(pythonPath, ":")
}

func ResolveEnv(envs []Env) []Env {
	vars := map[string]string{}
	for _, e := range envs {
		if e.SecretKey == "" {
			vars[e.Name] = e.Value
		}
	}
	for i, e := range envs {
		if e.SecretKey != "" {
			continue
		}
		t := template.New("gotpl")
		t = t.Funcs(apputil.FuncMap())
		if t, err := t.Parse(e.Value); err == nil {
			buffer := bytes.NewBuffer(make([]byte, 0))
			if err := t.ExecuteTemplate(buffer, "gotpl", vars); err == nil {
				envs[i].Value = buffer.String()
			}
		}
	}
	return envs
}
