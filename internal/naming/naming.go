// Package naming turns branch names into worktree folders and checks them
// against the user's name pattern.
package naming

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FolderName is branch with "/" replaced by "-".
func FolderName(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

// DefaultPath is the built-in worktree location: beside the main checkout.
const DefaultPath = "{repo_parent}/{repo}.{name}"

// WorktreePath fills in tmpl for branch in the repository whose main checkout
// is repoMain, expanding a leading "~". In a bare layout (repoMain is
// <repo>/.bare) the repository is the folder holding it, and the default
// location puts worktrees inside that folder, beside .bare.
func WorktreePath(tmpl, repoMain string, bare bool, branch string) string {
	root := repoMain
	if bare {
		root = filepath.Dir(repoMain)
		if tmpl == DefaultPath {
			return filepath.Join(root, FolderName(branch))
		}
	}
	p := strings.NewReplacer(
		"{repo_parent}", filepath.Dir(root),
		"{repo}", filepath.Base(root),
		"{name}", FolderName(branch),
	).Replace(tmpl)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return p
}

// Check tests branch against pattern. A mismatch is a warning in "warn" mode
// and an error in "require" mode; an empty pattern accepts everything.
func Check(pattern, mode, branch string) (warning string, err error) {
	if pattern == "" {
		return "", nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("name_pattern %q is not a valid regular expression: %w", pattern, err)
	}
	if re.MatchString(branch) {
		return "", nil
	}
	msg := fmt.Sprintf("%q doesn't match your name pattern %s.", branch, pattern)
	if mode == "require" {
		return "", errors.New(msg)
	}
	return msg, nil
}
