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

// WorktreePath fills in tmpl for branch in the repository whose main checkout
// is repoMain, expanding a leading "~".
func WorktreePath(tmpl, repoMain, branch string) string {
	p := strings.NewReplacer(
		"{repo_parent}", filepath.Dir(repoMain),
		"{repo}", filepath.Base(repoMain),
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
