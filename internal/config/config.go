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

// Packages is the parsed [packages] table. It distinguishes:
//   - base distro-keyed lists (`packages.all`, `packages.fedora`, ...)
//   - tag-conditional sub-tables (`[packages."tag:work"]`)
//   - non-native backend sub-tables (`[packages.flatpak]`, `[packages.brew]`)
//   - both combined (`[packages."tag:work".flatpak]`)
//
// The TOML key `tag:<name>` is reserved for tag-keyed sub-tables; the
// keys named in KnownBackends are reserved for non-native managers; any
// other key under [packages] is treated as a base distro list.
//
// Base, ByTag, and Backends are exported because (a) the in-package
// overlay merge needs to mutate them and (b) tests in sibling packages
// construct Packages literals directly. Regular runtime reads should go
// through [Config.PackagesFor] / [Config.PackagesForBackend] rather
// than touching these maps.
type Packages struct {
	// Base maps "all" or a distro key (ubuntu, debian, fedora) to a list
	// of package names to install on every run for matching distros.
	Base map[string][]string

	// ByTag maps a canonical tag key (the required tags, sorted and joined
	// by ".") to its own distro-keyed package lists. A single-tag block
	// ([packages."tag:work"]) keys on "work"; a multi-tag block
	// ([packages."tag:personal.tag:ubuntu"]) keys on "personal.ubuntu" and
	// contributes only when ALL its tags are active (AND).
	ByTag map[string]map[string][]string

	// Backends holds non-native managers (flatpak, brew) keyed by name.
	// Each backend mirrors the Base/ByTag shape so tag-conditional and
	// distro-conditional resolution work identically.
	Backends map[string]BackendPackages

	// Warnings is populated by UnmarshalTOML for typos and other
	// non-fatal issues (unknown keys, empty tag names). Load drains
	// these into Config.Warnings so `hm status` / `hm doctor` surface
	// them.
	Warnings []string
}

// BackendPackages is the parsed shape of a single non-native backend
// (`[packages.flatpak]`, `[packages.brew]`, ...). Same Base + ByTag
// model as the native side, so PackagesForBackend can reuse the
// resolution rule.
type BackendPackages struct {
	Base  map[string][]string
	ByTag map[string]map[string][]string
}

// TagKeyPrefix marks a key inside the [packages] table as a tag-keyed
// sub-table rather than a base distro list. See Packages.
const TagKeyPrefix = "tag:"

// KnownBackends lists the non-native package backends Homie recognizes.
// Names here are reserved as sub-table keys under [packages] and
// [packages."tag:X"]; the corresponding Manager lives in
// internal/packages.ForBackend. Adding a new backend means:
//  1. adding its name here, and
//  2. teaching packages.ForBackend to return its Manager.
var KnownBackends = map[string]struct{}{
	"flatpak": {},
	"brew":    {},
	"snap":    {},
}

// knownDistroKeys are the keys accepted as base distro lists or as
// sub-table keys inside `[packages."tag:X"]`. "all" applies to every
// distro; the others must match a supported distro. Keys outside this
// set are kept (so newer schemas don't hard-fail older binaries) but
// generate a warning.
var knownDistroKeys = map[string]struct{}{
	"all":    {},
	"ubuntu": {},
	"debian": {},
	"fedora": {},
}

