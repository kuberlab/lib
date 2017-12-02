package mlapp

import (
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Config) setGitRefs(volumes []v1.Volume, gitRefs map[string]string) {
	for source, ref := range gitRefs {
		fromConfig := c.VolumeByName(source)
		if fromConfig == nil {
			continue
		}
		for i, v := range volumes {
			if v.Name == fromConfig.CommonID() && v.GitRepo != nil {
				volumes[i].GitRepo.Revision = ref
			}
		}
	}
}
