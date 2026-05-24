// Package config loads and validates the user's homie.toml.
package config

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/repo"
)

// Config is the parsed shape of homie.toml.
type Config struct {
	User     User                `toml:"user"`
	Profile  Profile             `toml:"profile"`
	Packages map[string][]string `toml:"packages"`
	Tags     Tags                `toml:"tags"`
	Vars     map[string]string   `toml:"vars"`

	// Warnings holds non-fatal issues encountered while parsing
	// (e.g. unknown fields). Populated by Load.
	Warnings []string `toml:"-"`
}

// User holds identity from the [user] table. Both fields are required.
type User struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
}

// Profile holds the [profile] table.
type Profile struct {
	Name         string `toml:"name"`
	DefaultShell string `toml:"default_shell"`
}

// Tags holds the [tags] table.
type Tags struct {
	Extra []string `toml:"extra"`
}

// Load reads <repoDir>/homie.toml and validates required fields. Unknown
// fields are recorded as warnings rather than errors so users adding new
// schema keys for older binaries don't get hard failures.
func Load(repoDir string) (Config, error) {
	path := filepath.Join(repoDir, repo.ConfigFilename)
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}

	for _, k := range md.Undecoded() {
		c.Warnings = append(c.Warnings, fmt.Sprintf("unknown field in homie.toml: %s", k.String()))
	}

	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	switch {
	case c.User.Name == "":
		return fmt.Errorf("user.name is required")
	case c.User.Email == "":
		return fmt.Errorf("user.email is required")
	}
	return nil
}

// PackagesFor returns the packages to install for the given environment:
// packages.all merged with packages.<distro>, deduped, in stable order.
func (c Config) PackagesFor(env detect.Env) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, pkg := range c.Packages["all"] {
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}
	for _, pkg := range c.Packages[env.Distro] {
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}
	return out
}

// AllTags merges auto-detected tags (env.Tags) with the user's profile name
// and extra tags. Result is deduped and sorted for stable output.
func (c Config) AllTags(env detect.Env) []string {
	seen := make(map[string]struct{}, len(env.Tags)+len(c.Tags.Extra)+1)
	add := func(tag string) {
		if tag == "" {
			return
		}
		seen[tag] = struct{}{}
	}
	for _, t := range env.Tags {
		add(t)
	}
	add(c.Profile.Name)
	for _, t := range c.Tags.Extra {
		add(t)
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
