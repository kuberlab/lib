package dealerclient

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func Assert(want, got interface{}, t *testing.T) {
	if !reflect.DeepEqual(want, got) {
		_, file, line, _ := runtime.Caller(1)
		splitted := strings.Split(file, string(os.PathSeparator))
		t.Fatalf("%v:%v: Failed: got %v, want %v", splitted[len(splitted)-1], line, got, want)
	}
}

func TestMinimizeLimits(t *testing.T) {
	zero := int64(0)
	inf := int64(-1)
	toMinimize := &ResourceLimit{
		CPU:           nil,
		Memory:        nil,
		GPU:           &zero,
		Replicas:      0,
		ParallelRuns:  0,
		ExecutionTime: 0,
	}
	referLimits := &ResourceLimit{
		CPU:           nil,
		Memory:        nil,
		GPU:           &inf,
		Replicas:      -1,
		ParallelRuns:  -1,
		ExecutionTime: -1,
	}

	toMinimize.MinimizeTo(*referLimits)

	Assert(zero, *toMinimize.GPU, t)
}

func TestMinimizeLimits2(t *testing.T) {
	inf := int64(-1)
	cpu1 := resource.MustParse("1")
	mem4Gi := resource.MustParse("4Gi")
	toMinimize := &ResourceLimit{
		CPU:           &cpu1,
		Memory:        &mem4Gi,
		GPU:           &inf,
		Replicas:      0,
		ParallelRuns:  1,
		ExecutionTime: 0,
	}
	referLimits := &ResourceLimit{
		CPU:           nil,
		Memory:        nil,
		GPU:           &inf,
		Replicas:      -1,
		ParallelRuns:  -1,
		ExecutionTime: -1,
	}

	toMinimize.MinimizeTo(*referLimits)

	Assert(int64(1000), toMinimize.CPU.MilliValue(), t)
	Assert(int64(4295), toMinimize.Memory.ScaledValue(6), t)
}
