package mlapp

import (
	"github.com/kuberlab/lib/pkg/kubernetes"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *BoardConfig) generateHPA(deployment *v1beta1.Deployment, autoscaleCfg *Autoscale) *kubernetes.KubeResource {
	min := int32(1)
	max := int32(5)

	var target int32 = 0
	if autoscaleCfg.MinReplicas > 0 {
		min = autoscaleCfg.MinReplicas
	}
	if autoscaleCfg.MaxReplicas > 1 {
		max = autoscaleCfg.MaxReplicas
	}
	if autoscaleCfg.TargetAverageUtilization > 0 {
		target = autoscaleCfg.TargetAverageUtilization
	} else {
		limit := deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()
		request := deployment.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu()
		if limit.MilliValue() != 0 {
			// limit / request * 100 * 0.5
			target = int32(float64(limit.MilliValue()) / float64(request.MilliValue()) * 100 * 0.5)
		} else {
			target = 50
		}
	}

	hpa := &v2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.Name,
			Labels:    deployment.Labels,
			Namespace: deployment.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "autoscaling/v2beta1",
			Kind:       "HorizontalPodAutoscaler",
		},
		Spec: v2beta1.HorizontalPodAutoscalerSpec{
			MinReplicas: &min,
			MaxReplicas: max,
			ScaleTargetRef: v2beta1.CrossVersionObjectReference{
				APIVersion: deployment.APIVersion,
				Kind:       deployment.Kind,
				Name:       deployment.Name,
			},
			Metrics: []v2beta1.MetricSpec{
				{
					Type: v2beta1.ResourceMetricSourceType,
					Resource: &v2beta1.ResourceMetricSource{
						Name:                     v1.ResourceCPU,
						TargetAverageUtilization: &target,
					},
				},
			},
		},
	}

	gv := hpa.GroupVersionKind()
	return &kubernetes.KubeResource{
		Object: hpa,
		Name:   hpa.Name + ":hpa",
		Kind:   &gv,
	}
}
