package status_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/jared-christensen/trees/internal/status"
)

func TestClassify(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	base := func(f status.Facts) status.Facts {
		f.Now = now
		f.StaleAfter = 14 * day
		f.Base = "origin/main"
		if f.LastCommit.IsZero() {
			f.LastCommit = now.Add(-day)
		}
		return f
	}
	tests := []struct {
		name  string
		f     status.Facts
		state status.State
		why   []string
	}{
		{"dirty", base(status.Facts{Dirty: 2, OwnCommits: true}), status.InUse,
			[]string{"2 uncommitted files"}},
		{"one dirty file", base(status.Facts{Dirty: 1, OwnCommits: true}), status.InUse,
			[]string{"1 uncommitted file"}},
		{"dirty and merged", base(status.Facts{Dirty: 1, OwnCommits: true, Merged: true}), status.InUse,
			[]string{"1 uncommitted file", "branch itself is already merged"}},
		{"open PR", base(status.Facts{OpenPR: 7, OwnCommits: true}), status.InUse,
			[]string{"PR #7 open"}},
		{"open PR and merged", base(status.Facts{OpenPR: 7, MergedPR: 7, OwnCommits: true, Merged: true}), status.InUse,
			[]string{"PR #7 open", "PR #7 merged", "branch itself is already merged"}},
		{"no own commits fresh", base(status.Facts{}), status.InUse,
			[]string{"nothing committed yet"}},
		{"no own commits old", base(status.Facts{LastCommit: now.Add(-15 * day)}), status.Stale,
			[]string{"nothing committed yet"}},
		{"no own commits merged never safe", base(status.Facts{Merged: true, LastCommit: now.Add(-15 * day)}), status.Stale,
			[]string{"nothing committed yet"}},
		{"no own commits merged dirty", base(status.Facts{Merged: true, Dirty: 3}), status.InUse,
			[]string{"3 uncommitted files", "nothing committed yet"}},
		{"merged clean via PR", base(status.Facts{OwnCommits: true, Merged: true, MergedPR: 9}), status.SafeToDelete,
			[]string{"PR #9 merged"}},
		{"merged clean no PR", base(status.Facts{OwnCommits: true, Merged: true}), status.SafeToDelete,
			[]string{"merged into origin/main"}},
		{"merged with unpushed and gone", base(status.Facts{OwnCommits: true, Merged: true, Unpushed: 2, UpstreamGone: true}), status.SafeToDelete,
			[]string{"merged into origin/main", "remote branch deleted"}},
		{"merged with unpushed, upstream present", base(status.Facts{OwnCommits: true, Merged: true, Unpushed: 2}), status.SafeToDelete,
			[]string{"merged into origin/main"}},
		{"unmerged with unpushed and gone", base(status.Facts{OwnCommits: true, Unpushed: 2, UpstreamGone: true}), status.InUse,
			[]string{"remote branch deleted"}},
		{"merged PR but not merged, recent", base(status.Facts{OwnCommits: true, MergedPR: 9}), status.InUse,
			[]string{"PR #9 merged", "has changes not in origin/main"}},
		{"merged PR but not merged, old", base(status.Facts{OwnCommits: true, MergedPR: 9, LastCommit: now.Add(-20 * day)}), status.Stale,
			[]string{"PR #9 merged", "has changes not in origin/main"}},
		{"closed PR but merged wins", base(status.Facts{OwnCommits: true, ClosedPR: 4, Merged: true}), status.SafeToDelete,
			[]string{"merged into origin/main", "PR #4 closed"}},
		{"closed PR recent", base(status.Facts{OwnCommits: true, ClosedPR: 4}), status.Stale,
			[]string{"PR #4 closed"}},
		{"old and unmerged", base(status.Facts{OwnCommits: true, LastCommit: now.Add(-20 * day)}), status.Stale,
			[]string{"not merged yet"}},
		{"active unmerged", base(status.Facts{OwnCommits: true}), status.InUse,
			[]string{"not merged yet"}},
		{"merged PR, nothing committed", base(status.Facts{MergedPR: 9}), status.InUse,
			[]string{"nothing committed yet", "PR #9 merged"}},
		{"merged PR, unpushed and closed", base(status.Facts{OwnCommits: true, MergedPR: 9, ClosedPR: 4, Unpushed: 1}), status.Stale,
			[]string{"PR #9 merged", "has changes not in origin/main", "PR #4 closed", "1 commit never pushed"}},
		{"long-lived branch merged never safe", base(status.Facts{Protected: true, OwnCommits: true, Merged: true}), status.InUse,
			[]string{"long-lived branch", "branch itself is already merged"}},
		{"long-lived branch old never stale", base(status.Facts{Protected: true, OwnCommits: true, LastCommit: now.Add(-40 * day)}), status.InUse,
			[]string{"long-lived branch"}},
		{"one unpushed", base(status.Facts{OwnCommits: true, Unpushed: 1}), status.InUse,
			[]string{"1 commit never pushed"}},
		{"check failed never safe", base(status.Facts{CheckFailed: true, OwnCommits: true, Merged: true}), status.InUse,
			[]string{"couldn't check this worktree", "branch itself is already merged"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, why := status.Classify(tc.f)
			if state != tc.state {
				t.Errorf("state = %v, want %v", state, tc.state)
			}
			if !reflect.DeepEqual(why, tc.why) {
				t.Errorf("reasons = %q, want %q", why, tc.why)
			}
		})
	}
}

func TestLongLived(t *testing.T) {
	for name, want := range map[string]bool{
		"main": true, "master": true, "develop": true, "trunk": true,
		"feat/main": false, "development": false, "Main": false,
	} {
		if got := status.LongLived(name); got != want {
			t.Errorf("LongLived(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[status.State]string{
		status.InUse: "still in use", status.Stale: "stale, take a look", status.SafeToDelete: "safe to delete",
	} {
		if s.String() != want {
			t.Errorf("%d = %q, want %q", s, s.String(), want)
		}
	}
}
