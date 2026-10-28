package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/gittest"
)

func TestMainCheckoutFromWorktree(t *testing.T) {
	r := gittest.New(t)
	wt := r.AddWorktree("feat-a")
	got, err := git.MainCheckout(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	if got != r.Main {
		t.Fatalf("got %q, want %q", got, r.Main)
	}
}

func TestMainCheckoutFromSubdir(t *testing.T) {
	r := gittest.New(t)
	sub := filepath.Join(r.Main, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := git.MainCheckout(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != r.Main {
		t.Fatalf("got %q, want %q", got, r.Main)
	}
}

func TestRunnerPathWithSpace(t *testing.T) {
	r := gittest.New(t)
	if _, err := (git.Runner{Dir: r.Main}).Run(context.Background(), "status", "--porcelain"); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerErrorWrapsStderr(t *testing.T) {
	r := gittest.New(t)
	_, err := git.Runner{Dir: r.Main}.Run(context.Background(), "rev-parse", "--verify", "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "fatal: Needed a single revision") {
		t.Fatalf("error does not contain git's stderr: %v", err)
	}
	if (git.Runner{Dir: r.Main}).Ok(context.Background(), "rev-parse", "--verify", "nope") {
		t.Fatal("Ok should be false")
	}
	if !(git.Runner{Dir: r.Main}).Ok(context.Background(), "rev-parse", "--verify", "main") {
		t.Fatal("Ok should be true")
	}
}

func TestRunKeepsLeadingWhitespace(t *testing.T) {
	r := gittest.New(t)
	if err := os.WriteFile(filepath.Join(r.Main, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := git.Runner{Dir: r.Main}.Run(context.Background(), "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, " M") {
		t.Fatalf("got %q, want prefix %q", out, " M")
	}
}
