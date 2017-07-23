package mlapp

type ResourceRequest struct {
	Accelerators ResourceAccelerators `json:"accelerators"`
	Requests     ResourceReqLim       `json:"requests"`
	Limits       ResourceReqLim       `json:"limits"`
}

type ResourceAccelerators struct {
	GPU          uint `json:"gpu"`
	DedicatedGPU bool `json:"dedicated_gpu"`
}

type ResourceReqLim struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

func (req ResourceRequest) AsVars() map[string]interface{} {
	vars := make(map[string]interface{})
	if req.Accelerators.GPU != 0 {
		vars["GpuRequests"] = req.Accelerators.GPU
	}
	if req.Requests.Memory != "" {
		vars["MemoryRequests"] = req.Requests.Memory
	}
	if req.Limits.Memory != "" {
		vars["MemoryLimits"] = req.Requests.Memory
	}
	if req.Requests.CPU != "" {
		vars["CpuRequests"] = req.Requests.CPU
	}
	if req.Limits.CPU != "" {
		vars["CpuLimits"] = req.Requests.CPU
	}
	return vars
}
