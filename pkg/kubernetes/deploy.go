package kubernetes

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kuberlab/lib/pkg/apputil"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	batch_v1 "k8s.io/api/batch/v1"
	api_v1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	ApplyTimeout = 180 * time.Second
)

type KubeResource struct {
	Name    string
	Object  runtime.Object
	Kind    *schema.GroupVersionKind
	Deps    []*KubeResource
	Upgrade Upgrade
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
	if err := batch_v1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := extv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := api_v1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := rbacv1beta1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

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
		if _, err := kubeClient.CoreV1().Namespaces().Get(v.Name, meta_v1.GetOptions{}); err == nil {
			return nil
		} else {
			_, err := kubeClient.CoreV1().Namespaces().Create(v)
			return err
		}
	case *batch_v1.Job:
		if old, err := kubeClient.BatchV1().Jobs(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err = kubeClient.BatchV1().Jobs(v.Namespace).Create(v)
			return err
		} else {
			if resource.Upgrade != nil && resource.Upgrade(old, v) {
				err = kubeClient.BatchV1().Jobs(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{})
				if err != nil {
					return err
				}
				_, err = kubeClient.BatchV1().Jobs(v.Namespace).Create(v)
				return err
			}
			return nil
		}
	case *api_v1.ReplicationController:
		if _, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Update(v)
			return err
		}
	case *appsv1beta1.StatefulSet:
		if _, err := kubeClient.AppsV1beta1().StatefulSets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.AppsV1beta1().StatefulSets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.AppsV1beta1().StatefulSets(v.Namespace).Update(v)
			return err
		}
	case *api_v1.ConfigMap:
		if _, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Update(v)
			return err
		}
	case *policyv1beta1.PodDisruptionBudget:
		if _, err := kubeClient.PolicyV1beta1().PodDisruptionBudgets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.PolicyV1beta1().PodDisruptionBudgets(v.Namespace).Create(v)
			return err
		}
		return nil
	case *api_v1.Secret:
		if _, err := kubeClient.CoreV1().Secrets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().Secrets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.CoreV1().Secrets(v.Namespace).Update(v)
			return err
		}
	case *extv1beta1.DaemonSet:
		if _, err := kubeClient.ExtensionsV1beta1().DaemonSets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.ExtensionsV1beta1().DaemonSets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.ExtensionsV1beta1().DaemonSets(v.Namespace).Update(v)
			return err
		}
	case *extv1beta1.Deployment:
		return waitAndApply(kubeClient, v)
	case *api_v1.Service:
		if old, err := kubeClient.CoreV1().Services(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().Services(v.Namespace).Create(v)
			return err
		} else {
			old.Labels = v.Labels
			old.Spec.Selector = v.Spec.Selector
			_, err := kubeClient.CoreV1().Services(v.Namespace).Update(old)
			return err
		}
		return nil
	case *api_v1.ServiceAccount:
		if _, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Update(v)
			return err
		}
		return nil
	case *api_v1.PersistentVolumeClaim:
		if _, err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Create(v)
			return err
		}
		return nil
	case *api_v1.PersistentVolume:
		if _, err := kubeClient.CoreV1().PersistentVolumes().Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.CoreV1().PersistentVolumes().Create(v)
			return err
		}
		return nil
	case *rbacv1beta1.Role:
		if _, err := kubeClient.RbacV1beta1().Roles(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1beta1().Roles(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.RbacV1beta1().Roles(v.Namespace).Update(v)
			return err
		}
	case *rbacv1beta1.ClusterRole:
		if _, err := kubeClient.RbacV1beta1().ClusterRoles().Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1beta1().ClusterRoles().Create(v)
			return err
		} else {
			_, err := kubeClient.RbacV1beta1().ClusterRoles().Update(v)
			return err
		}
	case *rbacv1beta1.RoleBinding:
		if _, err := kubeClient.RbacV1beta1().RoleBindings(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1beta1().RoleBindings(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.RbacV1beta1().RoleBindings(v.Namespace).Update(v)
			return err
		}
	case *rbacv1beta1.ClusterRoleBinding:
		if _, err := kubeClient.RbacV1beta1().ClusterRoleBindings().Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.RbacV1beta1().ClusterRoleBindings().Create(v)
			return err
		} else {
			_, err := kubeClient.RbacV1beta1().ClusterRoleBindings().Update(v)
			return err
		}
	default:
		return errors.New("Undefined resource " + resource.Kind.GroupKind().Kind)
	}
}

func waitAndApply(client *kubernetes.Clientset, new *extv1beta1.Deployment) error {
	selector := make([]string, 0)
	for k, v := range new.Labels {
		selector = append(selector, fmt.Sprintf("%v==%v", k, v))
	}
	opts := meta_v1.ListOptions{
		LabelSelector: strings.Join(selector, ","),
	}

	check := func() (bool, error) {
		pods, err := client.CoreV1().Pods(new.Namespace).List(opts)
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
	if _, err := client.ExtensionsV1beta1().Deployments(new.Namespace).Get(new.Name, meta_v1.GetOptions{}); err != nil {
		_, err := client.ExtensionsV1beta1().Deployments(new.Namespace).Create(new)
		return err
	} else {
		_, err := client.ExtensionsV1beta1().Deployments(new.Namespace).Update(new)
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
		if err := kubeClient.CoreV1().Namespaces().Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *batch_v1.Job:
		if err := kubeClient.BatchV1().Jobs(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ReplicationController:
		if err := kubeClient.CoreV1().ReplicationControllers(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *appsv1beta1.StatefulSet:
		if err := kubeClient.AppsV1beta1().StatefulSets(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ConfigMap:
		if err := kubeClient.CoreV1().ConfigMaps(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *policyv1beta1.PodDisruptionBudget:
		if err := kubeClient.PolicyV1beta1().PodDisruptionBudgets(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.Secret:
		if err := kubeClient.CoreV1().Secrets(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *extv1beta1.DaemonSet:
		if err := kubeClient.ExtensionsV1beta1().DaemonSets(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *extv1beta1.Deployment:
		if err := kubeClient.ExtensionsV1beta1().Deployments(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.Service:
		if err := kubeClient.CoreV1().Services(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.ServiceAccount:
		if err := kubeClient.CoreV1().ServiceAccounts(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.PersistentVolumeClaim:
		if err := kubeClient.CoreV1().PersistentVolumeClaims(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *api_v1.PersistentVolume:
		if err := kubeClient.CoreV1().PersistentVolumes().Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1beta1.Role:
		if err := kubeClient.RbacV1beta1().Roles(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1beta1.ClusterRole:
		if err := kubeClient.RbacV1beta1().ClusterRoles().Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1beta1.RoleBinding:
		if err := kubeClient.RbacV1beta1().RoleBindings(v.Namespace).Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
			return err
		}
	case *rbacv1beta1.ClusterRoleBinding:
		if err := kubeClient.RbacV1beta1().ClusterRoleBindings().Delete(v.Name, &meta_v1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
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
