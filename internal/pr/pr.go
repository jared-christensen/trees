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

// Detect returns the provider for the remote's host when enabled: GitHub when
// gh is on PATH, Bitbucket when BITBUCKET_USERNAME and BITBUCKET_TOKEN are
// set. Otherwise None.
func Detect(remoteURL string, enabled bool) Provider {
	if !enabled {
		return None{}
	}
	if repo, ok := parseGitHubRemote(remoteURL); ok {
		path, err := lookPath("gh")
		if err != nil {
			return None{}
		}
		return newGitHub(path, repo)
	}
	if repo, ok := parseBitbucketRemote(remoteURL); ok {
		user, token, ok := bitbucketCredentials()
		if !ok {
			return None{}
		}
		return newBitbucket(repo, user, token)
	}
	return None{}
}
