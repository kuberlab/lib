package mlapp

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/kuberlab/lib/pkg/utils"
	"k8s.io/api/core/v1"
)

type VolumeMount struct {
	Name        string  `json:"name"`
	MountPath   string  `json:"mountPath"`
	ReadOnly    bool    `json:"readOnly,omitempty"`
	SubPath     string  `json:"subPath"`
	GitRevision *string `json:"gitRevision,omitempty"`
}
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
	ReadOnly      bool   `json:"readOnly"`
}

type VolumeSource struct {
	IsWorkspaceLocal      bool                                  `json:"isWorkspaceLocal"`
	HostPath              *v1.HostPathVolumeSource              `json:"hostPath,omitempty" protobuf:"bytes,1,opt,name=hostPath"`
	GitRepo               *GitRepoVolumeSource                  `json:"gitRepo,omitempty" protobuf:"bytes,5,opt,name=gitRepo"`
	NFS                   *v1.NFSVolumeSource                   `json:"nfs,omitempty" protobuf:"bytes,7,opt,name=nfs"`
	PersistentVolumeClaim *v1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" protobuf:"bytes,10,opt,name=persistentVolumeClaim"`
	EmptyDir              *v1.EmptyDirVolumeSource              `json:"emptyDir,omitempty" protobuf:"bytes,2,opt,name=emptyDir"`
	S3Bucket              *S3BucketSource                       `json:"s3bucket,omitempty" protobuf:"bytes,98,opt,name=s3bucket"`
	FlexVolume            *v1.FlexVolumeSource                  `json:"flexVolume,omitempty" protobuf:"bytes,12,opt,name=flexVolume"`
	PersistentStorage     *PersistentStorage                    `json:"persistentStorage,omitempty" protobuf:"bytes,99,opt,name=persistentStorage"`
	Dataset               *DatasetSource                        `json:"dataset,omitempty"`
	DatasetFS             *DatasetFSSource                      `json:"datasetFS,omitempty"`
}

func (v Volume) CommonID() string {
	if v.PersistentStorage != nil {
		return "kps-" + v.PersistentStorage.StorageName
	} else if v.NFS != nil {
		m := "rw"
		if v.NFS.ReadOnly {
			m = "r"
		}
		if v.ReadOnly {
			m = "r"
		}
		server := base64.RawURLEncoding.EncodeToString([]byte(v.NFS.Server + "-" + v.NFS.Path + "-" + m))
		return "nfs-" + strings.ToLower(strings.Replace(server, "_", "-", -1))
	}
	m := "org-"
	if v.ReadOnly {
		m = "org-r-"
	}
	return m + v.Name
}
func (v Volume) V1Volume() v1.Volume {
	r := v1.Volume{
		Name: v.CommonID(),
		VolumeSource: v1.VolumeSource{
			HostPath:              v.HostPath,
			NFS:                   v.NFS,
			EmptyDir:              v.EmptyDir,
			PersistentVolumeClaim: v.PersistentVolumeClaim,
			FlexVolume:            v.FlexVolume,
		},
	}
	if v.PersistentStorage != nil {
		r.PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{
			ClaimName: utils.KubeDeploymentEncode(v.PersistentStorage.StorageName),
		}
	}
	if v.GitRepo != nil {
		if v.GitRepo.AccountId != "" {
			r.EmptyDir = &v1.EmptyDirVolumeSource{}
		} else {
			// Copy git repo to not allow change it in the future.
			git := v.GitRepo.GitRepoVolumeSource
			r.GitRepo = &git
		}
	}
	return r
}

type GitRepoVolumeSource struct {
	v1.GitRepoVolumeSource `json:",inline" protobuf:"bytes,1,opt,name=volumeSource"`
	AccountId              string `json:"accountId,omitempty" protobuf:"bytes,2,opt,name=accountId"`
	PrivateKey             string `json:"private_key" protobuf:"bytes,3,opt,name=private_key"`
	UserName               string `json:"user_name" protobuf:"bytes,4,opt,name=user_name"`
	AccessToken            string `json:"access_token" protobuf:"bytes,5,opt,name=access_token"`
}

type S3BucketSource struct {
	Bucket    string `json:"bucket" protobuf:"bytes,1,opt,name=bucket"`
	Server    string `json:"server,omitempty" protobuf:"bytes,2,opt,name=server"`
	AccountId string `json:"accountId,omitempty" protobuf:"bytes,3,opt,name=accountId"`
}

type DatasetSource struct {
	Workspace string `json:"workspace" protobuf:"bytes,1,opt,name=workspace"`
	Dataset   string `json:"dataset,omitempty" protobuf:"bytes,2,opt,name=dataset"`
	Version   string `json:"version,omitempty" protobuf:"bytes,3,opt,name=version"`
	ServerURL string `json:"serverURL,omitempty" protobuf:"bytes,3,opt,name=serverURL"`
}

type DatasetFSSource struct {
	Workspace string `json:"workspace" protobuf:"bytes,1,opt,name=workspace"`
	Dataset   string `json:"dataset,omitempty" protobuf:"bytes,2,opt,name=dataset"`
	Version   string `json:"version,omitempty" protobuf:"bytes,3,opt,name=version"`
	Server    string `json:"server,omitempty" protobuf:"bytes,3,opt,name=server"`
}

type PersistentStorage struct {
	StorageName string `json:"storageName,omitempty" protobuf:"bytes,1,opt,name=storageName"`
	Size        string `json:"size" protobuf:"bytes,2,opt,name=size"`
}

func (v Volume) String() string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (v Volume) GetBoundID() string {
	if v.NFS != nil {
		p := "rw"
		if v.NFS.ReadOnly {
			p = "r"
		}
		if v.ReadOnly {
			p = "r"
		}
		return v.NFS.Server + "/" + v.NFS.Path + ":" + p
	} else if v.GitRepo != nil {
		return v.GitRepo.Repository + "/" + v.GitRepo.Directory + ":" + v.GitRepo.Revision
	} else if v.PersistentVolumeClaim != nil {
		return v.PersistentVolumeClaim.ClaimName
	}
	return v.Name
}
