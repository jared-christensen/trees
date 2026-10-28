package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/status"
)

func fixture() (*inventory.Inventory, []Item) {
	now := time.Now()
	inv := &inventory.Inventory{
		Main:       "/work/app",
		MainBranch: "main",
		Base:       "origin/main",
		HasRemote:  true,
	}
	items := []Item{
		{
			Branch: inventory.Branch{Name: "gone-branch", LastCommit: now.Add(-40 * 24 * time.Hour)},
			State:  status.Stale, Reasons: []string{"PR #3 closed"},
		},
		{
			Branch: inventory.Branch{Name: "done", LastCommit: now.Add(-2 * time.Hour),
				Worktree: &inventory.Worktree{Path: "/work/app.done-folder", Branch: "done"}},
			State: status.SafeToDelete, Reasons: []string{"merged into origin/main"},
		},
		{
			Branch: inventory.Branch{Name: "feat/x", LastCommit: now.Add(-1 * time.Hour),
				Worktree: &inventory.Worktree{Path: "/work/app.feat-x", Branch: "feat/x", Dirty: 3}},
			State: status.InUse, Reasons: []string{"3 uncommitted files"},
		},
		{
			Branch: inventory.Branch{Name: "old-merged", LastCommit: now.Add(-3 * 24 * time.Hour)},
			State:  status.SafeToDelete, Reasons: []string{"PR #9 merged"},
		},
		{
			Branch: inventory.Branch{Name: "lost", LastCommit: now.Add(-5 * 24 * time.Hour),
				Worktree: &inventory.Worktree{Path: "/elsewhere/lost-dir", Branch: "lost", Missing: true, Locked: true}},
			State: status.Stale, Reasons: []string{"not merged yet"},
		},
	}
	return inv, items
}

func render(inv *inventory.Inventory, items []Item) string {
	var b bytes.Buffer
	Render(&b, inv, items, false)
	return b.String()
}

