package mlapp

type MlApp struct {
	Metadata MlAppMeta  `json:"metadata"`
	Uix      []MlAppUix `json:"uix,omitempty"`
}

type MlAppMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Tasks     []MlAppTask       `json:"tasks,omitempty"`
}

type MlAppUix struct {
	Name        string        `json:"name"`
	VisibleName string        `json:"visibleName,omitempty"`
	Resources   MlAppResource `json:"resources,omitempty"`
	Ports       []MlAppPort   `json:"ports,omitempty"`
}

type MlAppPort struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol,omitempty"`
	Port       uint   `json:"port,omitempty"`
	TargetPort uint   `targetPort:"name,omitempty"`
}

type MlAppTask struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Resources MlAppResource     `json:"resources"`
}

type MlAppResource struct {
	Name            string            `json:"name"`
	Labels          map[string]string `json:"labels,omitempty"`
	Replicas        uint              `json:"replicas"`
	MinAvailable    uint              `json:"minAvailable"`
	RestartPolicy   string            `json:"restartPolicy"`
	MaxRestartCount uint              `json:"maxRestartCount"`
	Images          MlAppImages       `json:"images"`
	Command         string            `json:"command"`
	WorkDir         string            `json:"workDir"`
	Args            string            `json:"args,omitempty"`
	Env             []MLAppEnv        `json:"env"`
	Resources       MlAppResource     `json:"resources"`
}

type MlAppImages struct {
	CPU string `json:"cpu,omitempty"`
	GPU string `json:"gpu,omitempty"`
}

type MLAppEnv struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type MlAppResource struct {
	Accelerators MlAppResourceAccelerators `json:"accelerators"`
	Requests     MlAppResourceReqLim       `json:"requests"`
	Limits       MlAppResourceReqLim       `json:"limits"`
}

type MlAppResourceAccelerators struct {
	GPU uint `json:"gpu"`
}

type MlAppResourceReqLim struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}
