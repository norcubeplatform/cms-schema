package fact_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/norcubeplatform/cms-schema/fact"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func role(path, r, whenEmpty string) fact.Edit {
	steps := []fact.Step{}
	for _, p := range strings.Split(path, ".") {
		if name, ok := strings.CutSuffix(p, "[]"); ok {
			steps = append(steps, fact.Step{Key: name}, fact.Step{Item: true})
			continue
		}
		steps = append(steps, fact.Step{Key: p})
	}
	return fact.Edit{Fact: fact.Fact{Kind: fact.KindRole, Collection: "pages", Field: path, Role: r, WhenEmpty: whenEmpty}, File: "x.yml", Steps: steps}
}

// Golden files: each case applies its edits in turn to <in>.in.yml and
// must give <name>.want.yml, a second run changing nothing.
func TestApplyGolden(t *testing.T) {
	cases := []struct {
		name, in string
		edits    []fact.Edit
	}{
		{"role-insert", "collection", []fact.Edit{role("meta.title", schema.RoleSEOTitle, "")}},
		{"role-replace-keeps-comment", "collection", []fact.Edit{role("meta.description", schema.RoleSEODescription, schema.WhenEmptyGenerated)}},
		{"role-none-drops-when-empty", "collection", []fact.Edit{role("meta.description", schema.RoleNone, "")}},
		{"role-in-flow-mapping", "collection", []fact.Edit{role("meta.image", schema.RoleShareImage, schema.WhenEmptySiteDefault)}},
		{"role-in-list-item-after-folded-label", "collection", []fact.Edit{role("logos[].name", schema.RoleAlt, "")}},
		{"role-top-level", "collection", []fact.Edit{role("title", schema.RoleNone, "")}},
		{"several-on-one-file", "collection", []fact.Edit{
			role("meta.title", schema.RoleSEOTitle, ""),
			role("meta.description", schema.RoleSEODescription, ""),
			role("meta.image", schema.RoleShareImage, schema.WhenEmptyGenerated),
		}},
		{"locales-insert", "collection", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindTranslationLocales, Collection: "pages", Locales: []string{"cs"}}}}},
		{"locales-replace-block-list", "locales-block", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindTranslationLocales, Collection: "c", Locales: []string{"cs"}}}}},
		{"locales-replace-flow", "locales-flow", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindTranslationLocales, Collection: "c", Locales: []string{"no", "cs"}}}}},
		{"storage-insert-block", "config", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindOptimizedAtBuild, Storage: "images", OptimizedAtBuild: no()}, In: []string{"storages", "images"}}}},
		{"storage-insert-flow", "config", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindOptimizedAtBuild, Storage: "assets", OptimizedAtBuild: yes()}, In: []string{"storages", "assets"}}}},
		{"storage-replace", "config", []fact.Edit{{Fact: fact.Fact{Kind: fact.KindOptimizedAtBuild, Storage: "old", OptimizedAtBuild: yes()}, In: []string{"storages", "old"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", tc.in+".in.yml"))
			if err != nil {
				t.Fatal(err)
			}
			out := src
			for _, e := range tc.edits {
				if out, err = fact.Apply(out, e); err != nil {
					t.Fatal(err)
				}
			}
			golden := filepath.Join("testdata", tc.name+".want.yml")
			if *update {
				if err := os.WriteFile(golden, out, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != string(want) {
				t.Fatalf("got\n%s\nwant\n%s", out, want)
			}
			again := out
			for _, e := range tc.edits {
				if again, err = fact.Apply(again, e); err != nil {
					t.Fatal(err)
				}
			}
			if string(again) != string(out) {
				t.Errorf("not idempotent:\n%s", again)
			}
			var v any
			if err := yaml.Unmarshal(out, &v); err != nil {
				t.Errorf("not YAML: %v", err)
			}
		})
	}
}

func TestApplyRefuses(t *testing.T) {
	cases := []struct {
		name, src string
		edit      fact.Edit
		err       error
	}{
		{"two fields with one key", "fields:\n  - key: a\n    type: text\n  - key: a\n    type: text\n", role("a", schema.RoleSEOTitle, ""), fact.ErrAmbiguous},
		{"no such field", "fields:\n  - key: a\n    type: text\n", role("b", schema.RoleSEOTitle, ""), fact.ErrNotFound},
		{"a field that is a $ref", "fields:\n  - $ref: seo\n    key: seo\n", role("seo", schema.RoleNone, ""), fact.ErrAmbiguous},
		{"every value over several lines", "fields:\n  - key: |-\n      a\n    type: >-\n      text\n", role("a", schema.RoleSEOTitle, ""), fact.ErrUnplaceable},
		{"two shapes and none named", "fields:\n  - key: l\n    type: array\n    items:\n      - {key: a, type: text}\n      - {key: b, type: text}\n", fact.Edit{Fact: fact.Fact{Kind: fact.KindRole, Role: schema.RoleNone}, Steps: []fact.Step{{Key: "l"}, {Item: true}}}, fact.ErrAmbiguous},
		{"not a mapping", "- a\n", role("a", schema.RoleSEOTitle, ""), fact.ErrUnplaceable},
		{"no translations", "label: X\n", fact.Edit{Fact: fact.Fact{Kind: fact.KindTranslationLocales, Locales: []string{"cs"}}}, fact.ErrUnplaceable},
	}
	for _, tc := range cases {
		_, err := fact.Apply([]byte(tc.src), tc.edit)
		if !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
}

// A field whose only one-line keys come before a multi-line label gets
// the role after one of them.
func TestApplyAfterAnyOneLineKey(t *testing.T) {
	src := "fields:\n  - key: a\n    label: |-\n      Long\n      label\n    type: text\n"
	out, err := fact.Apply([]byte(src), role("a", schema.RoleSEOTitle, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := "fields:\n  - key: a\n    label: |-\n      Long\n      label\n    type: text\n    role: seoTitle\n"
	if string(out) != want {
		t.Fatalf("got\n%s", out)
	}
}

func TestFactCheck(t *testing.T) {
	bad := []fact.Fact{
		{Kind: "nope"},
		{Kind: fact.KindRole, Field: "a", Role: schema.RoleSEOTitle},
		{Kind: fact.KindRole, Collection: "c", Block: "b", Field: "a", Role: schema.RoleSEOTitle},
		{Kind: fact.KindRole, Collection: "c", Role: schema.RoleSEOTitle},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: "seo"},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleNone, WhenEmpty: schema.WhenEmptyGenerated},
		{Kind: fact.KindRole, Collection: "c", Field: "a", Role: schema.RoleAlt, WhenEmpty: schema.WhenEmptyGenerated},
		{Kind: fact.KindRole, Collection: "c", Field: strings.Repeat("a", fact.MaxFieldPath+1), Role: schema.RoleNone},
		{Kind: fact.KindRole, Collection: "../c", Field: "a", Role: schema.RoleNone},
		{Kind: fact.KindTranslationLocales, Collection: "c"},
		{Kind: fact.KindTranslationLocales, Collection: "c", Locales: []string{"cs", "cs"}},
		{Kind: fact.KindTranslationLocales, Collection: "c", Locales: []string{"c s"}},
		{Kind: fact.KindOptimizedAtBuild, Storage: "s"},
		{Kind: fact.KindOptimizedAtBuild, OptimizedAtBuild: yes()},
	}
	for _, f := range bad {
		if err := f.Check(); !errors.Is(err, fact.ErrInvalid) {
			t.Errorf("%+v: got %v", f, err)
		}
	}
	good := []fact.Fact{
		{Kind: fact.KindRole, Collection: "c", Field: "seo.title", Role: schema.RoleSEOTitle, WhenEmpty: schema.WhenEmptyGenerated},
		{Kind: fact.KindRole, Block: "seo", Field: "list[card].img", Role: schema.RoleNone},
		{Kind: fact.KindTranslationLocales, Collection: "c", Locales: []string{"cs", "en-GB"}},
		{Kind: fact.KindOptimizedAtBuild, Storage: "s", OptimizedAtBuild: no()},
	}
	for _, f := range good {
		if err := f.Check(); err != nil {
			t.Errorf("%+v: %v", f, err)
		}
	}
	if len(fact.Kinds()) != 3 {
		t.Errorf("kinds %v", fact.Kinds())
	}
}

// siteFiles is a small directory-layout site: pages pulls the seo block
// in by $ref, case studies is written inline in config.yml.
func siteFiles() fstest.MapFS {
	return fstest.MapFS{
		load.ConfigFile: {Data: []byte(`preview:
  defaultLocale: cs
  locales: [cs, en]
storages:
  images:
    type: repo
    path: public/images
collections:
  cases:
    label: Case studies
    path: src/content/cases
    translations:
      by: path
      localeFrom: path
    fields:
      - key: title
        type: text
        label: Title
`)},
		load.CollectionsDir + "/pages.yml": {Data: []byte(`label: Pages
path: src/content/pages
fields:
  - key: title
    type: text
    label: Title
  - $ref: seo
  - key: blocks
    type: array
    label: Blocks
    items:
      - $ref: card
`)},
		load.BlocksDir + "/seo.yml": {Data: []byte(`label: SEO
fields:
  # The title in search results.
  - key: title
    type: text
    label: Title
  - key: description
    type: paragraph
    label: Description
`)},
		load.BlocksDir + "/card.yml": {Data: []byte(`label: Card
fields:
  - key: img
    type: image
    label: Image
  - key: caption
    type: text
    label: Caption
`)},
	}
}

func TestMakeLocatesAcrossFiles(t *testing.T) {
	fsys := siteFiles()
	facts := []fact.Fact{
		{Kind: fact.KindRole, Collection: "pages", Field: "seo.title", Role: schema.RoleSEOTitle},
		{Kind: fact.KindRole, Collection: "pages", Field: "seo.description", Role: schema.RoleSEODescription, WhenEmpty: schema.WhenEmptySiteDefault},
		{Kind: fact.KindRole, Collection: "pages", Field: "blocks[].caption", Role: schema.RoleAlt},
		{Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"cs"}},
		{Kind: fact.KindOptimizedAtBuild, Storage: "images", OptimizedAtBuild: no()},
		{Kind: fact.KindRole, Collection: "pages", Field: "title", Role: schema.RoleNone},
	}
	p, err := fact.Make(fsys, facts)
	if err != nil {
		t.Fatal(err)
	}
	files := []string{}
	for _, c := range p.Changes {
		files = append(files, c.Path)
	}
	want := []string{load.BlocksDir + "/seo.yml", load.BlocksDir + "/card.yml", load.ConfigFile, load.CollectionsDir + "/pages.yml"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("files %v, want %v", files, want)
	}
	if p.Edits[3].File != load.ConfigFile || strings.Join(p.Edits[3].In, ".") != "collections.cases" {
		t.Errorf("inline collection edit %+v", p.Edits[3])
	}
	seo := string(p.Changes[0].After)
	if !strings.Contains(seo, "  # The title in search results.\n  - key: title\n    type: text\n    label: Title\n    role: seoTitle\n") ||
		!strings.Contains(seo, "    role: seoDescription\n    whenEmpty: siteDefault\n") {
		t.Errorf("seo.yml:\n%s", seo)
	}
	errs, err := p.NewErrors(fsys)
	if err != nil || len(errs) != 0 {
		t.Errorf("new errors %v %v", errs, err)
	}
	// The same facts again change nothing.
	again, err := fact.Make(p.FS(fsys), facts)
	if err != nil || len(again.Changes) != 0 {
		t.Errorf("not idempotent: %v %+v", err, again)
	}
}

func TestMakeRefuses(t *testing.T) {
	cases := []struct {
		name  string
		facts []fact.Fact
		err   error
		index int
	}{
		{"an unknown collection", []fact.Fact{{Kind: fact.KindRole, Collection: "nope", Field: "a", Role: schema.RoleNone}}, fact.ErrNotFound, 0},
		{"an unknown field", []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "seo.nope", Role: schema.RoleNone}}, fact.ErrNotFound, 0},
		{"a role the field's type cannot hold", []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "seo.title", Role: schema.RoleShareImage}}, fact.ErrInvalid, 0},
		{"a language the site lacks", []fact.Fact{{Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"de"}}}, fact.ErrInvalid, 0},
		{"a collection without language versions", []fact.Fact{{Kind: fact.KindTranslationLocales, Collection: "pages", Locales: []string{"cs"}}}, fact.ErrInvalid, 0},
		{"an unknown storage", []fact.Fact{{Kind: fact.KindOptimizedAtBuild, Storage: "nope", OptimizedAtBuild: yes()}}, fact.ErrNotFound, 0},
		{"a bad second fact", []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "title", Role: schema.RoleNone}, {Kind: "x"}}, fact.ErrInvalid, 1},
		{"a path ending at a list item", []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "blocks[]", Role: schema.RoleNone}}, fact.ErrInvalid, 0},
		{"a path with an empty key", []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "seo..title", Role: schema.RoleNone}}, fact.ErrInvalid, 0},
	}
	for _, tc := range cases {
		_, err := fact.Make(siteFiles(), tc.facts)
		var fe *fact.FactError
		if !errors.Is(err, tc.err) || !errors.As(err, &fe) || fe.Index != tc.index {
			t.Errorf("%s: got %v", tc.name, err)
		}
	}
	if _, err := fact.Make(siteFiles(), nil); !errors.Is(err, fact.ErrInvalid) {
		t.Errorf("no facts: %v", err)
	}
	many := make([]fact.Fact, fact.MaxFacts+1)
	if _, err := fact.Make(siteFiles(), many); !errors.Is(err, fact.ErrInvalid) {
		t.Errorf("too many facts: %v", err)
	}
}

