package mlapp

import (
	"fmt"

	"github.com/kuberlab/lib/pkg/types"
	"github.com/prometheus/common/model"
)

type ComponentMetric struct {
	ComponentName string    `json:"component_name"`
	ComponentType string    `json:"component_type"`
	Metrics       []*Metric `json:"metrics"`
	// Labels        model.Metric
}

type ComponentMetrics struct {
	Metrics []*ComponentMetric `json:"metrics"`
	Start   types.Time         `json:"start"`
	End     types.Time         `json:"end"`
}

type Metric struct {
	Name     string       `json:"name"`
	JoinedBy string       `json:"joined_by,omitempty"`
	Values   []Value      `json:"values"`
	Labels   model.Metric `json:"labels"`
}

type Value struct {
	Argument
	Value float64 `json:"value"`
}

func (v Value) String() string {
	ts := "<nil>"
	if v.Ts != nil {
		ts = fmt.Sprintf("%v", *v.Ts)
	}
	iter := "<nil>"
	if v.Iter != nil {
		iter = fmt.Sprintf("%v", *v.Iter)
	}
	return fmt.Sprintf("{Iter: %v, Ts: %v, Value: %v}", iter, ts, v.Value)
}

type Argument struct {
	Ts   *int64 `json:"ts,omitempty"`
	Iter *int64 `json:"iter,omitempty"`
}

type Values []Value

func (v Values) Len() int {
	return len(v)
}

func (v Values) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v Values) Less(i, j int) bool {
	vi := v[i]
	vj := v[j]
	if vi.Iter != nil && vj.Iter != nil {
		return *vi.Iter < *vj.Iter
	}
	if vi.Ts != nil && vj.Ts != nil {
		return *vi.Ts < *vj.Ts
	}
	return false
}
