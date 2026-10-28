// Package inventory gathers the raw facts about a repository's worktrees and
// branches. It never modifies the repository other than fetching.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jared-christensen/trees/internal/git"
)

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path, Branch string // Branch is "" when detached
	Head         string
	Detached     bool
	Missing      bool
	Locked       bool
	Bare         bool  // the repository itself in a bare layout, not a checkout
	Dirty        int   // count of `status --porcelain` lines; 0 when Missing
	CheckErr     error // why the folder couldn't be checked; then Dirty is unknown
}

// Branch is a local branch other than the default branch.
type Branch struct {
	Name, SHA, Upstream string
	UpstreamGone        bool
	Unpushed            int // commits on the branch not on any remote branch
	LastCommit          time.Time
	Worktree            *Worktree // nil if not checked out anywhere
}

// Inventory is a snapshot of the repository.
type Inventory struct {
	Main         string // main checkout path, or the repository in a bare layout
	Bare         bool   // Main is a bare repository with no checkout of its own
	MainBranch   string // branch checked out in the main checkout; "" when Bare
	MainDirty    int
	MainCheckErr error  // why the main checkout couldn't be checked
	Base         string // e.g. "origin/main", or "main" with no remote
	HasRemote    bool
	FetchErr     error      // nil if fetched or skipped on purpose
	Branches     []Branch   // excludes the default branch
	Detached     []Worktree // linked worktrees with no branch
	Worktrees    []Worktree // every checkout in git's order; excludes a bare repository
}

// Options controls Load.
type Options struct {
	Fetch        bool
	FetchTimeout time.Duration // zero means no timeout
}

// Load gathers the inventory for the repository whose main checkout is
// mainPath. A failed fetch is recorded in FetchErr, and a worktree that can't
// be checked in its CheckErr; other git failures are returned.
func Load(ctx context.Context, mainPath string, opt Options) (*Inventory, error) {
	g := git.Runner{Dir: mainPath}
	inv := &Inventory{}

	inv.HasRemote = g.Ok(ctx, "remote", "get-url", "origin")
	if opt.Fetch && inv.HasRemote {
		fctx := ctx
		if opt.FetchTimeout > 0 {
			var cancel context.CancelFunc
			fctx, cancel = context.WithTimeout(ctx, opt.FetchTimeout)
			defer cancel()
		}
		if _, err := g.Run(fctx, "fetch", "--prune", "--quiet", "origin"); err != nil {
			inv.FetchErr = err
		}
	}

	porcelain, err := g.Run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	wts := parseWorktrees(porcelain)
	if len(wts) == 0 {
		return nil, errors.New("git worktree list returned no worktrees")
	}
	for i := range wts {
		if wts[i].Bare {
			continue // the repository itself; there is no checkout to check
		}
		// Stat even when git says prunable: git also says that for a folder
		// it can't read, and an unreadable folder is not a missing one (a
		// missing folder skips the dirty check), so refuse to guess.
		if _, err := os.Stat(wts[i].Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				wts[i].Missing = true
			} else {
				wts[i].Missing = false
				wts[i].CheckErr = fmt.Errorf("checking worktree folder: %w", err)
			}
			continue
		}
		if wts[i].Missing {
			continue
		}
		out, err := Status(ctx, wts[i].Path)
		if err != nil {
			wts[i].CheckErr = err
			continue
		}
		wts[i].Dirty = countLines(out)
	}
	inv.Main = wts[0].Path
	inv.Bare = wts[0].Bare
	if !inv.Bare {
		inv.MainBranch = wts[0].Branch
		inv.MainDirty = wts[0].Dirty
		inv.MainCheckErr = wts[0].CheckErr
	}

	base, err := defaultBase(ctx, g, inv.HasRemote)
	if err != nil {
		return nil, err
	}
	inv.Base = base
	defaultName := strings.TrimPrefix(base, "origin/")

	byBranch := map[string]*Worktree{}
	for i := range wts {
		if wts[i].Bare {
			continue
		}
		inv.Worktrees = append(inv.Worktrees, wts[i])
		if wts[i].Branch != "" {
			byBranch[wts[i].Branch] = &wts[i]
		} else if i > 0 {
			inv.Detached = append(inv.Detached, wts[i])
		}
	}

	const format = "%(refname:short)%09%(objectname)%09%(upstream:short)%09%(upstream:track)%09%(committerdate:unix)"
	refs, err := g.Run(ctx, "for-each-ref", "--format="+format, "refs/heads")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(refs, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			return nil, fmt.Errorf("unexpected for-each-ref line %q", line)
		}
		if f[0] == defaultName {
			continue
		}
		b := Branch{
			Name:         f[0],
			SHA:          f[1],
			Upstream:     f[2],
			UpstreamGone: strings.Contains(f[3], "gone"),
			Worktree:     byBranch[f[0]],
		}
		if secs, err := strconv.ParseInt(f[4], 10, 64); err == nil {
			b.LastCommit = time.Unix(secs, 0)
		}
		if inv.HasRemote {
			n, err := g.Run(ctx, "rev-list", "--count", f[0], "--not", "--remotes")
			if err != nil {
				return nil, err
			}
			b.Unpushed, _ = strconv.Atoi(strings.TrimSpace(n))
		}
		inv.Branches = append(inv.Branches, b)
	}
	return inv, nil
}

// defaultBase returns the default branch ref: origin/HEAD's target, else the
// first existing of origin/main, origin/master, main, master.
func defaultBase(ctx context.Context, g git.Runner, hasRemote bool) (string, error) {
	exists := func(ref string) bool {
		return g.Ok(ctx, "rev-parse", "--verify", "--quiet", "refs/"+ref)
	}
	if hasRemote {
		if ref, err := g.Run(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && exists("remotes/"+ref) {
			return ref, nil
		}
	}
	candidates := []struct{ name, ref string }{
		{"origin/main", "remotes/origin/main"},
		{"origin/master", "remotes/origin/master"},
		{"main", "heads/main"},
		{"master", "heads/master"},
	}
	for _, c := range candidates {
		if exists(c.ref) {
			return c.name, nil
		}
	}
	return "", errors.New("cannot determine the default branch (no origin/HEAD, main or master)")
}

// Status runs `git status --porcelain` in dir. Untracked files and submodule
// changes are always included, whatever the user's status settings say, so a
// worktree with work in it never looks clean.
func Status(ctx context.Context, dir string) (string, error) {
	return git.Runner{Dir: dir}.Run(ctx, "status", "--porcelain", "--untracked-files=normal", "--ignore-submodules=none")
}

func countLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// parseWorktrees parses `git worktree list --porcelain`.
func parseWorktrees(out string) []Worktree {
	var wts []Worktree
	var cur *Worktree
	for _, line := range strings.Split(out, "\n") {
		key, val, _ := strings.Cut(strings.TrimRight(line, "\r"), " ")
		switch key {
		case "worktree":
			wts = append(wts, Worktree{Path: val})
			cur = &wts[len(wts)-1]
		case "HEAD":
			if cur != nil {
				cur.Head = val
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(val, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Missing = true
			}
		}
	}
	return wts
}
