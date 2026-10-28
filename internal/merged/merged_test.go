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

func TestHasOwnCommits(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat")
	run := git.Runner{Dir: r.Main}
	if merged.HasOwnCommits(context.Background(), run, "feat") {
		t.Fatal("fresh branch should have no own commits")
	}
	r.Commit(wt, "a.txt", "a")
	if !merged.HasOwnCommits(context.Background(), run, "feat") {
		t.Fatal("branch with a commit should have own commits")
	}
}

func TestHasOwnCommitsNoReflog(t *testing.T) {
	r := gittest.New(t)
	if !merged.HasOwnCommits(context.Background(), git.Runner{Dir: r.Main}, "does-not-exist") {
		t.Fatal("no reflog should assume work")
	}
}
