package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// bitbucketChunk is how many branches go into one query, keeping URLs short.
const bitbucketChunk = 20

// bitbucketMaxPages bounds how many result pages one query follows.
const bitbucketMaxPages = 5

// Bitbucket looks up pull requests through the Bitbucket Cloud REST API,
// authenticating with an Atlassian API token or an app password.
type Bitbucket struct {
	baseURL string // https://api.bitbucket.org/2.0
	repo    string // workspace/repo
	user    string
	token   string
	timeout time.Duration
	client  *http.Client
}

// bitbucketCredentials reads BITBUCKET_USERNAME and BITBUCKET_TOKEN (or
// BITBUCKET_APP_PASSWORD). ok is false when either half is missing.
func bitbucketCredentials() (user, token string, ok bool) {
	user = os.Getenv("BITBUCKET_USERNAME")
	token = os.Getenv("BITBUCKET_TOKEN")
	if token == "" {
		token = os.Getenv("BITBUCKET_APP_PASSWORD")
	}
	return user, token, user != "" && token != ""
}

func newBitbucket(repo, user, token string) *Bitbucket {
	return &Bitbucket{
		baseURL: "https://api.bitbucket.org/2.0",
		repo:    repo, user: user, token: token,
		timeout: defaultTimeout,
		client:  http.DefaultClient,
	}
}

// Lookup queries the named branches in parallel batches. With no branches it
// returns an empty map. Any failure, including timeout, yields an empty map:
// a partial answer could hide an open PR.
func (b *Bitbucket) Lookup(ctx context.Context, branches []string) map[string][]PR {
	empty := map[string][]PR{}
	if len(branches) == 0 {
		return empty
	}
	timeout := b.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		mu     sync.Mutex
		res    = map[string][]PR{}
		failed bool
		wg     sync.WaitGroup
	)
	for start := 0; start < len(branches); start += bitbucketChunk {
		chunk := branches[start:min(start+bitbucketChunk, len(branches))]
		wg.Add(1)
		go func() {
			defer wg.Done()
			prs, err := b.query(ctx, chunk)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = true
				return
			}
			for name, list := range prs {
				res[name] = append(res[name], list...)
			}
		}()
	}
	wg.Wait()
	if failed {
		return empty
	}
	for _, prs := range res {
		sort.SliceStable(prs, func(i, j int) bool { return prs[i].UpdatedAt.After(prs[j].UpdatedAt) })
	}
	return res
}

type bbPage struct {
	Next   string `json:"next"`
	Values []struct {
		ID        int       `json:"id"`
		State     string    `json:"state"`
		UpdatedOn time.Time `json:"updated_on"`
		Source    struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
	} `json:"values"`
}

func (b *Bitbucket) query(ctx context.Context, branches []string) (map[string][]PR, error) {
	conds := make([]string, len(branches))
	for i, name := range branches {
		conds[i] = "source.branch.name=" + bbQuote(name)
	}
	q := url.Values{}
	q.Set("q", strings.Join(conds, " OR "))
	q.Set("fields", "next,values.id,values.state,values.updated_on,values.source.branch.name,values.source.commit.hash")
	q.Set("pagelen", "50")
	for _, s := range []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"} {
		q.Add("state", s)
	}
	next := b.baseURL + "/repositories/" + b.repo + "/pullrequests?" + q.Encode()

	m := map[string][]PR{}
	for page := 0; next != "" && page < bitbucketMaxPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(b.user, b.token)
		resp, err := b.client.Do(req)
		if err != nil {
			return nil, err
		}
		var p bbPage
		err = json.NewDecoder(resp.Body).Decode(&p)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("bitbucket: %s", resp.Status)
		}
		if err != nil {
			return nil, err
		}
		for _, v := range p.Values {
			m[v.Source.Branch.Name] = append(m[v.Source.Branch.Name], PR{
				Number: v.ID, State: bitbucketState(v.State),
				HeadSHA: v.Source.Commit.Hash, UpdatedAt: v.UpdatedOn,
			})
		}
		next = p.Next
	}
	return m, nil
}

// bitbucketState maps Bitbucket's states onto OPEN, MERGED and CLOSED.
func bitbucketState(s string) string {
	switch s {
	case "OPEN", "MERGED":
		return s
	default: // DECLINED, SUPERSEDED
		return "CLOSED"
	}
}

// bbQuote quotes a branch name for Bitbucket's query language, which
// uses double quotes with backslash escapes.
func bbQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// parseBitbucketRemote returns "workspace/repo" for bitbucket.org remotes in
// scp-like, https and ssh forms, including the ssh.bitbucket.org host.
func parseBitbucketRemote(remote string) (string, bool) {
	return parseRemote(remote, "bitbucket.org", "ssh.bitbucket.org")
}
