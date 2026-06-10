package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/kurowski/homie/internal/render"
)

// TestHelpTemplatingResolves pins the issue this topic exists to fix:
// `hm help templating` — the guessable variants, and the bare command —
// must resolve to the template reference instead of "Unknown help
// topic", surfacing the one helper nobody can discover otherwise
// (hasTag).
func TestHelpTemplatingResolves(t *testing.T) {
	for _, args := range [][]string{
		{"help", "templating"},
		{"help", "template"},
		{"help", "templates"},
		{"templating"}, // Run-less, so the bare command shows its help
	} {
		if out := runTemplatingHelp(t, args); !strings.Contains(out, "hasTag") {
			t.Errorf("%v does not mention hasTag:\n%s", args, out)
		}
	}
}

// TestTemplatingHelpListsAllDataFields makes the keep-in-sync comment
// on render.Data mechanical: every field of the template data model
// must appear in the topic, so adding a field without updating the
// hand-written table fails here instead of silently drifting.
func TestTemplatingHelpListsAllDataFields(t *testing.T) {
	out := runTemplatingHelp(t, []string{"help", "templating"})
	rt := reflect.TypeOf(render.Data{})
	for i := 0; i < rt.NumField(); i++ {
		name := "." + rt.Field(i).Name
		if !strings.Contains(out, name) {
			t.Errorf("hm help templating omits %s — keep cmd/hm/templating.go in sync with render.Data", name)
		}
	}
}

func runTemplatingHelp(t *testing.T, args []string) string {
	t.Helper()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return buf.String()
}
