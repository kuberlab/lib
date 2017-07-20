package mlapp

type Config struct {
	Kind string `json:"kind"`
	Meta `json:"metadata"`
	Spec `json:"spec,omitempty"`
}

type Meta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type Spec struct {
	Tasks   []Task   `json:"tasks,omitempty"`
	Uix     []Uix    `json:"uix,omitempty"`
	Volumes []Volume `json:"volumes"`
}

type Uix struct {
	Meta        `json:",inline"`
	DisplayName string          `json:"displayName,omitempty"`
	Resources   ResourceRequest `json:"resources,omitempty"`
	Ports       []Port          `json:"ports,omitempty"`
	Volumes     []VolumeMount   `json:"volumes"`
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       uint   `json:"port,omitempty"`
	TargetPort uint   `targetPort:"name,omitempty"`
}

type Task struct {
	Meta      `json:",inline"`
	Resources []Resource `json:"resources"`
}

type Resource struct {
	Meta            `json:",inline"`
	Replicas        uint            `json:"replicas"`
	MinAvailable    uint            `json:"minAvailable"`
	RestartPolicy   string          `json:"restartPolicy"`
	MaxRestartCount uint            `json:"maxRestartCount"`
	Images          Images          `json:"images"`
	Command         string          `json:"command"`
	WorkDir         string          `json:"workDir"`
	Args            string          `json:"args,omitempty"`
	Env             []Env           `json:"env"`
	Resources       ResourceRequest `json:"resources"`
	Volumes         []VolumeMount   `json:"volumes"`
}

type Images struct {
	CPU string `json:"cpu,omitempty"`
	GPU string `json:"gpu,omitempty"`
}

type Env struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}
