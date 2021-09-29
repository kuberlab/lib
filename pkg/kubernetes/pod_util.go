package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pborman/uuid"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var PodTpl string = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
  labels:
    component: pip-install
    scope: mlboard
spec:
  restartPolicy: Never
  containers:
  - name: {{ .Container }}
    resources:
      requests:
        cpu: "100m"
        memory: "64Mi"
    image: {{ .Image }}
`

func GetPodSpec(name string, namespace string, image string, kubeVolume []v1.Volume, containerVolume []v1.VolumeMount, cmd, args []string) (*v1.Pod, error) {
	container := name
	name = strings.ToLower(fmt.Sprintf("%v-%v", name, uuid.New()))
	vars := map[string]interface{}{
		"Image":     image,
		"Name":      name,
		"Container": container,
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
	pod.Labels = map[string]string{"scope": "mlboard"}
	pod.DeepCopyObject()
	pod.Spec.Volumes = kubeVolume
	pod.Spec.Containers[0].VolumeMounts = containerVolume
	pod.Spec.Containers[0].Command = cmd
	pod.Spec.Containers[0].Args = args

	return pod, nil
}

func WaitPod(pod *v1.Pod, client *kubernetes.Clientset, timeout time.Duration) (bool, error) {
	var timeoutTimer *time.Timer
	if timeout == 0 {
		timeoutTimer = time.NewTimer(2 * time.Minute)
	} else {
		timeoutTimer = time.NewTimer(timeout)
	}
	ticker := time.NewTicker(time.Millisecond * 100)

	p := pod
	var err error

	defer ticker.Stop()
	defer timeoutTimer.Stop()
	for {
		select {
		case <-ticker.C:
			p, err = client.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, meta_v1.GetOptions{})
			if err != nil {
				return false, err
			}
			if p.Status.Phase == v1.PodRunning {
				return false, nil
			}
			if p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed {
				return true, nil
			}
		case <-timeoutTimer.C:
			client.CoreV1().Pods(p.Namespace).Delete(context.TODO(), p.Name, meta_v1.DeleteOptions{})
			return false, fmt.Errorf("Pod %v is not running. Current state: %v", p.Name, p.Status.Phase)
		}
	}
}

func WaitPodComplete(pod *v1.Pod, client *kubernetes.Clientset, timeout time.Duration) error {
	var timeoutTimer *time.Timer
	if timeout == 0 {
		timeoutTimer = time.NewTimer(time.Minute * 2)
	} else {
		timeoutTimer = time.NewTimer(timeout)
	}
	ticker := time.NewTicker(time.Millisecond * 100)

	defer ticker.Stop()
	defer timeoutTimer.Stop()
	for {
		select {
		case <-ticker.C:
			p, err := client.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, meta_v1.GetOptions{})
			if err != nil {
				return err
			}
			if p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed {
				return nil
			}
		case <-timeoutTimer.C:
			client.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, meta_v1.DeleteOptions{})
			return fmt.Errorf("Pod %v is still running. Killing pod", pod.Name)
		}
	}
}
