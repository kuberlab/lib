package mlapp

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
)

var deployTpl = `
kind: MLApp
metadata:
  name: mlapp
  labels: # Will be applayed to each resource
    key1: value1
spec:
  volumes:
    - name: lib
      nfs:
        server: 10.0.0.1
        path: /kuberlab
  uix:
    - name: jupyter
      displayName: Jupyter
      labels:
        key2: value2
      image: tensorflow/tensorflow
      env:
        - name: LOL
          value: "TRUE"
      resources:
        accelerators:
          gpu: 1
        requests:
          cpu: 100m
          memory: 1Gi
      volumes:
        - name: lib
          mountPath: /lib
      ports:
        - port: 80
          targetPort: 8888
          protocol: TCP
          name: http
`

func TestConfig_GenerateUIXResources(t *testing.T) {
	conf := Config{}
	err := yaml.Unmarshal([]byte(deployTpl), &conf)

	if err != nil {
		t.Fatal(err)
	}

	_, err = conf.GenerateUIXResources()
	if err != nil {
		t.Fatal(err)
	}
}
