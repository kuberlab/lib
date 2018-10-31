package apputil

import (
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

func gitRepo(v interface{}) string {
	return ParseGitURL(v).URL
}
func gitSubPath(v interface{}) string {
	return ParseGitURL(v).SubPath
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
