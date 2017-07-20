package mlapp

type MlApp struct {
	Metadata MlAppMeta
	Uix      []MlAppUix
}

type MlAppMeta struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Tasks     []MlAppTask
}

type MlAppUix struct {
	Name        string
	VisibleName string
	Resources   Resource
}

type MlAppTask struct {
	Name      string
	Labels    map[string]string
	Resources []MlAppResource
}

type MlAppResource struct {
	Name            string
	Labels          map[string]string
	Replicas        uint
	MinAvailable    uint
	MaxRestartCount uint
	Command         string
	WorkDir         string
	Args            string
	Resources       []Resource
}
