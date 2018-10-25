package mlapp

import "github.com/Sirupsen/logrus"

type GetRevisionsFunc func(task *Task) []TaskRevision
type AddRevisionFunc func(rev TaskRevision)
type RightVolumeFunc func(v Volume) bool

func (c *BoardConfig) injectVersionedRevisions(
	task *Task, getRevs GetRevisionsFunc, addRev AddRevisionFunc, checkVolume RightVolumeFunc) {
	// Detect explicitly set revisions.
	for _, taskRev := range getRevs(task) {
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
		if v.FlexVolume != nil {
			if !checkVolume(v) {
				continue
			}
			version, ok := v.FlexVolume.Options["version"]
			if ok {
				revisionMap[v.Name] = version
			}
		}
	}

	setRevision := func(name, revision string) {
		found := false
		for _, taskRev := range getRevs(task) {
			if taskRev.VolumeName == name {
				found = true
				taskRev.Revision = revision
			}
		}
		if !found {
			addRev(TaskRevision{VolumeName: name, Revision: revision})
		}
	}

	for k, v := range revisionMap {
		logrus.Infof("Set dataset revision [%v=%v]", k, v)
		setRevision(k, v)
	}

	// Set revisions in config.
	for _, taskRev := range getRevs(task) {
		for i, v := range c.Volumes {
			if v.Name == taskRev.VolumeName {
				if v.Dataset != nil {
					c.Volumes[i].Dataset.Version = taskRev.Revision
				} else if v.DatasetFS != nil {
					c.Volumes[i].DatasetFS.Version = taskRev.Revision
				} else if v.Model != nil {
					c.Volumes[i].Model.Version = taskRev.Revision
				}
			}
		}
	}
}


func (c *BoardConfig) InjectDatasetRevisions(task *Task) {
	getRevs := func(task *Task) []TaskRevision {
		return task.DatasetRevisions
	}
	addRev := func(rev TaskRevision) {
		task.DatasetRevisions = append(task.DatasetRevisions, rev)
	}
	checkVolume := func(v Volume) bool {
		if v.FlexVolume != nil {
			t, ok := v.FlexVolume.Options["type"]
			if !ok {
				return false
			} else {
				return t == "dataset"
			}
		}
		return false
	}
	c.injectVersionedRevisions(task, getRevs, addRev, checkVolume)
}


func (c *BoardConfig) InjectModelRevisions(task *Task) {
	getRevs := func(task *Task) []TaskRevision {
		return task.ModelRevisions
	}
	addRev := func(rev TaskRevision) {
		task.ModelRevisions = append(task.ModelRevisions, rev)
	}
	checkVolume := func(v Volume) bool {
		if v.FlexVolume != nil {
			t, ok := v.FlexVolume.Options["type"]
			if !ok {
				return false
			} else {
				return t == "model"
			}
		}
		return false
	}
	c.injectVersionedRevisions(task, getRevs, addRev, checkVolume)
}
