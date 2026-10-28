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
	repo := repoName(inv)
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
		if inMain(inv, it) {
			continue // covered by the main checkout note
		}
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
			name := Folder(inv.Main, inv.Bare, wt.Path) + suffixes(*wt)
			entry(w, p, it, name)
			if folderName(inv.Main, inv.Bare, wt.Path) != sanitize(it.Branch.Name) {
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
			fmt.Fprintf(w, "  %s%s  at %s\n", Folder(inv.Main, inv.Bare, wt.Path), suffixes(wt), head)
			switch {
			case wt.CheckErr != nil:
				reasons(w, p, []string{"couldn't check this worktree"})
			case wt.Dirty > 0:
				reasons(w, p, []string{plural(wt.Dirty, "uncommitted file", "uncommitted files")})
			}
		}
	}

	safe := cleanable(inv, items)
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

// cleanable counts what clean would remove: it leaves the main checkout's
// branch and locked worktrees alone.
func cleanable(inv *inventory.Inventory, items []Item) int {
	n := 0
	for _, it := range items {
		locked := it.Branch.Worktree != nil && it.Branch.Worktree.Locked
		if it.State == status.SafeToDelete && !inMain(inv, it) && !locked {
			n++
		}
	}
	return n
}

// stateKey is the state's name in JSON output.
func stateKey(s status.State) string {
	switch s {
	case status.Stale:
		return "stale"
	case status.SafeToDelete:
		return "safe_to_delete"
	default:
		return "in_use"
	}
}

// inMain reports whether the item's branch is checked out in the main checkout.
func inMain(inv *inventory.Inventory, it Item) bool {
	wt := it.Branch.Worktree
	return wt != nil && filepath.Clean(wt.Path) == filepath.Clean(inv.Main)
}

// repoName is the repository's name: the main checkout's folder, or for a
// bare layout like <repo>/.bare, the folder holding it.
func repoName(inv *inventory.Inventory) string {
	name := filepath.Base(inv.Main)
	if inv.Bare && strings.HasPrefix(name, ".") {
		return filepath.Base(filepath.Dir(inv.Main))
	}
	return name
}

func mainNote(inv *inventory.Inventory, defaultName string) string {
	if inv.Bare {
		return "" // no main checkout
	}
	var parts []string
	switch inv.MainBranch {
	case defaultName:
	case "":
		parts = append(parts, "is not on a branch")
	default:
		parts = append(parts, fmt.Sprintf("is on %s, not %s", inv.MainBranch, defaultName))
	}
	if inv.MainCheckErr != nil {
		parts = append(parts, "couldn't be checked for uncommitted files")
	} else if inv.MainDirty > 0 {
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
// folder beside the main checkout named "<repo>.<name>"; the folder name for
// one beside .bare in a bare layout; "<name> (Claude)" for one Claude Code
// made under .claude/worktrees; the path from the main checkout for any other
// folder inside it; else the full path with the home directory shortened to ~.
func Folder(main string, bare bool, path string) string {
	name, claude, ok := shortName(main, bare, path)
	switch {
	case claude:
		return name + " (Claude)"
	case ok:
		return name
	}
	if rel, err := filepath.Rel(main, path); err == nil && !bare && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return Tilde(path)
}

// folderName is the folder's own name, without the "<repo>." prefix.
func folderName(main string, bare bool, path string) string {
	if name, _, ok := shortName(main, bare, path); ok {
		return name
	}
	return filepath.Base(path)
}

// shortName is the name trees shows for a folder in one of the known layouts.
func shortName(main string, bare bool, path string) (name string, claude, ok bool) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	if dir == filepath.Dir(main) {
		if bare {
			return base, false, true
		}
		if n, ok := strings.CutPrefix(base, filepath.Base(main)+"."); ok && n != "" {
			return n, false, true
		}
	}
	if !bare && dir == filepath.Join(main, ".claude", "worktrees") {
		return base, true, true
	}
	return "", false, false
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
