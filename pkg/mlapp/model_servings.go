package mlapp

import (
	"fmt"

	"github.com/kuberlab/lib/pkg/dealerclient"
	"github.com/kuberlab/lib/pkg/kubernetes"
	"github.com/kuberlab/lib/pkg/types"
	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultModelPath      = "/model"
	ServingModelComponent = "serving-model"
)

type ModelServing struct {
	Uix
	Sources     []Volume    `json:"sources,omitempty"`
	DealerAPI   string      `json:"dealer_api,omitempty"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
	Workspace   string      `json:"workspace,omitempty"`
	ServingType ServingType `json:"type"`
}

type BoardModelServing struct {
	ModelServing
	VolumesData     []Volume `json:"volumes_data,omitempty"`
	Secrets         []Secret `json:"secrets,omitempty"`
	WorkspaceSecret string   `json:"workspace_secret,omitempty"`
}

func (serv *ModelServing) GPURequests() int64 {
	var gpus int64 = 0
	if serv.Uix.Resources != nil {
		gpus += int64(serv.Uix.Resources.Accelerators.GPU)
	}
	return gpus
}

func (serv *ModelServing) Type() string {
	return KindServing
}

type ServingModelResourceGenerator struct {
	UIXResourceGenerator
}

func (serving ServingModelResourceGenerator) ExportMetrics() bool {
	return true
}

func (serving ServingModelResourceGenerator) MetricsPort() int32 {
	return 9090
}

func (serving ServingModelResourceGenerator) LivenessPort() int32 {
	if len(serving.Ports) == 0 {
		return 0
	}
	return serving.Ports[0].TargetPort
}

func (serving ServingModelResourceGenerator) AllPorts() []Port {
	if !serving.ExportMetrics() {
		return serving.Ports
	}
	ports := serving.Ports
	for i, p := range ports {
		if p.Name == "" {
			ports[i].Name = fmt.Sprintf("port%v", i)
		}
	}
	metricPort := Port{
		Name:       "metric",
		Port:       serving.MetricsPort(),
		Protocol:   "TCP",
		TargetPort: serving.MetricsPort(),
	}
	ports = append(ports, metricPort)
	return ports
}

func (serving ServingModelResourceGenerator) Env() []Env {
	envs, _ := baseEnv(serving.c, serving.Resource)
	envs = append(envs,
		Env{
			Name:  "checkpoint_path",
			Value: "/model",
		},
		Env{
			Name:  "model_path",
			Value: "/model",
		},
	)
	return ResolveEnv(envs)
}
func (serving ServingModelResourceGenerator) Labels() map[string]string {
	return map[string]string{
		KUBERLAB_WS_LABEL:        serving.c.Workspace,
		KUBERLAB_WS_ID_LABEL:     serving.c.WorkspaceID,
		types.ComponentLabel:     serving.Uix.Name,
		types.ComponentTypeLabel: ServingModelComponent,
		types.ServingIDLabel:     serving.Name(),
	}
}

func (serving ServingModelResourceGenerator) Name() string {
	return fmt.Sprintf("%s", serving.Uix.Name)
}

func (serving ServingModelResourceGenerator) ComponentName() string {
	return utils.KubeDeploymentEncode(fmt.Sprintf("%s", serving.Name()))
}

func (c *BoardConfig) GenerateModelServing(serving BoardModelServing, dealerLimits bool) ([]*kubernetes.KubeResource, error) {
	var resources []*kubernetes.KubeResource

	// Do not use volume mounts, use mounts from sources
	serving.UseDefaultVolumeMapping = true

	volumes, mounts, err := c.KubeVolumesSpec(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly))
	if err != nil {
		return nil, err
	}

	initContainers, err := c.KubeInits(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly), nil, nil)
	if err != nil {
		return nil, err
	}

	if dealerLimits && serving.DealerAPI != "" && serving.WorkspaceSecret != "" {
		dealer, err := dealerclient.NewClient(
			serving.DealerAPI,
			&dealerclient.AuthOpts{
				WorkspaceSecret: serving.WorkspaceSecret,
				Workspace:       serving.Workspace,
				Insecure:        true,
			},
		)
		if err != nil {
			return nil, err
		}
		limits, err := dealer.GetWorkspaceLimit(serving.Workspace)
		if err != nil {
			return nil, err
		}
		c.BoardMetadata.Limits = limits
	}

	g := ServingModelResourceGenerator{
		UIXResourceGenerator: UIXResourceGenerator{
			c:              c,
			Uix:            serving.Uix,
			mounts:         mounts,
			volumes:        volumes,
			InitContainers: initContainers,
		},
	}

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

	deploy := res.Object.(*extv1beta1.Deployment)
	res.Deps = []*kubernetes.KubeResource{generateServingServiceFromDeployment(deploy)}

	for _, s := range c.Secrets {
		res.Deps = append(res.Deps, c.secret2kubeResource(s))
	}

	resources = append(resources, res)

	if serving.Autoscale != nil && serving.Autoscale.Enabled {
		if deploy.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().MilliValue() != 0 {
			autoscaler := c.generateHPA(deploy, serving.Autoscale)
			if autoscaler != nil {
				resources = append(resources, autoscaler)
			}
		}
	}

	return resources, nil
}

func (c *BoardConfig) secret2kubeResource(s Secret) *kubernetes.KubeResource {
	secret := &v1.Secret{
		StringData: s.Data,
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      c.GetSecretName(s),
			Namespace: c.GetNamespace(),
			Labels: map[string]string{
				types.ComponentTypeLabel: "serving-model",
				KUBERLAB_WS_ID_LABEL:     c.WorkspaceID,
				types.ServingIDLabel:     c.Name,
			},
		},
		Type: v1.SecretType(s.Type),
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
	}

	gv := secret.GroupVersionKind()

	return &kubernetes.KubeResource{
		Kind:   &gv,
		Name:   fmt.Sprintf("%v:secret", s.Name),
		Object: secret,
	}
}

func GenerateModelServing(serving BoardModelServing, dealerLimits bool, dockerSecret *v1.Secret) ([]*kubernetes.KubeResource, error) {
	cfg := &BoardConfig{
		Config: Config{
			Kind:        KindServing,
			Workspace:   serving.Workspace,
			WorkspaceID: serving.WorkspaceID,
			Meta: Meta{
				Name: serving.Name,
			},
			Spec: Spec{
				Volumes: serving.Sources,
			},
		},
		VolumesData: serving.VolumesData,
		Secrets:     serving.Secrets,
	}
	if dockerSecret != nil {
		cfg.Secrets = append(cfg.Secrets,
			Secret{Name: dockerSecret.Name, Type: string(dockerSecret.Type), Data: dockerSecret.StringData},
		)
	}
	return cfg.GenerateModelServing(serving, dealerLimits)
}

func generateServingServiceFromDeployment(serv *extv1beta1.Deployment) *kubernetes.KubeResource {
	labels := serv.Labels
	svc := &v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      serv.Name,
			Namespace: serv.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	for _, p := range serv.Spec.Template.Spec.Containers[0].Ports {
		svc.Spec.Ports = append(
			svc.Spec.Ports,
			v1.ServicePort{
				Name:       p.Name,
				TargetPort: intstr.FromInt(int(p.ContainerPort)),
				Protocol:   v1.Protocol(p.Protocol),
				Port:       p.ContainerPort,
			},
		)
	}
	groupKind := svc.GroupVersionKind()
	return &kubernetes.KubeResource{
		Name:   serv.Name + ":service",
		Object: svc,
		Kind:   &groupKind,
	}
}
