// Package config loads and validates the user's homie.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kurowski/homie/internal/detect"
	"github.com/kurowski/homie/internal/repo"
)

// HostsDir is the directory under the user repo that holds per-host
// overlay TOML files, looked up as hosts/<hostname>.toml.
const HostsDir = "hosts"

// Config is the parsed shape of homie.toml.
type Config struct {
	User     User              `toml:"user"`
	Profile  Profile           `toml:"profile"`
	Packages Packages          `toml:"packages"`
	Tags     Tags              `toml:"tags"`
	Vars     map[string]string `toml:"vars"`

	// Warnings holds non-fatal issues encountered while parsing
	// (e.g. unknown fields). Populated by Load.
	Warnings []string `toml:"-"`
}

// Packages is the parsed [packages] table. It distinguishes the base
// distro-keyed lists (`packages.all`, `packages.fedora`, ...) from
// tag-conditional sub-tables of the form `[packages."tag:work"]`.
//
// The TOML key `tag:<name>` is reserved for tag-keyed sub-tables; any
// other key under [packages] is treated as a base distro list.
type Packages struct {
	// Base maps "all" or a distro key (ubuntu, debian, fedora) to a list
	// of package names to install on every run for matching distros.
	Base map[string][]string

	// ByTag maps a tag name (the part after "tag:") to its own
	// distro-keyed package lists. These contribute only when the tag is
	// active for the current host.
	ByTag map[string]map[string][]string
}

// TagKeyPrefix marks a key inside the [packages] table as a tag-keyed
// sub-table rather than a base distro list. See Packages.
const TagKeyPrefix = "tag:"

// UnmarshalTOML decodes a heterogeneous [packages] table where some keys
// are arrays of strings (base distro lists) and others — those whose
// name starts with "tag:" — are themselves tables of distro -> list.
func (p *Packages) UnmarshalTOML(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("[packages] must be a table, got %T", data)
	}
	p.Base = make(map[string][]string)
	p.ByTag = make(map[string]map[string][]string)
	for k, v := range m {
		if strings.HasPrefix(k, TagKeyPrefix) {
			sub, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf(`[packages."%s"] must be a table of distro -> list, got %T`, k, v)
			}
			tag := strings.TrimPrefix(k, TagKeyPrefix)
			byDistro := make(map[string][]string, len(sub))
			for distro, raw := range sub {
				list, err := stringList(raw)
				if err != nil {
					return fmt.Errorf(`[packages."%s"].%s: %w`, k, distro, err)
				}
				byDistro[distro] = list
			}
			p.ByTag[tag] = byDistro
			continue
		}
		list, err := stringList(v)
		if err != nil {
			return fmt.Errorf("packages.%s: %w", k, err)
		}
		p.Base[k] = list
	}
	return nil
}

// stringList coerces a decoded TOML value into a []string. The
// BurntSushi/toml v1 decoder hands UnmarshalTOML the raw value as `any`
// — homogeneous string arrays come through as []any, so we re-type each
// element explicitly.
func stringList(v any) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected an array of strings, got %T", v)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("element %d: expected string, got %T", i, item)
		}
		out = append(out, s)
	}
	return out, nil
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
//
// If hostname is non-empty and <repoDir>/hosts/<hostname>.toml exists, it
// is deep-merged onto the base config: profile/user scalars replace when
// set, packages and tags.extra arrays append, vars override per-key.
func Load(repoDir, hostname string) (Config, error) {
	basePath := filepath.Join(repoDir, repo.ConfigFilename)
	c, err := loadFile(basePath)
	if err != nil {
		return Config{}, err
	}

	if hostname != "" {
		overlayPath := filepath.Join(repoDir, HostsDir, hostname+".toml")
		if _, statErr := os.Stat(overlayPath); statErr == nil {
			overlay, err := loadFile(overlayPath)
			if err != nil {
				return Config{}, err
			}
			c = merge(c, overlay)
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("stat %s: %w", overlayPath, statErr)
		}
	}

	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", basePath, err)
	}
	return c, nil
}

