package mlapp

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Sirupsen/logrus"
	kuberlab "github.com/kuberlab/lib/pkg/kubernetes"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (c *Config) setGitRefs(volumes []v1.Volume, task Task) {
	setRevision := func(vName string, rev string) {
		fromConfig := c.VolumeByName(vName)
		for i, v := range volumes {
			if v.Name == fromConfig.CommonID() && v.GitRepo != nil {
				if v.GitRepo.Revision == "" {
					volumes[i].GitRepo.Revision = rev
				}
			}
		}
	}

	for _, gitRev := range task.GitRevisions {
		if gitRev.Revision != "" {
			setRevision(gitRev.VolumeName, gitRev.Revision)
		}
	}
}

type RepoInfo struct {
	Dir      string
	URL      string
	Revision string
}

func (c *Config) DetermineGitSourceRevisions(client *kubernetes.Clientset, task Task) (map[string]string, error) {
	// First, collect all volumes to mount
	// Also, determine what exactly need to get

	// volumeName -> repoDir, repoUrl
	logrus.Info("Determine git source revisions...")
	gitRepos := make(map[string]*RepoInfo)
	res := make(map[string]string)

	volumesMap := make(map[string]v1.Volume)
	volumeMountsMap := make(map[string]v1.VolumeMount)

	volumeMountByName := func(mounts []VolumeMount, name string) *VolumeMount {
		for _, m := range mounts {
			if m.Name == name {
				vm := m
				return &vm
			}
		}
		return nil
	}

	for _, r := range task.Resources {
		rawMounts := r.VolumeMounts(c.Volumes)
		vs, mounts, err := c.KubeVolumesSpec(rawMounts)
		if err != nil {
			return nil, fmt.Errorf("Failed get volumes for '%s-%s': %v", task.Name, r.Name, err)
		}
		for _, vm := range mounts {
			volumeMountsMap[vm.Name] = vm
		}
		repoName := ""
		for _, v := range vs {
			volumesMap[v.Name] = v
			if v.GitRepo != nil {
				repoName = getGitRepoName(v.GitRepo.Repository)
				gitRepos[v.Name] = &RepoInfo{
					Dir: fmt.Sprintf("%v/%v", volumeMountsMap[v.Name].MountPath, repoName),
					URL: v.GitRepo.Repository,
				}

				resVolumeMount := volumeMountByName(rawMounts, c.VolumeByID(v.Name).Name)
				if resVolumeMount != nil && resVolumeMount.GitRevision != nil {
					gitRepos[v.Name].Revision = *resVolumeMount.GitRevision
					res[resVolumeMount.Name] = *resVolumeMount.GitRevision
				} else {
					gitRepos[v.Name].Revision = "master"
				}
			}
			if v.Secret == nil {
				// Unneeded.
				delete(volumesMap, v.Name)
				delete(volumeMountsMap, v.Name)
			}
		}
	}

	if len(gitRepos) == 0 || len(res) == len(gitRepos) {
		return res, nil
	}

	// Generate script for determining revisions.
	cmd := []string{"mkdir -p ~/.ssh"}
	for k, v := range gitRepos {
		// Skip repos which has explicit revision passed.
		if _, ok := res[c.VolumeByID(k).Name]; ok {
			continue
		}

		if strings.Contains(v.URL, "@") {
			// SSH.
			u := v.URL
			if !strings.HasPrefix(u, "ssh://") {
				u = "ssh://" + u
			}
			parsed, err := url.Parse(u)
			if err != nil {
				return nil, err
			}
			cmd = append(cmd, fmt.Sprintf("ssh-keyscan %v >> ~/.ssh/known_hosts > /dev/null 2> /dev/null", parsed.Host))
		}
		//cmd = append(cmd, fmt.Sprintf("cd %v", v.Dir))
		cmd = append(cmd, fmt.Sprintf("git ls-remote %v %v 2> /dev/null | head -1 | awk '{print $1}' | xargs printf '%v %%s\\n'", v.URL, v.Revision, k))
	}
	logrus.Infof("Generated cmd: %v", strings.Join(cmd, "; "))

	// Generate Pod, run it and read logs.
	volumes := make([]v1.Volume, 0)
	for _, v := range volumesMap {
		volumes = append(volumes, v)
	}
	volumeMounts := make([]v1.VolumeMount, 0)
	for _, v := range volumeMountsMap {
		volumeMounts = append(volumeMounts, v)
	}

	pod, err := kuberlab.GetPodSpec(
		"git-revs",
		c.GetNamespace(),
		"kuberlab/board-init:latest",
		volumes,
		volumeMounts,
		append([]string{"/bin/bash", "-c"}, strings.Join(cmd, "; ")),
		nil,
	)
	if err != nil {
		return nil, err
	}
	pod.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent

	pod, err = client.CoreV1().Pods(c.GetNamespace()).Create(pod)
	if err != nil {
		return nil, err
	}
	defer client.CoreV1().Pods(c.GetNamespace()).Delete(pod.Name, &meta_v1.DeleteOptions{})

	//if err = kuberlab.WaitPod(pod, client); err != nil {
	//	return nil, err
	//}
	if err = kuberlab.WaitPodComplete(pod, client); err != nil {
		return nil, err
	}

	logsRaw, err := client.CoreV1().Pods(pod.Namespace).GetLogs(
		pod.Name,
		&v1.PodLogOptions{
			Follow: true,
		},
	).DoRaw()
	if err != nil {
		return nil, err
	}

	logs := strings.Split(string(logsRaw), "\n")

	for _, l := range logs {
		if l == "" {
			continue
		}
		splitted := strings.Split(l, " ")
		if len(splitted) != 2 {
			continue
		}
		id := splitted[0]
		rev := splitted[1]

		res[c.VolumeByID(id).Name] = rev
	}
	return res, nil
}

func (c *Config) InjectGitRevisions(client *kubernetes.Clientset, task *Task) error {
	refs, err := c.DetermineGitSourceRevisions(client, *task)
	if err != nil {
		return err
	}
	logrus.Infof("Revisions: %v", refs)

	revisionExists := func(volumeName string) bool {
		for _, taskRev := range task.GitRevisions {
			if taskRev.VolumeName == volumeName {
				return true
			}
		}
		return false
	}

	for name, ref := range refs {
		if !revisionExists(name) {
			task.GitRevisions = append(task.GitRevisions, TaskGitRevision{Revision: ref, VolumeName: name})
		}
	}
	//for i, r := range task.Resources {
	//	for iv, v := range r.Volumes {
	//		if _, ok := refs[v.Name]; ok {
	//			task.Resources[i].Volumes[iv].GitRevision = utils.StrPtr(refs[v.Name])
	//		}
	//	}
	//}
	return nil
}
