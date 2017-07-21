package mlapp

import (
	"strings"

	"github.com/kuberlab/lib/pkg/kubernetes"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
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

const StatefulSetTpl = `
apiVersion: apps/v1beta1
kind: StatefulSet
metadata:
  name: {{ .JobID }}-{{ .JobName }}
  namespace: {{ .Name }}
  labels:
    kuberlab.io/job-id: {{ .MasterJob }}
    {{- range $key, $value := .Labels }}
	{{ $key }}: {{ $value }}
	{{- end }}
spec:
  replicas: {{ .Replicas }}
  serviceName: {{ .JobID }}-{{ .JobName }}
  template:
    metadata:
      labels:
        {{- range $key, $value := .Labels }}
	    {{ $key }}: {{ $value }}
	    {{- end }}
        service: {{ .JobID }}-{{ .JobName }}
        kuberlab.io/job-id: {{ .MasterJob }}
      {{- if .GpuRequests }}
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
          export PYTHONPATH=$PYTHONPATH:$KUBERLAB_PYTHONPATH;
          task_id=$(hostname | rev | cut -d ''-'' -f 1 | rev);
          echo "Start with task-id=$task_id";
          cd {{.ExecutionDir}};
          {{- if .GpuRequests }}
          export KUBE_GPU_AVAILABLE=$(nvidia-smi --query-gpu=index --format=csv,noheader | awk '{print}' ORS=',');
          export KUBE_GPU_COUNT={{ .GpuRequests }};
          {{- else }}
          export KUBE_GPU_COUNT=0;
          {{- end }}
          {{- if .WorkerHosts }}
          python {{ .Script }} --job_name {{ .JobName }} --task_index $task_id --num_gpus=${KUBE_GPU_COUNT} --ps_hosts "{{ .PsHosts }}" --worker_hosts "{{ .WorkerHosts }}" {{.ExtraArgs}};
          {{- else }}
          python {{ .Script }} --job_name {{ .JobName }} --task_index $task_id --num_gpus=${KUBE_GPU_COUNT} {{.ExtraArgs}};
          {{- end }}
          code=$?;
          echo "Script exit code: ${code}";
          while true; do  echo "waiting..."; curl -H "X-Source: ${POD_NAME}" -H "X-Result: ${code}" {{ .Callback }}; sleep 60; done;
          echo 'Wait deletion...';
          sleep 86400
        image: {{ .ContainerImage }}
        name: {{ .ContainerName }}
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: RUNDIR
            value: {{ .TrainDir }}
          {{- if .PythonPath }}
          - name: KUBERLAB_PYTHONPATH
            value: {{ .PythonPath }}
          {{- end }}
          {{- range .Env }}
          - name: {{ .Name }}
            value: '{{ .Value }}'
          {{- end }}
        # Auto-deleting metric from prometheus.
        ports:
        - containerPort: 2222
          name: cluster-port
          protocol: TCP
        resources:
          requests:
            {{- if .GpuRequests }}
            alpha.kubernetes.io/nvidia-gpu: {{ .GpuRequests }}
            {{- end }}
            {{- if .CpuRequests }}
            cpu: {{ .CpuRequests }}
            {{- end }}
            {{- if .MemoryRequests }}
            memory: {{ .MemoryRequests }}
            {{- end }}
          limits:
            {{- if .GpuRequests }}
            alpha.kubernetes.io/nvidia-gpu: {{ .GpuRequests }}
            {{- end }}
            {{- if .CpuLimits }}
            cpu: {{ .CpuLimits }}
            {{- end }}
            {{- if .MemoryLimits }}
            memory: {{ .MemoryLimits }}
            {{- end }}
      - name: monitoring
        image: kuberlab/gpu-monitoring:latest
        args:
          - /bin/sh
          - -c
          - >
            while true; do sleep 3; done
        env:
          - name: KUBERLAB_GPU
            value: "all"
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: JOB_NAME
            value: {{ .MasterJob }}
          - name: RUNDIR
            value: {{ .TrainDir }}
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          - name: PUSH_GATEWAY
            value: prometheus-push.{{ .MonitoringNamespace }}:9091
        lifecycle:
          preStop:
            exec:
              command:
               - /bin/bash
               - -c
               - curl -X DELETE http://prometheus-push.{{ .MonitoringNamespace }}:9091/metrics/job/{{ .MasterJob }}
        resources:
          requests:
            cpu: 200m
            memory: 100Mi
          limits:
            cpu: 500m
            memory: 1Gi
        volumeMounts:
        - name: docker
          mountPath: /var/run/docker.sock
      volumes:
      - name: docker
        hostPath:
          path: /var/run/docker.sock
`

// Must be called by spawner due to extracting the right IP for callback.
func (c *Config) GenerateTaskResources() ([]*kubernetes.KubeResource, error) {
	resources := []*kubernetes.KubeResource{}
	for _, task := range c.Tasks {
		for _, r := range task.Resources {
			labels := make(map[string]string, 0)
			joinMap(labels, c.Labels)
			joinMap(labels, task.Labels)
			joinMap(labels, r.Labels)

			vars := map[string]interface{}{
				"Component":    task.Name,
				"Name":         c.Name,
				"Labels":       labels,
				"Ports":        r.Ports,
				"ExecutionDir": r.WorkDir,
				"Command":      r.Command,
				"Args":         r.Args,
				"Replicas":     r.Replicas,
			}
			if r.RestartPolicy != "" {
				vars["RestartPolicy"] = r.RestartPolicy
			}
			if r.MaxRestartCount != 0 {
				vars["MaxRestartCount"] = r.MaxRestartCount
			}
			joinRawMap(vars, r.Resources.AsVars())
		}
	}
	return resources, nil
}

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
		joinRawMap(vars, uix.Resources.AsVars())

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
		svc := GenerateServiceDeploy(*res.Object.(*extv1beta1.Deployment), uix.Ports)
		kind := svc.GroupVersionKind()
		resources = append(
			resources,
			&kubernetes.KubeResource{Kind: &kind, Object: svc, Name: svc.Name},
		)
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
