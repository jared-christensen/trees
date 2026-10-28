// Package shell provides the shell function that lets `trees switch` change
// the calling shell's directory, and the hand-off the binary uses to ask for it.
package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// body is the same for zsh and bash. The binary writes the directory to a
// temp file named by TREES_CD_FILE (a child process can't cd its parent); the
// function reads it, removes the file straight away (so an interrupted run
// leaks nothing), and cds. The binary's exit code is preserved.
const body = `trees() {
  local _trees_f _trees_rc _trees_dir
  _trees_f=$(mktemp "${TMPDIR:-/tmp}/trees-cd.XXXXXX") || { command trees "$@"; return $?; }
  TREES_CD_FILE="$_trees_f" command trees "$@"
  _trees_rc=$?
  _trees_dir=
  if [ -s "$_trees_f" ]; then
    _trees_dir=$(cat "$_trees_f")
  fi
  rm -f "$_trees_f"
  if [ -n "$_trees_dir" ] && [ -d "$_trees_dir" ]; then
    builtin cd "$_trees_dir" || return $?
  fi
  return $_trees_rc
}
`

// Script returns the shell function for the named shell ("zsh" or "bash").
func Script(name string) (string, error) {
	switch name {
	case "zsh", "bash":
		return body, nil
	}
	return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash)", name)
}

// Cd asks the calling shell to change to path. With TREES_CD_FILE set (by the
// shell function) it writes the path there; otherwise it prints how to get
// there to stderr.
func Cd(path string) error { return cdTo(path, os.Stderr) }

func cdTo(path string, errOut io.Writer) error {
	if f := os.Getenv("TREES_CD_FILE"); f != "" {
		return os.WriteFile(f, []byte(path), 0o600)
	}
	quoted := "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
	fmt.Fprintf(errOut, "Run this to go there: cd %s\n", quoted)
	fmt.Fprintln(errOut, `Tip: add eval "$(trees init zsh)" to your shell config so switch can change directories.`)
	return nil
}
