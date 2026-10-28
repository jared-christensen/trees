package cli

import (
	"fmt"
	"io"

	"github.com/jared-christensen/trees/internal/shell"
)

// runInit prints the shell function for the named shell and returns the exit code.
func runInit(name string, out, errOut io.Writer) int {
	s, err := shell.Script(name)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	fmt.Fprint(out, s)
	return 0
}
