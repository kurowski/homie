package main

import (
	"bytes"
	"strings"
	"testing"
)

// fakeManager is a minimal packages.Manager used to drive doBootstrap
// without shelling out to a real package manager.
type fakeManager struct {
	name      string
	available bool
	installed map[string]bool
	installLog []string
}

func (f *fakeManager) Name() string             { return f.name }
func (f *fakeManager) IsAvailable() bool        { return f.available }
func (f *fakeManager) IsInstalled(p string) bool { return f.installed[p] }
func (f *fakeManager) Install(pkgs []string) error {
	f.installLog = append(f.installLog, pkgs...)
	for _, p := range pkgs {
		if f.installed == nil {
			f.installed = map[string]bool{}
		}
		f.installed[p] = true
	}
	return nil
}

func TestDoBootstrapNoopErrors(t *testing.T) {
	mgr := &fakeManager{name: "noop", available: true}
	err := doBootstrap(mgr, "exotix", new(bytes.Buffer))
	if err == nil || !strings.Contains(err.Error(), "exotix") {
		t.Errorf("expected unsupported-distro error mentioning %q, got %v", "exotix", err)
	}
}

func TestDoBootstrapUnavailableErrors(t *testing.T) {
	mgr := &fakeManager{name: "apt", available: false}
	err := doBootstrap(mgr, "ubuntu", new(bytes.Buffer))
	if err == nil || !strings.Contains(err.Error(), "apt") {
		t.Errorf("expected manager-not-on-PATH error, got %v", err)
	}
}

func TestDoBootstrapAllInstalledIsNoop(t *testing.T) {
	mgr := &fakeManager{
		name:      "apt",
		available: true,
		installed: map[string]bool{"git": true, "ca-certificates": true},
	}
	buf := new(bytes.Buffer)
	if err := doBootstrap(mgr, "ubuntu", buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mgr.installLog) != 0 {
		t.Errorf("expected no Install calls, got %v", mgr.installLog)
	}
	if !strings.Contains(buf.String(), "already installed") {
		t.Errorf("expected already-installed message, got %q", buf.String())
	}
}

func TestDoBootstrapInstallsMissing(t *testing.T) {
	mgr := &fakeManager{
		name:      "apt",
		available: true,
		installed: map[string]bool{"ca-certificates": true},
	}
	buf := new(bytes.Buffer)
	if err := doBootstrap(mgr, "ubuntu", buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mgr.installLog) != 1 || mgr.installLog[0] != "git" {
		t.Errorf("expected Install([git]), got %v", mgr.installLog)
	}
	if !strings.Contains(buf.String(), "git") {
		t.Errorf("expected output to name installing package, got %q", buf.String())
	}
}
