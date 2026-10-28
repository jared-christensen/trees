// Package git runs the git binary and discovers repositories.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Runner runs git commands in Dir (the current directory when empty).
type Runner struct{ Dir string }

// Run runs git with args and returns trimmed stdout. A failure returns an
// error that wraps git's stderr.
func (r Runner) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Ok reports whether git exits with code 0.
func (r Runner) Ok(ctx context.Context, args ...string) bool {
	_, err := r.Run(ctx, args...)
	return err == nil
}

// MainCheckout returns the absolute path of the main checkout of the
// repository containing dir, whether dir is in the main checkout, a
// subdirectory of it, or a linked worktree.
func MainCheckout(ctx context.Context, dir string) (string, error) {
	common, err := Runner{Dir: dir}.Run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), nil
	}
	return common, nil
}

const (
	minMajor = 2
	minMinor = 38
)

// CheckVersion fails when the installed git is older than 2.38.
func CheckVersion(ctx context.Context) error {
	out, err := Runner{}.Run(ctx, "--version")
	if err != nil {
		return fmt.Errorf("trees needs git 2.38 or newer, but git could not be run: %w", err)
	}
	major, minor, patch, err := parseVersion(out)
	if err != nil {
		return err
	}
	if !atLeast(major, minor, patch, minMajor, minMinor, 0) {
		return fmt.Errorf("trees needs git 2.38 or newer (you have %d.%d.%d).", major, minor, patch)
	}
	return nil
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// parseVersion extracts major, minor and patch from `git --version` output.
func parseVersion(s string) (major, minor, patch int, err error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("cannot parse git version from %q", s)
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	return major, minor, patch, nil
}

// atLeast reports whether major.minor.patch >= wantMajor.wantMinor.wantPatch.
func atLeast(major, minor, patch, wantMajor, wantMinor, wantPatch int) bool {
	if major != wantMajor {
		return major > wantMajor
	}
	if minor != wantMinor {
		return minor > wantMinor
	}
	return patch >= wantPatch
}
