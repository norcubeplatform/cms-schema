package resolve_test

import (
	"reflect"
	"testing"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

var portfolio = map[string]string{
	load.ConfigFile: "preview:\n  defaultLocale: cs\n  locales: [cs, en]\n",
	load.CollectionsDir + "/works.yml": `label: Works
path: works
localeFrom: none
fields:
  - {key: draft, type: select, label: State, role: draft, draftWhen: [wip], options: [wip, done]}
  - {key: robots, type: select, label: Robots, options: ['index,follow', 'noindex,follow', none]}
  - key: image
    type: image
    label: Photo
    alt: {from: [title, technique], separator: ' – '}
  - key: title
    type: group
    label: Title
    localized: true
    fallback: cs
    role: seoTitle
    titleTemplate: '{value} – Adéla'
    fields: [{key: cs, type: text, label: C}, {key: en, type: text, label: E}]
  - key: technique
    type: group
    label: Technique
    localized: true
    fields: [{key: cs, type: text, label: C}, {key: en, type: text, label: E}]
  - key: description
    type: group
    label: Description
    localized: true
    role: seoDescription
    excerpt: firstParagraph
    fields: [{key: cs, type: richtext, label: C}, {key: en, type: richtext, label: E}]
`,
}

// Roles on localized groups, a template, an excerpt, a draft select and
// a guessed robots select come back with what a reader needs.
func TestLocalizedRoles(t *testing.T) {
	cfg := loadSchema(t, portfolio)
	got := resolve.CollectionRoles(cfg, cfg.Collections["works"])
	want := []resolve.RoleField{
		{Role: schema.RoleSEOTitle, Path: "title", Label: "Title", Declared: true, Type: schema.TypeText, Localized: true, Fallback: "cs", TitleTemplate: "{value} – Adéla"},
		{Role: schema.RoleSEODescription, Path: "description", Label: "Description", Declared: true, Type: schema.TypeRichText, Localized: true, Excerpt: schema.ExcerptFirstParagraph},
		{Role: schema.RoleNoindex, Path: "robots", Label: "Robots", Type: schema.TypeSelect, When: []string{"noindex,follow", "none"}},
		{Role: schema.RoleDraft, Path: "draft", Label: "State", Declared: true, Type: schema.TypeSelect, When: []string{"wip"}},
	}
	if !reflect.DeepEqual(got.Fields, want) {
		t.Fatalf("roles:\n got %+v\nwant %+v", got.Fields, want)
	}
	wantImages := []resolve.ImageAlt{{Image: "image", Label: "Photo", AltFrom: []string{"title", "technique"}, Separator: " – ", Declared: true, AltLocalized: true}}
	if !reflect.DeepEqual(got.Images, wantImages) {
		t.Fatalf("images:\n got %+v\nwant %+v", got.Images, wantImages)
	}
}

func TestLocalizedValue(t *testing.T) {
	v := map[string]any{"cs": " Les ", "en": ""}
	cases := []struct {
		locale, fallback, want string
		borrowed               bool
	}{
		{"cs", "", "Les", false},
		{"en", "", "", false},
		{"en", "cs", "Les", true},
		{"cs", "cs", "Les", false},
	}
	for _, c := range cases {
		got, borrowed := resolve.LocalizedValue(v, c.locale, c.fallback)
		if got != c.want || borrowed != c.borrowed {
			t.Errorf("%s/%s: %q %v", c.locale, c.fallback, got, borrowed)
		}
	}
	if got, _ := resolve.LocalizedValue("a string", "cs", "cs"); got != "" {
		t.Errorf("a string is no language: %q", got)
	}
}

func TestComposeTitle(t *testing.T) {
	fields := []schema.Field{
		{Key: "kind", Type: schema.TypeSelect, Options: []schema.SelectOption{{Value: "solo", Label: "Samostatná", Labels: map[string]string{"en": "Solo"}}}},
		{Key: "venue", Type: schema.TypeText},
	}
	vars := resolve.SiteVariables(map[string]any{"variables": []any{
		map[string]any{"key": "site_name", "value": "Adéla"},
		map[string]any{"key": "city", "translations": map[string]any{"cs": "Brno", "en": "Brno, CZ"}},
	}}, "variables")
	ctx := resolve.TitleContext{Fields: fields, Frontmatter: map[string]any{"kind": "solo", "venue": "  Votruba "}, Locale: "en", DefaultLocale: "cs", Variables: vars}
	cases := []struct {
		template, value, want string
		ok                    bool
	}{
		{"{value} – {{site_name}}", "Forest", "Forest – Adéla", true},
		{"{field:kind|label} – {value} ({{ city }})", "X", "Solo – X (Brno, CZ)", true},
		{"{field:kind} at {field:venue}: {value}", "X", "solo at Votruba: X", true},
		{"{value} – {{nobody}}", "X", "", false},
		{"{value}", "Hi {{nobody}}", "", false},
		{"{nope} {value}", "X", "", false},
	}
	for _, c := range cases {
		got, ok := resolve.ComposeTitle(c.template, c.value, ctx)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %q %v, want %q %v", c.template, got, ok, c.want, c.ok)
		}
	}
	ctx.Locale = "cs"
	if got, _ := resolve.ComposeTitle("{field:kind|label} {value}", "X", ctx); got != "Samostatná X" {
		t.Errorf("cs label: %q", got)
	}
}

func TestFirstParagraph(t *testing.T) {
	cases := []struct {
		text string
		html bool
		want string
	}{
		{"First *one*, with [a link](/x/) and **bold**.\n\nSecond.", false, "First one, with a link and bold."},
		{"# Heading\n\nAfter the heading.", false, "After the heading."},
		{"Heading\n=======\n\nBody text.", false, "Body text."},
		{"```\ncode\n```\n\nText.", false, "Text."},
		{"One line\nwrapped here.\n\nNext.", false, "One line wrapped here."},
		{"> A quote\n> goes on.", false, "A quote goes on."},
		{"snake_case and 2*3*4 stay", false, "snake_case and 2*3*4 stay"},
		{"<h2>Title</h2><p>The <em>first</em> &amp; only.</p><p>Second</p>", true, "The first & only."},
		{"", false, ""},
	}
	for _, c := range cases {
		if got := resolve.FirstParagraph(c.text, c.html); got != c.want {
			t.Errorf("%q: %q, want %q", c.text, got, c.want)
		}
	}
}

func TestURLIn(t *testing.T) {
	coll := schema.Collection{Path: "works", PreviewPath: "/{locale}/portfolio/{slug}/", LocaleFrom: schema.LocaleFromNone}
	site := schema.PreviewSite{DefaultLocale: "cs", Locales: []string{"cs", "en"}}
	fm := map[string]any{"slug": map[string]any{"cs": "les", "en": "forest"}}
	if got := resolve.URLIn("works/a.md", coll, site, fm, "en"); got != "/en/portfolio/forest/" {
		t.Errorf("en: %q", got)
	}
	if got := resolve.URLIn("works/a.md", coll, site, map[string]any{}, "cs"); got != "/portfolio/a/" {
		t.Errorf("cs: %q", got)
	}
}
