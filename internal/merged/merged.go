// Package merged decides whether a branch's work already landed in a base.
package merged

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/jared-christensen/trees/internal/git"
)

// exitCode returns the process exit code wrapped in err, or -1.
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// IsMerged reports whether every change on branch is already in base. It is
// true when branch is an ancestor of base (regular merge, fast-forward) or
// when merging branch into base would produce exactly base's tree (squash or
// rebase merge). It is never true for a branch carrying changes base lacks.
// Any unexpected git failure is returned as an error so callers can refuse to
// delete; only a merge conflict is a clean "not merged".
func IsMerged(ctx context.Context, r git.Runner, base, branch string) (bool, error) {
	_, err := r.Run(ctx, "merge-base", "--is-ancestor", branch, base)
	if err == nil {
		return true, nil
	}
	if exitCode(err) != 1 {
		return false, err
	}

	out, err := r.Run(ctx, "merge-tree", "--write-tree", base, branch)
	if err != nil {
		if exitCode(err) == 1 { // conflicts
			return false, nil
		}
		return false, err
	}
	tree, _, _ := strings.Cut(out, "\n")
	baseTree, err := r.Run(ctx, "rev-parse", base+"^{tree}")
	if err != nil {
		return false, err
	}
	tree = strings.TrimSpace(tree)
	return tree != "" && tree == strings.TrimSpace(baseTree), nil
}

// HasOwnCommits reports whether branch has moved since it was created. The
// oldest reflog entry is the sha the branch started at; if the tip differs,
// the branch has its own commits. With no reflog it assumes there is work.
func HasOwnCommits(ctx context.Context, r git.Runner, branch string) bool {
	out, err := r.Run(ctx, "reflog", "show", "--format=%H", "refs/heads/"+branch)
	if err != nil || strings.TrimSpace(out) == "" {
		return true
	}
	lines := strings.Split(out, "\n")
	oldest := strings.TrimSpace(lines[len(lines)-1])
	tip, err := r.Run(ctx, "rev-parse", "refs/heads/"+branch)
	if err != nil {
		return true
	}
	return oldest != strings.TrimSpace(tip)
}
