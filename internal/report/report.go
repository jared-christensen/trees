// Package report renders the list of worktrees and branches.
package report

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jared-christensen/trees/internal/inventory"
	"github.com/jared-christensen/trees/internal/status"
)

// Item is one classified branch.
type Item struct {
	Branch  inventory.Branch
	State   status.State
	Reasons []string
}

const (
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
	off    = "\x1b[0m"
)

type painter bool

func (p painter) paint(code, s string) string {
	if !p {
		return s
	}
	return code + s + off
}

func stateColor(s status.State) string {
	switch s {
	case status.SafeToDelete:
		return green
	case status.Stale:
		return yellow
	default:
		return blue
	}
}

// Render writes the list for inv and items to w. Color codes are written only
// when color is true.
func Render(w io.Writer, inv *inventory.Inventory, items []Item, color bool) {
	p := painter(color)
	repo := filepath.Base(inv.Main)
	defaultName := strings.TrimPrefix(inv.Base, "origin/")

	fmt.Fprintf(w, "%s  %s, compared to %s\n", p.paint(bold, repo), Tilde(inv.Main), inv.Base)
	if inv.FetchErr != nil {
		fmt.Fprintln(w, p.paint(yellow, "Couldn't reach the remote, so this uses what was last fetched."))
	}
	if !inv.HasRemote {
		fmt.Fprintf(w, "No remote, comparing to local %s.\n", inv.Base)
	}
	if note := mainNote(inv, defaultName); note != "" {
		fmt.Fprintln(w, p.paint(yellow, note))
	}

	var withWT, without []Item
	for _, it := range items {
		if it.Branch.Worktree != nil {
			withWT = append(withWT, it)
		} else {
			without = append(without, it)
		}
	}
	sortItems(withWT)
	sortItems(without)

	if len(withWT) > 0 {
		fmt.Fprintf(w, "\n%s\n", p.paint(bold, "Worktrees"))
		for _, it := range withWT {
			wt := it.Branch.Worktree
			name := Folder(inv.Main, wt.Path) + suffixes(*wt)
			entry(w, p, it, name)
			if folderName(inv.Main, wt.Path) != sanitize(it.Branch.Name) {
				fmt.Fprintf(w, "      branch %s\n", it.Branch.Name)
			}
			reasons(w, p, it.Reasons)
		}
	}
	if len(without) > 0 {
		fmt.Fprintf(w, "\n%s\n", p.paint(bold, "Branches not in any worktree"))
		for _, it := range without {
			entry(w, p, it, it.Branch.Name)
			reasons(w, p, it.Reasons)
		}
	}
	if len(inv.Detached) > 0 {
		fmt.Fprintf(w, "\n%s\n", p.paint(bold, "Worktrees with no branch checked out"))
		for _, wt := range inv.Detached {
			head := wt.Head
			if len(head) > 7 {
				head = head[:7]
			}
			fmt.Fprintf(w, "  %s%s  at %s\n", Folder(inv.Main, wt.Path), suffixes(wt), head)
			if wt.Dirty > 0 {
				reasons(w, p, []string{plural(wt.Dirty, "uncommitted file", "uncommitted files")})
			}
		}
	}

	safe := 0
	for _, it := range items {
		if it.State == status.SafeToDelete {
			safe++
		}
	}
	fmt.Fprintln(w)
	if safe == 0 {
		fmt.Fprintln(w, "Nothing is safe to delete.")
	} else {
		fmt.Fprintf(w, "%s Run trees clean to remove them.\n", p.paint(green, fmt.Sprintf("%d safe to delete.", safe)))
	}
}

func entry(w io.Writer, p painter, it Item, name string) {
	fmt.Fprintf(w, "  %s  %s  %s\n",
		p.paint(stateColor(it.State), "● "+it.State.String()),
		name,
		p.paint(dim, Age(it.Branch.LastCommit, time.Now())))
}

// reasons prints the reasons on one line; uncommitted changes are called out
// in red because they block deletion.
func reasons(w io.Writer, p painter, why []string) {
	if len(why) == 0 {
		return
	}
	parts := make([]string, len(why))
	for i, r := range why {
		if strings.Contains(r, "uncommitted") {
			parts[i] = p.paint(red+bold, r)
		} else {
			parts[i] = r
		}
	}
	fmt.Fprintf(w, "      %s\n", strings.Join(parts, "; "))
}

func mainNote(inv *inventory.Inventory, defaultName string) string {
	var parts []string
	switch inv.MainBranch {
	case defaultName:
	case "":
		parts = append(parts, "is not on a branch")
	default:
		parts = append(parts, fmt.Sprintf("is on %s, not %s", inv.MainBranch, defaultName))
	}
	if inv.MainDirty > 0 {
		parts = append(parts, "has "+plural(inv.MainDirty, "uncommitted file", "uncommitted files"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "The main checkout " + strings.Join(parts, ", and ") + "."
}

func suffixes(wt inventory.Worktree) string {
	s := ""
	if wt.Missing {
		s += " (folder missing)"
	}
	if wt.Locked {
		s += " (locked)"
	}
	return s
}

// sortItems orders by state (in use, stale, safe to delete), newest first
// within a state.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].State != items[j].State {
			return items[i].State < items[j].State
		}
		return items[i].Branch.LastCommit.After(items[j].Branch.LastCommit)
	})
}

// Folder is how a worktree folder is shown: the part after "<repo>." for a
// folder beside the main checkout named "<repo>.<name>", else the full path
// with the home directory shortened to ~.
func Folder(main, path string) string {
	repo := filepath.Base(main)
	if filepath.Dir(path) == filepath.Dir(main) {
		if name, ok := strings.CutPrefix(filepath.Base(path), repo+"."); ok && name != "" {
			return name
		}
	}
	return Tilde(path)
}

// folderName is the folder's own name, without the "<repo>." prefix.
func folderName(main, path string) string {
	f := Folder(main, path)
	if f == Tilde(path) {
		return filepath.Base(path)
	}
	return f
}

func sanitize(branch string) string { return strings.ReplaceAll(branch, "/", "-") }

// Tilde replaces the home directory prefix of path with ~.
func Tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// Age is the time since t, in hours under a day and days after that.
func Age(t, now time.Time) string {
	if t.IsZero() {
		return "unknown age"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
