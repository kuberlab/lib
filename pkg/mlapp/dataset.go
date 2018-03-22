package mlapp

func (c *BoardConfig) InjectDatasetRevisions(task *Task) {
	// Detect explicitly set revisions.
	for _, taskRev := range task.DatasetRevisions {
		for _, v := range c.VolumesData {
			if v.Name == taskRev.VolumeName {
				if v.Dataset != nil {
					v.Dataset.Version = taskRev.Revision
				} else if v.DatasetFS != nil {
					v.DatasetFS.Version = taskRev.Revision
				}
			}
		}
	}
}
