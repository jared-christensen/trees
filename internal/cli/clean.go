package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/merged"
	"github.com/jared-christensen/trees/internal/report"
	"github.com/jared-christensen/trees/internal/status"
)

type cleanOptions struct {
	Yes         bool // skip the prompt
	NoPR        bool // skip pull request lookups
	Interactive bool // stdin is a terminal, so the prompt can be answered
	// ShellDir is the process's real working directory. It can differ from
	// the repo directory given with -C; worktrees containing either are kept.
	ShellDir string
}

// beforeDelete, when set, runs just before each target is re-verified. Tests
// use it to change the repository between the check and the deletion.
var beforeDelete func(Item)

// runClean removes branches (and their worktrees) that are safe to delete and
// returns the exit code.
func runClean(ctx context.Context, cwd string, in io.Reader, out, errOut io.Writer, opt cleanOptions) int {
	if err := git.CheckVersion(ctx); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	inv, items, _, err := collect(ctx, cwd, true, !opt.NoPR)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if inv.FetchErr != nil {
		fmt.Fprintf(errOut, "Couldn't reach the remote, so nothing was deleted: %v\n", inv.FetchErr)
		return 1
	}
	if !inv.HasRemote {
		fmt.Fprintf(out, "No remote, comparing to local %s.\n", inv.Base)
	}

	here := []string{canonical(cwd)}
	if opt.ShellDir != "" {
		here = append(here, canonical(opt.ShellDir))
	}
	var targets []Item
	for _, it := range items {
		if it.State != status.SafeToDelete {
			continue
		}
		if why := skipReason(inv, it, here); why != "" {
			fmt.Fprintf(out, "Skipping %s: %s.\n", it.Branch.Name, why)
			continue
		}
		targets = append(targets, it)
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, "Nothing to delete.")
		return 0
	}

	fmt.Fprintln(out, "These will be deleted (remote branches are left alone):")
	for _, it := range targets {
		wt := it.Branch.Worktree
		switch {
		case wt == nil:
			fmt.Fprintf(out, "  %s  branch only\n", it.Branch.Name)
		case wt.Missing:
			fmt.Fprintf(out, "  %s  branch, and the record of its missing folder %s\n", it.Branch.Name, report.Tilde(wt.Path))
		default:
			fmt.Fprintf(out, "  %s  branch and folder %s\n", it.Branch.Name, report.Tilde(wt.Path))
			if note := ignoredNote(ctx, wt.Path); note != "" {
				fmt.Fprintf(out, "      %s\n", note)
			}
		}
	}

	if !opt.Yes {
		if !opt.Interactive {
			fmt.Fprintln(errOut, "Confirmation needed: run trees clean --yes to delete without asking.")
			return 1
		}
		fmt.Fprintf(out, "Delete these %d? [y/N] ", len(targets))
		answer, _ := bufio.NewReader(in).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Nothing was deleted.")
			return 0
		}
	}

	r := git.Runner{Dir: inv.Main}
	code := 0
	pruned := false
	for _, it := range targets {
		if beforeDelete != nil {
			beforeDelete(it)
		}
		b := it.Branch
		if why := recheck(ctx, r, inv.Base, b); why != "" {
			fmt.Fprintf(out, "Skipping %s: %s.\n", b.Name, why)
			continue
		}
		if wt := b.Worktree; wt != nil {
			if wt.Missing {
				// git refuses to delete a branch still recorded as checked out.
				if !pruned {
					if _, err := r.Run(ctx, "worktree", "prune"); err != nil {
						fmt.Fprintf(errOut, "Couldn't prune worktree records: %v\n", err)
						code = 1
						continue
					}
					pruned = true
				}
			} else if _, err := r.Run(ctx, "worktree", "remove", wt.Path); err != nil {
				fmt.Fprintf(errOut, "Couldn't remove the folder for %s, so its branch was kept: %v\n", b.Name, err)
				code = 1
				continue
			}
		}
		if _, err := r.Run(ctx, "branch", "-D", b.Name); err != nil {
			fmt.Fprintf(errOut, "Couldn't delete branch %s: %v\n", b.Name, err)
			code = 1
			continue
		}
		fmt.Fprintf(out, "✓ %s  (restore: git branch %s %s)\n", b.Name, b.Name, short(b.SHA))
	}
	if _, err := r.Run(ctx, "worktree", "prune"); err != nil {
		fmt.Fprintf(errOut, "Couldn't prune worktree records: %v\n", err)
		code = 1
	}
	return code
}

// ignoredNote names the ignored files (.env, local notes) that removing the
// worktree at dir would delete along with it, or returns "" when there are none.
func ignoredNote(ctx context.Context, dir string) string {
	names, err := inventory.Ignored(ctx, dir)
	if err != nil {
		return "may also remove ignored files (couldn't list them)"
	}
	if len(names) == 0 {
		return ""
	}
	const shown = 3
	note := "also removes ignored files: "
	if len(names) <= shown {
		return note + strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s%s and %d more", note, strings.Join(names[:shown], ", "), len(names)-shown)
}

// skipReason says why a safe-to-delete item must still be left alone, or "".
func skipReason(inv *inventory.Inventory, it Item, here []string) string {
	wt := it.Branch.Worktree
	if wt == nil {
		return ""
	}
	if wt.Locked {
		return "its worktree is locked"
	}
	if canonical(wt.Path) == canonical(inv.Main) {
		return "it's checked out in the main checkout"
	}
	if !wt.Missing {
		for _, h := range here {
			if within(h, canonical(wt.Path)) {
				return "you're in its worktree (" + report.Tilde(wt.Path) + ")"
			}
		}
	}
	return ""
}

// recheck verifies, just before deleting, that the branch hasn't moved, its
// worktree is still clean and the branch is still merged. It returns why the
// branch must be skipped, or "".
func recheck(ctx context.Context, r git.Runner, base string, b inventory.Branch) string {
	sha, err := r.Run(ctx, "rev-parse", "--verify", "refs/heads/"+b.Name)
	if err != nil {
		return "couldn't read the branch: " + err.Error()
	}
	if strings.TrimSpace(sha) != b.SHA {
		return "the branch changed since it was checked"
	}
	if wt := b.Worktree; wt != nil && wt.CheckErr != nil {
		return "couldn't check its folder: " + wt.CheckErr.Error()
	}
	if wt := b.Worktree; wt != nil && !wt.Missing {
		st, err := inventory.Status(ctx, wt.Path)
		if err != nil {
			return "couldn't check its folder for uncommitted files: " + err.Error()
		}
		if strings.TrimSpace(st) != "" {
			return "its folder now has uncommitted files"
		}
	}
	ok, err := merged.IsMerged(ctx, r, base, "refs/heads/"+b.Name)
	if err != nil {
		return "couldn't check if merged: " + err.Error()
	}
	if !ok {
		return "it's no longer merged into " + base
	}
	return ""
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

// canonical makes path absolute with symlinks resolved, as far as possible.
func canonical(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Clean(path)
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
