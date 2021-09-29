package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/kuberlab/lib/pkg/apputil"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/autoscaling/v2beta1"
	batch_v1 "k8s.io/api/batch/v1"
	api_v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	ApplyTimeout = 180 * time.Second
)

var (
	MlBoardKubeVersion *version.Info
)

type KubeResource struct {
	Name    string
	Object  runtime.Object
	Kind    *schema.GroupVersionKind
	Deps    []*KubeResource
	Upgrade Upgrade
}

func init() {
	if err := batch_v1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	if err := appsv1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	if err := api_v1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	if err := rbacv1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	if err := v2beta1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
}

func GetTemplate(tpl string, vars interface{}) (string, error) {
	t := template.New("gotpl")
	t = t.Funcs(apputil.FuncMap())
	t, err := t.Parse(tpl)
	if err != nil {
		fmt.Println("=======================")
		fmt.Println(tpl)
		fmt.Println("=======================")
		return "", fmt.Errorf("Failed parse template %v", err)
	}
	buffer := bytes.NewBuffer(make([]byte, 0))

	if err := t.ExecuteTemplate(buffer, "gotpl", vars); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func GetKubeResource(name string, data string, tranform func(runtime.Object) error) (*KubeResource, error) {
	d := scheme.Codecs.UniversalDeserializer()
	o, i, err := d.Decode([]byte(data), nil, nil)
	if err != nil {
		logrus.Infoln("**************")
		fmt.Println(data)
		logrus.Infoln("**************")
		return nil, fmt.Errorf("Failed decode object %v: ", err)
	}
	if tranform != nil {
		err = tranform(o)
		if err != nil {
			return nil, fmt.Errorf("Failed transform object %v: ", err)
		}
	}
	return &KubeResource{
		Name:   name,
		Object: o,
		Kind:   i,
	}, nil
}

func GetTemplatedResource(tpl string, name string, vars interface{}) (*KubeResource, error) {
	data, err := GetTemplate(tpl, vars)
	if err != nil {
		return nil, err
	}
	return GetKubeResource(name, data, Noop)
}

func Apply(client *kubernetes.Clientset, resources []*KubeResource) error {
	for _, r := range resources {
		if len(r.Deps) > 0 {
			if err := Apply(client, r.Deps); err != nil {
				return err
			}
		}
		if err := applyResource(client, r); err != nil {
			return err
		}
	}
	return nil
}

func Noop(_ runtime.Object) error {
	return nil
}

func applyResource(kubeClient *kubernetes.Clientset, resource *KubeResource) error {
	logrus.Infof("Apply %v, %v", resource.Name, resource.Kind)
	switch v := resource.Object.(type) {
	case *api_v1.Namespace:
		if _, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err == nil {
			return nil
		} else {
			_, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		}
	case *batch_v1.Job:
		if old, err := kubeClient.BatchV1().Jobs(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err = kubeClient.BatchV1().Jobs(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			if resource.Upgrade != nil && resource.Upgrade(old, v) {
				err = kubeClient.BatchV1().Jobs(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{})
				if err != nil {
					return err
				}
				_, err = kubeClient.BatchV1().Jobs(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
				return err
			}
			return nil
		}
	case *api_v1.ReplicationController:
		if _, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *appsv1.StatefulSet:
		if _, err := kubeClient.AppsV1().StatefulSets(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.AppsV1().StatefulSets(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.AppsV1().StatefulSets(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *api_v1.ConfigMap:
		if _, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *policyv1.PodDisruptionBudget:
		if _, err := kubeClient.PolicyV1().PodDisruptionBudgets(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.PolicyV1().PodDisruptionBudgets(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		}
		return nil
	case *api_v1.Secret:
		if _, err := kubeClient.CoreV1().Secrets(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().Secrets(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.CoreV1().Secrets(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *appsv1.DaemonSet:
		if _, err := kubeClient.AppsV1().DaemonSets(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.AppsV1().DaemonSets(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.AppsV1().DaemonSets(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *appsv1.Deployment:
		return waitAndApply(kubeClient, v)
	case *v2beta1.HorizontalPodAutoscaler:
		if old, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			old.Labels = v.Labels
			_, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *api_v1.Service:
		if old, err := kubeClient.CoreV1().Services(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().Services(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			old.Labels = v.Labels
			old.Spec.Selector = v.Spec.Selector
			old.Spec.Ports = v.Spec.Ports
			_, err := kubeClient.CoreV1().Services(v.Namespace).Update(context.TODO(), old, meta_v1.UpdateOptions{})
			return err
		}
	case *api_v1.ServiceAccount:
		if _, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
		return nil
	case *api_v1.PersistentVolumeClaim:
		if _, err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		}
		return nil
	case *api_v1.PersistentVolume:
		if _, err := kubeClient.CoreV1().PersistentVolumes().Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().PersistentVolumes().Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		}
		return nil
	case *rbacv1.Role:
		if _, err := kubeClient.RbacV1().Roles(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1().Roles(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.RbacV1().Roles(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *rbacv1.ClusterRole:
		if _, err := kubeClient.RbacV1().ClusterRoles().Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1().ClusterRoles().Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.RbacV1().ClusterRoles().Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *rbacv1.RoleBinding:
		if _, err := kubeClient.RbacV1().RoleBindings(v.Namespace).Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1().RoleBindings(v.Namespace).Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.RbacV1().RoleBindings(v.Namespace).Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	case *rbacv1.ClusterRoleBinding:
		if _, err := kubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), v, meta_v1.CreateOptions{})
			return err
		} else {
			_, err := kubeClient.RbacV1().ClusterRoleBindings().Update(context.TODO(), v, meta_v1.UpdateOptions{})
			return err
		}
	default:
		return errors.New("Undefined resource " + resource.Kind.GroupKind().Kind)
	}
}

func waitAndApply(client *kubernetes.Clientset, new *appsv1.Deployment) error {
	selector := make([]string, 0)
	for k, v := range new.Labels {
		selector = append(selector, fmt.Sprintf("%v==%v", k, v))
	}
	opts := meta_v1.ListOptions{
		LabelSelector: strings.Join(selector, ","),
	}

	check := func() (bool, error) {
		pods, err := client.CoreV1().Pods(new.Namespace).List(context.TODO(), opts)
		if err != nil {
			return false, err
		}
		if podListAny(pods, isTerminating) {
			// Retry
			return false, nil
		} else {
			return true, nil
		}
	}

	res, err := check()
	if err != nil {
		return err
	}

	if !res {
		ticker := time.NewTicker(time.Second * 2)
		timeout := time.NewTimer(ApplyTimeout)
		fall := false
		for {
			select {
			case <-ticker.C:
				res, err = check()
				if err != nil {
					return err
				}
				if res {
					fall = true
				}
			case <-timeout.C:
				fall = true
			}
			if fall {
				break
			}
		}
	}
	if _, err := client.AppsV1().Deployments(new.Namespace).Get(context.TODO(), new.Name, meta_v1.GetOptions{}); err != nil {
		_, err := client.AppsV1().Deployments(new.Namespace).Create(context.TODO(), new, meta_v1.CreateOptions{})
		return err
	} else {
		_, err := client.AppsV1().Deployments(new.Namespace).Update(context.TODO(), new, meta_v1.UpdateOptions{})
		return err
	}

	return nil
}

func podListAny(list *api_v1.PodList, predicate func(pod api_v1.Pod) bool) bool {
	if list == nil {
		return false
	}
	for _, p := range list.Items {
		if predicate(p) {
			return true
		}
	}
	return false
}

func DeleteResource(kubeClient *kubernetes.Clientset, resource *KubeResource) error {
	logrus.Infof("Delete %v, %v", resource.Name, resource.Kind)
	var propagation = meta_v1.DeletePropagationForeground
	switch v := resource.Object.(type) {
	case *api_v1.Namespace:
		if err := kubeClient.CoreV1().Namespaces().Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *batch_v1.Job:
		if err := kubeClient.BatchV1().Jobs(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ReplicationController:
		if err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *appsv1.StatefulSet:
		if err := kubeClient.AppsV1().StatefulSets(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ConfigMap:
		if err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *policyv1.PodDisruptionBudget:
		if err := kubeClient.PolicyV1beta1().PodDisruptionBudgets(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.Secret:
		if err := kubeClient.CoreV1().Secrets(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *appsv1.DaemonSet:
		if err := kubeClient.ExtensionsV1beta1().DaemonSets(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *appsv1.Deployment:
		if err := kubeClient.ExtensionsV1beta1().Deployments(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.Service:
		if err := kubeClient.CoreV1().Services(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ServiceAccount:
		if err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.PersistentVolumeClaim:
		if err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.PersistentVolume:
		if err := kubeClient.CoreV1().PersistentVolumes().Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1.Role:
		if err := kubeClient.RbacV1beta1().Roles(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1.ClusterRole:
		if err := kubeClient.RbacV1beta1().ClusterRoles().Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1.RoleBinding:
		if err := kubeClient.RbacV1beta1().RoleBindings(v.Namespace).Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1.ClusterRoleBinding:
		if err := kubeClient.RbacV1beta1().ClusterRoleBindings().Delete(context.TODO(), v.Name, meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	default:
		return errors.New("Undefined resource " + resource.Kind.GroupKind().Kind)
	}

	for _, dep := range resource.Deps {
		if err := DeleteResource(kubeClient, dep); err != nil {
			return err
		}
	}

	return nil
}

func AlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

type Upgrade func(old runtime.Object, new runtime.Object) bool
