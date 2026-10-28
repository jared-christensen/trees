package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jared-christensen/trees/internal/config"
	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/naming"
	"github.com/jared-christensen/trees/internal/shell"
)

// runCreate makes a worktree for branch (creating the branch if needed), runs
// the setup commands and hands the shell the new worktree. It returns the
// exit code.
func runCreate(ctx context.Context, cwd, branch string, in io.Reader, out, errOut io.Writer) int {
	if err := git.CheckVersion(ctx); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	mainPath, err := git.MainCheckout(ctx, cwd)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	g := git.Runner{Dir: mainPath}
	if !g.Ok(ctx, "check-ref-format", "--branch", branch) {
		fmt.Fprintf(errOut, "%q isn't a valid branch name.\n", branch)
		return 1
	}
	cfg, err := config.Load(mainPath)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}

	// 1. Name check.
	warning, err := naming.Check(cfg.NamePattern, cfg.NamePatternMode, branch)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if warning != "" {
		fmt.Fprintln(errOut, "Warning: "+warning)
	}

	// 2. Already checked out somewhere: go there.
	inv, err := inventory.Load(ctx, mainPath, inventory.Options{Fetch: false})
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	existing := ""
	if inv.MainBranch == branch {
		existing = inv.Main
	}
	for _, b := range inv.Branches {
		if b.Name == branch && b.Worktree != nil && !b.Worktree.Missing {
			existing = b.Worktree.Path
		}
	}
	if existing != "" {
		fmt.Fprintf(out, "%s is already in %s.\n", branch, existing)
		if err := shell.Cd(existing); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}

	// 3. Fetch (soft).
	inv, err = inventory.Load(ctx, mainPath, inventory.Options{
		Fetch:        true,
		FetchTimeout: time.Duration(cfg.FetchTimeoutSeconds) * time.Second,
	})
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if inv.FetchErr != nil {
		fmt.Fprintln(errOut, "Couldn't reach the remote, so this uses what was last fetched.")
	}

	// 5 (checked before any change). The target must not exist.
	path := naming.WorktreePath(cfg.Path, mainPath, branch)
	if _, err := os.Lstat(path); err == nil {
		fmt.Fprintf(errOut, "%s already exists.\n", path)
		return 1
	}

	short := strings.TrimPrefix(filepath.Base(path), filepath.Base(mainPath)+".")

	// 4. Add the worktree.
	var args []string
	fromBase := false
	switch {
	case g.Ok(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+branch):
		args = []string{"worktree", "add", path, branch}
	case g.Ok(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch):
		args = []string{"worktree", "add", "--track", "-b", branch, path, "origin/" + branch}
	default:
		args = []string{"worktree", "add", "--no-track", "-b", branch, path, inv.Base}
		fromBase = true
	}
	if _, err := g.Run(ctx, args...); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if fromBase {
		fmt.Fprintf(out, "Created %s on branch %s from %s.\n", short, branch, inv.Base)
	} else {
		fmt.Fprintf(out, "Created %s on branch %s.\n", short, branch)
	}

	// 6. Setup commands.
	code := 0
	for _, c := range cfg.Setup {
		fmt.Fprintf(out, "Running setup: %s\n", c)
		cmd := exec.CommandContext(ctx, "sh", "-c", c)
		cmd.Dir = path
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(errOut, "Setup failed: %s\nThe worktree was kept at %s.\n", c, path)
			code = 1
			break
		}
	}

	// 7. Switch there, even after a setup failure.
	if err := shell.Cd(path); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return code
}
