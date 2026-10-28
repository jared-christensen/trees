package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jared-christensen/trees/internal/gittest"
	"github.com/jared-christensen/trees/internal/inventory"
)

func wtNames(wts []inventory.Worktree) []string {
	var n []string
	for _, w := range wts {
		n = append(n, filepath.Base(w.Path))
	}
	return n
}

func TestFindWorktree(t *testing.T) {
	inv := &inventory.Inventory{
		Main:       "/p/repo",
		MainBranch: "main",
		Branches: []inventory.Branch{
			{Name: "feat/login", Worktree: &inventory.Worktree{Path: "/p/repo.login", Branch: "feat/login"}},
			{Name: "login-fix", Worktree: &inventory.Worktree{Path: "/p/repo.login-fix", Branch: "login-fix"}},
			{Name: "logins", Worktree: &inventory.Worktree{Path: "/p/repo.logins", Branch: "logins"}},
			{Name: "gone", Worktree: &inventory.Worktree{Path: "/p/repo.gone", Branch: "gone", Missing: true}},
			{Name: "nowt"},
		},
		Detached: []inventory.Worktree{{Path: "/p/repo.scratch"}},
	}
	check := func(q string, want ...string) {
		t.Helper()
		got := wtNames(findWorktree(inv, q))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("findWorktree(%q) = %v, want %v", q, got, want)
		}
	}
	check("login", "repo.login")         // exact short name beats substrings
	check("login-fix", "repo.login-fix") // exact branch
	check("feat/login", "repo.login")    // exact branch with slash
	check("repo.login-fix", "repo.login-fix")
	check("repo", "repo")              // main by repo name
	check("LOGIN-F", "repo.login-fix") // case-insensitive substring
	check("scr", "repo.scratch")
	check("gone") // missing skipped
	check("nothing")
	check("ogi", "repo.login", "repo.login-fix", "repo.logins")
}

func TestSwitchCommand(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	a := r.AddWorktree("alpha")
	r.AddWorktree("alphabet")
	cdFile := filepath.Join(t.TempDir(), "cd")
	t.Setenv("TREES_CD_FILE", cdFile)

	run := func(q string) result {
		var out, errOut bytes.Buffer
		code := runSwitch(context.Background(), r.Main, q, &out, &errOut)
		return result{code, out.String(), errOut.String()}
	}

	if res := run("alpha"); res.code != 0 {
		t.Fatalf("alpha: %+v", res)
	}
	if got, _ := os.ReadFile(cdFile); string(got) != a {
		t.Errorf("cd file = %q, want %q", got, a)
	}

	res := run("alph")
	if res.code != 1 || !strings.Contains(res.errOut, `Several worktrees match "alph":`) ||
		!strings.Contains(res.errOut, "  alpha  (branch alpha)") || !strings.Contains(res.errOut, "  alphabet  (branch alphabet)") {
		t.Errorf("ambiguous: %+v", res)
	}

	res = run("zzz")
	if res.code != 1 || !strings.Contains(res.errOut, `No worktree matches "zzz".`) {
		t.Errorf("none: %+v", res)
	}
}

func TestSwitchAfterSwitch(t *testing.T) {
	isolate(t)
	cfgDir := os.Getenv("XDG_CONFIG_HOME")
	r := gittest.New(t)
	a := r.AddWorktree("alpha")
	t.Setenv("TREES_CD_FILE", filepath.Join(t.TempDir(), "cd"))
	marker := filepath.Join(t.TempDir(), "marker")
	writeCfg := func(cmd string) {
		if err := os.MkdirAll(filepath.Join(cfgDir, "trees"), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "[defaults]\nafter_switch = '" + cmd + "'\n"
		if err := os.WriteFile(filepath.Join(cfgDir, "trees", "config.toml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeCfg("pwd > " + marker + "; echo hello")
	var out, errOut bytes.Buffer
	if code := runSwitch(context.Background(), r.Main, "alpha", &out, &errOut); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	got, _ := os.ReadFile(marker)
	if strings.TrimSpace(string(got)) != a {
		t.Errorf("ran in %q, want %q", got, a)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("after_switch output not streamed: %q", out.String())
	}

	// A failing hook warns but still succeeds.
	writeCfg("exit 3")
	out.Reset()
	errOut.Reset()
	if code := runSwitch(context.Background(), r.Main, "alpha", &out, &errOut); code != 0 || !strings.Contains(errOut.String(), "after_switch") {
		t.Errorf("failing hook: code %d, stderr %q", code, errOut.String())
	}

	// Without the wrapper (no cd file) the hook does not run.
	t.Setenv("TREES_CD_FILE", "")
	os.Remove(marker)
	writeCfg("touch " + marker)
	out.Reset()
	errOut.Reset()
	if code := runSwitch(context.Background(), r.Main, "alpha", &out, &errOut); code != 0 {
		t.Fatalf("code %d", code)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("after_switch ran without a cd hand-off")
	}
}

func TestInitCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runInit("zsh", &out, &errOut); code != 0 || !strings.Contains(out.String(), "command trees") {
		t.Errorf("zsh: %d %q", code, errOut.String())
	}
	out.Reset()
	if code := runInit("fish", &out, &errOut); code != 1 || out.Len() != 0 {
		t.Errorf("fish: %d", code)
	}
}

func TestShellWrapperEndToEnd(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	isolate(t)
	bin := filepath.Join(t.TempDir(), "trees")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/trees")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	r := gittest.New(t)
	wt := r.AddWorktree("feature")

	script := `export PATH="$1:$PATH"; eval "$(trees init bash)"; trees switch feature -C "$2"; echo "rc=$?"; pwd; trees switch nomatch -C "$2" 2>/dev/null; echo "rc=$?"; pwd`
	cmd := exec.Command(bash, "-c", script, "bash", filepath.Dir(bin), r.Main)
	cmd.Env = append(os.Environ(), "TREES_CD_FILE=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 4 || lines[0] != "rc=0" || lines[1] != wt || lines[2] != "rc=1" || lines[3] != wt {
		t.Errorf("output:\n%s\nwant rc=0, %s, rc=1, %s", out, wt, wt)
	}
}

func TestSwitchBareLayout(t *testing.T) {
	isolate(t)
	r := gittest.New(t)
	dir := r.BareLayout()
	cdFile := filepath.Join(t.TempDir(), "cd")
	t.Setenv("TREES_CD_FILE", cdFile)

	var out, errOut bytes.Buffer
	if code := runSwitch(context.Background(), dir, "main", &out, &errOut); code != 0 {
		t.Fatalf("switch main: %d %s", code, errOut.String())
	}
	if got, _ := os.ReadFile(cdFile); string(got) != filepath.Join(dir, "main") {
		t.Errorf("cd file = %q, want the main worktree", got)
	}
	if code := runSwitch(context.Background(), dir, ".bare", &out, &errOut); code != 1 {
		t.Errorf("the bare repository is not a worktree to switch to")
	}
}
