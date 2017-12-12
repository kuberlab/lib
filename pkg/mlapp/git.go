package mlapp

import (
	"k8s.io/client-go/pkg/api/v1"
)

func (c *Config) setGitRefs(volumes []v1.Volume, taskRes TaskResource) {
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

	for _, tv := range taskRes.Volumes {
		if tv.GitRevision != nil {
			setRevision(tv.Name, *tv.GitRevision)
		}
	}
}
