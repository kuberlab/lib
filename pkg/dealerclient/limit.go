package dealerclient

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceLimit struct {
	CPU           *resource.Quantity `json:"cpu,omitempty,omitempty"`
	Memory        *resource.Quantity `json:"memory,omitempty,omitempty"`
	GPU           *int64             `json:"gpu,omitempty"`
	Replicas      int64              `json:"replicas,omitempty"`
	ParallelRuns  int64              `json:"parallel_runs,omitempty"`
	ExecutionTime int64              `json:"execution_time,omitempty"`
}

func (r *ResourceLimit) MinimizeTo(limit ResourceLimit) {
	minCPU := minQuantity(r.CPUQuantity(), limit.CPUQuantity())
	minMemory := minQuantity(r.MemoryQuantity(), limit.MemoryQuantity())

	if limit.GPU != nil && r.GPU != nil {
		if *limit.GPU >= 0 && *r.GPU > *limit.GPU || *r.GPU < 0 {
			r.GPU = limit.GPU
		}
	} else if r.GPU == nil && limit.GPU != nil {
		r.GPU = limit.GPU
	}

	if r.Replicas > limit.Replicas && limit.Replicas > 0 || r.Replicas <= 0 {
		r.Replicas = limit.Replicas
	}
	if r.ExecutionTime > limit.ExecutionTime && limit.Replicas > 0 || r.ExecutionTime <= 0 {
		r.ExecutionTime = limit.ExecutionTime
	}
	r.Memory = nil
	r.CPU = nil
	if minCPU != nil {
		r.CPU = minCPU
	}
	if minMemory != nil {
		r.Memory = minMemory
	}
}

func (r *ResourceLimit) CPUQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}

	if r.CPU != nil && r.CPU.MilliValue() <= 0 {
		return nil
	}

	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.CPU != nil && !r.CPU.IsZero() {
		q = r.CPU
	} else {
		return nil
	}
	return q
}

func (r *ResourceLimit) GPUQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}
	if r.GPU == nil || *r.GPU < 0 {
		// No limit
		return nil
	}
	q := &resource.Quantity{Format: resource.DecimalSI}
	q.Set(*r.GPU)
	return q
}

func (r *ResourceLimit) MemoryQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}

	if r.Memory != nil && r.Memory.Value() <= 0 {
		return nil
	}

	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.Memory != nil && !r.Memory.IsZero() {
		q = r.Memory
	} else {
		return nil
	}
	return q
}

func minQuantity(val *resource.Quantity, limit *resource.Quantity) *resource.Quantity {
	if val == nil || val.Value() < 0 {
		return limit
	}
	if limit == nil || limit.Value() < 0 {
		return val
	}
	if val.Cmp(*limit) < 0 {
		return val
	} else {
		return limit
	}
}
