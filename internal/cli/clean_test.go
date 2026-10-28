package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/gittest"
	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/status"
)

// isolate keeps the user's real trees config out of tests.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

type result struct {
	code        int
	out, errOut string
}

func clean(t *testing.T, cwd, stdin string, opt cleanOptions) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runClean(context.Background(), cwd, strings.NewReader(stdin), &out, &errOut, opt)
	t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	return result{code, out.String(), errOut.String()}
}

// mergedWorktree makes branch with a worktree and one commit, pushes it and
// squash-merges it into origin's main.
func mergedWorktree(r *gittest.Repo, branch string) string {
	wt := r.AddWorktree(branch)
	r.Commit(wt, branch+".txt", "work on "+branch)
	r.Git(wt, "push", "-u", "origin", branch)
	r.SquashMerge(branch)
	return wt
}

func branchExists(r *gittest.Repo, name string) bool {
	return r.Git(r.Main, "branch", "--list", name) != ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCleanRemovesOnlySafe(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	doneSHA := r.Git(r.Main, "rev-parse", "done")
	dirty := mergedWorktree(r, "dirty")
	if err := os.WriteFile(filepath.Join(dirty, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := r.AddWorktree("new")
	wip := r.AddWorktree("wip")
	r.Commit(wip, "wip.txt", "wip")
	r.Git(wip, "push", "-u", "origin", "wip")

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if branchExists(r, "done") || exists(done) {
		t.Errorf("done should be gone")
	}
	for _, b := range []string{"dirty", "new", "wip"} {
		if !branchExists(r, b) {
			t.Errorf("branch %s was deleted", b)
		}
	}
	for _, p := range []string{dirty, fresh, wip} {
		if !exists(p) {
			t.Errorf("folder %s was deleted", p)
		}
	}
	remote := r.Git(r.Origin, "branch", "--list")
	for _, b := range []string{"done", "dirty", "wip", "main"} {
		if !strings.Contains(remote, b) {
			t.Errorf("origin lost branch %s: %q", b, remote)
		}
	}
	want := "✓ done  (restore: git branch done " + doneSHA[:10] + ")"
	if !strings.Contains(res.out, want) {
		t.Errorf("missing restore line %q", want)
	}
	if strings.Contains(res.out, "Delete these") {
		t.Errorf("--yes should skip the prompt")
	}
}

func TestCleanSkipsCurrentWorktree(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	sub := filepath.Join(done, "sub dir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	res := clean(t, sub, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if !branchExists(r, "done") || !exists(done) {
		t.Errorf("current worktree was removed")
	}
	if !strings.Contains(res.out, "you're in its worktree") {
		t.Errorf("output doesn't say why done was skipped")
	}
}

func TestCleanPrunesMissingFolder(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	if err := os.RemoveAll(done); err != nil {
		t.Fatal(err)
	}

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if branchExists(r, "done") {
		t.Errorf("branch done should be deleted")
	}
	if list := r.Git(r.Main, "worktree", "list"); strings.Contains(list, done) {
		t.Errorf("worktree record not pruned:\n%s", list)
	}
}

func TestCleanStopsWhenFetchFails(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	r.Git(r.Main, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "nope.git"))

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 1 {
		t.Errorf("want exit 1, got %d", res.code)
	}
	if !strings.Contains(res.errOut, "Couldn't reach the remote, so nothing was deleted: ") {
		t.Errorf("missing fetch failure message")
	}
	if !branchExists(r, "done") || !exists(done) {
		t.Errorf("something was deleted after a failed fetch")
	}
}

func TestCleanNoRemote(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	r.Git(r.Main, "remote", "remove", "origin")
	done := r.AddWorktree("done")
	r.Commit(done, "done.txt", "done work")
	r.Git(r.Main, "merge", "--no-ff", "-m", "merge done", "done")

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if branchExists(r, "done") || exists(done) {
		t.Errorf("done should be deleted with no remote")
	}
}

func TestCleanSkipsLocked(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	r.Git(r.Main, "worktree", "lock", done)

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if !branchExists(r, "done") || !exists(done) {
		t.Errorf("locked worktree was removed")
	}
	if !strings.Contains(res.out, "locked") {
		t.Errorf("output doesn't say done is locked")
	}
}

func TestCleanPrompt(t *testing.T) {
	for _, tc := range []struct {
		answer  string
		deleted bool
	}{{"y\n", true}, {"yes\n", true}, {"n\n", false}, {"\n", false}, {"", false}} {
		t.Run(strings.TrimSpace(tc.answer), func(t *testing.T) {
			isolate(t)
			r := gittest.New(t)
			mergedWorktree(r, "done")
			res := clean(t, r.Main, tc.answer, cleanOptions{Interactive: true})
			if res.code != 0 {
				t.Errorf("exit %d", res.code)
			}
			if !strings.Contains(res.out, "Delete these 1? [y/N] ") {
				t.Errorf("missing prompt")
			}
			if branchExists(r, "done") == tc.deleted {
				t.Errorf("answer %q: deleted=%v, want %v", tc.answer, !branchExists(r, "done"), tc.deleted)
			}
		})
	}
}

func TestCleanNeedsConfirmationWhenNotInteractive(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	mergedWorktree(r, "done")
	res := clean(t, r.Main, "y\n", cleanOptions{})
	if res.code != 1 {
		t.Errorf("want exit 1, got %d", res.code)
	}
	if !strings.Contains(res.errOut, "--yes") {
		t.Errorf("should say confirmation is needed")
	}
	if !branchExists(r, "done") {
		t.Errorf("deleted without confirmation")
	}
}

func TestCleanReverifiesBeforeDeleting(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	beforeDelete = func(Item) {
		_ = os.WriteFile(filepath.Join(done, "late.txt"), []byte("x"), 0o644)
	}
	t.Cleanup(func() { beforeDelete = nil })

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if !branchExists(r, "done") || !exists(done) {
		t.Errorf("worktree dirtied after the check was still removed")
	}
	if !strings.Contains(res.out, "uncommitted") {
		t.Errorf("output doesn't say why done was skipped")
	}
}

func TestMergeErrorIsNeverSafe(t *testing.T) {
	b := inventory.Branch{Name: "x"}
	f := status.Facts{OwnCommits: true, Merged: true, Base: "origin/main"}
	it := classify(b, f, errors.New("boom"))
	if it.State == status.SafeToDelete {
		t.Errorf("merge check error gave SafeToDelete")
	}
	if !strings.Contains(strings.Join(it.Reasons, "; "), "couldn't check if merged") {
		t.Errorf("reasons %v lack the merge-check note", it.Reasons)
	}
}

func TestCleanSkipsShellWorktreeWithRepoDir(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	sub := filepath.Join(done, "sub dir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// As in `trees -C <main> clean` run from inside done's worktree.
	res := clean(t, r.Main, "", cleanOptions{Yes: true, ShellDir: sub})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if !branchExists(r, "done") || !exists(done) {
		t.Errorf("the shell's worktree was removed")
	}
	if !strings.Contains(res.out, "you're in its worktree") {
		t.Errorf("output doesn't say why done was skipped")
	}
}

func TestCleanKeepsUntrackedWhenStatusHidesThem(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	r.Git(r.Main, "config", "status.showUntrackedFiles", "no")
	done := mergedWorktree(r, "done")
	if err := os.WriteFile(filepath.Join(done, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, items, _, err := collect(context.Background(), r.Main, true, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Branch.Name == "done" && it.State == status.SafeToDelete {
			t.Errorf("worktree with an untracked file is safe to delete: %v", it.Reasons)
		}
	}

	clean(t, r.Main, "", cleanOptions{Yes: true})
	if !branchExists(r, "done") || !exists(filepath.Join(done, "notes.txt")) {
		t.Errorf("untracked file was deleted")
	}
}

func TestListAndCleanSurviveUncheckableWorktree(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	broken := mergedWorktree(r, "broken")
	// A broken .git file makes status fail in that worktree only.
	if err := os.WriteFile(filepath.Join(broken, ".git"), []byte("gitdir: /nowhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := runList(context.Background(), r.Main, &out, &errOut, listOptions{NoPR: true}); code != 0 {
		t.Fatalf("list failed: %d\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "couldn't check this worktree") {
		t.Errorf("list doesn't say the worktree couldn't be checked:\n%s", out.String())
	}

	res := clean(t, r.Main, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if !branchExists(r, "broken") || !exists(broken) {
		t.Errorf("a worktree that couldn't be checked was removed")
	}
	if branchExists(r, "done") || exists(done) {
		t.Errorf("done should still be cleaned")
	}
}

func TestRecheckRefusesUncheckedWorktree(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	done := mergedWorktree(r, "done")
	r.Git(r.Main, "fetch")
	b := inventory.Branch{
		Name:     "done",
		SHA:      r.Git(r.Main, "rev-parse", "done"),
		Worktree: &inventory.Worktree{Path: done, Branch: "done", CheckErr: errors.New("boom")},
	}
	if why := recheck(context.Background(), git.Runner{Dir: r.Main}, "origin/main", b); why == "" {
		t.Errorf("recheck passed a worktree that couldn't be checked")
	}
}

func TestListAndCleanBareLayout(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	dir := r.BareLayout()
	feat := filepath.Join(dir, "feat")
	r.Git(dir, "worktree", "add", "-b", "feat", feat)
	r.Commit(feat, "feat.txt", "feat work")
	r.Git(feat, "push", "-u", "origin", "feat")
	wip := filepath.Join(dir, "wip")
	r.Git(dir, "worktree", "add", "-b", "wip", wip)

	// Merge feat in origin with a scratch clone.
	scratch := filepath.Join(t.TempDir(), "scratch")
	r.Git(dir, "clone", r.Origin, scratch)
	r.Git(scratch, "merge", "--no-ff", "-m", "merge feat", "origin/feat")
	r.Git(scratch, "push", "origin", "main")

	var out, errOut bytes.Buffer
	if code := runList(context.Background(), filepath.Join(dir, "main"), &out, &errOut, listOptions{NoPR: true}); code != 0 {
		t.Fatalf("list failed: %d\n%s", code, errOut.String())
	}
	t.Logf("list:\n%s", out.String())
	for _, want := range []string{"safe to delete", "wip"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output lacks %q", want)
		}
	}

	res := clean(t, dir, "", cleanOptions{Yes: true})
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if exists(feat) || r.Git(dir, "branch", "--list", "feat") != "" {
		t.Errorf("feat should be cleaned")
	}
	if !exists(wip) || !exists(filepath.Join(dir, "main")) {
		t.Errorf("wip or main worktree was removed")
	}
}

func TestCleanDisclosesIgnoredFiles(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	r.Commit(r.Main, ".gitignore", ".env\n*.log\nnotes/\n")
	r.Git(r.Main, "push", "origin", "main")
	done := mergedWorktree(r, "done")
	for _, f := range []string{".env", "a.log", "b.log", "notes/todo.md"} {
		path := filepath.Join(done, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := mergedWorktree(r, "other")
	if err := os.WriteFile(filepath.Join(other, ".env"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := clean(t, r.Main, "n\n", cleanOptions{Interactive: true})
	want := "  done  branch and folder " + done + "\n" +
		"      also removes ignored files: .env, a.log, b.log and 1 more\n"
	if !strings.Contains(res.out, want) {
		t.Errorf("missing disclosure %q", want)
	}
	if !strings.Contains(res.out, "      also removes ignored files: .env\n") {
		t.Errorf("missing disclosure for other")
	}
	if strings.Index(res.out, "also removes") > strings.Index(res.out, "Delete these") {
		t.Errorf("disclosure must come before the prompt")
	}
	if !branchExists(r, "done") || !exists(filepath.Join(done, ".env")) {
		t.Errorf("answering no deleted something")
	}
}
