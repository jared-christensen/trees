// Package pr looks up pull request status for branches. It is an optional
// extra: every failure is silent and yields an empty result.
package pr

import (
	"context"
	"time"
)

// PR is a pull request as reported by the hosting provider.
type PR struct {
	Number    int
	State     string // OPEN, MERGED or CLOSED
	HeadSHA   string
	UpdatedAt time.Time
}

// Provider looks up pull requests for branch names. The result is keyed by
// branch name, newest PR first. It never returns nil.
type Provider interface {
	Lookup(ctx context.Context, branches []string) map[string][]PR
}

// None is the provider used when PR status is unavailable or disabled.
type None struct{}

// Lookup always returns an empty map.
func (None) Lookup(context.Context, []string) map[string][]PR { return map[string][]PR{} }

// Detect returns a GitHub provider when enabled, the remote is on github.com
// and gh is on PATH; otherwise None.
func Detect(remoteURL string, enabled bool) Provider {
	if !enabled {
		return None{}
	}
	repo, ok := parseGitHubRemote(remoteURL)
	if !ok {
		return None{}
	}
	path, err := lookPath("gh")
	if err != nil {
		return None{}
	}
	return newGitHub(path, repo)
}