func TestRenderGroupsWorktreesFirst(t *testing.T) {
	inv, items := fixture()
	out := render(inv, items)
	t.Log("\n" + out)

	wt := strings.Index(out, "\nWorktrees\n")
	br := strings.Index(out, "\nBranches not in any worktree\n")
	if wt < 0 || br < 0 || wt > br {
		t.Fatalf("want Worktrees section before branches section, got:\n%s", out)
	}
	for _, phrase := range []string{"still in use", "stale, take a look", "safe to delete"} {
		if !strings.Contains(out, "● "+phrase+"  ") {
			t.Errorf("missing state phrase %q", phrase)
		}
	}
	if !strings.HasPrefix(out, "app  /work/app, compared to origin/main\n") {
		t.Errorf("bad header:\n%s", out)
	}
	if !strings.Contains(out, "2 safe to delete. Run trees clean to remove them.") {
		t.Errorf("missing footer with count 2:\n%s", out)
	}
	// Worktrees sorted by state (in use, stale, safe), branches likewise.
	order := []string{"feat-x", "lost-dir", "done-folder", "gone-branch", "old-merged"}
	last := -1
	for _, name := range order {
		i := strings.Index(out, name)
		if i < last {
			t.Errorf("%q out of order:\n%s", name, out)
		}
		last = i
	}
	// Sibling folders show the suffix after "<repo>.", others the full path.
	if strings.Contains(out, "app.feat-x") || !strings.Contains(out, "  feat-x  ") {
		t.Errorf("sibling folder not shortened:\n%s", out)
	}
	if !strings.Contains(out, "/elsewhere/lost-dir (folder missing) (locked)") {
		t.Errorf("missing/locked suffixes absent:\n%s", out)
	}
	if !strings.Contains(out, "      branch done\n") {
		t.Errorf("want branch line when folder differs from branch:\n%s", out)
	}
	if !strings.Contains(out, "      3 uncommitted files\n") {
		t.Errorf("reasons line missing:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("color codes with color off:\n%q", out)
	}
	if strings.Contains(out, "Couldn't reach") || strings.Contains(out, "No remote") || strings.Contains(out, "main checkout") {
		t.Errorf("unexpected warning lines:\n%s", out)
	}
}

func TestRenderCollapsesMatchingNames(t *testing.T) {
	inv := &inventory.Inventory{Main: "/work/app", MainBranch: "main", Base: "origin/main", HasRemote: true}
	items := []Item{
		{Branch: inventory.Branch{Name: "feat-a", LastCommit: time.Now(),
			Worktree: &inventory.Worktree{Path: "/work/app.feat-a", Branch: "feat-a"}},
			State: status.InUse, Reasons: []string{"not merged yet"}},
		{Branch: inventory.Branch{Name: "feat/b", LastCommit: time.Now(),
			Worktree: &inventory.Worktree{Path: "/work/app.feat-b", Branch: "feat/b"}},
			State: status.InUse, Reasons: []string{"not merged yet"}},
	}
	out := render(inv, items)
	if strings.Contains(out, "branch feat-a") {
		t.Errorf("matching names should collapse:\n%s", out)
	}
	if strings.Contains(out, "branch feat/b") {
		t.Errorf("slash branch matching sanitized folder should collapse:\n%s", out)
	}
	if !strings.Contains(out, "Nothing is safe to delete.") {
		t.Errorf("want zero-count footer:\n%s", out)
	}
}

func TestRenderFetchWarning(t *testing.T) {
	inv, items := fixture()
	inv.FetchErr = errors.New("timeout")
	out := render(inv, items)
	if !strings.Contains(out, "\nCouldn't reach the remote, so this uses what was last fetched.\n") {
		t.Errorf("missing fetch warning:\n%s", out)
	}
}

func TestRenderNoRemote(t *testing.T) {
	inv, items := fixture()
	inv.HasRemote = false
	inv.Base = "main"
	out := render(inv, items)
	if !strings.Contains(out, "\nNo remote, comparing to local main.\n") {
		t.Errorf("missing no-remote note:\n%s", out)
	}
}

func TestRenderMainCheckoutNote(t *testing.T) {
	inv, items := fixture()
	inv.MainBranch = "hotfix"
	inv.MainDirty = 1
	out := render(inv, items)
	if !strings.Contains(out, "\nThe main checkout is on hotfix, not main, and has 1 uncommitted file.\n") {
		t.Errorf("missing main checkout note:\n%s", out)
	}
}

func TestRenderDetached(t *testing.T) {
	inv, items := fixture()
	inv.Detached = []inventory.Worktree{{Path: "/work/app.review", Head: "0123456789abcdef", Detached: true, Dirty: 2}}
	out := render(inv, items)
	br := strings.Index(out, "Branches not in any worktree")
	d := strings.Index(out, "Worktrees with no branch checked out")
	if d < 0 || d < br {
		t.Fatalf("detached section missing or misplaced:\n%s", out)
	}
	if !strings.Contains(out, "  review  at 0123456\n      2 uncommitted files\n") {
		t.Errorf("detached entry wrong:\n%s", out)
	}
}

func TestRenderColor(t *testing.T) {
	inv, items := fixture()
	var b bytes.Buffer
	Render(&b, inv, items, true)
	if !strings.Contains(b.String(), "\x1b[") {
		t.Errorf("want color codes with color on")
	}
}

func TestRenderSkipsMainCheckoutAndCountsOnlyCleanable(t *testing.T) {
	inv, items := fixture()
	inv.MainBranch = "hotfix"
	items = append(items,
		Item{Branch: inventory.Branch{Name: "hotfix", LastCommit: time.Now(),
			Worktree: &inventory.Worktree{Path: "/work/app", Branch: "hotfix"}},
			State: status.SafeToDelete, Reasons: []string{"merged into origin/main"}},
		Item{Branch: inventory.Branch{Name: "pinned", LastCommit: time.Now(),
			Worktree: &inventory.Worktree{Path: "/work/app.pinned", Branch: "pinned", Locked: true}},
			State: status.SafeToDelete, Reasons: []string{"merged into origin/main"}},
	)
	out := render(inv, items)
	if strings.Contains(out, "branch hotfix") || strings.Contains(out, "  /work/app  ") {
		t.Errorf("main checkout listed as an item:\n%s", out)
	}
	if !strings.Contains(out, "pinned (locked)") {
		t.Errorf("locked worktree should still be listed:\n%s", out)
	}
	if !strings.Contains(out, "\n2 safe to delete. Run trees clean to remove them.\n") {
		t.Errorf("footer should count only what clean would remove:\n%s", out)
	}
}
