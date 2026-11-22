package validate_test

import (
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
	"github.com/norcubeplatform/cms-schema/validate"
)

// roleRules are the findings these tests look at.
var roleRules = map[report.Rule]bool{
	report.RuleSchemaUnknownRole: true, report.RuleSchemaDuplicateRole: true,
	report.RuleSchemaRoleTypeMismatch: true, report.RuleSchemaMisplacedRole: true,
	report.RuleSchemaInvalidAlt: true, report.RuleSchemaUnknownKnob: true,
	report.RuleSchemaInvalidTranslationLocales: true, report.RuleSchemaInvalidWhenEmpty: true,
}

func schemaFindings(t *testing.T, files map[string]string) []report.Finding {
	t.Helper()
	res, err := validate.Site(site(files), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	var out []report.Finding
	for _, f := range res.Findings {
		if roleRules[f.Rule] {
			out = append(out, f)
		}
	}
	return out
}

// A collection whose fields are the YAML lines given, indented as items
// of `fields:`.
func collectionWith(fields string) string {
	return "label: Pages\npath: content\nfields:\n" + fields
}

const seoBlock = `label: SEO
fields:
  - key: page_title
    type: text
    label: Meta title
    role: seoTitle
  - key: page_description
    type: paragraph
    label: Meta description
    role: seoDescription
    whenEmpty: siteDefault
  - key: featured_image
    type: image
    label: Share image
    role: shareImage
    whenEmpty: generated
    alt: featured_image_alt
  - key: featured_image_alt
    type: text
    label: Share image alt
  - key: canonical_url
    type: link
    label: Canonical URL
    role: canonical
  - key: no_index
    type: boolean
    label: Hide from search engines
    role: noindex
`

const logosBlock = `label: Logos
fields:
  - key: list
    type: array
    label: Logos
    items:
      - key: logo
        type: group
        label: Logo
        fields:
          - key: name
            type: text
            label: Client name
            role: alt
          - key: img
            type: image
            label: Logo
  - key: background
    type: image
    label: Background
    decorative: true
`

// Every declaration the schema language offers, used as documented,
// passes without a finding.
func TestRolesWellDeclared(t *testing.T) {
	files := map[string]string{
		load.ConfigFile: "preview:\n  defaultLocale: cs\n  locales: [cs, en]\n",
		load.CollectionsDir + "/pages.yml": collectionWith(`  - key: title
    type: text
    label: Title
  - $ref: seo
  - key: blocks
    type: array
    label: Blocks
    items:
      - $ref: logos
`),
		load.BlocksDir + "/seo.yml":   seoBlock,
		load.BlocksDir + "/logos.yml": logosBlock,
	}
	if got := schemaFindings(t, files); len(got) != 0 {
		t.Fatalf("well-declared roles reported: %v", got)
	}
}

func TestRoleMisuse(t *testing.T) {
	cases := []struct {
		name   string
		fields string
		rule   report.Rule
		path   string
		says   string
	}{
		{
			"an unknown role",
			"  - key: t\n    type: text\n    label: T\n    role: seo.title\n",
			report.RuleSchemaUnknownRole, "fields.t", "not a role",
		},
		{
			"a role that is not a string",
			"  - key: t\n    type: text\n    label: T\n    role: [seoTitle]\n",
			report.RuleSchemaUnknownRole, "fields.t", "not a role",
		},
		{
			"a share image on a text field",
			"  - key: t\n    type: text\n    label: T\n    role: shareImage\n",
			report.RuleSchemaRoleTypeMismatch, "fields.t", "cannot hold",
		},
		{
			"noindex on a select",
			"  - key: robots\n    type: select\n    label: Robots\n    options: [index, noindex]\n    role: noindex\n",
			report.RuleSchemaRoleTypeMismatch, "fields.robots", "cannot hold",
		},
		{
			"an SEO title in rich text",
			"  - key: t\n    type: richtext\n    label: T\n    role: seoTitle\n",
			report.RuleSchemaRoleTypeMismatch, "fields.t", "cannot hold",
		},
		{
			"one role on two fields, across a group",
			"  - key: title\n    type: text\n    label: Title\n    role: seoTitle\n  - key: seo\n    type: group\n    label: SEO\n    fields:\n      - key: title\n        type: text\n        label: Meta title\n        role: seoTitle\n",
			report.RuleSchemaDuplicateRole, "fields.seo.fields.title", "already declares",
		},
		{
			"a page role inside a list",
			"  - key: items\n    type: array\n    label: Items\n    items:\n      - key: item\n        type: group\n        label: Item\n        fields:\n          - key: title\n            type: text\n            label: Title\n            role: seoTitle\n",
			report.RuleSchemaMisplacedRole, "fields.items.items[item].fields.title", "repeats per item",
		},
		{
			"a page role on a list of primitives",
			"  - key: titles\n    type: array\n    label: Titles\n    items:\n      - key: t\n        type: text\n        label: T\n        role: seoTitle\n",
			report.RuleSchemaMisplacedRole, "fields.titles.items[t]", "repeats per item",
		},
		{
			"alt text with no image beside it",
			"  - key: caption\n    type: text\n    label: Caption\n    role: alt\n",
			report.RuleSchemaMisplacedRole, "fields.caption", "holds no image",
		},
		{
			"alt text beside two images",
			"  - key: a\n    type: image\n    label: A\n  - key: b\n    type: image\n    label: B\n  - key: caption\n    type: text\n    label: Caption\n    role: alt\n",
			report.RuleSchemaInvalidAlt, "fields.caption", "holds 2 images",
		},
		{
			"two alt texts for one image",
			"  - key: img\n    type: image\n    label: Img\n  - key: a\n    type: text\n    label: A\n    role: alt\n  - key: b\n    type: text\n    label: B\n    role: alt\n",
			report.RuleSchemaDuplicateRole, "fields.b", "already declares",
		},
		{
			"alt text for a decorative image",
			"  - key: img\n    type: image\n    label: Img\n    decorative: true\n  - key: a\n    type: text\n    label: A\n    role: alt\n",
			report.RuleSchemaInvalidAlt, "fields.a", "is decorative",
		},
		{
			"role alt beside an image that names another field",
			"  - key: img\n    type: image\n    label: Img\n    alt: b\n  - key: a\n    type: text\n    label: A\n    role: alt\n  - key: b\n    type: text\n    label: B\n",
			report.RuleSchemaInvalidAlt, "fields.a", "names \"b\"",
		},
		{
			"alt naming no sibling",
			"  - key: img\n    type: image\n    label: Img\n    alt: img_alt\n",
			report.RuleSchemaInvalidAlt, "fields.img", "no field in the same list",
		},
		{
			"alt naming a field in another group",
			"  - key: img\n    type: image\n    label: Img\n    alt: seo.alt\n  - key: seo\n    type: group\n    label: SEO\n    fields:\n      - key: alt\n        type: text\n        label: Alt\n",
			report.RuleSchemaInvalidAlt, "fields.img", "no field in the same list",
		},
		{
			"alt naming a number",
			"  - key: img\n    type: image\n    label: Img\n    alt: order\n  - key: order\n    type: number\n    label: Order\n",
			report.RuleSchemaInvalidAlt, "fields.img", "is a number field",
		},
		{
			"alt naming the image itself",
			"  - key: img\n    type: image\n    label: Img\n    alt: img\n",
			report.RuleSchemaInvalidAlt, "fields.img", "names itself",
		},
		{
			"alt that is not a key",
			"  - key: img\n    type: image\n    label: Img\n    alt: true\n",
			report.RuleSchemaInvalidAlt, "fields.img", "names no field",
		},
		{
			"decorative with alt text",
			"  - key: img\n    type: image\n    label: Img\n    decorative: true\n    alt: a\n  - key: a\n    type: text\n    label: A\n",
			report.RuleSchemaInvalidAlt, "fields.img", "decorative and also",
		},
		{
			"decorative that is not a boolean",
			"  - key: img\n    type: image\n    label: Img\n    decorative: yes please\n",
			report.RuleSchemaInvalidAlt, "fields.img", "true or false",
		},
		{
			"whenEmpty with a value it does not take",
			"  - key: og\n    type: image\n    label: OG\n    role: shareImage\n    whenEmpty: fallback\n",
			report.RuleSchemaInvalidWhenEmpty, "fields.og", "not a value it takes",
		},
		{
			"whenEmpty that is not a string",
			"  - key: og\n    type: image\n    label: OG\n    role: shareImage\n    whenEmpty: true\n",
			report.RuleSchemaInvalidWhenEmpty, "fields.og", "not a value it takes",
		},
		{
			"whenEmpty without a role",
			"  - key: og\n    type: image\n    label: OG\n    whenEmpty: generated\n",
			report.RuleSchemaInvalidWhenEmpty, "fields.og", "no role",
		},
		{
			"whenEmpty on alt text",
			"  - key: img\n    type: image\n    label: Img\n  - key: a\n    type: text\n    label: A\n    role: alt\n    whenEmpty: generated\n",
			report.RuleSchemaInvalidWhenEmpty, "fields.a", "role alt",
		},
		{
			"whenEmpty on noindex",
			"  - key: hide\n    type: boolean\n    label: Hide\n    role: noindex\n    whenEmpty: siteDefault\n",
			report.RuleSchemaInvalidWhenEmpty, "fields.hide", "role noindex",
		},
		{
			"whenEmpty beside an unknown role is the role's problem",
			"  - key: og\n    type: image\n    label: OG\n    role: ogImage\n    whenEmpty: generated\n",
			report.RuleSchemaUnknownRole, "fields.og", "not a role",
		},
		{
			"alt on a field that is not an image",
			"  - key: t\n    type: text\n    label: T\n    alt: a\n  - key: a\n    type: text\n    label: A\n",
			report.RuleSchemaUnknownKnob, "fields.t", "does not read",
		},
	}
	for _, tc := range cases {
		files := map[string]string{load.ConfigFile: "", load.CollectionsDir + "/pages.yml": collectionWith(tc.fields)}
		got := schemaFindings(t, files)
		if len(got) != 1 || got[0].Rule != tc.rule || got[0].Path != tc.path || !strings.Contains(got[0].Message, tc.says) {
			t.Errorf("%s: got %v, want one %s at %s saying %q", tc.name, got, tc.rule, tc.path, tc.says)
			continue
		}
		if tc.rule != report.RuleSchemaUnknownKnob && (got[0].Severity != report.SeverityError || got[0].File != load.CollectionsDir+"/pages.yml") {
			t.Errorf("%s: want an error in the collection file, got %v", tc.name, got[0])
		}
		if got[0].Hint == "" && tc.rule != report.RuleSchemaInvalidAlt {
			t.Errorf("%s: no hint", tc.name)
		}
	}
}

// A block with a page role, used as an item of a list, is reported where
// it is used; the block on its own is fine.
func TestPageRoleInsideAListedBlock(t *testing.T) {
	files := map[string]string{
		load.ConfigFile: "",
		load.CollectionsDir + "/pages.yml": collectionWith(`  - key: sections
    type: array
    label: Sections
    items:
      - $ref: seo
  - key: legacy
    type: blocks
    label: Legacy
    components: [seo]
`),
		load.BlocksDir + "/seo.yml": seoBlock,
	}
	got := rules(schemaFindings(t, files))
	file := load.CollectionsDir + "/pages.yml"
	if got[file+"#fields.sections.items[seo].fields.page_title schema.misplaced_role"] != 1 ||
		got[file+"#fields.legacy schema.misplaced_role"] != 5 || len(got) != 6 {
		t.Fatalf("got %v", got)
	}
}

// Roles also checks a config that was never written, as the schema
// generator proposes it.
func TestRolesOnAParsedConfig(t *testing.T) {
	cfg, err := schema.Parse(map[string]any{
		"collections": map[string]any{"pages": map[string]any{"fields": []any{
			map[string]any{"key": "t", "type": "text", "label": "T", "role": "shareImage"},
		}}},
		"blocks": map[string]any{"seo": map[string]any{"label": "SEO", "fields": []any{
			map[string]any{"key": "a", "type": "text", "label": "A", "role": "seoTitle"},
			map[string]any{"key": "b", "type": "text", "label": "B", "role": "seoTitle"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := rules(validate.Roles(cfg))
	if got[load.CollectionsDir+"/pages.yml#fields.t schema.role_type_mismatch"] != 1 || got[load.BlocksDir+"/seo.yml#fields.b schema.duplicate_role"] != 1 || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if validate.Roles(nil) != nil {
		t.Fatal("a nil config has no findings")
	}
}

func TestTranslationLocales(t *testing.T) {
	cases := []struct {
		name, config, locales string
		says                  []string
	}{
		{"a subset of the site's languages", twoLocales, "[cs]", nil},
		{"all of them", twoLocales, "[cs, en]", nil},
		{"not declared", twoLocales, "", nil},
		{"a language the site lacks", twoLocales, "[cs, de]", []string{`"de", which is not in preview.locales`}},
		{"a site without languages", "", "[cs]", []string{"declares no preview.locales"}},
		{"an empty list", twoLocales, "[]", []string{"lists no language"}},
		{"not a list", twoLocales, "cs", []string{"not a list"}},
		{"a repeat", twoLocales, "[cs, cs]", []string{`"cs" twice`}},
		{"something that is not a code", twoLocales, "[cs, 3]", []string{"not a language code"}},
	}
	for _, tc := range cases {
		block := "translations:\n  by: field:translationKey\n"
		if tc.locales != "" {
			block += "  locales: " + tc.locales + "\n"
		}
		files := map[string]string{load.ConfigFile: tc.config, load.CollectionsDir + "/pages.yml": pagesCollection(block)}
		got := schemaFindings(t, files)
		if len(got) != len(tc.says) {
			t.Errorf("%s: got %v, want %d findings", tc.name, got, len(tc.says))
			continue
		}
		for i, want := range tc.says {
			if f := got[i]; f.Rule != report.RuleSchemaInvalidTranslationLocales || f.Path != "translations.locales" || f.Severity != report.SeverityError || !strings.Contains(f.Message, want) || f.Hint == "" {
				t.Errorf("%s: got %v, want an error at translations.locales saying %q", tc.name, f, want)
			}
		}
	}
}
