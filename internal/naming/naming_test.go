package naming

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFolderName(t *testing.T) {
	if got := FolderName("feature/x"); got != "feature-x" {
		t.Errorf("got %q", got)
	}
}

func TestWorktreePath(t *testing.T) {
	got := WorktreePath("{repo_parent}/{repo}.{name}", "/a/has space/app", "ABC-1-x")
	if want := "/a/has space/app.ABC-1-x"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	got = WorktreePath("{repo_parent}/{repo}.{name}", "/a/app", "feat/y")
	if want := "/a/app.feat-y"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	home, _ := os.UserHomeDir()
	got = WorktreePath("~/wt/{repo}-{name}", "/a/app", "z")
	if want := filepath.Join(home, "wt", "app-z"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheck(t *testing.T) {
	if w, err := Check("", "require", "anything"); w != "" || err != nil {
		t.Errorf("empty pattern: %q %v", w, err)
	}
	if w, err := Check(`^ABC-\d+`, "warn", "ABC-12-x"); w != "" || err != nil {
		t.Errorf("match: %q %v", w, err)
	}
	want := `"nope" doesn't match your name pattern ^ABC-\d+.`
	if w, err := Check(`^ABC-\d+`, "warn", "nope"); w != want || err != nil {
		t.Errorf("warn: %q %v", w, err)
	}
	w, err := Check(`^ABC-\d+`, "require", "nope")
	if w != "" || err == nil || err.Error() != want {
		t.Errorf("require: %q %v", w, err)
	}
	if _, err := Check("(", "warn", "x"); err == nil || !strings.Contains(err.Error(), "name_pattern") {
		t.Errorf("bad regex: %v", err)
	}
}
