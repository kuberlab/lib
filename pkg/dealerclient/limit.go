package dealerclient

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceLimit struct {
	CPU           *resource.Quantity `json:"cpu,omitempty"`
	Memory        *resource.Quantity `json:"memory,omitempty"`
	GPU           int64              `json:"gpu"`
	CPUMi         int64              `json:"cpu_mi"`
	MemoryMB      int64              `json:"memory_mb"`
	Replicas      int64              `json:"replicas"`
	ParallelRuns  int64              `json:"parallel_runs"`
	ExecutionTime int64              `json:"execution_time"`
}

func (r *ResourceLimit) MinimizeTo(limit ResourceLimit) {
	minCPU := minQuantity(r.CPUQuantity(), limit.CPUQuantity())
	minMemory := minQuantity(r.MemoryQuantity(), limit.MemoryQuantity())
	if r.GPU > limit.GPU {
		r.GPU = limit.GPU
	}
	if r.Replicas > limit.Replicas && limit.Replicas != 0 || r.Replicas == 0 {
		r.Replicas = limit.Replicas
	}
	if r.ExecutionTime > limit.ExecutionTime && limit.Replicas != 0 || r.ExecutionTime == 0 {
		r.ExecutionTime = limit.ExecutionTime
	}
	r.Memory = nil
	r.CPU = nil
	if minCPU != nil {
		r.CPUMi = minCPU.MilliValue()
	} else {
		r.CPUMi = 0
	}
	if minMemory != nil {
		r.MemoryMB = minMemory.ScaledValue(resource.Mega)
	} else {
		r.MemoryMB = 0
	}
}

func (r *ResourceLimit) CPUQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}

	if (r.CPU != nil && r.CPU.MilliValue() <= 0) || r.CPUMi < 0 {
		return nil
	}

	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.CPUMi != 0 {
		q.SetMilli(r.CPUMi)
	} else if r.CPU != nil && !r.CPU.IsZero() {
		q = r.CPU
		r.CPUMi = q.MilliValue()
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
	if r.GPU < 0 {
		// No limit
		return nil
	}
	q := &resource.Quantity{Format: resource.DecimalSI}
	q.Set(r.GPU)
	return q
}

func (r *ResourceLimit) MemoryQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}

	if r.MemoryMB < 0 || (r.Memory != nil && r.Memory.Value() <= 0) {
		return nil
	}

	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.MemoryMB != 0 {
		q.SetScaled(r.MemoryMB, resource.Mega)
	} else if r.Memory != nil && !r.Memory.IsZero() {
		q = r.Memory
		r.MemoryMB = q.ScaledValue(resource.Mega)
	} else {
		return nil
	}
	return q
}

func minQuantity(val *resource.Quantity, limit *resource.Quantity) *resource.Quantity {
	if val == nil {
		return limit
	}
	if limit == nil {
		return val
	}
	if val.Cmp(*limit) < 0 {
		return val
	} else {
		return limit
	}
}
