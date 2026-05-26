package tree

import (
	"strings"
	"testing"
)

func TestParseDir(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		wantTags []string
		wantOK   bool
	}{
		{name: "dotfiles", base: "dotfiles", wantTags: nil, wantOK: true},
		{name: "dotfiles.tag-work", base: "dotfiles", wantTags: []string{"work"}, wantOK: true},
		{name: "dotfiles.tag-work.tag-kde", base: "dotfiles", wantTags: []string{"work", "kde"}, wantOK: true},
		{name: "dotfiles.backup", base: "dotfiles", wantOK: false},
		{name: "dotfiles.tag-", base: "dotfiles", wantOK: false},
		{name: "templates.tag-work", base: "templates", wantTags: []string{"work"}, wantOK: true},
		{name: "something-else", base: "dotfiles", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseDir(tc.name, tc.base)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if strings.Join(got, ",") != strings.Join(tc.wantTags, ",") {
				t.Errorf("tags = %v, want %v", got, tc.wantTags)
			}
		})
	}
}
