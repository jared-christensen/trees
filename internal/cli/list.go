package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/jared-christensen/trees/internal/git"
	"github.com/jared-christensen/trees/internal/report"
)

type listOptions struct {
	NoFetch bool
	NoPR    bool
	Color   bool
	JSON    bool
}

// runList prints the report for the repository containing cwd and returns
// the exit code.
func runList(ctx context.Context, cwd string, out, errOut io.Writer, opt listOptions) int {
	if err := git.CheckVersion(ctx); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	inv, items, _, err := collect(ctx, cwd, !opt.NoFetch, !opt.NoPR)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if opt.JSON {
		if err := report.JSON(out, inv, items); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}
	report.Render(out, inv, items, opt.Color)
	return 0
}