// UnmarshalTOML decodes a heterogeneous [packages] table. Each top-level
// key is dispatched by value shape:
//
//   - array of strings → base distro list. Unknown distro names warn.
//   - table named "tag:X" → tag sub-table; its members are decoded by
//     the same shape rule (arrays are distro lists for the tag, tables
//     are tag-keyed backend lists).
//   - any other table → non-native backend sub-table. Backend names
//     outside KnownBackends are accepted into Backends with a warning
//     so the file is forward-compatible with newer hm binaries; the
//     warning surfaces typos and gives `hm doctor` something to report
//     at apply time.
func (p *Packages) UnmarshalTOML(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("[packages] must be a table, got %T", data)
	}
	p.Base = make(map[string][]string)
	p.ByTag = make(map[string]map[string][]string)
	p.Backends = make(map[string]BackendPackages)

	for k, v := range m {
		if strings.HasPrefix(k, TagKeyPrefix) {
			sub, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf(`[packages."%s"] must be a table, got %T`, k, v)
			}
			canonical, _, err := parseTagKey(k)
			if err != nil {
				return err
			}
			if err := p.absorbTagTable(canonical, sub, fmt.Sprintf(`[packages."%s"]`, k)); err != nil {
				return err
			}
			continue
		}
		// Non-tag key: a table is a backend, an array is a distro list.
		// Dispatch by value shape so unknown backend names get a warning
		// instead of a hard error, matching homie.toml's "unknown fields
		// are warnings, not errors" forward-compat promise.
		if sub, isTable := v.(map[string]any); isTable {
			if !isBackendName(k) {
				p.warnf(`packages.%s looks like a backend but isn't recognized — known: flatpak, brew, snap. Entries are loaded but no Manager will install them.`, k)
			}
			lists, err := p.decodeDistroLists(sub, fmt.Sprintf("[packages.%s]", k))
			if err != nil {
				return err
			}
			be := p.Backends[k]
			be.Base = lists
			p.Backends[k] = be
			continue
		}
		list, err := stringList(v)
		if err != nil {
			return fmt.Errorf("packages.%s: %w", k, err)
		}
		if _, known := knownDistroKeys[k]; !known {
			p.warnf(`packages.%s is not a recognized distro key — known: all, ubuntu, debian, fedora (use [packages."tag:%s"] for a tag-keyed list)`, k, k)
		}
		p.Base[k] = list
	}
	return nil
}

// absorbTagTable processes the body of a [packages."tag:X[.tag:Y...]"]
// sub-table: arrays become the block's distro lists, tables become its
// per-backend entries. canonical is the parsed canonical tag key (sorted
// tags joined by "."), so single- and multi-tag blocks land in the same
// ByTag map keyed identically. Note the asymmetry — backend entries inside
// a tag table land in Backends[name].ByTag[canonical] rather than
// ByTag[canonical]; if a tag table holds *only* backend sub-tables,
// ByTag[canonical] stays unset because there are no native packages to
// register for it.
func (p *Packages) absorbTagTable(canonical string, sub map[string]any, ctx string) error {
	tagBase := make(map[string][]string)
	for k, v := range sub {
		// Same shape-dispatch rule as the top level: tables are
		// backends (known or unknown-with-warning), arrays are
		// distro lists.
		if sub2, isTable := v.(map[string]any); isTable {
			if !isBackendName(k) {
				p.warnf(`%s.%s looks like a backend but isn't recognized — known: flatpak, brew, snap. Entries are loaded but no Manager will install them.`, ctx, k)
			}
			lists, err := p.decodeDistroLists(sub2, fmt.Sprintf("%s.%s", ctx, k))
			if err != nil {
				return err
			}
			be := p.Backends[k]
			if be.ByTag == nil {
				be.ByTag = make(map[string]map[string][]string)
			}
			be.ByTag[canonical] = lists
			p.Backends[k] = be
			continue
		}
		list, err := stringList(v)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", ctx, k, err)
		}
		if _, known := knownDistroKeys[k]; !known {
			p.warnf(`%s.%s is not a recognized distro key — known: all, ubuntu, debian, fedora`, ctx, k)
		}
		tagBase[k] = list
	}
	if len(tagBase) > 0 {
		p.ByTag[canonical] = tagBase
	}
	return nil
}

