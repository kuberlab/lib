package dealerclient

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceLimit struct {
	// Deprecated: use CPUMi instead.
	CPU *resource.Quantity `json:"cpu,omitempty"`
	// Deprecated: use MemoryMB instead.
	Memory        *resource.Quantity `json:"memory,omitempty"`
	GPU           int64              `json:"gpu,omitempty"`
	CPUMi         int64              `json:"cpu_mi,omitempty"`
	MemoryMB      int64              `json:"memory_mb,omitempty"`
	Replicas      int64              `json:"replicas,omitempty"`
	ParallelRuns  int64              `json:"parallel_runs,omitempty"`
	ExecutionTime int64              `json:"execution_time,omitempty"`
}

func (r *ResourceLimit) CPUQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}

	if r.CPU.MilliValue() <= 0 || r.CPUMi <= 0 {
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

	if r.MemoryMB <= 0 || r.Memory.Value() <= 0 {
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
