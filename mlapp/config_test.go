package mlapp

import (
	"testing"
	"gopkg.in/yaml.v2"
	"strings"
	"runtime"
	"reflect"
	"os"
	"fmt"
)

var cfg = `
kind: MLApp
metadata:
  name: name
  namespace: namespace
  labels: # Will be applayed to each resource
    key: value
  tasks:
  - name: model
    labels:
      key: value  # Will be applayed to each resource
    resources:
    - name: workers
      labels:
        key: value  # Will be applayed to each resource
      replicas: 1
      minAvailable: 1
      restartPolicy: Always,Never,OnFailure
      maxRestartCount: 1
      images:
        gpu: image-gpu
        cpu: image-cpu
      command: python
      workdir: directory
      args: ""
      env:
      - name: NAME
        value: value
      - name: PYTHONPATH
        value: /usr/local # Will be extended as /usr/local:KUBERLAB_PYYHON_LIB
      resources:
        accelerators:
          gpu: 1
        requests:
          cpu: 100mi
          memory: 1Gi
        limits:
          cpu: 100mi
          memory: 1Gi
  uix:
    - name: jupyter
      displayName: Jupyter
      resources:
        accelerators:
          gpu: 1
        requests:
          cpu: 100mi
          memory: 1Gi
        limits:
          cpu: 100mi
          memory: 1Gi
      ports:
        - port: 80
          targetPort: 8082
          protocol: TCP
          name: http
`

func Assert(want, got interface{}, t *testing.T) {
	if !reflect.DeepEqual(want, got) {
		_, file, line, _ := runtime.Caller(1)
		splitted := strings.Split(file, string(os.PathSeparator))
		t.Fatalf("%v:%v: Failed: got %v, want %v", splitted[len(splitted)-1], line, got, want)
	}
}

func TestUnmarshalConfig(t *testing.T) {
	conf := Config{}
	err := yaml.Unmarshal([]byte(cfg), &conf)

	if err != nil {
		t.Fatal(err)
	}

	Assert("MLApp", conf.Kind, t)
	Assert("name", conf.Metadata.Name, t)
	Assert("namespace", conf.Metadata.Namespace, t)
	Assert("value", conf.Metadata.Labels["key"], t)
	Assert("model", conf.Metadata.Tasks[0].Name, t)
	Assert("value", conf.Metadata.Tasks[0].Labels["key"], t)
	Assert("workers", conf.Metadata.Tasks[0].Resources[0].Name, t)
	Assert("value", conf.Metadata.Tasks[0].Resources[0].Labels["key"], t)
	fmt.Println(conf.Metadata.Tasks[0].Resources[0])
	Assert(uint(1), conf.Metadata.Tasks[0].Resources[0].Replicas, t)
	Assert(uint(1), conf.Metadata.Tasks[0].Resources[0].MinAvailable, t)
	Assert("python", conf.Metadata.Tasks[0].Resources[0].Command, t)
	Assert("", conf.Metadata.Tasks[0].Resources[0].Args, t)
	Assert(uint(1), conf.Metadata.Tasks[0].Resources[0].MaxRestartCount, t)
	Assert("Always,Never,OnFailure", conf.Metadata.Tasks[0].Resources[0].RestartPolicy, t)
	Assert("directory", conf.Metadata.Tasks[0].Resources[0].WorkDir, t)
	Assert("name", conf.Metadata.Tasks[0].Resources[0].Env[0].Name, t)
	Assert("value", conf.Metadata.Tasks[0].Resources[0].Env[0].Value, t)
	Assert(uint(1), conf.Metadata.Tasks[0].Resources[0].Resources.Accelerators.GPU, t)

	// UIX
	Assert("jupyter", conf.Metadata.Uix[0].Name, t)
	Assert("Jupyter", conf.Metadata.Uix[0].DisplayName, t)
	Assert("http", conf.Metadata.Uix[0].Ports[0].Name, t)
	Assert(uint(80), conf.Metadata.Uix[0].Ports[0].Port, t)
	Assert("TCP", conf.Metadata.Uix[0].Ports[0].Protocol, t)
	Assert(uint(8082), conf.Metadata.Uix[0].Ports[0].TargetPort, t)
}
