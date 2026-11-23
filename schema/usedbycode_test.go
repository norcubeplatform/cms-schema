package schema_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/schema"
)

func TestCheckCodePattern(t *testing.T) {
	for _, p := range []string{"logo.svg", "og/*.png", "**/*.jpg", "icons/**", "a/[ab].png", "dir/sub/file name.webp"} {
		if err := schema.CheckCodePattern(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"", "/abs.png", "../up.png", "a/./b.png", "a//b", "a**.png", "a\\b.png", "bad[.png", "x\n.png", strings.Repeat("a", schema.MaxUsedByCodePattern+1)} {
		if err := schema.CheckCodePattern(p); !errors.Is(err, schema.ErrCodePattern) {
			t.Errorf("%q: got %v", p, err)
		}
	}
}

func TestMatchCodePattern(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"logo.svg", "logo.svg", true},
		{"logo.svg", "sub/logo.svg", false},
		{"*.svg", "logo.svg", true},
		{"*.svg", "sub/logo.svg", false},
		{"**/*.svg", "logo.svg", true},
		{"**/*.svg", "a/b/logo.svg", true},
		{"og/**", "og/cs/home.jpg", true},
		{"og/**", "og", true},
		{"og/**", "other/home.jpg", false},
		{"a/**/z.png", "a/z.png", true},
		{"a/**/z.png", "a/b/c/z.png", true},
		{"a/**/z.png", "a/b/c/y.png", false},
		{"../x", "x", false},
		{"x", "", false},
	}
	for _, c := range cases {
		if got := schema.MatchCodePattern(c.pattern, c.rel); got != c.want {
			t.Errorf("%q ~ %q: got %v", c.pattern, c.rel, got)
		}
	}
}

func TestStorageKeepsFile(t *testing.T) {
	s := schema.Storage{Path: "public/images", UsedByCode: []string{"logo.svg", "og/**"}}
	for p, want := range map[string]bool{
		"public/images/logo.svg":     true,
		"public/images/og/a/b.png":   true,
		"public/images/other.png":    false,
		"public/imageslogo.svg":      false,
		"src/public/images/logo.svg": false,
	} {
		if got := s.KeepsFile(p); got != want {
			t.Errorf("%s: got %v", p, got)
		}
	}
	if (schema.Storage{UsedByCode: []string{"*"}}).KeepsFile("x") {
		t.Error("a storage without a path keeps nothing")
	}
}
