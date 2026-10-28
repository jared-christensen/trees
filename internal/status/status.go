// Package status turns facts about a branch into a state and reasons.
package status

import (
	"fmt"
	"time"
)

// State is how safe a worktree or branch is to remove.
type State int

const (
	InUse State = iota
	Stale
	SafeToDelete
)

func (s State) String() string {
	switch s {
	case Stale:
		return "stale, take a look"
	case SafeToDelete:
		return "safe to delete"
	default:
		return "still in use"
	}
}

// Facts is everything Classify needs. PR numbers are 0 when there is none.
type Facts struct {
	CheckFailed                bool // the worktree folder couldn't be checked
	Dirty                      int
	OpenPR, MergedPR, ClosedPR int
	OwnCommits, Merged         bool
	LastCommit, Now            time.Time
	StaleAfter                 time.Duration
	Unpushed                   int
	UpstreamGone               bool
	Base                       string // name used in "merged into <base>"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Classify applies the precedence rules and returns the state with the
// reasons that apply, in display order.
func Classify(f Facts) (State, []string) {
	old := f.Now.Sub(f.LastCommit) > f.StaleAfter

	var state State
	switch {
	case f.CheckFailed || f.Dirty > 0 || f.OpenPR > 0:
		state = InUse
	case !f.OwnCommits:
		if old {
			state = Stale
		} else {
			state = InUse
		}
	case f.Merged:
		state = SafeToDelete
	case f.ClosedPR > 0 || old:
		state = Stale
	default:
		state = InUse
	}

	var why []string
	if f.CheckFailed {
		why = append(why, "couldn't check this worktree")
	}
	if f.Dirty > 0 {
		why = append(why, plural(f.Dirty, "uncommitted file", "uncommitted files"))
	}
	if f.OpenPR > 0 {
		why = append(why, fmt.Sprintf("PR #%d open", f.OpenPR))
	}
	if !f.OwnCommits {
		why = append(why, "nothing committed yet")
	}
	switch {
	case f.MergedPR > 0:
		why = append(why, fmt.Sprintf("PR #%d merged", f.MergedPR))
		if f.OwnCommits && !f.Merged {
			// The PR merged, but git sees work here that never landed.
			why = append(why, "has changes not in "+f.Base)
		}
	case f.Merged && state == SafeToDelete:
		why = append(why, "merged into "+f.Base)
	}
	if f.Merged && f.OwnCommits && state != SafeToDelete {
		why = append(why, "branch itself is already merged")
	}
	if f.ClosedPR > 0 {
		why = append(why, fmt.Sprintf("PR #%d closed", f.ClosedPR))
	}
	// Commits on a merged branch, or one whose remote branch was deleted,
	// were usually pushed once; "never pushed" would mislead.
	if f.Unpushed > 0 && !f.Merged && !f.UpstreamGone {
		why = append(why, plural(f.Unpushed, "commit", "commits")+" never pushed")
	}
	if f.UpstreamGone {
		why = append(why, "remote branch deleted")
	}
	if len(why) == 0 {
		why = []string{"not merged yet"}
	}
	return state, why
}