// decodeDistroLists turns a TOML sub-table into a distro-keyed list
// map, warning on keys outside the known distro set. Shared by base
// and tag-scoped backend tables.
func (p *Packages) decodeDistroLists(m map[string]any, ctx string) (map[string][]string, error) {
	lists := make(map[string][]string, len(m))
	for k, v := range m {
		list, err := stringList(v)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", ctx, k, err)
		}
		if _, known := knownDistroKeys[k]; !known {
			p.warnf(`%s.%s is not a recognized distro key — known: all, ubuntu, debian, fedora`, ctx, k)
		}
		lists[k] = list
	}
	return lists, nil
}

func isBackendName(k string) bool {
	_, ok := KnownBackends[k]
	return ok
}

// parseTagKey turns a [packages."tag:X[.tag:Y...]"] key into the set of
// tags that must all be active for the block to apply, plus a canonical
// map key: the sorted tags joined by ".". The leading "tag:" is confirmed
// by the caller. Every "."-separated segment must be "tag:<name>" with a
// non-empty name; anything else (a bare segment, a trailing ".", an empty
// "tag:") is a malformed key and returns an error.
//
// Tag names can't contain "." — the same constraint the home/scripts trees
// impose — so "." is an unambiguous segment delimiter and the canonical key
// round-trips back to the tags via strings.Split. Sorting makes the key
// order-independent: "tag:a.tag:b" and "tag:b.tag:a" collapse to one block
// (so host overlays merge regardless of how each was written).
func parseTagKey(key string) (canonical string, tags []string, err error) {
	segments := strings.Split(key, ".")
	tags = make([]string, 0, len(segments))
	for _, seg := range segments {
		name, ok := strings.CutPrefix(seg, TagKeyPrefix)
		if !ok || name == "" {
			return "", nil, fmt.Errorf(`malformed package tag key [packages."%s"]: each "."-separated segment must be "tag:<name>" with a non-empty name (e.g. "tag:personal.tag:ubuntu")`, key)
		}
		tags = append(tags, name)
	}
	sort.Strings(tags)
	return strings.Join(tags, "."), tags, nil
}

// tagsAllActive reports whether every tag in a canonical tag key (sorted
// tags joined by ".") is present in the active set — the AND semantics of a
// tag-keyed block. An empty key never matches.
func tagsAllActive(canonical string, active map[string]struct{}) bool {
	if canonical == "" {
		return false
	}
	for _, t := range strings.Split(canonical, ".") {
		if _, ok := active[t]; !ok {
			return false
		}
	}
	return true
}

// displayTagKey turns a canonical key ("personal.ubuntu") back into the
// homie.toml form ("tag:personal.tag:ubuntu") for messages.
func displayTagKey(canonical string) string {
	return TagKeyPrefix + strings.Join(strings.Split(canonical, "."), "."+TagKeyPrefix)
}

