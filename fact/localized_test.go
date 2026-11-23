package fact_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/fact"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/schema"
)

// A portfolio collection whose texts hold every language.
func portfolioFiles() fstest.MapFS {
	return fstest.MapFS{
		load.ConfigFile: {Data: []byte("preview:\n  defaultLocale: cs\n  locales: [cs, en]\n")},
		load.CollectionsDir + "/works.yml": {Data: []byte(`label: Works
path: src/content/works
localeFrom: none
fields:
  - key: kind
    type: select
    label: Kind
    options: [solo, group]
  - key: robots
    type: select
    label: Robots
    options: ['index,follow', 'noindex,follow']
  - key: draft
    type: boolean
    label: Draft
  - key: image
    type: image
    label: Photo
  - key: title
    type: group
    label: Title
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
  - key: technique
    type: group
    label: Technique
    localized: true
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
  - key: description
    type: group
    label: Description
    localized: true
    fields:
      - {key: cs, type: richtext, label: Czech}
      - {key: en, type: richtext, label: English}
`)},
	}
}

// What a dashboard confirms in one click: a localized group with its
// role and template, a robots select's values, a draft field, a rich
// text description's excerpt and a composed alt text.
func TestMakeLocalizedFacts(t *testing.T) {
	fsys := portfolioFiles()
	facts := []fact.Fact{
		{Kind: fact.KindLocalized, Collection: "works", Field: "title", Fallback: "cs"},
		{Kind: fact.KindRole, Collection: "works", Field: "title", Role: schema.RoleSEOTitle, TitleTemplate: "{field:kind} – {value} – Adéla"},
		{Kind: fact.KindRole, Collection: "works", Field: "robots", Role: schema.RoleNoindex, When: []string{"noindex,follow"}},
		{Kind: fact.KindRole, Collection: "works", Field: "draft", Role: schema.RoleDraft},
		{Kind: fact.KindRole, Collection: "works", Field: "description", Role: schema.RoleSEODescription, Excerpt: schema.ExcerptFirstParagraph},
		{Kind: fact.KindAlt, Collection: "works", Field: "image", AltFrom: []string{"title", "technique"}, Separator: " – "},
	}
	p, err := fact.Make(fsys, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 {
		t.Fatalf("changes %+v", p.Changes)
	}
	got := string(p.Changes[0].After)
	for _, want := range []string{
		"    label: Title\n    role: seoTitle\n    titleTemplate: '{field:kind} – {value} – Adéla'\n    localized: true\n    fallback: cs\n",
		"    label: Robots\n    role: noindex\n    noindexWhen: ['noindex,follow']\n",
		"    label: Draft\n    role: draft\n",
		"    label: Description\n    role: seoDescription\n    excerpt: firstParagraph\n    localized: true\n",
		"    label: Photo\n    alt: {from: [title, technique], separator: \" – \"}\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if errs, err := p.NewErrors(fsys); err != nil || len(errs) != 0 {
		t.Errorf("new errors %v %v", errs, err)
	}
	// A role fact on a group goes after the group is localized: the plan
	// applies in order, and the same facts again change nothing.
	if again, err := fact.Make(p.FS(fsys), facts[1:]); err != nil || len(again.Changes) != 0 {
		t.Errorf("not idempotent: %v %+v", err, again)
	}
	// Another role drops the knobs the old one had.
	p2, err := fact.Make(p.FS(fsys), []fact.Fact{{Kind: fact.KindRole, Collection: "works", Field: "robots", Role: schema.RoleNone}})
	if err != nil {
		t.Fatal(err)
	}
	if after := string(p2.Changes[0].After); strings.Contains(after, "noindexWhen") || !strings.Contains(after, "    label: Robots\n    role: none\n    options:") {
		t.Errorf("role none kept noindexWhen:\n%s", after)
	}
}

func TestMakeLocalizedRefuses(t *testing.T) {
	cases := []struct {
		name string
		f    fact.Fact
		err  error
	}{
		{"a role on a plain group", fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "title", Role: schema.RoleSEOTitle}, fact.ErrInvalid},
		{"a select without its values", fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "robots", Role: schema.RoleNoindex}, fact.ErrInvalid},
		{"a value that is no option", fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "robots", Role: schema.RoleNoindex, When: []string{"hidden"}}, fact.ErrInvalid},
		{"a rich text description without excerpt", fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "description", Role: schema.RoleSEODescription}, fact.ErrInvalid},
		{"localized on a field that is no group", fact.Fact{Kind: fact.KindLocalized, Collection: "works", Field: "kind"}, fact.ErrInvalid},
		{"a fallback the site lacks", fact.Fact{Kind: fact.KindLocalized, Collection: "works", Field: "title", Fallback: "de"}, fact.ErrInvalid},
		{"alt on a field that is no image", fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "kind", Alt: "title"}, fact.ErrInvalid},
		{"alt from a plain group", fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "image", Alt: "title"}, fact.ErrInvalid},
		{"alt from a missing field", fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "image", AltFrom: []string{"nope"}}, fact.ErrNotFound},
	}
	for _, tc := range cases {
		if _, err := fact.Make(portfolioFiles(), []fact.Fact{tc.f}); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v", tc.name, err)
		}
	}
	bad := []fact.Fact{
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleSEODescription, TitleTemplate: "{value}"},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleSEOTitle, TitleTemplate: "{nope}"},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleSEOTitle, Excerpt: schema.ExcerptFirstParagraph},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleCanonical, When: []string{"x"}},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleNoindex, When: []string{"x", "x"}},
		{Kind: fact.KindAlt, Collection: "c", Field: "a"},
		{Kind: fact.KindAlt, Collection: "c", Field: "a", Alt: "b", AltFrom: []string{"c"}},
		{Kind: fact.KindAlt, Collection: "c", Field: "a", Alt: "b", Separator: "-"},
		{Kind: fact.KindAlt, Collection: "c", Field: "a", AltFrom: []string{"b.c"}},
		{Kind: fact.KindLocalized, Collection: "c"},
		{Kind: fact.KindLocalized, Collection: "c", Field: "a", Fallback: "c s"},
	}
	for _, f := range bad {
		if err := f.Check(); !errors.Is(err, fact.ErrInvalid) {
			t.Errorf("%+v: got %v", f, err)
		}
	}
}
