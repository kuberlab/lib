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
