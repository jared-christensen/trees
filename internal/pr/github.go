package pr

import (
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
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
	remote = strings.TrimSpace(remote)
	var path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() != "github.com" {
			return "", false
		}
		path = u.Path
	case strings.HasPrefix(remote, "git@github.com:"):
		path = strings.TrimPrefix(remote, "git@github.com:")
	default:
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return path, true
}
