package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptZshBash(t *testing.T) {
	for _, name := range []string{"zsh", "bash"} {
		t.Run(name, func(t *testing.T) {
			s, err := Script(name)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"TREES_CD_FILE", "command trees"} {
				if !strings.Contains(s, want) {
					t.Errorf("script lacks %q", want)
				}
			}
			bin, err := exec.LookPath(name)
			if err != nil {
				t.Skipf("%s not installed", name)
			}
			cmd := exec.Command(bin, "-n")
			cmd.Stdin = strings.NewReader(s)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s -n: %v\n%s", name, err, out)
			}
		})
	}
}

func TestScriptUnknownShell(t *testing.T) {
	_, err := Script("fish")
	if err == nil || !strings.Contains(err.Error(), "zsh") || !strings.Contains(err.Error(), "bash") {
		t.Fatalf("err = %v, want one listing supported shells", err)
	}
}

func TestCdWritesFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cd")
	t.Setenv("TREES_CD_FILE", f)
	if err := Cd("/a b/it's"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "/a b/it's" {
		t.Errorf("file = %q", got)
	}
}

func TestCdHint(t *testing.T) {
	t.Setenv("TREES_CD_FILE", "")
	var b strings.Builder
	if err := cdTo("/a b/it's", &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `Run this to go there: cd '/a b/it'\''s'`) {
		t.Errorf("hint missing or unescaped:\n%s", out)
	}
	if !strings.Contains(out, `Tip: add eval "$(trees init zsh)" to your shell config so switch can change directories.`) {
		t.Errorf("tip missing:\n%s", out)
	}
}
