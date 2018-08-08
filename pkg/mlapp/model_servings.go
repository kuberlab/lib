package mlapp

import (
	"fmt"

	"github.com/ghodss/yaml"
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
	Sources         []Volume `json:"sources,omitempty"`
	VolumesData     []Volume `json:"volumes_data,omitempty"`
	Secrets         []Secret `json:"secrets,omitempty"`
	DealerAPI       string   `json:"dealer_api,omitempty"`
	ModelID         string   `json:"model_id,omitempty"`
	Model           string   `json:"model,omitempty"`
	ModelURL        string   `json:"model_url,omitempty"`
	WorkspaceID     string   `json:"workspace_id,omitempty"`
	Workspace       string   `json:"workspace,omitempty"`
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

func (c *BoardConfig) GenerateModelServing(serving ModelServing, dealerLimits bool) ([]*kubernetes.KubeResource, error) {
	var resources []*kubernetes.KubeResource

	// Do not use volume mounts, use mounts from sources
	serving.UseDefaultVolumeMapping = true

	volumesSpec, mountsSpec, err := c.KubeVolumesSpec(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly))
	if err != nil {
		return nil, err
	}
	volumes := []v1.Volume{{
		Name: "kuberlab-model",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}}
	mounts := []v1.VolumeMount{{
		Name:      "kuberlab-model",
		MountPath: defaultModelPath,
		ReadOnly:  false,
	}}

	inits, err := c.KubeInits(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath, c.DefaultReadOnly), nil, nil)
	if err != nil {
		return nil, err
	}

	initContainers := []InitContainers{
		{
			Name:  "init-model",
			Image: "kuberlab/board-init",
			Command: fmt.Sprintf(
				`["/bin/sh", "-c", "mkdir -p %v; curl -L -o m.tgz %v && tar -xzvf m.tgz -C %v"]`,
				defaultModelPath, serving.ModelURL, defaultModelPath,
			),
			Mounts: map[string]interface{}{"volumeMounts": mounts},
		},
	}
	initContainers = append(initContainers, inits...)
	if len(volumesSpec) > 0 {
		volumes = append(volumes, volumesSpec...)
	}
	if len(mountsSpec) > 0 {
		mounts = append(mounts, mountsSpec...)
	}

	if dealerLimits && serving.DealerAPI != "" && serving.WorkspaceSecret != "" {
		dealer, err := dealerclient.NewClient(
			serving.DealerAPI,
			&dealerclient.AuthOpts{WorkspaceSecret: serving.WorkspaceSecret, Workspace: serving.Workspace},
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
	res, err := kubernetes.GetTemplatedResource(DeploymentTpl, g.ComponentName()+":resource", g)
	if err != nil {
		return nil, fmt.Errorf("Failed parse template '%s': %v", g.ComponentName(), err)
	}

	data, _ := yaml.Marshal(res.Object)
	fmt.Println(string(data))

	res.Deps = []*kubernetes.KubeResource{generateServingServiceFromDeployment(res.Object.(*extv1beta1.Deployment))}

	for _, s := range c.Secrets {
		res.Deps = append(res.Deps, c.secret2kubeResource(s))
	}

	resources = append(resources, res)
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

func GenerateModelServing(serving ModelServing, dealerLimits bool, dockerSecret *v1.Secret) ([]*kubernetes.KubeResource, error) {
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
