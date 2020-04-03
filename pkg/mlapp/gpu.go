package mlapp

type ComponentRequests interface {
	GPURequests() int64
	MemoryMBLimits() map[string]int64
	CPUMiLimits() map[string]int64
	DisableGPU(num int) int
	Type() string
}
