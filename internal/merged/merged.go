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

// HasOwnCommits reports whether branch carries work of its own, so that a
// branch that is merely level with or behind base is not mistaken for finished
// work. It is true when any of these holds:
//
//	(a) the branch's reflog records a commit-creating entry (commit, cherry-pick,
//	    revert, am), which catches local work even after it was merged with a
//	    merge commit;
//	(b) the current tip or any sha the branch has pointed at is not an ancestor
//	    of base, which catches work pulled from elsewhere that was later
//	    squash-merged; or
//	(c) the tip is an ancestor of base but not on base's first-parent history,
//	    so it arrived through a merge commit; this catches merged branches whose
//	    reflog has expired or never recorded the commits (a teammate's branch).
//
// A fast-forward from base hits none of these: its tip is on base's
// first-parent history. Any git error returns false: the never-deletable
// direction.
func HasOwnCommits(ctx context.Context, r git.Runner, base, branch string) bool {
	ref := "refs/heads/" + branch
	tip, err := r.Run(ctx, "rev-parse", "--verify", ref)
	if err != nil {
		return false
	}
	subjects, err := r.Run(ctx, "reflog", "show", "--format=%gs", ref)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(subjects, "\n") {
		for _, p := range []string{"commit", "cherry-pick", "revert", "am"} {
			if strings.HasPrefix(line, p) {
				return true
			}
		}
	}
	shas, err := r.Run(ctx, "reflog", "show", "--format=%H", ref)
	if err != nil {
		return false
	}
	candidates := append(strings.Split(shas, "\n"), tip)
	seen := map[string]bool{}
	for _, sha := range candidates {
		sha = strings.TrimSpace(sha)
		if sha == "" || seen[sha] {
			continue
		}
		seen[sha] = true
		_, err := r.Run(ctx, "merge-base", "--is-ancestor", sha, base)
		if err == nil {
			continue
		}
		if exitCode(err) == 1 {
			return true
		}
		return false
	}
	// Every candidate, the tip included, is an ancestor of base.
	// Excluding the tip's parents stops the walk at the tip's age, so this
	// lists only base's first-parent commits newer than the tip.
	tip = strings.TrimSpace(tip)
	firstParent, err := r.Run(ctx, "rev-list", "--first-parent", base, "--not", tip+"^@")
	if err != nil {
		return false
	}
	for _, sha := range strings.Split(firstParent, "\n") {
		if strings.TrimSpace(sha) == tip {
			return false
		}
	}
	return true
}
