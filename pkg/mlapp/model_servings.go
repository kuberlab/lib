package mlapp

import (
	"errors"
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
	Source          *GitRepoVolumeSource `json:"source,omitempty"`
	DealerAPI       string               `json:"dealer_api"`
	ModelID         string               `json:"model_id,omitempty"`
	Model           string               `json:"model,omitempty"`
	ModelURL        string               `json:"model_url,omitempty"`
	WorkspaceID     string               `json:"workspace_id,omitempty"`
	Workspace       string               `json:"workspace,omitempty"`
	WorkspaceSecret string               `json:"workspace_secret,omitempty"`
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

func (serv ModelServing) Volume() *Volume {
	if len(serv.Volumes) != 1 {
		return nil
	}
	return &Volume{
		Name:      serv.Volumes[0].Name,
		MountPath: serv.Volumes[0].MountPath,
		SubPath:   serv.Volumes[0].SubPath,
		VolumeSource: VolumeSource{
			GitRepo: serv.Source,
		},
	}
}

func (serv ModelServing) KubeVolumes() ([]v1.Volume, []v1.VolumeMount, error) {
	var volumes []v1.Volume
	var mounts []v1.VolumeMount

	if len(serv.Volumes) != 1 {
		return nil, nil, errors.New("Required exact 1 volume.")
	}

	volumes = append(
		volumes,
		v1.Volume{
			Name: serv.Volumes[0].Name,
			VolumeSource: v1.VolumeSource{
				GitRepo: &serv.Source.GitRepoVolumeSource,
			},
		},
	)
	mounts = append(
		mounts,
		v1.VolumeMount{
			Name:      serv.Volumes[0].Name,
			MountPath: serv.Volumes[0].MountPath,
			ReadOnly:  serv.Volumes[0].ReadOnly,
			SubPath:   serv.Volumes[0].SubPath,
		},
	)
	return volumes, mounts, nil
}

type ServingModelResourceGenerator struct {
	UIXResourceGenerator
}

func (serving ServingModelResourceGenerator) Env() []Env {
	envs := baseEnv(serving.c, serving.Resource)
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

	volumesSpec, mountsSpec, err := c.KubeVolumesSpec(serving.VolumeMounts(c.VolumesData, c.DefaultMountPath))
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
	resources = append(resources, res)
	return resources, nil
}

func GenerateModelServing(serving ModelServing, dealerLimits bool) ([]*kubernetes.KubeResource, error) {
	vol := serving.Volume()
	var volData = make([]Volume, 0)
	if vol != nil {
		volData = append(volData, *vol)
	}
	cfg := &BoardConfig{
		Config: Config{
			Kind:        KindServing,
			Workspace:   serving.Workspace,
			WorkspaceID: serving.WorkspaceID,
			Meta: Meta{
				Name: serving.Name,
			},
		},
		VolumesData: volData,
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
