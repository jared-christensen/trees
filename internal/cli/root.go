// Package cli wires up the trees command line.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:           "trees",
		Short:         "List, clean, create and switch git worktrees",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

// Execute runs the root command and returns the process exit code.
func Execute(version string) int {
	if err := newRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
