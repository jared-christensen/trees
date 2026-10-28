package pr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseGitHubRemote(t *testing.T) {
	good := []string{
		"git@github.com:o/r.git",
		"https://github.com/o/r",
		"https://github.com/o/r.git",
		"ssh://git@github.com/o/r.git",
	}
	for _, u := range good {
		got, ok := parseGitHubRemote(u)
		if !ok || got != "o/r" {
			t.Errorf("parseGitHubRemote(%q) = %q, %v; want o/r, true", u, got, ok)
		}
	}
	bad := []string{
		"git@bitbucket.org:o/r.git",
		"https://bitbucket.org/o/r.git",
		"https://github.company.com/o/r.git",
		"",
	}
	for _, u := range bad {
		if got, ok := parseGitHubRemote(u); ok {
			t.Errorf("parseGitHubRemote(%q) = %q, true; want false", u, got)
		}
	}
}

func TestGroupPRs(t *testing.T) {
	fixture := `[
	 {"number":1,"state":"CLOSED","headRefName":"feat/a","headRefOid":"aaa","updatedAt":"2026-01-01T00:00:00Z"},
	 {"number":3,"state":"OPEN","headRefName":"feat/a","headRefOid":"ccc","updatedAt":"2026-03-01T00:00:00Z"},
	 {"number":2,"state":"MERGED","headRefName":"feat/b","headRefOid":"bbb","updatedAt":"2026-02-01T00:00:00Z"}
	]`
	got, err := groupPRs([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got["feat/a"]) != 2 || len(got["feat/b"]) != 1 {
		t.Fatalf("unexpected grouping: %+v", got)
	}
	if got["feat/a"][0].Number != 3 || got["feat/a"][1].Number != 1 {
		t.Errorf("not newest first: %+v", got["feat/a"])
	}
	if p := got["feat/b"][0]; p.State != "MERGED" || p.HeadSHA != "bbb" || p.UpdatedAt.IsZero() {
		t.Errorf("bad fields: %+v", p)
	}
	if _, err := groupPRs([]byte("not json")); err == nil {
		t.Error("want error on bad json")
	}
}

func fakeGH(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLookupFailsQuietly(t *testing.T) {
	t.Run("exit 1", func(t *testing.T) {
		g := &GitHub{ghPath: fakeGH(t, "exit 1"), repo: "o/r", timeout: 5 * time.Second}
		start := time.Now()
		if got := g.Lookup(context.Background(), []string{"x"}); len(got) != 0 {
			t.Errorf("want empty map, got %v", got)
		}
		if time.Since(start) > 5*time.Second {
			t.Error("too slow")
		}
	})
	t.Run("missing binary", func(t *testing.T) {
		g := &GitHub{ghPath: "/nonexistent/gh", repo: "o/r", timeout: 5 * time.Second}
		if got := g.Lookup(context.Background(), nil); got == nil || len(got) != 0 {
			t.Errorf("want empty non-nil map, got %v", got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		g := &GitHub{ghPath: fakeGH(t, "exec sleep 10"), repo: "o/r", timeout: 200 * time.Millisecond}
		start := time.Now()
		got := g.Lookup(context.Background(), nil)
		if len(got) != 0 {
			t.Errorf("want empty map, got %v", got)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("Lookup took %v, want prompt return", d)
		}
	})
}

func TestLookupSuccess(t *testing.T) {
	g := &GitHub{
		ghPath:  fakeGH(t, `echo '[{"number":7,"state":"OPEN","headRefName":"b","headRefOid":"s","updatedAt":"2026-01-01T00:00:00Z"}]'`),
		repo:    "o/r",
		timeout: 5 * time.Second,
	}
	got := g.Lookup(context.Background(), []string{"b"})
	if len(got["b"]) != 1 || got["b"][0].Number != 7 {
		t.Errorf("got %v", got)
	}
}

func TestDetect(t *testing.T) {
	if _, ok := Detect("https://github.com/o/r", false).(None); !ok {
		t.Error("disabled should be None")
	}
	t.Setenv("BITBUCKET_USERNAME", "")
	t.Setenv("BITBUCKET_TOKEN", "")
	t.Setenv("BITBUCKET_APP_PASSWORD", "")
	if _, ok := Detect("https://bitbucket.org/o/r", true).(None); !ok {
		t.Error("bitbucket without credentials should be None")
	}
	if _, ok := Detect("https://gitlab.com/o/r", true).(None); !ok {
		t.Error("unknown host should be None")
	}
	t.Setenv("BITBUCKET_USERNAME", "me@example.com")
	t.Setenv("BITBUCKET_TOKEN", "tok")
	if b, ok := Detect("git@ssh.bitbucket.org:ws/r.git", true).(*Bitbucket); !ok || b.repo != "ws/r" {
		t.Errorf("want Bitbucket provider for ws/r, got %#v", b)
	}
}
