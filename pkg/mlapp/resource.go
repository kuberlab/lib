package mlapp

import (
	"github.com/kuberlab/lib/pkg/dealerclient"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceRequest struct {
	Accelerators ResourceAccelerators        `json:"accelerators"`
	Requests     *dealerclient.ResourceLimit `json:"requests,omitempty"`
	Limits       *dealerclient.ResourceLimit `json:"limits,omitempty"`
}

type ResourceAccelerators struct {
	GPU uint `json:"gpu"`
}

func ResourceSpec(r *ResourceRequest, limitVal *dealerclient.ResourceLimit, defaultReq dealerclient.ResourceLimit) ResourceRequest {

	if r == nil {
		r = &ResourceRequest{}
	}
	var gpuLimitCluster *resource.Quantity
	if limitVal == nil {
		limitVal = &dealerclient.ResourceLimit{}
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
		Limits: &dealerclient.ResourceLimit{
			CPU:    cpu2,
			Memory: memory2,
		},
		Requests: &dealerclient.ResourceLimit{
			CPU:    cpu1,
			Memory: memory1,
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
