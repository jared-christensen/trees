// Package config loads the user's trees settings from the user config file.
// Settings are layered: built-in defaults < [defaults] < [repos."<repoMain>"].
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Settings are the effective settings for one repository.
type Settings struct {
	Path                string   `toml:"path"`              // default "{repo_parent}/{repo}.{name}"
	NamePattern         string   `toml:"name_pattern"`      // regex, "" = off
	NamePatternMode     string   `toml:"name_pattern_mode"` // "warn" (default) or "require"
	Setup               []string `toml:"setup"`
	AfterSwitch         string   `toml:"after_switch"`
	StaleDays           int      `toml:"stale_days"`            // default 14
	PullRequests        *bool    `toml:"pull_requests"`         // default true
	FetchTimeoutSeconds int      `toml:"fetch_timeout_seconds"` // default 5
}

// layer is one config section; nil fields mean "not set in this layer".
type layer struct {
	Path                *string  `toml:"path"`
	NamePattern         *string  `toml:"name_pattern"`
	NamePatternMode     *string  `toml:"name_pattern_mode"`
	Setup               []string `toml:"setup"`
	AfterSwitch         *string  `toml:"after_switch"`
	StaleDays           *int     `toml:"stale_days"`
	PullRequests        *bool    `toml:"pull_requests"`
	FetchTimeoutSeconds *int     `toml:"fetch_timeout_seconds"`
}

type file struct {
	Defaults layer            `toml:"defaults"`
	Repos    map[string]layer `toml:"repos"`
}

// Defaults returns the built-in settings.
func Defaults() Settings {
	t := true
	return Settings{
		Path:                "{repo_parent}/{repo}.{name}",
		NamePatternMode:     "warn",
		StaleDays:           14,
		PullRequests:        &t,
		FetchTimeoutSeconds: 5,
	}
}

// FilePath returns the config file location: $XDG_CONFIG_HOME/trees/config.toml,
// else ~/.config/trees/config.toml.
func FilePath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "trees", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating config: %w", err)
	}
	return filepath.Join(home, ".config", "trees", "config.toml"), nil
}

// Load returns the effective settings for the repo whose main checkout is repoMain.
// A missing config file yields the defaults.
func Load(repoMain string) (Settings, error) {
	s := Defaults()
	path, err := FilePath()
	if err != nil {
		return s, err
	}
	var f file
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		keys := make([]string, len(un))
		for i, k := range un {
			keys[i] = k.String()
		}
		return Settings{}, fmt.Errorf("%s: unknown config key(s): %s", path, strings.Join(keys, ", "))
	}
	layers := []struct {
		name string
		l    layer
	}{{"[defaults]", f.Defaults}}
	if r, ok := f.Repos[repoMain]; ok {
		layers = append(layers, struct {
			name string
			l    layer
		}{fmt.Sprintf("[repos.%q]", repoMain), r})
	}
	for _, ly := range layers {
		if err := ly.l.validate(); err != nil {
			return Settings{}, fmt.Errorf("%s: %s: %w", path, ly.name, err)
		}
		ly.l.applyTo(&s)
	}
	return s, nil
}

func (l layer) validate() error {
	if l.NamePattern != nil {
		if _, err := regexp.Compile(*l.NamePattern); err != nil {
			return fmt.Errorf("name_pattern: invalid regexp: %w", err)
		}
	}
	if m := l.NamePatternMode; m != nil && *m != "warn" && *m != "require" {
		return fmt.Errorf("name_pattern_mode: must be \"warn\" or \"require\", got %q", *m)
	}
	if l.StaleDays != nil && *l.StaleDays <= 0 {
		return fmt.Errorf("stale_days: must be positive, got %d", *l.StaleDays)
	}
	if l.FetchTimeoutSeconds != nil && *l.FetchTimeoutSeconds <= 0 {
		return fmt.Errorf("fetch_timeout_seconds: must be positive, got %d", *l.FetchTimeoutSeconds)
	}
	return nil
}

func (l layer) applyTo(s *Settings) {
	if l.Path != nil {
		s.Path = *l.Path
	}
	if l.NamePattern != nil {
		s.NamePattern = *l.NamePattern
	}
	if l.NamePatternMode != nil {
		s.NamePatternMode = *l.NamePatternMode
	}
	if l.Setup != nil {
		s.Setup = l.Setup
	}
	if l.AfterSwitch != nil {
		s.AfterSwitch = *l.AfterSwitch
	}
	if l.StaleDays != nil {
		s.StaleDays = *l.StaleDays
	}
	if l.PullRequests != nil {
		s.PullRequests = l.PullRequests
	}
	if l.FetchTimeoutSeconds != nil {
		s.FetchTimeoutSeconds = *l.FetchTimeoutSeconds
	}
}
