package mlapp

func (c *BoardConfig) InjectDatasetRevisions(task *Task) {
	// Detect explicitly set revisions.
	for _, taskRev := range task.DatasetRevisions {
		for _, v := range c.VolumesData {
			if v.Name == taskRev.VolumeName {
				if v.FlexVolume != nil {
					_, ok := v.FlexVolume.Options["version"]
					if ok {
						v.FlexVolume.Options["version"] = taskRev.Revision
					}
				}
			}
		}
	}

	// Detect explicitly set revisions.
	for _, taskRev := range task.DatasetRevisions {
		for i, v := range c.Volumes {
			if v.Name == taskRev.VolumeName {
				if v.Dataset != nil {
					c.Volumes[i].Dataset.Version = taskRev.Revision
				} else if v.DatasetFS != nil {
					c.Volumes[i].DatasetFS.Version = taskRev.Revision
				}
			}
		}
	}
}
