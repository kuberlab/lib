package mlapp

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/pborman/uuid"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func (c *BoardConfig) setRevisions(volumes []v1.Volume, task Task) {
	c.setGitRevisions(volumes, task)
	c.setPlukeRevisions(volumes, task)
}

func (c *BoardConfig) setPlukeRevisions(volumes []v1.Volume, task Task) {
	setRevision := func(vName string, rev string) {
		fromConfig := c.volumeByName(vName)
		if fromConfig == nil {
			return
		}
		for i, v := range volumes {
			if v.Name == fromConfig.CommonID() && v.FlexVolume != nil && v.FlexVolume.Options["kuberlabFS"] == "plukefs" {
				volumes[i].FlexVolume.Options["version"] = rev
			}
		}
	}

	for _, dsRev := range task.DatasetRevisions {
		if dsRev.Revision != "" {
			setRevision(dsRev.VolumeName, dsRev.Revision)
		}
	}
	for _, mdlRev := range task.ModelRevisions {
		if mdlRev.Revision != "" {
			setRevision(mdlRev.VolumeName, mdlRev.Revision)
		}
	}
}

func (c *BoardConfig) setGitRevisions(volumes []v1.Volume, task Task) {
	setRevision := func(vName string, rev string) {
		fromConfig := c.volumeByName(vName)
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
	URL        string
	Revision   string
	PrivateKey string
}

func (c *BoardConfig) existingRevisions(task Task) map[string]string {
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

func (c *BoardConfig) privateKeyFor(account string) string {
	suf := fmt.Sprintf("-%s", account)
	for _, s := range c.Secrets {
		if s.Name == "gitconfig" {
			for k, v := range s.Data {
				if strings.HasSuffix(k, suf) {
					return v
				}
			}
		}
	}
	return ""
}

func (c *BoardConfig) DetermineGitSourceRevisions(client *kubernetes.Clientset, task Task) (map[string]string, error) {
	// First, collect all volumes to mount
	// Also, determine what exactly need to get

	// volumeName -> repoDir, repoUrl
	logrus.Info("Determine git source revisions...")

	reposToDetect := make(map[string]*RepoInfo)
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

	// Add repos to determine their current revisions.
	for _, v := range c.VolumesData {
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
			var repo = v.GitRepo.Repository
			var pkey string
			if v.GitRepo.AccountId != "" {
				pkey = c.privateKeyFor(v.GitRepo.AccountId)
				repo = strings.Replace(repo, "-"+v.GitRepo.AccountId, "", -1)
			}
			reposToDetect[v.Name] = &RepoInfo{
				URL:        repo,
				PrivateKey: pkey,
			}
		}
	}

	// Get rid of public repos and detect them locally.
	for k, v := range reposToDetect {
		// Detect locally.
		cmd := fmt.Sprintf("git ls-remote %v %v 2> /dev/null | head -1 | awk '{print $1}' | xargs printf '%v %%s\\n'", v.URL, v.Revision, k)

		if v.PrivateKey != "" {
			keyFileName := uuid.New()
			if err := ioutil.WriteFile(keyFileName, []byte(v.PrivateKey), 0600); err != nil {
				return nil, err
			}
			cmd = fmt.Sprintf(`GIT_SSH_COMMAND='ssh -i %v -o StrictHostKeyChecking=no' %v`, keyFileName, cmd)
			//noinspection GoDeferInLoop
			defer os.Remove(keyFileName)
		}

		logrus.Infof("Exec locally: %v", cmd)
		out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Failed detect git revisions: %v, %v", string(out), err)
		}
		parseLogsAddRevs(out, res)
		if _, ok := res[k]; ok {
			delete(reposToDetect, k)
		}
	}

	if len(reposToDetect) == 0 {
		return res, nil
	}

	return res, nil
}

func parseLogsAddRevs(logsRaw []byte, res map[string]string) {
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
}

func (c *BoardConfig) InjectGitRevisions(client *kubernetes.Clientset, task *Task) error {
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
			task.GitRevisions = append(task.GitRevisions, TaskRevision{Revision: ref, VolumeName: name})
		}
	}
	return nil
}
