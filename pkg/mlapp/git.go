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
				volumes[i].GitRepo.Revision = rev
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

func (c *Config) existingRevisions(task Task) map[string]string {
	// First, populate revisions for volumes in config (default).
	revs := make(map[string]string)
	for _, v := range c.Volumes {
		if v.GitRepo != nil && v.GitRepo.Revision != "" {
			revs[v.Name] = v.GitRepo.Revision
		}
	}

	// Next, populate revisions from taskResources.
	for _, res := range task.Resources {
		for _, vm := range res.Volumes {
			if vm.GitRevision != nil {
				revs[vm.Name] = *vm.GitRevision
			}
		}
	}
	return revs
}

func (c *Config) DetermineGitSourceRevisions(client *kubernetes.Clientset, task Task) (map[string]string, error) {
	// First, collect all volumes to mount
	// Also, determine what exactly need to get

	// volumeName -> repoDir, repoUrl
	logrus.Info("Determine git source revisions...")

	gitRepos := make(map[string]*RepoInfo)
	res := make(map[string]string)
	defaultRevisions := c.existingRevisions(task)
	logrus.Debugf("Default revisions: %v", defaultRevisions)

	// Detect explicitly set revisions.
	for _, taskRev := range task.GitRevisions {
		res[taskRev.VolumeName] = taskRev.Revision
	}

	volumeUsedInTask := func(name string) bool {
		for _, res := range task.Resources {
			if res.UseDefaultVolumeMapping {
				return true
			}
			for _, vm := range res.Volumes {
				if vm.Name == name {
					return true
				}
			}
		}
		return false
	}
	volumesMap := make(map[string]*v1.Volume)

	// Add repos to determine their current revisions.
	for _, v := range c.Volumes {
		if v.GitRepo == nil {
			continue
		}
		if !volumeUsedInTask(v.Name) {
			continue
		}
		if _, ok := res[v.Name]; !ok {
			// Try to pick up default revision.
			if _, ok := defaultRevisions[v.Name]; ok {
				res[v.Name] = defaultRevisions[v.Name]
				continue
			}

			repoName := getGitRepoName(v.GitRepo.Repository)
			gitRepos[v.Name] = &RepoInfo{
				Dir: fmt.Sprintf("/rev-detect/%v", repoName),
				URL: v.GitRepo.Repository,
			}
			vv := v.V1Volume()
			volumesMap[v.Name] = &vv
			volumesMap[v.Name].Name = v.Name
		}
	}
	if len(gitRepos) == 0 {
		return res, nil
	}

	// Generate script for determining revisions.
	cmd := []string{"mkdir -p ~/.ssh"}
	for k, v := range gitRepos {
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
	volumeMounts := make([]v1.VolumeMount, 0)
	if len(c.Secrets) > 0 {
		vol, vom, err := c.getSecretVolumes(c.Secrets)
		if err != nil {
			return nil, err
		}
		if len(vol) > 0 {
			volumes = append(volumes, vol...)
		}
		if len(vom) > 0 {
			volumeMounts = append(volumeMounts, vom...)
		}
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
	logrus.Infof("Result: %v", string(logsRaw))
	logs := strings.Split(string(logsRaw), "\n")

	for _, l := range logs {
		if l == "" {
			continue
		}
		splitted := strings.Split(l, " ")
		if len(splitted) != 2 {
			continue
		}
		name := splitted[0]
		rev := splitted[1]

		res[name] = rev
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
	return nil
}
