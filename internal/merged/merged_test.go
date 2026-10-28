package merged_test

import (
	"context"
	"testing"

	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/gittest"
	"github.com/jared-christensen/trees/internal/merged"
)

func check(t *testing.T, r *gittest.Repo, branch string, want bool) {
	t.Helper()
	got, err := merged.IsMerged(context.Background(), git.Runner{Dir: r.Main}, "origin/main", branch)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("IsMerged(%s) = %v, want %v", branch, got, want)
	}
}

func TestIsMergedRegularMerge(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.Merge("feat")
	r.Git(r.Main, "fetch")
	check(t, r, "feat", true)
}

func TestIsMergedSquash(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.Commit(wt, "b.txt", "b")
	r.SquashMerge("feat")
	r.Git(r.Main, "fetch")
	check(t, r, "feat", true)
}

func TestIsMergedSquashThenMoreCommits(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.Commit(wt, "b.txt", "b")
	r.SquashMerge("feat")
	r.Commit(wt, "c.txt", "c")
	r.Git(r.Main, "fetch")
	check(t, r, "feat", false)
}

func TestIsMergedAfterMainMoved(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.SquashMerge("feat")
	r.Git(r.Main, "pull", "--ff-only")
	r.Commit(r.Main, "other.txt", "other")
	r.Git(r.Main, "push", "origin", "main")
	r.Git(r.Main, "fetch")
	check(t, r, "feat", true)
}

func TestIsMergedConflictIsFalse(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "README.md", "feature side")
	r.Commit(r.Main, "README.md", "main side")
	r.Git(r.Main, "push", "origin", "main")
	r.Git(r.Main, "fetch")
	check(t, r, "feat", false)
}

func TestIsMergedErrorsOnBadRef(t *testing.T) {
	r := gittest.New(t)
	got, err := merged.IsMerged(context.Background(), git.Runner{Dir: r.Main}, "origin/main", "nope")
	if err == nil || got {
		t.Fatalf("got (%v, %v), want (false, error)", got, err)
	}
}

func own(r *gittest.Repo, branch string) bool {
	return merged.HasOwnCommits(context.Background(), git.Runner{Dir: r.Main}, "origin/main", branch)
}

func TestHasOwnCommits(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	if own(r, "feat") {
		t.Fatal("fresh branch should have no own commits")
	}
	r.Commit(wt, "a.txt", "a")
	if !own(r, "feat") {
		t.Fatal("branch with a commit should have own commits")
	}
}

func TestHasOwnCommitsFastForwardIsNotOwn(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(r.Main, "other.txt", "other")
	r.Git(r.Main, "push", "origin", "main")
	r.Git(r.Main, "fetch")
	r.Git(wt, "merge", "--ff-only", "origin/main")
	if own(r, "feat") {
		t.Fatal("fast-forward from base is not own work")
	}
}

func TestHasOwnCommitsMergedWithMergeCommit(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.Merge("feat")
	r.Git(r.Main, "fetch")
	if !own(r, "feat") {
		t.Fatal("local commit merged with --no-ff is still own work")
	}
}

func TestHasOwnCommitsTrackedRemoteWork(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("tmp")
	r.Commit(wt, "x.txt", "x")
	r.Git(wt, "push", "origin", "HEAD:refs/heads/x")
	r.Git(r.Main, "fetch")
	r.Git(r.Main, "branch", "--track", "x", "origin/x")
	if !own(r, "x") {
		t.Fatal("work pulled from a remote branch not on base is own work")
	}
}

func TestHasOwnCommitsMissingBranch(t *testing.T) {
	r := gittest.New(t)
	if own(r, "does-not-exist") {
		t.Fatal("missing branch must be false")
	}
}

func TestHasOwnCommitsMergeCommitAfterReflogExpired(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(wt, "a.txt", "a")
	r.Merge("feat")
	r.Git(r.Main, "fetch")
	r.Git(r.Main, "reflog", "expire", "--expire=now", "--all")
	if !own(r, "feat") {
		t.Fatal("branch merged with --no-ff is own work even with no reflog")
	}
}

func TestHasOwnCommitsTrackedRemoteWorkMergedWithMergeCommit(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("tmp")
	r.Commit(wt, "x.txt", "x")
	r.Git(wt, "push", "origin", "HEAD:refs/heads/x")
	r.Git(r.Main, "fetch")
	r.Git(r.Main, "branch", "--track", "x", "origin/x")
	r.Merge("x")
	r.Git(r.Main, "fetch")
	if !own(r, "x") {
		t.Fatal("a teammate's branch merged with --no-ff is own work")
	}
}

func TestHasOwnCommitsFastForwardAfterReflogExpiredIsNotOwn(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	r.Commit(r.Main, "other.txt", "other")
	r.Git(r.Main, "push", "origin", "main")
	r.Git(r.Main, "fetch")
	r.Git(wt, "merge", "--ff-only", "origin/main")
	r.Git(r.Main, "reflog", "expire", "--expire=now", "--all")
	if own(r, "feat") {
		t.Fatal("fast-forward from base is not own work")
	}
}
