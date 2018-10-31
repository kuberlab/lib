package apputil

import (
	"testing"
)

func TestParseGitURL(t *testing.T) {
	var tests = map[string]GitInfo{
		"https://github.com/org/repo":                     {"https://github.com/org/repo", "repo", ""},
		"https://github.com/org/repo/sub/dir":             {"https://github.com/org/repo", "repo/sub/dir", ""},
		"https://github.com/org/repo/tree/master/sub/dir": {"https://github.com/org/repo", "repo/sub/dir", ""},
		"https://github.com/org/repo/tree/rev/sub/dir":    {"https://github.com/org/repo", "repo/sub/dir", "rev"},
		"https://bitbucket.org/org/repo/sub/dir":          {"https://bitbucket.org/org/repo", "repo/sub/dir", ""},
		"https://bitbucket.org/org/repo/src/rev/sub/dir":  {"https://bitbucket.org/org/repo", "repo/sub/dir", "rev"},
		"git@github.com:org/repo.git":                     {"git@github.com:org/repo.git", "repo", ""},
		"git@github.com:org/repo.git/sub/dir":             {"git@github.com:org/repo.git", "repo/sub/dir", ""},
	}
	for r, i := range tests {
		if p := ParseGitURL(r); p != i {
			t.Errorf("Parse url '%s' error: expected %v, got %v", r, i, p)
		}
	}
}
