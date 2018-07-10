package mlapp

type BaseServing struct {
	// common
	Uix  `json:",inline"`
	Spec ServingSpec `json:"spec"`

	// task serving
	TaskName  string                 `json:"taskName,omitempty"`
	Build     string                 `json:"build,omitempty"`
	BuildInfo map[string]interface{} `json:"build_info,omitempty"`

	// model serving
	Source          *GitRepoVolumeSource `json:"source,omitempty"`
	VolumesData     []Volume             `json:"volumes_data,omitempty"`
	Secrets         []Secret             `json:"secrets,omitempty"`
	DealerAPI       string               `json:"dealer_api,omitempty"`
	ModelID         string               `json:"model_id,omitempty"`
	Model           string               `json:"model,omitempty"`
	ModelURL        string               `json:"model_url,omitempty"`
	WorkspaceID     string               `json:"workspace_id,omitempty"`
	Workspace       string               `json:"workspace,omitempty"`
	WorkspaceSecret string               `json:"workspace_secret,omitempty"`
}
