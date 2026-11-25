package fact_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/fact"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/schema"
)

// errorCase is one refusal the fact package gives: a fact checked on
// its own, a plan made over a site, or an edit applied to one file.
type errorCase struct {
	name  string
	check *fact.Fact
	make  []fact.Fact
	fsys  fstest.MapFS
	src   string
	edit  *fact.Edit
}

func (c errorCase) run() error {
	switch {
	case c.check != nil:
		return c.check.Check()
	case c.edit != nil:
		_, err := fact.Apply([]byte(c.src), *c.edit)
		return err
	}
	fsys := c.fsys
	if fsys == nil {
		fsys = errorSite()
	}
	_, err := fact.Make(fsys, c.make)
	return err
}

// errorSite is siteFiles with what the refusals below need on top.
func errorSite() fstest.MapFS {
	fsys := siteFiles()
	fsys[load.ConfigFile] = &fstest.MapFile{Data: []byte(`preview:
  defaultLocale: cs
  locales: [cs, en]
storages:
  images:
    type: repo
    path: public/images
  cloud:
    type: s3
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
`)}
	fsys[load.CollectionsDir+"/works.yml"] = &fstest.MapFile{Data: []byte(`label: Works
path: src/content/works
fields:
  - key: title
    type: text
    label: Title
  - key: twice
    type: text
    label: Twice
  - key: twice
    type: text
    label: Twice again
  - key: name
    type: group
    label: Name
    fields:
      - {key: cs, type: text, label: Czech}
  - key: names
    type: group
    label: Names
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: de, type: text, label: German}
  - key: robots
    type: select
    label: Robots
    options: [index, hide]
  - key: photo
    type: image
    label: Photo
  - key: count
    type: number
    label: Count
  - key: list
    type: array
    label: List
    items:
      - {key: a, type: text, label: A}
      - {key: b, type: text, label: B}
  - key: same
    type: array
    label: Same
    items:
      - {key: a, type: text, label: A}
      - {key: a, type: text, label: A again}
  - key: sections
    type: blocks
    label: Sections
    components: [card]
  - $ref: nowhere
`)}
	return fsys
}

// declaredTwice declares the cases collection in config.yml and in its
// own file.
func declaredTwice() fstest.MapFS {
	fsys := errorSite()
	fsys[load.CollectionsDir+"/cases.yml"] = &fstest.MapFile{Data: []byte("label: Cases\npath: src/content/cases2\nfields:\n  - {key: title, type: text, label: Title}\n")}
	return fsys
}

func long(n int) string { return strings.Repeat("a", n) }

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "v" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
	}
	return out
}

