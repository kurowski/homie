package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHelpTemplatingResolves pins the issue this topic exists to fix:
// `hm help templating` — and the guessable variants — must resolve to
// the template reference instead of "Unknown help topic", surfacing
// the one helper nobody can discover otherwise (hasTag).
func TestHelpTemplatingResolves(t *testing.T) {
	for _, topic := range []string{"templating", "template", "templates"} {
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"help", topic})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("help %s: %v", topic, err)
			continue
		}
		if !strings.Contains(buf.String(), "hasTag") {
			t.Errorf("help %s does not mention hasTag:\n%s", topic, buf.String())
		}
	}
}
