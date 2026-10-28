package pr

import (
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"time"
)

var lookPath = exec.LookPath

const defaultTimeout = 5 * time.Second

// GitHub looks up pull requests through the gh CLI.
type GitHub struct {
	ghPath  string
	repo    string // owner/repo
	timeout time.Duration
}

func newGitHub(ghPath, repo string) *GitHub {
	return &GitHub{ghPath: ghPath, repo: repo, timeout: defaultTimeout}
}

// Lookup runs a single gh call and groups the results by head branch. When
// branches is non-empty only those branches are returned. Any failure,
// including timeout, yields an empty map.
func (g *GitHub) Lookup(ctx context.Context, branches []string) map[string][]PR {
	empty := map[string][]PR{}
	timeout := g.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, g.ghPath, "pr", "list",
		"--repo", g.repo, "--state", "all", "--limit", "300",
		"--json", "number,state,headRefName,headRefOid,updatedAt")
	// Don't wait on pipes held open by orphaned grandchildren after a kill.
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return empty
	}
	all, err := groupPRs(out)
	if err != nil {
		return empty
	}
	if len(branches) == 0 {
		return all
	}
	res := map[string][]PR{}
	for _, b := range branches {
		if prs, ok := all[b]; ok {
			res[b] = prs
		}
	}
	return res
}

type ghPR struct {
	Number      int       `json:"number"`
	State       string    `json:"state"`
	HeadRefName string    `json:"headRefName"`
	HeadRefOid  string    `json:"headRefOid"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func groupPRs(data []byte) (map[string][]PR, error) {
	var raw []ghPR
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	m := map[string][]PR{}
	for _, r := range raw {
		m[r.HeadRefName] = append(m[r.HeadRefName], PR{
			Number: r.Number, State: r.State, HeadSHA: r.HeadRefOid, UpdatedAt: r.UpdatedAt,
		})
	}
	for _, prs := range m {
		sort.SliceStable(prs, func(i, j int) bool { return prs[i].UpdatedAt.After(prs[j].UpdatedAt) })
	}
	return m, nil
}

// parseGitHubRemote returns "owner/repo" for github.com remotes in scp-like,
// https and ssh forms. Any other host (including GitHub Enterprise) is not
// recognised.
func parseGitHubRemote(remote string) (string, bool) {
	return parseRemote(remote, "github.com")
}

// parseRemote returns the "owner/repo" path of a remote whose host is one of
// hosts, in scp-like (git@host:o/r), https and ssh forms.
func parseRemote(remote string, hosts ...string) (string, bool) {
	remote = strings.TrimSpace(remote)
	var host, path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(remote, ":"):
		hostPart, rest, _ := strings.Cut(remote, ":")
		if _, h, ok := strings.Cut(hostPart, "@"); ok {
			hostPart = h
		}
		host, path = hostPart, rest
	default:
		return "", false
	}
	if !slices.Contains(hosts, host) {
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return path, true
}
