package mlapp

import (
	"strings"

	"github.com/kuberlab/lib/pkg/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const DeploymentTpl = `
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: "{{ .Component }}-{{ .Name }}"
  namespace: {{ .Name }}
  labels:
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
    {{- end }}
    workspace: {{ .Name }}
    component: {{ .Component }}
spec:
  replicas: 1
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
        {{ $key }}: {{ $value }}
        {{- end }}
        workspace: {{ .Name }}
        component: {{ .Component }}
    spec:
      containers:
      - name: {{ .Name }}-{{ .Component }}
        {{- if .Command }}
        command: ["{{ .Command }}"]
        {{- end }}
        {{- if .Args }}
          {{- if gt (len .Args) 0 }}
        args:
          {{- range .Args }}
            - {{ . }}
          {{- end }}
          {{- end }}
        {{- end }}
        image: "{{ .Image }}"
        {{- if gt (len .Env) 0 }}
        env:
          {{- range .Env }}
            - name: {{ .Name }}
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
            {{- if .GpuRequests }}
            alpha.kubernetes.io/nvidia-gpu: {{ .GpuRequests }}
            {{- end }}
            {{- if .CpuRequests }}
            cpu: "{{ .CpuRequests }}"
            {{- end }}
            {{- if .MemoryRequests }}
            memory: "{{ .MemoryRequests }}"
            {{- end }}
          limits:
            {{- if .GpuRequests }}
            alpha.kubernetes.io/nvidia-gpu: {{ .GpuRequests }}
            {{- end }}
            {{- if .CpuLimits }}
            cpu: "{{ .CpuLimits }}"
            {{- end }}
            {{- if .MemoryLimits }}
            memory: "{{ .MemoryLimits }}"
            {{- end }}
`

func (c *Config) GenerateUIXResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, uix := range c.Uix {
		labels := make(map[string]string, 0)
		joinMap(labels, c.Labels)
		joinMap(labels, uix.Labels)

		vars := map[string]interface{}{
			"Component": uix.Name,
			"Name":      c.Name,
			"Image":     uix.Image,
			"Labels":    labels,
			"Ports":     uix.Ports,
			"Env":       uix.Env,
		}

		reqs := uix.Resources
		if reqs.Accelerators.GPU != 0 {
			vars["GpuRequests"] = reqs.Accelerators.GPU
		}
		if reqs.Requests.Memory != "" {
			vars["MemoryRequests"] = reqs.Requests.Memory
		}
		if reqs.Limits.Memory != "" {
			vars["MemoryLimits"] = reqs.Requests.Memory
		}
		if reqs.Requests.CPU != "" {
			vars["CpuRequests"] = reqs.Requests.CPU
		}
		if reqs.Limits.CPU != "" {
			vars["CpuLimits"] = reqs.Requests.CPU
		}

		if uix.Args != "" {
			vars["Args"] = strings.Split(uix.Args, " ")
		}
		if uix.Command != "" {
			vars["Command"] = uix.Command
		}

		insertVolumes := func(o runtime.Object) error {
			d := o.(*extv1beta1.Deployment)
			v, vmounts, err := c.KubeVolumesSpec(uix.Volumes)
			if err != nil {
				return err
			}
			d.Spec.Template.Spec.Volumes = v
			d.Spec.Template.Spec.Containers[0].VolumeMounts = vmounts
			return nil
		}
		data, err := kubernetes.GetTemplate(DeploymentTpl, vars)
		if err != nil {
			return nil, err
		}

		res, err := kubernetes.GetKubeResource(uix.Name, data, insertVolumes)
		if err != nil {
			return nil, err
		}
		resources = append(resources, res)
		svc := GenerateService(*res.Object.(*extv1beta1.Deployment), uix.Ports)
		kind := svc.GroupVersionKind()
		resources = append(
			resources,
			&kubernetes.KubeResource{Kind: &kind, Object: svc, Name: svc.Name},
		)
	}
	return resources, nil
}

func GenerateService(deploy extv1beta1.Deployment, portSpec []Port) *v1.Service {
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
