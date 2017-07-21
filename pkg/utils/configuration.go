package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/pkg/api/v1"
)

type TFJobConfiguration struct {
	Cmd                 string
	ExecutionDir        string
	Args                string
	MonitoringNamespace string
	NodeAllocator       *string
	Images              Images
	Requests            Requests
	Volumes             []Volume
	EnvVars             []v1.EnvVar
	TimeoutMinutes      uint
}

type Images struct {
	GpuImage  string
	PSImage   string
	BaseImage string
}

type Requests struct {
	GPU         uint
	Memory      string
	MemoryLimit string
	CPU         string
	CPULimit    string
	WorkerPods  uint
	PsPods      uint
}

type Volume struct {
	v1.Volume
	IsLibDir      bool   `json:"isLibDir"`
	IsTrainLogDir bool   `json:"isTrainLogDir"`
	MountPath     string `json:"mountPath"`
	SubPath       string `json:"subPath"`
}

func (c TFJobConfiguration) String() string {
	date, err := json.MarshalIndent(&c, "", "    ")
	if err != nil {
		return ""
	}
	return string(date)
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
func (c TFJobConfiguration) GetNodeAllocator() (allocatorName string) {
	if c.NodeAllocator != nil && len(*c.NodeAllocator) > 0 {
		allocatorName = *c.NodeAllocator
		return
	}
	for _, e := range c.EnvVars {
		if e.Name == "RUNTIME_ALLOCATOR_TEMPLATE" && len(e.Value) > 0 {
			allocatorName = e.Value
			return
		}
	}
	return
}
func (c TFJobConfiguration) KubeMounts() ([]v1.Volume, []v1.VolumeMount) {
	added := make(map[string]string)
	names := make(map[string]string)
	kvolumes := make([]v1.Volume, 0)
	kvolumesMount := make([]v1.VolumeMount, 0)
	for _, v := range c.Volumes {
		id := v.GetBoundID()
		if duplicate, ok := added[id]; ok {
			if duplicate == v.Name {
				continue
			}
			names[v.Name] = duplicate
		} else {
			names[v.Name] = v.Name
			added[id] = v.Name
			kvolumes = append(kvolumes, v.Volume)
		}
		kvolumesMount = append(kvolumesMount, v1.VolumeMount{
			Name:      names[v.Name],
			SubPath:   v.SubPath,
			MountPath: v.MountPath,
		})
	}
	return kvolumes, kvolumesMount
}

func (c TFJobConfiguration) GetTrainDir() string {
	trainDir := "/tmp/train_dir"
	for _, v := range c.Volumes {
		if v.IsTrainLogDir {
			return v.MountPath
		}
	}
	return trainDir
}

func (c TFJobConfiguration) GetPythonPath() string {
	path := ""
	libDirs := []string{}
	for _, v := range c.Volumes {
		if v.IsLibDir {
			libDirs = append(libDirs, v.MountPath)
		}
	}
	if len(libDirs) > 0 {
		path = strings.Join(libDirs, ":")
	}
	return path
}

func GetConfiguration() (*TFJobConfiguration, error) {
	confFileName := "conf.json"
	if len(os.Getenv("KUBERNETES_SERVICE_PORT")) > 0 {
		confFileName = "/etc/tensorflow-spawner/conf.json"
	}
	file, err := os.Open(confFileName)
	if err != nil {
		return nil, fmt.Errorf("Failed read job configuration: %v", err)
	}
	dec := json.NewDecoder(file)
	conf := TFJobConfiguration{}
	if err := dec.Decode(&conf); err != nil {
		return nil, fmt.Errorf("Failed parse job configuration: %v", err)
	}

	if len(conf.MonitoringNamespace) < 1 {
		conf.MonitoringNamespace = "kuberlab"
	}

	if conf.TimeoutMinutes == 0 {
		conf.TimeoutMinutes = 60
	}

	return &conf, nil
}