// loadFile decodes a single TOML file into a Config, capturing unknown
// fields as warnings rather than failing. Validation is not performed
// here — Load applies it after merging the optional host overlay.
func loadFile(path string) (Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}
	for _, k := range md.Undecoded() {
		c.Warnings = append(c.Warnings, fmt.Sprintf("unknown field in %s: %s", filepath.Base(path), k.String()))
	}
	return c, nil
}

// merge deep-merges overlay onto base. Scalars in user/profile replace
// when the overlay sets them non-empty. Packages arrays append per-key
// (both base distro lists and tag-keyed sub-tables). Tags.Extra appends.
// Vars override per-key.
func merge(base, overlay Config) Config {
	if overlay.User.Name != "" {
		base.User.Name = overlay.User.Name
	}
	if overlay.User.Email != "" {
		base.User.Email = overlay.User.Email
	}
	if overlay.Profile.Name != "" {
		base.Profile.Name = overlay.Profile.Name
	}
	if overlay.Profile.DefaultShell != "" {
		base.Profile.DefaultShell = overlay.Profile.DefaultShell
	}
	if len(overlay.Packages.Base) > 0 {
		if base.Packages.Base == nil {
			base.Packages.Base = make(map[string][]string, len(overlay.Packages.Base))
		}
		for k, v := range overlay.Packages.Base {
			base.Packages.Base[k] = appendUnique(base.Packages.Base[k], v)
		}
	}
	if len(overlay.Packages.ByTag) > 0 {
		if base.Packages.ByTag == nil {
			base.Packages.ByTag = make(map[string]map[string][]string, len(overlay.Packages.ByTag))
		}
		for tag, byDistro := range overlay.Packages.ByTag {
			if base.Packages.ByTag[tag] == nil {
				base.Packages.ByTag[tag] = make(map[string][]string, len(byDistro))
			}
			for distro, v := range byDistro {
				base.Packages.ByTag[tag][distro] = appendUnique(base.Packages.ByTag[tag][distro], v)
			}
		}
	}
	base.Tags.Extra = append(base.Tags.Extra, overlay.Tags.Extra...)
	if len(overlay.Vars) > 0 {
		if base.Vars == nil {
			base.Vars = make(map[string]string, len(overlay.Vars))
		}
		for k, v := range overlay.Vars {
			base.Vars[k] = v
		}
	}
	base.Warnings = append(base.Warnings, overlay.Warnings...)
	return base
}

// appendUnique appends every entry in extra to base that isn't already
// present, preserving order. Used by merge so that overlapping package
// lists between base and overlay don't leave duplicates in
// Config.Packages — PackagesFor dedupes on read, but `hm status` and any
// future reader benefits from a clean in-memory shape.
func appendUnique(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, s := range base {
		seen[s] = struct{}{}
	}
	for _, s := range extra {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
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

// PackagesFor returns the packages to install for the given environment.
//
// The set is the union, in this order, of:
//  1. packages.all
//  2. packages.<distro>
//  3. for each active tag (sorted by tag name for determinism):
//     a. [packages."tag:<tag>"].all
//     b. [packages."tag:<tag>"].<distro>
//
// Duplicates are removed on insertion so a package mentioned in multiple
// sub-tables installs exactly once. Tags that have no matching sub-table
// contribute nothing — they aren't an error.
func (c Config) PackagesFor(env detect.Env) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(pkg string) {
		if _, ok := seen[pkg]; ok {
			return
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}

	for _, pkg := range c.Packages.Base["all"] {
		add(pkg)
	}
	for _, pkg := range c.Packages.Base[env.Distro] {
		add(pkg)
	}
	for _, tag := range c.AllTags(env) {
		byDistro := c.Packages.ByTag[tag]
		if byDistro == nil {
			continue
		}
		for _, pkg := range byDistro["all"] {
			add(pkg)
		}
		for _, pkg := range byDistro[env.Distro] {
			add(pkg)
		}
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
