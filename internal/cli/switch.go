package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jared-christensen/trees/internal/config"
	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/shell"
)

// switchable lists the worktrees switch can go to: the main checkout, every
// branch's worktree, and detached ones. Missing folders are left out.
func switchable(inv *inventory.Inventory) []inventory.Worktree {
	wts := []inventory.Worktree{{Path: inv.Main, Branch: inv.MainBranch}}
	seen := map[string]bool{inv.Main: true}
	add := func(w inventory.Worktree) {
		if w.Missing || seen[w.Path] {
			return
		}
		seen[w.Path] = true
		wts = append(wts, w)
	}
	for _, b := range inv.Branches {
		if b.Worktree != nil {
			add(*b.Worktree)
		}
	}
	for _, w := range inv.Detached {
		add(w)
	}
	return wts
}

// shortName is a worktree's folder name without the "<repo>." prefix.
func shortName(inv *inventory.Inventory, w inventory.Worktree) string {
	base := filepath.Base(w.Path)
	return strings.TrimPrefix(base, filepath.Base(inv.Main)+".")
}

// findWorktree returns the worktrees matching query by branch, folder name or
// short name. An exact match wins; otherwise every case-insensitive substring
// match is returned.
func findWorktree(inv *inventory.Inventory, query string) []inventory.Worktree {
	if query == "" {
		return nil
	}
	var exact, partial []inventory.Worktree
	lq := strings.ToLower(query)
	for _, w := range switchable(inv) {
		names := []string{w.Branch, filepath.Base(w.Path), shortName(inv, w)}
		isExact, isPartial := false, false
		for _, n := range names {
			if n == "" {
				continue
			}
			if n == query {
				isExact = true
			}
			if strings.Contains(strings.ToLower(n), lq) {
				isPartial = true
			}
		}
		if isExact {
			exact = append(exact, w)
		} else if isPartial {
			partial = append(partial, w)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return partial
}

// runSwitch hands the shell the worktree matching query and returns the exit
// code. after_switch runs only when the shell wrapper will do the cd.
func runSwitch(ctx context.Context, cwd, query string, out, errOut io.Writer) int {
	if err := git.CheckVersion(ctx); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	mainPath, err := git.MainCheckout(ctx, cwd)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	cfg, err := config.Load(mainPath)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	inv, err := inventory.Load(ctx, mainPath, inventory.Options{Fetch: false})
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}

	matches := findWorktree(inv, query)
	switch len(matches) {
	case 0:
		fmt.Fprintf(errOut, "No worktree matches %q.\n", query)
		return 1
	case 1:
	default:
		fmt.Fprintf(errOut, "Several worktrees match %q:\n", query)
		for _, w := range matches {
			branch := w.Branch
			if branch == "" {
				branch = "detached"
			}
			fmt.Fprintf(errOut, "  %s  (branch %s)\n", shortName(inv, w), branch)
		}
		return 1
	}

	target := matches[0].Path
	if err := shell.Cd(target); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if cfg.AfterSwitch != "" && os.Getenv("TREES_CD_FILE") != "" {
		cmd := exec.CommandContext(ctx, "sh", "-c", cfg.AfterSwitch)
		cmd.Dir = target
		cmd.Stdout, cmd.Stderr = out, errOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(errOut, "Warning: after_switch failed: %v\n", err)
		}
	}
	return 0
}
