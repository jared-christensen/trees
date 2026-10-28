package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := filepath.Join(dir, "trees", "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultsWhenNoFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := Load("/x/y")
	if err != nil {
		t.Fatal(err)
	}
	if s.Path != "{repo_parent}/{repo}.{name}" || s.NamePatternMode != "warn" || s.StaleDays != 14 ||
		s.FetchTimeoutSeconds != 5 || s.PullRequests == nil || !*s.PullRequests || s.NamePattern != "" {
		t.Fatalf("bad defaults: %+v", s)
	}
}

func TestRepoSectionOverridesDefaults(t *testing.T) {
	writeCfg(t, `
[defaults]
stale_days = 30
after_switch = "code ."
[repos."/x/y"]
stale_days = 7
pull_requests = false
`)
	s, err := Load("/x/y")
	if err != nil {
		t.Fatal(err)
	}
	if s.StaleDays != 7 || s.AfterSwitch != "code ." || s.PullRequests == nil || *s.PullRequests {
		t.Fatalf("repo: %+v", s)
	}
	o, err := Load("/other")
	if err != nil {
		t.Fatal(err)
	}
	if o.StaleDays != 30 || !*o.PullRequests {
		t.Fatalf("other: %+v", o)
	}
}

func TestRepoKeyWithSpace(t *testing.T) {
	writeCfg(t, "[repos.\"/a/has space\"]\nsetup = [\"make\"]\n")
	s, err := Load("/a/has space")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Setup) != 1 || s.Setup[0] != "make" {
		t.Fatalf("%+v", s)
	}
}

func TestBadTOMLError(t *testing.T) {
	p := writeCfg(t, "[defaults\nstale_days = 3\n")
	_, err := Load("/x")
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("want path in error, got %v", err)
	}
}

func TestHomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	h := t.TempDir()
	t.Setenv("HOME", h)
	p := filepath.Join(h, ".config", "trees", "config.toml")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("[defaults]\nstale_days = 3\n"), 0o644)
	s, err := Load("/x")
	if err != nil || s.StaleDays != 3 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := map[string]string{
		"name_pattern":          "[defaults]\nname_pattern = \"(\"\n",
		"name_pattern_mode":     "[defaults]\nname_pattern_mode = \"bogus\"\n",
		"stale_days":            "[repos.\"/x\"]\nstale_days = 0\n",
		"fetch_timeout_seconds": "[defaults]\nfetch_timeout_seconds = -1\n",
	}
	for key, body := range cases {
		t.Run(key, func(t *testing.T) {
			p := writeCfg(t, body)
			_, err := Load("/x")
			if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), p) {
				t.Fatalf("want error naming %s and file, got %v", key, err)
			}
		})
	}
}

func TestUnknownKeys(t *testing.T) {
	p := writeCfg(t, "[defaults]\nstale_dayz = 3\n[repos.\"/x\"]\nbogus = 1\n")
	_, err := Load("/x")
	if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "stale_dayz") {
		t.Fatalf("got %v", err)
	}
}

func TestLayerMergePerField(t *testing.T) {
	writeCfg(t, `
[defaults]
name_pattern = "^[a-z]+$"
name_pattern_mode = "require"
fetch_timeout_seconds = 9
[repos."/x"]
path = "/w/{name}"
`)
	s, err := Load("/x")
	if err != nil {
		t.Fatal(err)
	}
	if s.NamePattern != "^[a-z]+$" || s.NamePatternMode != "require" || s.FetchTimeoutSeconds != 9 || s.Path != "/w/{name}" || s.StaleDays != 14 {
		t.Fatalf("%+v", s)
	}
}
