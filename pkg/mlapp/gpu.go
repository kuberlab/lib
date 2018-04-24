package mlapp

type ComponentRequests interface {
	GPURequests() int64
	Type() string
}
