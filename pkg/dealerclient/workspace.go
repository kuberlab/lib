package dealerclient

import (
	"fmt"

	"github.com/kuberlab/lib/pkg/errors"
)

type Workspace struct {
	Name        string
	DisplayName string
	Type        string
	Can         []string
}

func (c *Client) GetWorkspace(workspace string) (*Workspace, error) {
	u := fmt.Sprintf("/workspace/%v", workspace)

	var ws = &Workspace{}
	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	_, err = c.Do(req, ws)

	if err != nil {
		return nil, err
	}
	return ws, nil
}

func (c *Client) GetWorkspaceLimit(workspace string) (*ResourceLimit, error) {
	if c.auth.WorkspaceSecret == "" {
		return nil, errors.New("Workspace secret auth required.")
	}
	u := fmt.Sprintf("/secret/%v/limits", c.auth.WorkspaceSecret)

	var limit = &ResourceLimit{}
	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	_, err = c.Do(req, limit)

	if err != nil {
		return nil, err
	}
	return limit, nil
}
