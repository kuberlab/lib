package mlapp

type Resource struct {
	Accelerators ResourceAccelerators
	Requests     ResourceRequests
	Limits       ResourceRequests
}

type ResourceAccelerators struct {
	GPU uint
}

type ResourceRequests struct {
	CPU    string
	Memory string
}
