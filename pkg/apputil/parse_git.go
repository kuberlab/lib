package apputil

import (
	"net/url"
	"strings"
)

type GitInfo struct {
	URL      string
	SubPath  string
	Revision string
}

var knownGitHosts = map[string]string{
	"github.com":    "tree",
	"bitbucket.org": "src",
}

func ParseGitURL(v interface{}) (g GitInfo) {
	switch t := v.(type) {
	case string:
		g.URL = t
		var port_is_owner bool
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
		var h = v.Hostname()
		var known bool
		var knownSuff string
		if !port_is_owner {
			if p3, ok := knownGitHosts[h]; ok && len(p) > 4 && p3 == p[3] {
				if p[4] != "master" {
					g.Revision = p[4]
				}
				known = true
				knownSuff = "/" + p[3] + "/" + p[4]
			}
		}
		g.SubPath = strings.TrimSuffix(p[2], ".git")
		var dirParts []string
		if !known && len(p) > 4 {
			dirParts = append(dirParts, p[3], p[4])
		}
		if len(p) > 4 {
			dirParts = append(dirParts, p[5:]...)
		}
		if len(dirParts) > 0 {
			var dir = "/" + strings.Join(dirParts, "/")
			g.SubPath = g.SubPath + dir
			g.URL = strings.TrimSuffix(g.URL, dir)
			g.URL = strings.TrimSuffix(g.URL, knownSuff)
		}
	}
	return
}
