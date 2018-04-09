package mlapp

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceRequest struct {
	Accelerators ResourceAccelerators `json:"accelerators"`
	Requests     *ResourceLimit       `json:"requests"`
	Limits       *ResourceLimit       `json:"limits"`
}

type ResourceAccelerators struct {
	GPU uint `json:"gpu"`
}

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

	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.CPU != nil && !r.CPU.IsZero() {
		q = r.CPU
		r.CPUMi = q.MilliValue()
	} else if r.CPUMi != 0 {
		q.SetMilli(r.CPUMi)
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
	q := &resource.Quantity{Format: resource.DecimalSI}
	q.Set(r.GPU)
	return q
}

func (r *ResourceLimit) MemoryQuantity() *resource.Quantity {
	// In templates
	if r == nil {
		return nil
	}
	q := &resource.Quantity{Format: resource.DecimalSI}
	if r.Memory != nil && !r.Memory.IsZero() {
		q = r.Memory
		r.MemoryMB = q.ScaledValue(resource.Mega)
	} else if r.MemoryMB != 0 {
		q.SetScaled(r.MemoryMB, resource.Mega)
	} else {
		return nil
	}
	return q
}

func ResourceSpec(r *ResourceRequest, limitVal *ResourceLimit, defaultReq ResourceLimit) ResourceRequest {
	if r == nil {
		r = &ResourceRequest{}
	}
	var gpuLimitCluster *resource.Quantity
	if limitVal == nil {
		limitVal = &ResourceLimit{}
		//no gpu limit
		gpuLimitCluster = nil
	} else {
		//gpu limit from global
		gpuLimitCluster = limitVal.GPUQuantity()
	}
	cpuRequest := r.Requests.CPUQuantity()
	cpuDefault := defaultReq.CPUQuantity()
	cpuLimit := r.Limits.CPUQuantity()
	cpuLimitCluster := limitVal.CPUQuantity()
	cpu1, cpu2 := setQuantity(cpuRequest, cpuDefault, cpuLimit, cpuLimitCluster)

	memoryRequest := r.Requests.MemoryQuantity()
	memoryDefault := defaultReq.MemoryQuantity()
	memoryLimit := r.Limits.MemoryQuantity()
	memoryLimitCluster := limitVal.MemoryQuantity()
	memory1, memory2 := setQuantity(memoryRequest, memoryDefault, memoryLimit, memoryLimitCluster)

	gpuRequest := quantityUint(r.Accelerators.GPU)
	gpuDefault := quantityUint(0)
	gpuLimit := quantityUint(0)
	gpu1, _ := setQuantity(gpuRequest, gpuDefault, gpuLimit, gpuLimitCluster)

	return ResourceRequest{
		Accelerators: ResourceAccelerators{
			GPU: quantity2Uint(gpu1),
		},
		Limits: &ResourceLimit{
			CPUMi:    cpu2.MilliValue(),
			MemoryMB: memory2.ScaledValue(resource.Mega),
		},
		Requests: &ResourceLimit{
			CPUMi:    cpu1.MilliValue(),
			MemoryMB: memory1.ScaledValue(resource.Mega),
		},
	}
}

func setQuantity(req *resource.Quantity, defaultReq *resource.Quantity, limit *resource.Quantity, clusterLimit *resource.Quantity) (*resource.Quantity, *resource.Quantity) {
	limit = minQuantity(limit, clusterLimit)
	if limit != nil && req == nil {
		return limitQuantity(defaultReq, limit), limit
	}
	return limitQuantity(req, limit), limit
}
func limitQuantity(val *resource.Quantity, limit *resource.Quantity) *resource.Quantity {
	if val == nil {
		return nil
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

func quantity2Uint(v *resource.Quantity) uint {
	if v == nil {
		return 0
	}
	i, _ := v.AsInt64()
	return uint(i)
}
func quantity2String(v *resource.Quantity) string {
	if v == nil {
		return ""
	}
	return v.String()
}
func quantityUint(v uint) *resource.Quantity {
	if v == 0 {
		return nil
	}
	return resource.NewQuantity(int64(v), resource.DecimalSI)
}
func quantityString(v string) *resource.Quantity {
	if v == "" {
		return nil
	}
	q, err := resource.ParseQuantity(v)
	if err != nil {
		return nil
	}
	return &q
}
