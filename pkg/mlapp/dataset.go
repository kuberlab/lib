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

	// Set datasetRevisions in task
	revisionMap := make(map[string]string)
	for _, v := range c.VolumesData {
		from := c.VolumeByName(v.Name)
		if v.FlexVolume != nil && (from.Dataset != nil || from.DatasetFS != nil) {
			revisionMap[v.Name] = v.FlexVolume.Options["version"]
		}
	}

	setRevision := func(name, revision string) {
		found := false
		for _, taskRev := range task.DatasetRevisions {
			if taskRev.VolumeName == name {
				found = true
				taskRev.Revision = revision
			}
		}
		if !found {
			task.DatasetRevisions = append(
				task.DatasetRevisions, TaskRevision{VolumeName: name, Revision: revision},
			)
		}
	}

	for k, v := range revisionMap {
		setRevision(k, v)
	}

	// Set revisions in config.
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
