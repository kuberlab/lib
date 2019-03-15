package mlapp

import "github.com/Sirupsen/logrus"

type GetRevisionsFunc func(task *Task) []TaskRevision
type AddRevisionFunc func(rev TaskRevision)
type RightVolumeFunc func(v Volume) bool

func (c *BoardConfig) injectVersionedRevisions(
	task *Task, getRevs GetRevisionsFunc, addRev AddRevisionFunc, checkVolume RightVolumeFunc) {
	// Detect default revisions from config
	revisionMap := make(map[string]string)
	for _, v := range c.VolumesData {
		if !checkVolume(v) {
			continue
		}
		version, ok := v.FlexVolume.Options["version"]
		if ok {
			revisionMap[v.Name] = version
		}
	}
	// Override revisions from task
	for _, rev := range getRevs(task) {
		revisionMap[rev.VolumeName] = rev.Revision
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