func errorCases() []errorCase {
	role := func(f fact.Fact) *fact.Fact { f.Kind = fact.KindRole; return &f }
	chk := func(f fact.Fact) *fact.Fact { return &f }
	mk := func(f fact.Fact) []fact.Fact { return []fact.Fact{f} }
	works := func(field, r string) fact.Fact {
		return fact.Fact{Kind: fact.KindRole, Collection: "works", Field: field, Role: r}
	}
	edit := func(f fact.Fact, in []string, steps ...fact.Step) *fact.Edit {
		return &fact.Edit{Fact: f, File: "x.yml", In: in, Steps: steps}
	}
	none := fact.Fact{Kind: fact.KindRole, Role: schema.RoleNone}
	return []errorCase{
		// Fact.Check.
		{name: "check/role-owner", check: role(fact.Fact{Collection: "a", Block: "b", Field: "x", Role: schema.RoleNone})},
		{name: "check/localized-owner", check: chk(fact.Fact{Kind: fact.KindLocalized, Field: "x"})},
		{name: "check/alt-owner", check: chk(fact.Fact{Kind: fact.KindAlt, Field: "x", Alt: "y"})},
		{name: "check/role-field", check: role(fact.Fact{Collection: "a", Role: schema.RoleNone})},
		{name: "check/localized-field", check: chk(fact.Fact{Kind: fact.KindLocalized, Collection: "a"})},
		{name: "check/alt-field", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Alt: "y"})},
		{name: "check/role-unknown", check: role(fact.Fact{Collection: "a", Field: "x", Role: "bogus"})},
		{name: "check/when-empty", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleCanonical, WhenEmpty: schema.WhenEmptyGenerated})},
		{name: "check/title-template-role", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEODescription, TitleTemplate: "{value}"})},
		{name: "check/template-empty", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: " "})},
		{name: "check/template-too-long", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "{value}" + long(300)})},
		{name: "check/template-lone-close", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "{value} }"})},
		{name: "check/template-unclosed", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "{value} {"})},
		{name: "check/template-field-path", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "{value} {field:a b}"})},
		{name: "check/template-placeholder", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "{value} {foo}"})},
		{name: "check/template-parts", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: strings.Repeat("{value}x", 26)})},
		{name: "check/template-no-value", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, TitleTemplate: "Title"})},
		{name: "check/excerpt", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, Excerpt: schema.ExcerptFirstParagraph})},
		{name: "check/when-role", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleSEOTitle, When: []string{"a"}})},
		{name: "check/when-too-many", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleNoindex, When: names(101)})},
		{name: "check/when-blank", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleNoindex, When: []string{" "}})},
		{name: "check/when-twice", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleNoindex, When: []string{"a", "a"}})},
		{name: "check/role-extra", check: role(fact.Fact{Collection: "a", Field: "x", Role: schema.RoleNone, Fallback: "cs"})},
		{name: "check/fallback-code", check: chk(fact.Fact{Kind: fact.KindLocalized, Collection: "a", Field: "x", Fallback: "c s"})},
		{name: "check/alt-one", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x"})},
		{name: "check/alt-key", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", Alt: "a b"})},
		{name: "check/alt-from-too-many", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", AltFrom: names(11)})},
		{name: "check/alt-from-blank", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", AltFrom: []string{""}})},
		{name: "check/alt-from-twice", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", AltFrom: []string{"a", "a"}})},
		{name: "check/alt-from-key", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", AltFrom: []string{"a b"}})},
		{name: "check/separator", check: chk(fact.Fact{Kind: fact.KindAlt, Collection: "a", Field: "x", Alt: "y", Separator: "-"})},
		{name: "check/locales-owner", check: chk(fact.Fact{Kind: fact.KindTranslationLocales, Locales: []string{"cs"}})},
		{name: "check/locales-count", check: chk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "a"})},
		{name: "check/locales-code", check: chk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "a", Locales: []string{"c s"}})},
		{name: "check/locales-twice", check: chk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "a", Locales: []string{"cs", "cs"}})},
		{name: "check/optimized", check: chk(fact.Fact{Kind: fact.KindOptimizedAtBuild})},
		{name: "check/used-by-code", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s"})},
		{name: "check/pattern-empty", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{""}})},
		{name: "check/pattern-too-long", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{long(301)}})},
		{name: "check/pattern-absolute", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"/a.svg"}})},
		{name: "check/pattern-backslash", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{`a\b.svg`}})},
		{name: "check/pattern-control", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"a\x01b.svg"}})},
		{name: "check/pattern-empty-name", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"a//b.svg"}})},
		{name: "check/pattern-dot-name", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"../a.svg"}})},
		{name: "check/pattern-double-star", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"a**b/c.svg"}})},
		{name: "check/pattern-invalid", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"["}})},
		{name: "check/paths-twice", check: chk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"a.svg", "a.svg"}})},
		{name: "check/kind", check: chk(fact.Fact{Kind: "x"})},
		{name: "check/name", check: role(fact.Fact{Collection: "a b", Field: "x", Role: schema.RoleNone})},
		// fact.Make.
		{name: "make/count"},
		{name: "make/locales-collection", make: mk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "nope", Locales: []string{"cs"}})},
		{name: "make/locales-no-versions", make: mk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "pages", Locales: []string{"cs"}})},
		{name: "make/locales-language", make: mk(fact.Fact{Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"de"}})},
		{name: "make/optimized-storage", make: mk(fact.Fact{Kind: fact.KindOptimizedAtBuild, Storage: "nope", OptimizedAtBuild: yes()})},
		{name: "make/used-by-code-storage", make: mk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "nope", Paths: []string{"a.svg"}})},
		{name: "make/used-by-code-no-files", make: mk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "cloud", Paths: []string{"a.svg"}})},
		{name: "make/used-by-code-no-file", make: mk(fact.Fact{Kind: fact.KindUsedByCode, Storage: "images", Paths: []string{"nope.svg"}})},
		{name: "make/role-cannot-hold", make: mk(works("count", schema.RoleShareImage))},
		{name: "make/role-localized-cannot-hold", fsys: portfolioFiles(), make: mk(fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "technique", Role: schema.RoleShareImage})},
		{name: "make/role-when-option", make: mk(fact.Fact{Kind: fact.KindRole, Collection: "works", Field: "robots", Role: schema.RoleNoindex, When: []string{"gone"}})},
		{name: "make/localized-not-group", make: mk(fact.Fact{Kind: fact.KindLocalized, Collection: "works", Field: "title"})},
		{name: "make/localized-count", make: mk(fact.Fact{Kind: fact.KindLocalized, Collection: "works", Field: "name"})},
		{name: "make/localized-field", make: mk(fact.Fact{Kind: fact.KindLocalized, Collection: "works", Field: "names"})},
		{name: "make/alt-not-image", make: mk(fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "title", Alt: "count"})},
		{name: "make/alt-no-field", make: mk(fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "photo", Alt: "ghost"})},
		{name: "make/alt-cannot-hold", make: mk(fact.Fact{Kind: fact.KindAlt, Collection: "works", Field: "photo", Alt: "count"})},
		{name: "make/collection", make: mk(fact.Fact{Kind: fact.KindRole, Collection: "nope", Field: "a", Role: schema.RoleNone})},
		{name: "make/block", make: mk(fact.Fact{Kind: fact.KindRole, Block: "nope", Field: "a", Role: schema.RoleNone})},
		{name: "make/field", make: mk(works("seo.nope", schema.RoleNone))},
		{name: "make/two-fields", make: mk(works("twice", schema.RoleNone))},
		{name: "make/item-of-scalar", make: mk(works("title[].a", schema.RoleNone))},
		{name: "make/block-in-list", make: mk(works("sections[hero].title", schema.RoleNone))},
		{name: "make/unnamed-shape", make: mk(works("list[].a", schema.RoleNone))},
		{name: "make/shape", make: mk(works("list[c].a", schema.RoleNone))},
		{name: "make/shapes-named-twice", make: mk(works("same[a].b", schema.RoleNone))},
		{name: "make/declared-twice", fsys: declaredTwice(), make: mk(fact.Fact{Kind: fact.KindRole, Collection: "cases", Field: "title", Role: schema.RoleNone})},
		{name: "make/path-list-first", make: mk(works("[a].b", schema.RoleNone))},
		{name: "make/path", make: mk(works("seo..title", schema.RoleNone))},
		// fact.Apply.
		{name: "apply/kind", src: "label: X\n", edit: edit(fact.Fact{Kind: "x"}, nil)},
		{name: "apply/no-mapping-at", src: "label: X\n", edit: edit(none, []string{"collections", "a"})},
		{name: "apply/within", src: "label: X\n", edit: edit(fact.Fact{Kind: fact.KindTranslationLocales, Locales: []string{"cs"}}, nil)},
		{name: "apply/no-fields", src: "label: X\n", edit: edit(none, nil, fact.Step{Key: "a"})},
		{name: "apply/no-field", src: "fields:\n  - key: a\n    type: text\n", edit: edit(none, nil, fact.Step{Key: "b"})},
		{name: "apply/two-fields", src: "fields:\n  - key: a\n    type: text\n  - key: a\n    type: text\n", edit: edit(none, nil, fact.Step{Key: "a"})},
		{name: "apply/field-ref", src: "fields:\n  - $ref: seo\n    key: seo\n", edit: edit(none, nil, fact.Step{Key: "seo"})},
		{name: "apply/no-items", src: "fields:\n  - key: l\n    type: array\n", edit: edit(none, nil, fact.Step{Key: "l"}, fact.Step{Item: true})},
		{name: "apply/items-not-list", src: "fields:\n  - key: l\n    type: array\n    items: text\n", edit: edit(none, nil, fact.Step{Key: "l"}, fact.Step{Item: true})},
		{name: "apply/shape", src: "fields:\n  - key: l\n    type: array\n    items:\n      - {key: a, type: text}\n", edit: edit(none, nil, fact.Step{Key: "l"}, fact.Step{Item: true, Shape: "b"})},
		{name: "apply/shapes", src: "fields:\n  - key: l\n    type: array\n    items:\n      - {key: a, type: text}\n      - {key: b, type: text}\n", edit: edit(none, nil, fact.Step{Key: "l"}, fact.Step{Item: true})},
		{name: "apply/shape-ref", src: "fields:\n  - key: l\n    type: array\n    items:\n      - $ref: card\n", edit: edit(none, nil, fact.Step{Key: "l"}, fact.Step{Item: true})},
		{name: "apply/no-key-line", src: "fields:\n  - key: |-\n      a\n    type: >-\n      text\n", edit: edit(fact.Fact{Kind: fact.KindRole, Role: schema.RoleSEOTitle}, nil, fact.Step{Key: "a"})},
		{name: "apply/not-yaml", src: "a: [\n", edit: edit(none, nil)},
		{name: "apply/not-mapping", src: "- a\n", edit: edit(none, nil)},
		{name: "apply/empty-mapping", src: "{}\n", edit: edit(none, nil)},
		{name: "apply/several-lines", src: "fields:\n  - key: a\n    role: |-\n      x\n", edit: edit(fact.Fact{Kind: fact.KindRole, Role: schema.RoleSEOTitle}, nil, fact.Step{Key: "a"})},
		{name: "apply/shares-line", src: "fields:\n  - whenEmpty: generated\n    key: a\n    role: seoTitle\n", edit: edit(none, nil, fact.Step{Key: "a"})},
		{name: "apply/used-by-code-too-many", src: "storages:\n  s:\n    type: repo\n    usedByCode: [" + strings.Join(names(500), ", ") + "]\n", edit: edit(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"new.svg"}}, []string{"storages", "s"})},
		{name: "apply/used-by-code-no-mapping", src: "label: X\n", edit: edit(fact.Fact{Kind: fact.KindUsedByCode, Storage: "s", Paths: []string{"new.svg"}}, []string{"storages", "s"})},
	}
}

// TestErrorCorpusEnglish pins the text of every refusal in the corpus:
// what fact.Make returns and the CMS shows when it refuses facts.
func TestErrorCorpusEnglish(t *testing.T) {
	got := map[string]string{}
	for _, c := range errorCases() {
		err := c.run()
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		got[c.name] = err.Error()
	}
	golden := filepath.Join("testdata", "errors.golden.json")
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *update {
		if err := os.WriteFile(golden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if got[k] != want[k] {
			t.Errorf("%s:\n got %q\nwant %q", k, got[k], want[k])
		}
	}
	if len(want) != len(got) {
		t.Errorf("%d cases, golden has %d", len(got), len(want))
	}
}
