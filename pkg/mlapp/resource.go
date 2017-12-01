package mlapp

import "k8s.io/apimachinery/pkg/api/resource"
type ResourceRequest struct {
	Accelerators ResourceAccelerators `json:"accelerators"`
	Requests     ResourceReqLim       `json:"requests"`
	Limits       ResourceReqLim       `json:"limits"`
}

type ResourceAccelerators struct {
	GPU uint `json:"gpu,omitempty"`
}

func ResourceSpec(r ResourceRequest,limitVal ResourceReqLim,defaultReq ResourceReqLim) ResourceRequest{
	cpuRequest := quantityString(r.Requests.CPU)
	cpuDefault := quantityString(defaultReq.CPU)
	cpuLimit := quantityString(r.Limits.CPU)
	cpuLimitCluster := quantityString(limitVal.CPU)
	cpu1,cpu2 := setQuantity(cpuRequest,cpuDefault,cpuLimit,cpuLimitCluster)

	memoryRequest := quantityString(r.Requests.Memory)
	memoryDefault := quantityString(defaultReq.Memory)
	memoryLimit := quantityString(r.Limits.Memory)
	memoryLimitCluster := quantityString(limitVal.Memory)
	memory1,memory2 := setQuantity(memoryRequest,memoryDefault,memoryLimit,memoryLimitCluster)

}
func (r ResourceRequest) limit(limitVal *ResourceReqLim,defaultReq ResourceReqLim) *ResourceRequest{
	var limits *ResourceReqLim
	var requests *ResourceReqLim
	if r.Limits!=nil{
		limits = r.Limits.limit(limitVal)
	} else if limitVal!=nil{
		limits = &ResourceReqLim{
			CPU: limitVal.CPU,
			Memory: limitVal.Memory,
		}
	}

	if limits!=nil{
		if r.Requests==nil{
			requests = &defaultReq
		} else{
			requests = &ResourceReqLim{
				CPU: r.Requests.CPU,
				Memory: r.Requests.Memory,
			}
			if requests.CPUQuantity()==nil{
				requests.CPU = defaultReq.CPU
			}
			if requests.MemoryQuantity()==nil{
				requests.Memory = defaultReq.Memory
			}
		}
	} else {
		requests = r.Requests
	}
	var gpu *uint
	if r.Accelerators!=nil && r.Accelerators.GPU!=nil{
		gpu = r.Accelerators.GPU
	}
	if gpu!=nil && limitVal!=nil && limitVal.GPU!=nil && (*limitVal.GPU)<(*gpu){
		gpu = limitVal.GPU
	}
	if gpu!=nil || isNotEmptyResource(limits) || isNotEmptyResource(requests){
		return &ResourceRequest{
			Accelerators: &ResourceAccelerators{
				GPU: gpu,
			},
			Limits: limits,
			Requests: requests,
		}
	}
	return nil
}
type ResourceReqLim struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	GPU    uint   `json:"gpu"`
}

func (r ResourceReqLim) CPUQuantity() *resource.Quantity{
	return quantityString(r.CPU)
}
func (r ResourceReqLim) MemoryQuantity() *resource.Quantity{
	return quantityString(r.Memory)
}
func isNotEmptyResource(val *ResourceReqLim) bool{
	if val==nil{
		return false
	}
	return val.CPU!=nil || val.Memory!=nil
}


func setQuantity(req *resource.Quantity,defaultReq *resource.Quantity,limit *resource.Quantity,clusterLimit *resource.Quantity) (*resource.Quantity,*resource.Quantity){
	limit = minQuantity(limit,clusterLimit)
	if limit!=nil && req==nil{
		return limitQuantity(defaultReq,limit),limit
	}
	return limitQuantity(req,limit),limit
}
func limitQuantity(val *resource.Quantity,limit *resource.Quantity) *resource.Quantity{
	if val==nil{
		return nil
	}
	if limit==nil{
		return val
	}
	if val.Cmp(*limit)<0{
		return val
	} else{
		return limit
	}
}
func minQuantity(val *resource.Quantity,limit *resource.Quantity) *resource.Quantity{
	if val==nil{
		return limit
	}
	if limit==nil{
		return val
	}
	if val.Cmp(*limit)<0{
		return val
	} else{
		return limit
	}
}

func quantityUint(v uint) *resource.Quantity{
	if v==0{
		return nil
	}
	return resource.NewQuantity(v,resource.DecimalSI)
}
func quantityString(v string) *resource.Quantity{
	if v==""{
		return nil
	}
	q,err := resource.ParseQuantity(v)
	if err!=nil{
		return nil
	}
	return &q
}