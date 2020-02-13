package mlapp

type ComponentRequests interface {
	GPURequests() int64
	DisableGPU(num int) int
	Type() string
}
