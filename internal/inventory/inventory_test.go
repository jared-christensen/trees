package inventory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jared-christensen/trees/internal/gittest"
)

func load(t *testing.T, r *gittest.Repo) *Inventory {
	t.Helper()
	inv, err := Load(context.Background(), r.Main, Options{Fetch: true, FetchTimeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return inv
}

func find(inv *Inventory, name string) *Branch {
	for i := range inv.Branches {
		if inv.Branches[i].Name == name {
			return &inv.Branches[i]
		}
	}
	return nil
}

func TestLoadPairsBranchesAndWorktrees(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	r.Git(r.Main, "branch", "feat-b")
	inv := load(t, r)

	if inv.Main != r.Main || inv.MainBranch != "main" || inv.Base != "origin/main" || !inv.HasRemote {
		t.Fatalf("unexpected header: %+v", inv)
	}
	a := find(inv, "feat-a")
	if a == nil || a.Worktree == nil || a.Worktree.Path != wt {
		t.Fatalf("feat-a: %+v", a)
	}
	if b := find(inv, "feat-b"); b == nil || b.Worktree != nil {
		t.Fatalf("feat-b: %+v", b)
	}
	if find(inv, "main") != nil {
		t.Fatal("main must not be in Branches")
	}
}

func TestLoadBranchFacts(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	sha := r.Commit(wt, "a.txt", "a")
	inv := load(t, r)
	a := find(inv, "feat-a")
	if a.SHA != sha || a.Unpushed != 1 || a.LastCommit.IsZero() || a.Upstream != "" {
		t.Fatalf("%+v", a)
	}
	r.Git(wt, "push", "-u", "origin", "feat-a")
	a = find(load(t, r), "feat-a")
	if a.Upstream != "origin/feat-a" || a.Unpushed != 0 || a.UpstreamGone {
		t.Fatalf("%+v", a)
	}
	r.Git(r.Origin, "branch", "-D", "feat-a")
	a = find(load(t, r), "feat-a")
	if !a.UpstreamGone {
		t.Fatalf("expected gone: %+v", a)
	}
}

func TestLoadMainWithOtherBranch(t *testing.T) {
	r := gittest.New(t)
	r.Git(r.Main, "checkout", "-b", "side")
	inv := load(t, r)
	s := find(inv, "side")
	if inv.MainBranch != "side" || s == nil || s.Worktree == nil || s.Worktree.Path != r.Main {
		t.Fatalf("%+v %+v", inv, s)
	}
	if find(inv, "main") != nil {
		t.Fatal("main must not be in Branches")
	}
}

func TestLoadDirtyCount(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	for _, f := range []string{"x.txt", "y.txt"} {
		if err := os.WriteFile(filepath.Join(wt, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inv := load(t, r)
	if d := find(inv, "feat-a").Worktree.Dirty; d != 2 {
		t.Fatalf("Dirty = %d", d)
	}
	if inv.MainDirty != 0 {
		t.Fatalf("MainDirty = %d", inv.MainDirty)
	}
}

func TestLoadMissingFolder(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	w := find(load(t, r), "feat-a").Worktree
	if w == nil || !w.Missing || w.Dirty != 0 {
		t.Fatalf("%+v", w)
	}
}

func TestLoadLocked(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	r.Git(r.Main, "worktree", "lock", wt)
	if w := find(load(t, r), "feat-a").Worktree; !w.Locked {
		t.Fatalf("%+v", w)
	}
}

func TestLoadDetached(t *testing.T) {
	r := gittest.New(t)
	path := filepath.Join(filepath.Dir(r.Main), "det")
	r.Git(r.Main, "worktree", "add", "--detach", path)
	inv := load(t, r)
	if len(inv.Detached) != 1 || inv.Detached[0].Path != path || !inv.Detached[0].Detached || inv.Detached[0].Branch != "" {
		t.Fatalf("%+v", inv.Detached)
	}
}

func TestLoadNoRemote(t *testing.T) {
	r := gittest.New(t)
	r.Git(r.Main, "remote", "remove", "origin")
	r.Git(r.Main, "branch", "feat-b")
	inv := load(t, r)
	if inv.HasRemote || inv.Base != "main" || inv.FetchErr != nil {
		t.Fatalf("%+v", inv)
	}
	if b := find(inv, "feat-b"); b == nil {
		t.Fatalf("%+v", b)
	}
}

func TestLoadFetchFailureIsSoft(t *testing.T) {
	r := gittest.New(t)
	r.Git(r.Main, "branch", "feat-b")
	r.Git(r.Main, "remote", "set-url", "origin", filepath.Join(filepath.Dir(r.Main), "nope.git"))
	inv := load(t, r)
	if inv.FetchErr == nil || find(inv, "feat-b") == nil {
		t.Fatalf("%+v", inv)
	}
}

func TestLoadNotARepo(t *testing.T) {
	if _, err := Load(context.Background(), t.TempDir(), Options{}); err == nil {
		t.Fatal("expected error")
	}
}
