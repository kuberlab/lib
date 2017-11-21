package mlapp

import (
	"encoding/json"

	"k8s.io/client-go/pkg/api/v1"
	"encoding/base64"
	"strings"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultStorageClass = "glusterfs"
)

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	SubPath   string `json:"subPath"`
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
}

type VolumeSource struct {
	HostPath              *v1.HostPathVolumeSource              `json:"hostPath,omitempty" protobuf:"bytes,1,opt,name=hostPath"`
	GitRepo               *GitRepoVolumeSource                  `json:"gitRepo,omitempty" protobuf:"bytes,5,opt,name=gitRepo"`
	NFS                   *v1.NFSVolumeSource                   `json:"nfs,omitempty" protobuf:"bytes,7,opt,name=nfs"`
	PersistentVolumeClaim *v1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" protobuf:"bytes,10,opt,name=persistentVolumeClaim"`
	EmptyDir              *v1.EmptyDirVolumeSource              `json:"emptyDir,omitempty" protobuf:"bytes,2,opt,name=emptyDir"`
	S3Bucket              *S3BucketSource                       `json:"s3bucket,omitempty" protobuf:"bytes,98,opt,name=s3bucket"`
	FlexVolume            *v1.FlexVolumeSource                  `json:"flexVolume,omitempty" protobuf:"bytes,12,opt,name=flexVolume"`
	PersistentStorage     *PersistentStorage                    `json:"persistentStorage,omitempty" protobuf:"bytes,99,opt,name=persistentStorage"`
}

func (v Volume) CommonID() string {
	if v.PersistentStorage != nil {
		return "kps-" + v.PersistentStorage.StorageName
	} else if v.NFS!=nil{
		m := "rw"
		if v.NFS.ReadOnly{
			m = "r"
		}
		server := base64.RawURLEncoding.EncodeToString([]byte(v.NFS.Server+"-"+v.NFS.Path+"-"+m))
		return "nfs-"+strings.ToLower(strings.Replace(server,"_","-",-1))
	}
	return "org-"+v.Name
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
			ClaimName: v.PersistentStorage.StorageName,
		}
	}
	if v.GitRepo != nil {
		if v.GitRepo.AccountId != "" {
			r.EmptyDir = &v1.EmptyDirVolumeSource{}
		} else {
			r.GitRepo = &v.GitRepo.GitRepoVolumeSource
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
		return v.NFS.Server + "/" + v.NFS.Path + ":" + p
	} else if v.GitRepo != nil {
		return v.GitRepo.Repository + "/" + v.GitRepo.Directory + ":" + v.GitRepo.Revision
	} else if v.PersistentVolumeClaim != nil {
		return v.PersistentVolumeClaim.ClaimName
	}
	return v.Name
}

func ParsePVC(v PersistentStorage, namespace string, labels map[string]string) (*v1.PersistentVolumeClaim, error) {
	q, err := resource.ParseQuantity(v.Size)
	if err != nil {
		return nil, fmt.Errorf("Invalid kuberlab storage size %v", err)
	}
	storageClass := DefaultStorageClass
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      v.StorageName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
			StorageClassName: &storageClass,
			Resources: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceStorage: q,
				},
			},
		},
	}
	return pvc, nil
}
