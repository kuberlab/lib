package mlapp

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
)

var cfg = `
kind: MLApp
metadata:
  name: name
  namespace: namespace
  labels: # Will be applayed to each resource
    key: value
spec:
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
	Assert("name", conf.Name, t)
	Assert("value", conf.Labels["key"], t)
	Assert("model", conf.Tasks[0].Name, t)
	Assert("value", conf.Tasks[0].Labels["key"], t)
	Assert("workers", conf.Tasks[0].Resources[0].Name, t)
	Assert("value", conf.Tasks[0].Resources[0].Labels["key"], t)
	Assert(uint(1), conf.Tasks[0].Resources[0].Replicas, t)
	Assert(uint(1), conf.Tasks[0].Resources[0].MinAvailable, t)
	Assert("python", conf.Tasks[0].Resources[0].Command, t)
	Assert("", conf.Tasks[0].Resources[0].RawArgs, t)
	Assert(uint(1), conf.Tasks[0].Resources[0].MaxRestartCount, t)
	Assert("Always,Never,OnFailure", conf.Tasks[0].Resources[0].RestartPolicy, t)
	Assert("directory", conf.Tasks[0].Resources[0].WorkDir, t)
	Assert("NAME", conf.Tasks[0].Resources[0].Env[0].Name, t)
	Assert("value", conf.Tasks[0].Resources[0].Env[0].Value, t)
	Assert(uint(1), conf.Tasks[0].Resources[0].Resources.Accelerators.GPU, t)

	// UIX
	Assert("jupyter", conf.Uix[0].Name, t)
	Assert("Jupyter", conf.Uix[0].DisplayName, t)
	Assert("http", conf.Uix[0].Ports[0].Name, t)
	Assert(int32(80), conf.Uix[0].Ports[0].Port, t)
	Assert("TCP", conf.Uix[0].Ports[0].Protocol, t)
	Assert(int32(8082), conf.Uix[0].Ports[0].TargetPort, t)
}
