package mlapp

type Config struct {
	Kind     string `json:"kind"`
	Metadata Meta   `json:"metadata"`
}

type Meta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Tasks     []Task            `json:"tasks,omitempty"`
	Uix       []Uix             `json:"uix,omitempty"`
}

type Uix struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName,omitempty"`
	Resources   ResourceRequest `json:"resources,omitempty"`
	Ports       []Port          `json:"ports,omitempty"`
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       uint   `json:"port,omitempty"`
	TargetPort uint   `targetPort:"name,omitempty"`
}

type Task struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Resources []Resource        `json:"resources"`
	Volumes   []Volume          `json:"volumes"`
}

type Resource struct {
	Name            string            `json:"name"`
	Labels          map[string]string `json:"labels,omitempty"`
	Replicas        uint              `json:"replicas"`
	MinAvailable    uint              `json:"minAvailable"`
	RestartPolicy   string            `json:"restartPolicy"`
	MaxRestartCount uint              `json:"maxRestartCount"`
	Images          Images            `json:"images"`
	Command         string            `json:"command"`
	WorkDir         string            `json:"workDir"`
	Args            string            `json:"args,omitempty"`
	Env             []Env             `json:"env"`
	Resources       ResourceRequest   `json:"resources"`
}

type Images struct {
	CPU string `json:"cpu,omitempty"`
	GPU string `json:"gpu,omitempty"`
}

type Env struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}
