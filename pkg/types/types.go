package types

type ClusterStats struct {
	TaskCount      uint `json:"task_count"`
	ContainerCount uint `json:"container_count"`
	GPUUsed        uint `json:"gpu_used"`
	GPUCapacity    uint `json:"gpu_capacity"`
}

type GPU struct {
	Capacity  uint          `json:"capacity"`
	Used      uint          `json:"used"`
	Consumers []GPUConsumer `json:"consumers"`
}

type GPUConsumer struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Workspace   string `json:"workspace"`
	WorkspaceID string `json:"workspace_id"`
}
