package dealerclient

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/json-iterator/go"
	"github.com/kuberlab/lib/pkg/errors"
	"net"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Client struct {
	Client    *http.Client
	BaseURL   *url.URL
	UserAgent string

	auth *AuthOpts
}

type AuthOpts struct {
	Token           string
	Cookie          string
	Headers         http.Header
	Workspace       string
	WorkspaceSecret string
	Insecure        bool
}

type Dataset struct {
	DisplayName   string
	Name          string
	Published     bool
	WorkspaceName string
}

type Workspace struct {
	Name        string
	DisplayName string
	Type        string
	Can         []string
}

type NewVersion struct {
	Version   string `json:"version"`
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"`
	Latest    bool   `json:"latest"`
}

func NewClient(baseURL string, auth *AuthOpts) (*Client, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	if auth.Headers != nil {
		hd := make(http.Header)
		for k, v := range auth.Headers {
			if k == "Authorization" || k == "Cookie" || k == "X-Workspace-Name" || k == "X-Workspace-Secret" {
				hd[k] = v
			}
		}
		auth.Headers = hd
	}
	// Clone default transport
	var transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if base.Scheme == "https" && auth.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: auth.Insecure}
	}

	base.Path = "/api/v0.2"
	baseClient := &http.Client{Timeout: time.Minute * 10, Transport: transport}
	return &Client{
		BaseURL:   base,
		Client:    baseClient,
		UserAgent: "go-dealerclient/1",
		auth:      auth,
	}, nil
}

func (c *Client) sanitizeURL(urlStr string) string {
	secret := c.auth.Headers.Get("X-Workspace-Secret")
	if secret == "" {
		secret = c.auth.WorkspaceSecret
		if secret == "" {
			return urlStr
		}
	}

	return strings.Replace(
		urlStr,
		fmt.Sprintf("secret/%v", secret),
		"secret/[sanitized]",
		-1,
	)
}

func (c *Client) getUrl(urlStr string) string {
	workspace := c.auth.Headers.Get("X-Workspace-Name")
	secret := c.auth.Headers.Get("X-Workspace-Secret")

	if workspace == "" && secret == "" {
		return urlStr
	}

	splitted := strings.Split(urlStr, "/")
	if len(splitted) < 3 {
		return urlStr
	}
	workspaceInURL := splitted[2]
	if workspace != workspaceInURL {
		return urlStr
	}
	return strings.Replace(
		urlStr,
		fmt.Sprintf("workspace/%v", workspace), fmt.Sprintf("secret/%v", secret), -1,
	)
}

func (c *Client) NewRequest(method, urlStr string, body interface{}) (*http.Request, error) {
	u := c.BaseURL.String()
	u = strings.TrimSuffix(u, "/") + c.getUrl(urlStr)

	var buf io.ReadWriter
	var reqBody io.Reader
	if body != nil {
		rd, ok := body.(io.Reader)
		if ok {
			// plain io.Reader
			reqBody = rd
		} else {
			// As JSON
			buf = new(bytes.Buffer)
			err := json.NewEncoder(buf).Encode(body)
			if err != nil {
				return nil, err
			}
			reqBody = buf
		}
	}

	req, err := http.NewRequest(method, u, reqBody)
	if err != nil {
		return nil, err
	}
	if c.auth != nil {
		if c.auth.Headers != nil {
			req.Header = c.auth.Headers
		}
		if c.auth.Cookie != "" {
			req.Header.Set("Cookie", c.auth.Cookie)
		}
		if c.auth.Token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", c.auth.Token))
		}
		if c.auth.Workspace != "" && c.auth.WorkspaceSecret != "" {
			req.Header.Set("X-Workspace-Name", c.auth.Workspace)
			req.Header.Set("X-Workspace-Secret", c.auth.WorkspaceSecret)
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	return req, nil
}

// Do sends an API request and returns the API response.  The API response is
// JSON decoded and stored in the value pointed to by v, or returned as an
// error if an API error has occurred.  If v implements the io.Writer
// interface, the raw response body will be written to v, without attempting to
// first decode it.
func (c *Client) Do(req *http.Request, v interface{}) (*http.Response, error) {
	logrus.Debugf("[go-dealerclient] %v %v", req.Method, c.sanitizeURL(req.URL.String()))
	resp, err := c.Client.Do(req)
	if err != nil {
		if e, ok := err.(*url.Error); ok {
			return nil, errors.New(c.sanitizeURL(e.Error()))
		}
		return nil, errors.New(c.sanitizeURL(err.Error()))
	}

	defer func() {
		// Drain up to 512 bytes and close the body to let the Transport reuse the connection
		_, _ = io.CopyN(ioutil.Discard, resp.Body, 512)
		_ = resp.Body.Close()
	}()

	if resp, err = checkResponse(resp, err); err != nil {
		return resp, err
	}
	if v != nil {
		if w, ok := v.(io.Writer); ok {
			_, _ = io.Copy(w, resp.Body)
		} else {
			err = json.NewDecoder(resp.Body).Decode(v)
			if err == io.EOF {
				err = nil // ignore EOF errors caused by empty response body
			}
		}
	}

	return resp, err
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

func (c *Client) DeleteDataset(workspace, name string) error {
	u := fmt.Sprintf("/workspace/%v/dataset/%v", workspace, name)

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

type DealerError struct {
	Status     string
	Error      string
	Reason     string
	StatusCode int
}

func checkResponse(resp *http.Response, err error) (*http.Response, error) {
	if err != nil || resp.StatusCode >= 400 {
		if err != nil {
			return &http.Response{StatusCode: http.StatusInternalServerError}, err
		} else {
			messageBytes, _ := ioutil.ReadAll(resp.Body)
			// Try use dealerError
			e := &DealerError{}
			err = json.Unmarshal(messageBytes, e)
			if err != nil {
				message := strconv.Itoa(resp.StatusCode) + ": " + string(messageBytes)
				return resp, errors.NewStatus(resp.StatusCode, message)
			} else {
				return resp, errors.NewStatusReason(e.StatusCode, e.Error, e.Reason)
			}
		}
	}
	return resp, nil
}
