package cli

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jared-christensen/trees/internal/config"
	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/merged"
	"github.com/jared-christensen/trees/internal/pr"
	"github.com/jared-christensen/trees/internal/report"
	"github.com/jared-christensen/trees/internal/status"
)

// Item is one classified branch. It lives in report so Render can take it.
type Item = report.Item

// maxParallel bounds the per-branch merged checks running at once.
const maxParallel = 8

// collect gathers and classifies everything list and clean need for the
// repository containing cwd. PR lookups (when prs is true and enabled in the
// config) run alongside the merged checks.
func collect(ctx context.Context, cwd string, fetch, prs bool) (*inventory.Inventory, []Item, config.Settings, error) {
	mainPath, err := git.MainCheckout(ctx, cwd)
	if err != nil {
		return nil, nil, config.Settings{}, err
	}
	cfg, err := config.Load(mainPath)
	if err != nil {
		return nil, nil, config.Settings{}, err
	}
	inv, err := inventory.Load(ctx, mainPath, inventory.Options{
		Fetch:        fetch,
		FetchTimeout: time.Duration(cfg.FetchTimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, nil, cfg, err
	}
	r := git.Runner{Dir: inv.Main}

	names := make([]string, len(inv.Branches))
	for i, b := range inv.Branches {
		names[i] = b.Name
	}
	prDone := make(chan map[string][]pr.PR, 1)
	go func() {
		var url string
		if inv.HasRemote {
			url, _ = r.Run(ctx, "remote", "get-url", "origin")
		}
		enabled := prs && cfg.PullRequests != nil && *cfg.PullRequests
		prDone <- pr.Detect(strings.TrimSpace(url), enabled).Lookup(ctx, names)
	}()

	type check struct {
		merged, own bool
		err         error
	}
	checks := make([]check, len(inv.Branches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	for i, b := range inv.Branches {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, name string) {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := merged.IsMerged(ctx, r, inv.Base, "refs/heads/"+name)
			checks[i] = check{merged: m, err: err, own: merged.HasOwnCommits(ctx, r, inv.Base, name)}
		}(i, b.Name)
	}
	wg.Wait()
	prsByBranch := <-prDone

	now := time.Now()
	items := make([]Item, len(inv.Branches))
	for i, b := range inv.Branches {
		f := status.Facts{
			Protected:    status.LongLived(b.Name),
			OwnCommits:   checks[i].own,
			Merged:       checks[i].merged,
			LastCommit:   b.LastCommit,
			Now:          now,
			StaleAfter:   time.Duration(cfg.StaleDays) * 24 * time.Hour,
			Unpushed:     b.Unpushed,
			UpstreamGone: b.UpstreamGone,
			Base:         inv.Base,
		}
		if b.Worktree != nil {
			f.Dirty = b.Worktree.Dirty
			f.CheckFailed = b.Worktree.CheckErr != nil
		}
		f.OpenPR, f.MergedPR, f.ClosedPR = newestPRs(prsByBranch[b.Name])
		items[i] = classify(b, f, checks[i].err)
	}
	return inv, items, cfg, nil
}

// classify turns facts into an Item. A failed merged check counts as not
// merged, so the branch can never be safe to delete because of an error.
func classify(b inventory.Branch, f status.Facts, mergeErr error) Item {
	if mergeErr != nil {
		f.Merged = false
	}
	state, why := status.Classify(f)
	if mergeErr != nil {
		const note = "couldn't check if merged"
		if len(why) == 1 && why[0] == "not merged yet" {
			why = []string{note}
		} else {
			why = append(why, note)
		}
	}
	return Item{Branch: b, State: state, Reasons: why}
}

// newestPRs returns the number of the newest open, merged and closed PR in
// prs (which is newest first); 0 where there is none.
func newestPRs(prs []pr.PR) (open, mergedPR, closed int) {
	for _, p := range prs {
		switch p.State {
		case "OPEN":
			if open == 0 {
				open = p.Number
			}
		case "MERGED":
			if mergedPR == 0 {
				mergedPR = p.Number
			}
		case "CLOSED":
			if closed == 0 {
				closed = p.Number
			}
		}
	}
	return open, mergedPR, closed
}
