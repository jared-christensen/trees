// Package gittest builds throwaway git repositories for tests.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo is a bare origin plus a clone of it with one commit on main. Both live
// under a directory with a space in its name so path handling is exercised.
type Repo struct {
	Origin string
	Main   string

	t    *testing.T
	root string
}

// New creates the origin and main clone under t.TempDir().
func New(t *testing.T) *Repo {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "has space")
	r := &Repo{
		t:      t,
		root:   root,
		Origin: filepath.Join(root, "origin.git"),
		Main:   filepath.Join(root, "main"),
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r.Git(root, "init", "--bare", r.Origin)
	r.Git(root, "clone", r.Origin, r.Main)
	r.Git(r.Main, "checkout", "-B", "main")
	r.Commit(r.Main, "README.md", "initial commit")
	r.Git(r.Main, "push", "-u", "origin", "main")
	r.Git(r.Origin, "symbolic-ref", "HEAD", "refs/heads/main")
	return r
}

// Git runs git in dir and returns trimmed output, failing the test on error.
// User identity, default branch and signing are pinned so tests do not depend
// on the machine's git configuration.
func (r *Repo) Git(dir string, args ...string) string {
	r.t.Helper()
	full := append([]string{
		"-c", "user.name=Test User",
		"-c", "user.email=test@example.com",
		"-c", "init.defaultBranch=main",
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Commit writes file (content is the message) in dir, commits it and returns
// the new commit's sha.
func (r *Repo) Commit(dir, file, msg string) string {
	r.t.Helper()
	path := filepath.Join(dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(msg+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.Git(dir, "add", file)
	r.Git(dir, "commit", "-m", msg)
	return r.Git(dir, "rev-parse", "HEAD")
}

// AddWorktree creates branch and a sibling worktree of Main for it, and
// returns the worktree path.
func (r *Repo) AddWorktree(branch string) string {
	r.t.Helper()
	path := filepath.Join(r.root, branch)
	r.Git(r.Main, "worktree", "add", "-b", branch, path)
	return path
}

// SquashMerge squash-merges branch (from Main) into main in the origin, using
// a scratch clone, and pushes the result. Main is not updated.
func (r *Repo) SquashMerge(branch string) {
	r.t.Helper()
	scratch := filepath.Join(r.root, "scratch-"+branch)
	r.Git(r.root, "clone", r.Origin, scratch)
	r.Git(scratch, "checkout", "main")
	r.Git(scratch, "fetch", r.Main, branch)
	r.Git(scratch, "merge", "--squash", "FETCH_HEAD")
	r.Git(scratch, "commit", "-m", "squash "+branch)
	r.Git(scratch, "push", "origin", "main")
}

// Merge merges branch (from Main) into main in the origin with --no-ff, using
// a scratch clone, and pushes the result. Main is not updated.
func (r *Repo) Merge(branch string) {
	r.t.Helper()
	scratch := filepath.Join(r.root, "merge-"+branch)
	r.Git(r.root, "clone", r.Origin, scratch)
	r.Git(scratch, "checkout", "main")
	r.Git(scratch, "fetch", r.Main, branch)
	r.Git(scratch, "merge", "--no-ff", "-m", "merge "+branch, "FETCH_HEAD")
	r.Git(scratch, "push", "origin", "main")
}
