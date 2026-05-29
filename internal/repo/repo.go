// Package repo resolves the path to the user environment repo Homie operates on.
package repo

import (
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFilename = "homie.toml"

// Find returns the user environment repo root. It checks $HM_REPO first, then
// walks up from the current working directory looking for a homie.toml.
func Find() (string, error) {
	if env := os.Getenv("HM_REPO"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", fmt.Errorf("resolve HM_REPO=%q: %w", env, err)
		}
		if _, err := os.Stat(filepath.Join(abs, ConfigFilename)); err != nil {
			return "", fmt.Errorf("HM_REPO=%q has no %s: %w", abs, ConfigFilename, err)
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ConfigFilename)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in %s or any parent (set $HM_REPO to point at your environment repo)", ConfigFilename, cwd)
		}
		dir = parent
	}
}
