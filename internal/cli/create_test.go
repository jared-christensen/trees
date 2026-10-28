package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jared-christensen/trees/internal/gittest"
)

func create(t *testing.T, cwd, branch string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runCreate(context.Background(), cwd, branch, strings.NewReader(""), &out, &errOut)
	t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	return result{code, out.String(), errOut.String()}
}

func cdFileEnv(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "cd")
	t.Setenv("TREES_CD_FILE", f)
	return f
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "trees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateNewBranchFromBase(t *testing.T) {
	isolate(t)
	cd := cdFileEnv(t)
	r := gittest.New(t)
	// Advance origin/main without fetching in Main: local refs are behind.
	r.AddWorktree("other")
	r.Commit(filepath.Join(filepath.Dir(r.Main), "other"), "o.txt", "other work")
	r.SquashMerge("other")
	res := create(t, r.Main, "feat/new")
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	path := filepath.Join(filepath.Dir(r.Main), "main.feat-new")
	if got := readFile(t, cd); got != path {
		t.Errorf("cd = %q, want %q", got, path)
	}
	if want := "Created feat-new on branch feat/new from origin/main."; !strings.Contains(res.out, want) {
		t.Errorf("stdout = %q", res.out)
	}
	tip := r.Git(r.Main, "rev-parse", "origin/main")
	if parent := r.Git(r.Main, "rev-parse", "feat/new"); parent != tip {
		t.Errorf("branch at %s, want origin/main %s", parent, tip)
	}
	if up := r.Git(r.Main, "for-each-ref", "--format=%(upstream)", "refs/heads/feat/new"); up != "" {
		t.Errorf("new branch tracks %q", up)
	}
}

func TestCreateExistingLocal(t *testing.T) {
	isolate(t)
	cdFileEnv(t)
	r := gittest.New(t)
	r.Git(r.Main, "branch", "loc")
	if res := create(t, r.Main, "loc"); res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	path := filepath.Join(filepath.Dir(r.Main), "main.loc")
	if got := r.Git(path, "rev-parse", "--abbrev-ref", "HEAD"); got != "loc" {
		t.Errorf("HEAD = %s", got)
	}
}

func TestCreateFromRemoteBranch(t *testing.T) {
	isolate(t)
	cdFileEnv(t)
	r := gittest.New(t)
	r.Git(r.Main, "branch", "rem")
	r.Git(r.Main, "push", "origin", "rem")
	r.Git(r.Main, "branch", "-D", "rem")
	r.Git(r.Main, "fetch")
	if res := create(t, r.Main, "rem"); res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if got := r.Git(r.Main, "config", "--get", "branch.rem.remote"); got != "origin" {
		t.Errorf("remote = %q", got)
	}
	if got := r.Git(r.Main, "config", "--get", "branch.rem.merge"); got != "refs/heads/rem" {
		t.Errorf("merge = %q", got)
	}
}

func TestCreateAlreadyCheckedOut(t *testing.T) {
	isolate(t)
	cd := cdFileEnv(t)
	r := gittest.New(t)
	existing := r.AddWorktree("here")
	res := create(t, r.Main, "here")
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if got := readFile(t, cd); got != existing {
		t.Errorf("cd = %q, want %q", got, existing)
	}
	if !strings.Contains(res.out, "here is already in "+existing+".") {
		t.Errorf("stdout = %q", res.out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(r.Main), "main.here")); err == nil {
		t.Error("created a second worktree")
	}
}

func TestCreateRunsSetup(t *testing.T) {
	isolate(t)
	cdFileEnv(t)
	writeConfig(t, "[defaults]\nsetup = ['touch ready']\n")
	r := gittest.New(t)
	res := create(t, r.Main, "s1")
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(r.Main), "main.s1", "ready")); err != nil {
		t.Error(err)
	}
	if !strings.Contains(res.out, "Running setup: touch ready") {
		t.Errorf("stdout = %q", res.out)
	}
}

func TestCreateSetupFailureKeepsWorktree(t *testing.T) {
	isolate(t)
	cd := cdFileEnv(t)
	writeConfig(t, "[defaults]\nsetup = ['exit 3', 'touch never']\n")
	r := gittest.New(t)
	res := create(t, r.Main, "s2")
	path := filepath.Join(filepath.Dir(r.Main), "main.s2")
	if res.code != 1 {
		t.Errorf("exit %d", res.code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("worktree removed")
	}
	if _, err := os.Stat(filepath.Join(path, "never")); err == nil {
		t.Error("ran past the failure")
	}
	if !strings.Contains(res.errOut, "Setup failed: exit 3") || !strings.Contains(res.errOut, path) {
		t.Errorf("stderr = %q", res.errOut)
	}
	if got := readFile(t, cd); got != path {
		t.Errorf("cd = %q", got)
	}
}

func TestCreateRejects(t *testing.T) {
	isolate(t)
	cdFileEnv(t)
	r := gittest.New(t)

	res := create(t, r.Main, "bad..name")
	if res.code != 1 || !strings.Contains(res.errOut, "bad..name") {
		t.Errorf("invalid name: %+v", res)
	}

	path := filepath.Join(filepath.Dir(r.Main), "main.taken")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	res = create(t, r.Main, "taken")
	if res.code != 1 || !strings.Contains(res.errOut, path+" already exists.") {
		t.Errorf("exists: %+v", res)
	}
	if r.Git(r.Main, "branch", "--list", "taken") != "" {
		t.Error("branch was created")
	}

	writeConfig(t, "[defaults]\nname_pattern = '^ABC-'\nname_pattern_mode = 'require'\n")
	res = create(t, r.Main, "nope")
	if res.code != 1 || !strings.Contains(res.errOut, `"nope" doesn't match your name pattern ^ABC-.`) {
		t.Errorf("require: %+v", res)
	}
	writeConfig(t, "[defaults]\nname_pattern = '^ABC-'\n")
	res = create(t, r.Main, "warnme")
	if res.code != 0 || !strings.Contains(res.errOut, `"warnme" doesn't match your name pattern ^ABC-.`) {
		t.Errorf("warn: %+v", res)
	}
}

func TestCreateBareLayout(t *testing.T) {
	isolate(t)
	cd := cdFileEnv(t)
	r := gittest.New(t)
	dir := r.BareLayout()
	res := create(t, filepath.Join(dir, "main"), "feat/x")
	if res.code != 0 {
		t.Fatalf("exit %d", res.code)
	}
	want := filepath.Join(dir, "feat-x")
	if got := readFile(t, cd); got != want {
		t.Errorf("cd = %q, want %q (beside .bare, not .bare.feat-x)", got, want)
	}
	if !strings.Contains(res.out, "Created feat-x on branch feat/x") {
		t.Errorf("want short name in output, got %q", res.out)
	}
}
