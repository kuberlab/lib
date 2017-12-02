package mlapp

import "k8s.io/apimachinery/pkg/api/resource"

type ResourceRequest struct {
	Accelerators ResourceAccelerators `json:"accelerators"`
	Requests     ResourceReqLim       `json:"requests"`
	Limits       ResourceReqLim       `json:"limits"`
}

type ResourceAccelerators struct {
	GPU uint `json:"gpu"`
}

type ResourceReqLim struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	GPU    uint   `json:"gpu"`
}

func ResourceSpec(r *ResourceRequest, limitVal *ResourceReqLim, defaultReq ResourceReqLim) ResourceRequest {
	if r == nil {
		r = &ResourceRequest{}
	}
	var gpuLimitCluster *resource.Quantity
	if limitVal == nil {
		limitVal = &ResourceReqLim{}
		//no gpu limit
		gpuLimitCluster = nil
	} else {
		//gpu limit from global
		gpuLimitCluster = quantityUint(limitVal.GPU)
	}
	cpuRequest := quantityString(r.Requests.CPU)
	cpuDefault := quantityString(defaultReq.CPU)
	cpuLimit := quantityString(r.Limits.CPU)
	cpuLimitCluster := quantityString(limitVal.CPU)
	cpu1, cpu2 := setQuantity(cpuRequest, cpuDefault, cpuLimit, cpuLimitCluster)

	memoryRequest := quantityString(r.Requests.Memory)
	memoryDefault := quantityString(defaultReq.Memory)
	memoryLimit := quantityString(r.Limits.Memory)
	memoryLimitCluster := quantityString(limitVal.Memory)
	memory1, memory2 := setQuantity(memoryRequest, memoryDefault, memoryLimit, memoryLimitCluster)

	gpuRequest := quantityUint(r.Accelerators.GPU)
	gpuDefault := quantityUint(0)
	gpuLimit := quantityUint(0)
	gpu1, _ := setQuantity(gpuRequest, gpuDefault, gpuLimit, gpuLimitCluster)

	return ResourceRequest{
		Accelerators: ResourceAccelerators{
			GPU: quantity2Uint(gpu1),
		},
		Limits: ResourceReqLim{
			CPU:    quantity2String(cpu2),
			Memory: quantity2String(memory2),
		},
		Requests: ResourceReqLim{
			CPU:    quantity2String(cpu1),
			Memory: quantity2String(memory1),
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
