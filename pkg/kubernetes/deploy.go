package kubernetes

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	api_v1 "k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	batch_v1 "k8s.io/client-go/pkg/apis/batch/v1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	policyv1beta1 "k8s.io/client-go/pkg/apis/policy/v1beta1"
	rbacv1beta1 "k8s.io/client-go/pkg/apis/rbac/v1beta1"
)

type KubeResource struct {
	Name    string
	Object  runtime.Object
	Kind    *schema.GroupVersionKind
	Deps    []*KubeResource
	Upgrade Upgrade
}

func GetTemplate(tpl string, vars map[string]interface{}) (string, error) {
	t := template.New("gotpl")
	t, err := t.Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("Failed parse template %v", err)
	}
	buffer := bytes.NewBuffer(make([]byte, 0))

	if err := t.ExecuteTemplate(buffer, "gotpl", vars); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func GetKubeResource(name string, data string, tranform func(runtime.Object) error) (*KubeResource, error) {
	if err := batch_v1.AddToScheme(api.Scheme); err != nil {
		return nil, err
	}
	if err := extv1beta1.AddToScheme(api.Scheme); err != nil {
		return nil, err
	}
	if err := api_v1.AddToScheme(api.Scheme); err != nil {
		return nil, err
	}
	if err := rbacv1beta1.AddToScheme(api.Scheme); err != nil {
		return nil, err
	}
	d := api.Codecs.UniversalDeserializer()
	o, i, err := d.Decode([]byte(data), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed decode object %v: ", err)
	}
	err = tranform(o)
	if err != nil {
		return nil, fmt.Errorf("Failed transform object %v: ", err)
	}
	return &KubeResource{
		Name:   name,
		Object: o,
		Kind:   i,
	}, nil
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
	switch v := resource.Object.(type) {
	case *api_v1.Namespace:
		if _, err := kubeClient.Namespaces().Get(v.Name, meta_v1.GetOptions{}); err == nil {
			return nil
		} else {
			_, err := kubeClient.Namespaces().Create(v)
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
		if _, err := kubeClient.ReplicationControllers(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.ReplicationControllers(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.ReplicationControllers(v.Namespace).Update(v)
			return err
		}
	case *appsv1beta1.StatefulSet:
		if _, err := kubeClient.StatefulSets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.StatefulSets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.StatefulSets(v.Namespace).Update(v)
			return err
		}
	case *api_v1.ConfigMap:
		if _, err := kubeClient.ConfigMaps(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.ConfigMaps(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.ConfigMaps(v.Namespace).Update(v)
			return err
		}
	case *policyv1beta1.PodDisruptionBudget:
		if _, err := kubeClient.PodDisruptionBudgets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.PodDisruptionBudgets(v.Namespace).Create(v)
			return err
		}
		return nil
	case *api_v1.Secret:
		if _, err := kubeClient.Secrets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.Secrets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.Secrets(v.Namespace).Update(v)
			return err
		}
	case *extv1beta1.DaemonSet:
		if _, err := kubeClient.DaemonSets(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.DaemonSets(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.DaemonSets(v.Namespace).Update(v)
			return err
		}
	case *extv1beta1.Deployment:
		if _, err := kubeClient.ExtensionsV1beta1().Deployments(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.ExtensionsV1beta1().Deployments(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.ExtensionsV1beta1().Deployments(v.Namespace).Update(v)
			return err
		}
	case *api_v1.Service:
		if _, err := kubeClient.Services(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.Services(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.Services(v.Namespace).Update(v)
			return err
		}
		return nil
	case *api_v1.ServiceAccount:
		if _, err := kubeClient.ServiceAccounts(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.ServiceAccounts(v.Namespace).Create(v)
			return err
		} else {
			_, err := kubeClient.ServiceAccounts(v.Namespace).Update(v)
			return err
		}
		return nil
	case *api_v1.PersistentVolumeClaim:
		if _, err := kubeClient.PersistentVolumeClaims(v.Namespace).Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.PersistentVolumeClaims(v.Namespace).Create(v)
			return err
		}
		return nil
	case *api_v1.PersistentVolume:
		if _, err := kubeClient.PersistentVolumes().Get(v.Name, meta_v1.GetOptions{}); err != nil {
			_, err := kubeClient.PersistentVolumes().Create(v)
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

func AlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

type Upgrade func(old runtime.Object, new runtime.Object) bool