// A collection written both inline and in its own file has no one
// place to write to.
func TestMakeRefusesTwoSources(t *testing.T) {
	fsys := siteFiles()
	fsys[load.CollectionsDir+"/cases.yml"] = &fstest.MapFile{Data: []byte("label: Cases\npath: src/content/cases\ntranslations: {by: path, localeFrom: path}\nfields: []\n")}
	_, err := fact.Make(fsys, []fact.Fact{{Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"cs"}}})
	if !errors.Is(err, fact.ErrAmbiguous) {
		t.Errorf("got %v", err)
	}
}

// A fact that would make the schema invalid is caught by NewErrors: two
// fields with one page role.
func TestNewErrors(t *testing.T) {
	fsys := siteFiles()
	p, err := fact.Make(fsys, []fact.Fact{
		{Kind: fact.KindRole, Collection: "pages", Field: "title", Role: schema.RoleSEOTitle},
		{Kind: fact.KindRole, Collection: "pages", Field: "seo.title", Role: schema.RoleSEOTitle},
	})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := p.NewErrors(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 || errs[0].Rule != report.RuleSchemaDuplicateRole {
		t.Errorf("errors %v", errs)
	}
	// Errors the site already has are not the plan's.
	fsys[load.BlocksDir+"/broken.yml"] = &fstest.MapFile{Data: []byte("label: B\nfields:\n  - key: x\n    type: nope\n    label: X\n")}
	p, err = fact.Make(fsys, []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "title", Role: schema.RoleNone}})
	if err != nil {
		t.Fatal(err)
	}
	if errs, err := p.NewErrors(fsys); err != nil || len(errs) != 0 {
		t.Errorf("errors %v %v", errs, err)
	}
}

