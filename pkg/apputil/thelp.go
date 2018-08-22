package apputil

import (
	"net/url"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/ghodss/yaml"
)

func ToYaml(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		// Swallow errors inside of a template.
		return ""
	}
	return string(data)
}

type GitInfo struct {
	URL     string
	SubPath string
}

func gitRepo(v interface{}) string {
	return gitInfo(v).URL
}
func gitSubPath(v interface{}) string {
	return gitInfo(v).SubPath
}
func gitInfo(v interface{}) (g GitInfo) {
	switch t := v.(type) {
	case string:
		g.URL = t
		port_is_owner := false
		if p := strings.Split(t, "@"); len(p) > 1 {
			t = "https://" + p[1]
			port_is_owner = true

		}
		v, err := url.Parse(t)
		if err != nil {
			return
		}
		path := v.Path
		if port_is_owner {
			if p := v.Port(); p != "" {
				path = "/" + p + path
			}
		}
		p := strings.Split(path, "/")
		if len(p) < 3 {
			return
		}
		repo := p[2]
		g.SubPath = strings.TrimSuffix(repo, ".git")
		if len(p) > 3 {
			dir := "/" + strings.Join(p[3:], "/")
			g.SubPath = g.SubPath + dir
			g.URL = strings.TrimSuffix(g.URL, dir)
		}
		return
	default:
		return
	}
}

func FuncMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")
	// Add some extra functionality
	extra := template.FuncMap{
		"toYaml":     ToYaml,
		"gitSubPath": gitSubPath,
		"gitRepo":    gitRepo,
	}
	for k, v := range extra {
		f[k] = v
	}
	return f
}
