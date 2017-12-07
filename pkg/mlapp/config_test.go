package mlapp

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/kuberlab/lib/pkg/example"
)

func Assert(want, got interface{}, t *testing.T) {
	if !reflect.DeepEqual(want, got) {
		_, file, line, _ := runtime.Caller(1)
		splitted := strings.Split(file, string(os.PathSeparator))
		t.Fatalf("%v:%v: Failed: got %v, want %v", splitted[len(splitted)-1], line, got, want)
	}
}

func TestUnmarshalConfig(t *testing.T) {

	conf := Config{}
	err := yaml.Unmarshal([]byte(example.TF_EXAMPLE), &conf)

	if err != nil {
		t.Fatal(err)
	}

	Assert("MLApp", conf.Kind, t)
	Assert("tfexample", conf.Name, t)
	Assert("testValue", conf.Labels["testKey"], t)
	Assert(1, len(conf.Tasks), t)
	Assert("model", conf.Tasks[0].Name, t)
	Assert("testModelValue", conf.Tasks[0].Labels["testModelKey"], t)
	Assert(2, len(conf.Tasks[0].Resources), t)
	Assert("worker", conf.Tasks[0].Resources[0].Name, t)
	Assert("testWorkerValue", conf.Tasks[0].Resources[0].Labels["testWorkerKey"], t)
	Assert(uint(2), conf.Tasks[0].Resources[0].Replicas, t)
	Assert(1, conf.Tasks[0].Resources[0].MaxRestartCount, t)
	Assert(true, conf.Tasks[0].Resources[0].AllowFail, t)
	Assert(int32(9000), conf.Tasks[0].Resources[0].Port, t)
	Assert("doneConditionValue", conf.Tasks[0].Resources[0].DoneCondition, t)
	Assert("image-cpu", conf.Tasks[0].Resources[0].Images.CPU, t)
	Assert("image-gpu", conf.Tasks[0].Resources[0].Images.GPU, t)
	Assert(1, len(conf.Tasks[0].Resources[0].Command), t)
	Assert("python", conf.Tasks[0].Resources[0].Command[0], t)
	Assert("Never", conf.Tasks[0].Resources[0].RestartPolicy, t)
	//Assert(1, len(conf.Tasks[0].Resources[0].Args), t)
	//Assert("--log-dir=$TRAINING_DIR", conf.Tasks[0].Resources[0].Args[0], t)
	Assert("directory", conf.Tasks[0].Resources[0].WorkDir, t)
	Assert(2, len(conf.Tasks[0].Resources), t)
	Assert("v1", conf.Tasks[0].Resources[0].Env[0].Value, t)
	Assert("TEST_ENV_V1", conf.Tasks[0].Resources[0].Env[0].Name, t)
	Assert("v1", conf.Tasks[0].Resources[0].Env[0].Value, t)
	Assert(uint(1), conf.Tasks[0].Resources[0].Resources.Accelerators.GPU, t)
	Assert(2, len(conf.Tasks[0].Resources[0].Volumes), t)

	// UIX
	Assert(1, len(conf.Uix), t)
	Assert("jupyter", conf.Uix[0].Name, t)
	Assert("Jupyter", conf.Uix[0].DisplayName, t)
	Assert("http", conf.Uix[0].Ports[0].Name, t)
	Assert(int32(80), conf.Uix[0].Ports[0].Port, t)
	Assert("TCP", conf.Uix[0].Ports[0].Protocol, t)
	Assert(int32(8082), conf.Uix[0].Ports[0].TargetPort, t)

	// Serving
	Assert(1, len(conf.Serving), t)
	Assert("test-serv", conf.Serving[0].Name, t)
	Assert("Test serving", conf.Serving[0].DisplayName, t)
	Assert("task-name", conf.Serving[0].TaskName, t)
	Assert("5", conf.Serving[0].Build, t)

	// Volumes
	Assert(2, len(conf.Volumes), t)
	Assert("lib", conf.Volumes[0].Name, t)
	Assert(true, conf.Volumes[0].IsLibDir, t)
	Assert(true, conf.Volumes[1].IsTrainLogDir, t)
	Assert("/workspace/lib", conf.Volumes[0].MountPath, t)
	Assert("lib", conf.Volumes[0].SubPath, t)
	Assert("test", conf.Volumes[0].ClusterStorage, t)
	Assert("/test", conf.Volumes[0].HostPath.Path, t)

}