func (p *Packages) warnf(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
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
	// Packages has a custom UnmarshalTOML, so md.Undecoded() can't see
	// typos inside [packages]. Drain the warnings it collected into the
	// Config-level slice with the source filename prefixed for context.
	for _, w := range c.Packages.Warnings {
		c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s", filepath.Base(path), w))
	}
	c.Packages.Warnings = nil
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
	if len(overlay.Packages.Backends) > 0 {
		if base.Packages.Backends == nil {
			base.Packages.Backends = make(map[string]BackendPackages, len(overlay.Packages.Backends))
		}
		for name, overlayBe := range overlay.Packages.Backends {
			baseBe := base.Packages.Backends[name]
			if baseBe.Base == nil {
				baseBe.Base = make(map[string][]string, len(overlayBe.Base))
			}
			for k, v := range overlayBe.Base {
				baseBe.Base[k] = appendUnique(baseBe.Base[k], v)
			}
			if len(overlayBe.ByTag) > 0 && baseBe.ByTag == nil {
				baseBe.ByTag = make(map[string]map[string][]string, len(overlayBe.ByTag))
			}
			for tag, byDistro := range overlayBe.ByTag {
				if baseBe.ByTag[tag] == nil {
					baseBe.ByTag[tag] = make(map[string][]string, len(byDistro))
				}
				for distro, v := range byDistro {
					baseBe.ByTag[tag][distro] = appendUnique(baseBe.ByTag[tag][distro], v)
				}
			}
			base.Packages.Backends[name] = baseBe
		}
	}
	base.Tags.Extra = appendUnique(base.Tags.Extra, overlay.Tags.Extra)
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

// PackagesFor returns the native packages to install for the given
// environment.
//
// The set is the union, in this order, of:
//  1. packages.all
//  2. packages.<distro>
//  3. for each tag-keyed block whose tags are ALL active (blocks visited
//     in canonical-key order for determinism):
//     a. [packages."tag:..."].all
//     b. [packages."tag:..."].<distro>
//
// A block keyed on a single tag applies when that tag is active; a block
// keyed on several (`[packages."tag:X.tag:Y"]`) applies only when every one
// is active (AND). Duplicates are removed on insertion so a package
// mentioned in multiple blocks installs exactly once. Blocks whose tags
// aren't all active contribute nothing — they aren't an error.
func (c Config) PackagesFor(env detect.Env) []string {
	return c.resolvePackages(env, c.Packages.Base, c.Packages.ByTag)
}

// PackagesForBackend returns the packages declared for the named
// non-native backend (e.g. "flatpak", "brew") using the same resolution
// rule as PackagesFor — base entries plus matching tag-keyed entries,
// deduped, in deterministic order. Returns nil if the backend isn't
// mentioned in homie.toml.
func (c Config) PackagesForBackend(env detect.Env, backend string) []string {
	be, ok := c.Packages.Backends[backend]
	if !ok {
		return nil
	}
	return c.resolvePackages(env, be.Base, be.ByTag)
}

func (c Config) resolvePackages(env detect.Env, base map[string][]string, byTag map[string]map[string][]string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(pkg string) {
		if _, ok := seen[pkg]; ok {
			return
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}

	for _, pkg := range base["all"] {
		add(pkg)
	}
	for _, pkg := range base[env.Distro] {
		add(pkg)
	}
	// Skip the active-set build (and its sort) when no tag-keyed entries
	// exist for this source — the common case for repos that don't use
	// the feature.
	if len(byTag) == 0 {
		return out
	}
	active := make(map[string]struct{})
	for _, t := range c.AllTags(env) {
		active[t] = struct{}{}
	}
	// Visit blocks in canonical-key order so the output is deterministic
	// regardless of map iteration order.
	keys := make([]string, 0, len(byTag))
	for k := range byTag {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !tagsAllActive(key, active) {
			continue
		}
		byDistro := byTag[key]
		for _, pkg := range byDistro["all"] {
			add(pkg)
		}
		for _, pkg := range byDistro[env.Distro] {
			add(pkg)
		}
	}
	return out
}

// ActiveTagBlocks returns the display form (e.g. "tag:personal.tag:ubuntu")
// of every multi-tag [packages] block — native or any backend — whose tags
// are all active for env. Single-tag blocks are omitted (they aren't an
// AND-condition worth surfacing). Result is deduped and sorted; used by
// `hm doctor` to confirm which AND-conditions applied on this host.
func (c Config) ActiveTagBlocks(env detect.Env) []string {
	active := make(map[string]struct{})
	for _, t := range c.AllTags(env) {
		active[t] = struct{}{}
	}
	seen := make(map[string]struct{})
	collect := func(byTag map[string]map[string][]string) {
		for key := range byTag {
			if !strings.Contains(key, ".") { // single-tag block, not an AND
				continue
			}
			if tagsAllActive(key, active) {
				seen[displayTagKey(key)] = struct{}{}
			}
		}
	}
	collect(c.Packages.ByTag)
	for _, be := range c.Packages.Backends {
		collect(be.ByTag)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
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
