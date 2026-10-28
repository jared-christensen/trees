// Package cli wires up the trees command line.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// exitCode is returned from a command whose messages are already printed; it
// carries the process exit code.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func asError(code int) error {
	if code == 0 {
		return nil
	}
	return exitCode(code)
}

// streams are the command's standard input and output.
type streams struct {
	in          io.Reader
	out, errOut io.Writer
	// isTerminal reports whether the reader or writer is a terminal.
	isTerminal func(any) bool
}

func newRootCmd(version string, s streams) *cobra.Command {
	var dir string
	var listOpt listOptions

	// workDir is -C if given (relative to the current directory), else the
	// current directory.
	workDir := func() (string, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		if dir == "" {
			return wd, nil
		}
		path := dir
		if !filepath.IsAbs(path) {
			path = filepath.Join(wd, path)
		}
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("-C %s: no such directory", dir)
		}
		return path, nil
	}

	runListCmd := func(cmd *cobra.Command, _ []string) error {
		cwd, err := workDir()
		if err != nil {
			return err
		}
		opt := listOpt
		opt.Color = s.isTerminal(s.out) && os.Getenv("NO_COLOR") == ""
		return asError(runList(cmd.Context(), cwd, s.out, s.errOut, opt))
	}
	addListFlags := func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&listOpt.NoFetch, "no-fetch", false, "use what was last fetched instead of fetching")
		cmd.Flags().BoolVar(&listOpt.NoPR, "no-pr", false, "skip pull request lookups")
	}

	root := &cobra.Command{
		Use:           "trees",
		Short:         "List, clean, create and switch git worktrees",
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runListCmd,
	}
	root.SetIn(s.in)
	root.SetOut(s.out)
	root.SetErr(s.errOut)
	root.PersistentFlags().StringVarP(&dir, "dir", "C", "", "run as if trees was started in `path`")
	addListFlags(root)

	list := &cobra.Command{
		Use:   "list",
		Short: "Show worktrees and branches and which are safe to delete",
		Args:  cobra.NoArgs,
		RunE:  runListCmd,
	}
	addListFlags(list)

	var cleanOpt cleanOptions
	clean := &cobra.Command{
		Use:   "clean",
		Short: "Delete worktrees and branches whose work is already merged",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := workDir()
			if err != nil {
				return err
			}
			opt := cleanOpt
			opt.Interactive = s.isTerminal(s.in)
			if opt.ShellDir, err = os.Getwd(); err != nil {
				return err
			}
			return asError(runClean(cmd.Context(), cwd, s.in, s.out, s.errOut, opt))
		},
	}
	clean.Flags().BoolVarP(&cleanOpt.Yes, "yes", "y", false, "delete without asking")
	clean.Flags().BoolVar(&cleanOpt.NoPR, "no-pr", false, "skip pull request lookups")

	root.AddCommand(list, clean)
	return root
}

// isTerminal reports whether v is an *os.File connected to a terminal.
func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Execute runs the root command and returns the process exit code.
func Execute(version string) int {
	s := streams{in: os.Stdin, out: os.Stdout, errOut: os.Stderr, isTerminal: isTerminal}
	if err := newRootCmd(version, s).Execute(); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			return int(code)
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