func TestFactString(t *testing.T) {
	cases := map[string]fact.Fact{
		"pages.seo.title: role seoTitle, whenEmpty generated": {Kind: fact.KindRole, Collection: "pages", Field: "seo.title", Role: schema.RoleSEOTitle, WhenEmpty: schema.WhenEmptyGenerated},
		"block seo.title: role none":                          {Kind: fact.KindRole, Block: "seo", Field: "title", Role: schema.RoleNone},
		"cases: translations.locales [cs]":                    {Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"cs"}},
		"storage images: optimizedAtBuild true":               {Kind: fact.KindOptimizedAtBuild, Storage: "images", OptimizedAtBuild: yes()},
	}
	for want, f := range cases {
		if got := f.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// A field pulled in from an input is edited in the input's file, whose
// root is the field; a legacy single file holds everything inline.
func TestMakeInputsAndLegacy(t *testing.T) {
	fsys := siteFiles()
	fsys[load.InputsDir+"/og.yml"] = &fstest.MapFile{Data: []byte("type: image # the share card\nlabel: Share image\n")}
	fsys[load.CollectionsDir+"/pages.yml"] = &fstest.MapFile{Data: []byte("label: Pages\npath: src/content/pages\nfields:\n  - key: title\n    type: text\n    label: Title\n  - $ref: og\n")}
	p, err := fact.Make(fsys, []fact.Fact{{Kind: fact.KindRole, Collection: "pages", Field: "og", Role: schema.RoleShareImage, WhenEmpty: schema.WhenEmptyGenerated}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Path != load.InputsDir+"/og.yml" ||
		string(p.Changes[0].After) != "type: image # the share card\nlabel: Share image\nrole: shareImage\nwhenEmpty: generated\n" {
		t.Fatalf("changes %+v", p.Changes)
	}

	legacy := fstest.MapFS{load.LegacyConfigFile: {Data: []byte(`preview: {defaultLocale: cs, locales: [cs, en]}
collections:
  pages:
    label: Pages
    path: src/content/pages
    translations: {by: path, localeFrom: path}
    fields:
      - key: seo_title
        type: text
        label: SEO title
`)}}
	p, err = fact.Make(legacy, []fact.Fact{
		{Kind: fact.KindRole, Collection: "pages", Field: "seo_title", Role: schema.RoleNone},
		{Kind: fact.KindTranslationLocales, Collection: "pages", Locales: []string{"cs"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "translations: {by: path, locales: [cs], localeFrom: path}\n    fields:\n      - key: seo_title\n        type: text\n        label: SEO title\n        role: none\n"
	if len(p.Changes) != 1 || !strings.HasSuffix(string(p.Changes[0].After), want) {
		t.Fatalf("legacy:\n%s", p.Changes[0].After)
	}
}
