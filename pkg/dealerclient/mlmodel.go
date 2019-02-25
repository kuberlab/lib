package dealerclient

import (
	"fmt"
)

type Model struct {
	DisplayName   string
	Name          string
	Published     bool
	WorkspaceName string
}

func (c *Client) DeleteModel(workspace, name string) error {
	u := fmt.Sprintf("/workspace/%v/mlmodel/%v", workspace, name)

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

func (c *Client) CreateModel(workspace, name string, public bool, skipPluke bool) error {
	u := fmt.Sprintf("/workspace/%v/mlmodel", workspace)

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

func (c *Client) CheckModel(workspace, name string) error {
	u := fmt.Sprintf("/workspace/%v/mlmodel-check", workspace)

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

func (c *Client) CreateSpec(workspace, name string, spec interface{}) error {
	u := fmt.Sprintf("/workspace/%v/mlmodel/%v/spec", workspace, name)

	req, err := c.NewRequest("PUT", u, spec)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CreateSpecForVersion(workspace, name, version string, spec interface{}) error {
	u := fmt.Sprintf("/workspace/%v/mlmodel/%v/versions/%v/spec", workspace, name, version)

	req, err := c.NewRequest("PUT", u, spec)
	if err != nil {
		return err
	}
	_, err = c.Do(req, nil)

	if err != nil {
		return err
	}
	return nil
}

func (c *Client) ListModels(workspace string) ([]Dataset, error) {
	u := fmt.Sprintf("/workspace/%v/mlmodel?all=true", workspace)

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

func (c *Client) GetModel(workspace string, name string) (*Dataset, error) {
	u := fmt.Sprintf("/workspace/%v/mlmodel/%v", workspace, name)

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
