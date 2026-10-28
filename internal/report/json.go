package report

import (
	"encoding/json"
	"io"
	"time"

	"github.com/jared-christensen/trees/internal/inventory"
)

// JSON output is a stable shape for scripts and agents: one entry per branch
// (the main checkout's branch included, marked main_checkout) and one per
// worktree with no branch checked out.
type jsonReport struct {
	Repo         string         `json:"repo"`
	Path         string         `json:"path"`
	Base         string         `json:"base"`
	FetchFailed  bool           `json:"fetch_failed"`
	Branches     []jsonBranch   `json:"branches"`
	Detached     []jsonWorktree `json:"detached_worktrees"`
	SafeToDelete int            `json:"safe_to_delete"`
}

type jsonBranch struct {
	Name         string        `json:"name"`
	State        string        `json:"state"` // in_use, stale or safe_to_delete
	Reasons      []string      `json:"reasons"`
	LastActivity time.Time     `json:"last_activity"`
	Unpushed     int           `json:"unpushed_commits"`
	MainCheckout bool          `json:"main_checkout"`
	Worktree     *jsonWorktree `json:"worktree"`
}

type jsonWorktree struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Head    string `json:"head,omitempty"`
	Dirty   int    `json:"uncommitted_files"`
	Missing bool   `json:"missing"`
	Locked  bool   `json:"locked"`
	Checked bool   `json:"checked"` // false when the folder couldn't be read
}

// JSON writes the report as indented JSON.
func JSON(w io.Writer, inv *inventory.Inventory, items []Item) error {
	sortItems(items)
	rep := jsonReport{
		Repo: repoName(inv), Path: inv.Main, Base: inv.Base,
		FetchFailed: inv.FetchErr != nil,
		Branches:    []jsonBranch{}, Detached: []jsonWorktree{},
	}
	for _, it := range items {
		b := jsonBranch{
			Name: it.Branch.Name, State: stateKey(it.State), Reasons: it.Reasons,
			LastActivity: it.Branch.LastCommit, Unpushed: it.Branch.Unpushed,
			MainCheckout: inMain(inv, it),
		}
		if wt := it.Branch.Worktree; wt != nil && !b.MainCheckout {
			j := jsonWT(inv, *wt)
			b.Worktree = &j
		}
		if b.Reasons == nil {
			b.Reasons = []string{}
		}
		rep.Branches = append(rep.Branches, b)
	}
	for _, wt := range inv.Detached {
		rep.Detached = append(rep.Detached, jsonWT(inv, wt))
	}
	rep.SafeToDelete = cleanable(inv, items)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func jsonWT(inv *inventory.Inventory, wt inventory.Worktree) jsonWorktree {
	return jsonWorktree{
		Path: wt.Path, Name: folderName(inv.Main, inv.Bare, wt.Path), Head: wt.Head,
		Dirty: wt.Dirty, Missing: wt.Missing, Locked: wt.Locked, Checked: wt.CheckErr == nil,
	}
}
