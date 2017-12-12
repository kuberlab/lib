package kubernetes

import (
	"fmt"
	"strings"
	"time"

	"github.com/pborman/uuid"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

var PodTpl string = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  restartPolicy: Never
  containers:
  - name: git-revs
    resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
    image: {{ .Image }}
`

func GetPodSpec(name string, namespace string, image string, kubeVolume []v1.Volume, containerVolume []v1.VolumeMount, cmd, args []string) (*v1.Pod, error) {
	name = strings.ToLower(fmt.Sprintf("%v-%v", name, uuid.New()))
	vars := map[string]interface{}{
		"Image":     image,
		"Name":      name,
		"Namespace": namespace,
	}
	data, err := GetTemplate(PodTpl, vars)
	if err != nil {
		return nil, fmt.Errorf("Failed generate pod pip install: %v", err)
	}
	o, err := GetKubeResource(name, data, Noop)
	if err != nil {
		return nil, err
	}
	pod := o.Object.(*v1.Pod)
	pod.Spec.Volumes = kubeVolume
	pod.Spec.Containers[0].VolumeMounts = containerVolume
	pod.Spec.Containers[0].Command = cmd
	pod.Spec.Containers[0].Args = args

	return pod, nil
}

func WaitPod(pod *v1.Pod, client *kubernetes.Clientset) error {
	timeout := time.NewTimer(time.Minute)
	ticker := time.NewTicker(time.Millisecond * 100)

	defer ticker.Stop()
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			p, err := client.Pods(pod.Namespace).Get(pod.Name, meta_v1.GetOptions{})
			if err != nil {
				return err
			}
			if p.Status.Phase == v1.PodRunning {
				return nil
			}
		case <-timeout.C:
			client.Pods(pod.Namespace).Delete(pod.Name, &meta_v1.DeleteOptions{})
			return fmt.Errorf("Pod %v is not running.", pod.Name)
		}
	}
}

func WaitPodComplete(pod *v1.Pod, client *kubernetes.Clientset) error {
	timeout := time.NewTimer(time.Minute)
	ticker := time.NewTicker(time.Millisecond * 100)

	defer ticker.Stop()
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			p, err := client.Pods(pod.Namespace).Get(pod.Name, meta_v1.GetOptions{})
			if err != nil {
				return err
			}
			if p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed {
				return nil
			}
		case <-timeout.C:
			client.Pods(pod.Namespace).Delete(pod.Name, &meta_v1.DeleteOptions{})
			return fmt.Errorf("Pod %v is still running. Killing pod", pod.Name)
		}
	}
}
