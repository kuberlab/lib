package dealerclient

import (
	"fmt"
	"net/http"

	"github.com/kuberlab/lib/pkg/errors"
)

type Dataset struct {
	DisplayName   string
	Name          string
	Published     bool
	WorkspaceName string
}

type NewVersion struct {
	Version   string `json:"version"`
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Latest    bool   `json:"latest"`
}

func (c *Client) DeleteDataset(workspace, name string) error {
	u := fmt.Sprintf("/workspace/%v/dataset/%v?force=true", workspace, name)

	req, err := c.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CreateDataset(workspace, name string, public bool, skipPluke bool) error {
	u := fmt.Sprintf("/workspace/%v/dataset", workspace)

	if skipPluke {
		u = fmt.Sprintf("%v?skip_pluk=true", u)
	}

	ds := &Dataset{
		Name:          name,
		WorkspaceName: workspace,
		Published:     public,
		DisplayName:   name,
	}
	req, err := c.NewRequest("POST", u, ds)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CheckDataset(workspace, name string) error {
	u := fmt.Sprintf("/workspace/%v/dataset-check", workspace)

	ds := &Dataset{
		Name:          name,
		WorkspaceName: workspace,
		Published:     false,
		DisplayName:   name,
	}
	req, err := c.NewRequest("POST", u, ds)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	if err != nil {
		return err
	}
	return nil
}

func (c *Client) ReportNewVersion(version NewVersion) error {
	if version.Workspace == "" {
		return errors.NewStatus(http.StatusBadRequest, "'workspace' field required.")
	}
	u := fmt.Sprintf("/workspace/%v/pluke/new-version", version.Workspace)

	req, err := c.NewRequest("POST", u, version)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	return err
}

func (c *Client) ListDatasets(workspace string) ([]Dataset, error) {
	u := fmt.Sprintf("/workspace/%v/dataset?all=true", workspace)

	var ds = make([]Dataset, 0)
	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	_, err = c.Do(req, &ds)

	if err != nil {
		return nil, err
	}
	return ds, nil
}

func (c *Client) GetDataset(workspace string, name string) (*Dataset, error) {
	u := fmt.Sprintf("/workspace/%v/dataset/%v", workspace, name)

	var ds = &Dataset{}
	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	_, err = c.Do(req, ds)

	if err != nil {
		return nil, err
	}
	return ds, nil
}
