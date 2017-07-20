package mlapp

type Resource struct {
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
}
