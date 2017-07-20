package mlapp

import "k8s.io/client-go/pkg/api/v1"

type Volume struct {
	// as in v1.Volume
	VolumeSource `json:",inline" protobuf:"bytes,2,opt,name=volumeSource"`
	// as in v1.Volume
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`

	ClusterStorage string `json:"clusterStorage,omitempty"`
	//Broken         bool   `json:"broken"`

	MountPath     string `json:"mountPath"`
	SubPath       string `json:"subPath"`
	IsTrainLogDir bool   `json:"isTrainLogDir"`
	IsLibDir      bool   `json:"isLibDir"`
}

type VolumeSource struct {
	HostPath              *v1.HostPathVolumeSource              `json:"hostPath,omitempty" protobuf:"bytes,1,opt,name=hostPath"`
	GitRepo               *v1.GitRepoVolumeSource               `json:"gitRepo,omitempty" protobuf:"bytes,5,opt,name=gitRepo"`
	NFS                   *v1.NFSVolumeSource                   `json:"nfs,omitempty" protobuf:"bytes,7,opt,name=nfs"`
	PersistentVolumeClaim *v1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" protobuf:"bytes,10,opt,name=persistentVolumeClaim"`
}

func (v Volume) v1Volume() v1.Volume {
	return v1.Volume{
		Name: v.Name,
		VolumeSource: v1.VolumeSource{
			HostPath: v.HostPath,
			GitRepo:  v.GitRepo,
			NFS:      v.NFS,
			PersistentVolumeClaim: v.PersistentVolumeClaim,
		},
	}
}
