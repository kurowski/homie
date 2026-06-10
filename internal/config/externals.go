package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kurowski/homie/internal/detect"
)

// Externals is the parsed [externals] table: external git repos to keep
// cloned and updated on disk, keyed by destination path. Two key shapes
// are recognized:
//
//	[externals."~/.zsh/plugins/zsh-autosuggestions"]   # always applies
//	repo = "https://github.com/zsh-users/zsh-autosuggestions"
//	ref  = "v0.7.1"                                    # optional pin
//
//	[externals."tag:desktop"."~/.config/some/theme"]   # tag-gated (AND)
//	repo = "https://github.com/example/theme"
//
// Like Packages, Base and ByTag are exported for the in-package overlay
// merge and for sibling-package test literals; runtime reads go through
// [Config.ExternalsFor].
type Externals struct {
	// Base maps a destination path (as written: ~/..., $HOME/..., or
	// absolute) to its spec. Applies on every host.
	Base map[string]ExternalSpec

	// ByTag maps a canonical tag key (sorted tags joined by ".") to its
	// own destination-keyed specs, applying only when ALL the key's tags
	// are active — the same AND rule as [packages."tag:X"].
	ByTag map[string]map[string]ExternalSpec

	// Warnings is populated by UnmarshalTOML for unknown spec keys and
	// other non-fatal issues; Load drains it into Config.Warnings.
	Warnings []string
}

// ExternalSpec is the body of one [externals."<dest>"] entry.
type ExternalSpec struct {
	// Repo is the clone URL. Required.
	Repo string

	// Ref pins the checkout to a branch, tag, or commit: checked out
	// (detached) and held there until the value changes. Empty means
	// track the remote default branch, fast-forwarding on each apply.
	Ref string
}

// External is one resolved entry from ExternalsFor: the spec plus the
// destination it applies to on this host.
type External struct {
	Dest string
	Repo string
	Ref  string
}

// UnmarshalTOML decodes the [externals] table. Top-level keys are
// dispatched by name: "tag:..." keys are tag-gated sub-tables of
// destination-keyed specs, anything else is a destination path whose
// value must be a {repo, ref} table.
func (e *Externals) UnmarshalTOML(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("[externals] must be a table, got %T", data)
	}
	e.Base = make(map[string]ExternalSpec)
	e.ByTag = make(map[string]map[string]ExternalSpec)

	for k, v := range m {
		sub, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf(`[externals."%s"] must be a table, got %T`, k, v)
		}
		if !strings.HasPrefix(k, TagKeyPrefix) {
			spec, err := e.decodeSpec(sub, fmt.Sprintf(`[externals."%s"]`, k))
			if err != nil {
				return err
			}
			e.Base[k] = spec
			continue
		}
		canonical, _, err := parseTagKey(k)
		if err != nil {
			return err
		}
		block := make(map[string]ExternalSpec, len(sub))
		for dest, dv := range sub {
			dm, ok := dv.(map[string]any)
			if !ok {
				return fmt.Errorf(`[externals."%s"."%s"] must be a table, got %T`, k, dest, dv)
			}
			spec, err := e.decodeSpec(dm, fmt.Sprintf(`[externals."%s"."%s"]`, k, dest))
			if err != nil {
				return err
			}
			block[dest] = spec
		}
		if _, dup := e.ByTag[canonical]; dup {
			e.warnf(`[externals."%s"] duplicates an earlier block that resolves to the same tags ("%s") — the later one wins, they don't merge; combine them into one block`, k, displayTagKey(canonical))
		}
		e.ByTag[canonical] = block
	}
	return nil
}

func (e *Externals) decodeSpec(m map[string]any, ctx string) (ExternalSpec, error) {
	var spec ExternalSpec
	for k, v := range m {
		s, isString := v.(string)
		switch k {
		case "repo":
			if !isString {
				return ExternalSpec{}, fmt.Errorf("%s.repo: expected a string, got %T", ctx, v)
			}
			spec.Repo = s
		case "ref":
			if !isString {
				return ExternalSpec{}, fmt.Errorf("%s.ref: expected a string, got %T", ctx, v)
			}
			spec.Ref = s
		default:
			e.warnf("%s: unknown key %q (known: repo, ref)", ctx, k)
		}
	}
	if spec.Repo == "" {
		return ExternalSpec{}, fmt.Errorf("%s: repo is required", ctx)
	}
	return spec, nil
}

func (e *Externals) warnf(format string, args ...any) {
	e.Warnings = append(e.Warnings, fmt.Sprintf(format, args...))
}

// ExternalsFor returns the externals to sync for the given environment,
// sorted by destination: every base entry plus every entry from tag
// blocks whose tags are all active.
//
// When the same destination is declared more than once, the entry with
// more required tags wins (base counts as zero) — the same
// more-specific-wins rule the home/ trees use. Two declarations at the
// same specificity with different settings are an error; identical
// duplicates collapse silently.
func (c Config) ExternalsFor(env detect.Env) ([]External, error) {
	type candidate struct {
		spec   ExternalSpec
		ntags  int
		origin string
	}
	picked := make(map[string]candidate)
	consider := func(dest string, spec ExternalSpec, ntags int, origin string) error {
		cur, taken := picked[dest]
		switch {
		case !taken || ntags > cur.ntags:
			picked[dest] = candidate{spec: spec, ntags: ntags, origin: origin}
		// ExternalSpec is two string fields, so != is a full value
		// compare; revisit if it ever grows a non-comparable field.
		case ntags == cur.ntags && spec != cur.spec:
			return fmt.Errorf("externals: %q is declared by both %s and %s with different settings — combine them or make one more specific", dest, cur.origin, origin)
		}
		return nil
	}

	for _, dest := range sortedKeys(c.Externals.Base) {
		// Base destinations are unique map keys; consider can't error here.
		_ = consider(dest, c.Externals.Base[dest], 0, "[externals]")
	}
	if len(c.Externals.ByTag) > 0 {
		active := make(map[string]struct{})
		for _, t := range c.AllTags(env) {
			active[t] = struct{}{}
		}
		// Canonical-key order makes conflict errors deterministic.
		for _, key := range sortedKeys(c.Externals.ByTag) {
			if !tagsAllActive(key, active) {
				continue
			}
			block := c.Externals.ByTag[key]
			ntags := strings.Count(key, ".") + 1
			origin := fmt.Sprintf(`[externals."%s"]`, displayTagKey(key))
			for _, dest := range sortedKeys(block) {
				if err := consider(dest, block[dest], ntags, origin); err != nil {
					return nil, err
				}
			}
		}
	}

	out := make([]External, 0, len(picked))
	for _, dest := range sortedKeys(picked) {
		c := picked[dest]
		out = append(out, External{Dest: dest, Repo: c.spec.Repo, Ref: c.spec.Ref})
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
