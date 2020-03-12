package mlapp

type ServingType string

const (
	ServingTypeTask      ServingType = "task"
	ServingTypeModel     ServingType = "model"
	ServingTypeInference ServingType = "inference"
)

type UniversalServing struct {
	// common
	Uix       `json:",inline"`
	Type      ServingType      `json:"type,omitempty"`
	Spec      ServingSpec      `json:"spec,omitempty"`
	ModelSpec ServingModelSpec `json:"model_spec,omitempty"`

	// task serving
	TaskName  string                 `json:"taskName,omitempty"`
	Build     string                 `json:"build,omitempty"`
	BuildInfo map[string]interface{} `json:"build_info,omitempty"`

	// model serving
	Sources     []Volume `json:"sources,omitempty"`
	ModelID     string   `json:"model_id,omitempty"`
	Model       string   `json:"model,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
}

type UniversalServingPrivate struct {
	UniversalServing

	// additional private info (for calls to ml-board, it should not be on UI)
	VolumesData     []Volume `json:"volumes_data,omitempty"`
	Secrets         []Secret `json:"secrets,omitempty"`
	WorkspaceSecret string   `json:"workspace_secret,omitempty"`
	DealerAPI       string   `json:"dealer_api,omitempty"`
}
