package validate_test

import (
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
)

const threeLocales = "preview:\n  defaultLocale: cs\n  locales: [cs, en, it]\n"

// A group of the three languages, indented as an item of `fields:`, with
// the extra lines given after its label.
func langGroup(key, typ, extra string) string {
	return "  - key: " + key + "\n    type: group\n    label: " + key + "\n" + extra +
		"    fields:\n      - {key: cs, type: " + typ + ", label: Czech}\n      - {key: en, type: " + typ + ", label: English}\n      - {key: it, type: " + typ + ", label: Italian}\n"
}

// The keys of this release, used as documented, give no finding.
func TestLocalizedKeysUsedWell(t *testing.T) {
	files := map[string]string{
		load.ConfigFile: threeLocales,
		load.CollectionsDir + "/works.yml": "label: Works\npath: works\nlocaleFrom: none\ntranslations:\n  locales: [cs, en]\nfields:\n" +
			"  - key: kind\n    type: select\n    label: Kind\n    options:\n      - {value: solo, label: Solo, labels: {cs: Samostatná, en: Solo}}\n" +
			"  - {key: draft, type: select, label: State, role: draft, draftWhen: [wip], options: [wip, done]}\n" +
			"  - {key: robots, type: select, label: Robots, role: noindex, noindexWhen: ['noindex,follow'], options: ['index,follow', 'noindex,follow']}\n" +
			"  - key: image\n    type: image\n    label: Photo\n    alt: {from: [title, technique], separator: ' – '}\n" +
			langGroup("title", "text", "    localized: true\n    fallback: cs\n    role: seoTitle\n    titleTemplate: '{field:kind|label} – {value} – {{site_name}}'\n") +
			langGroup("technique", "text", "    localized: true\n") +
			langGroup("description", "richtext", "    localized: true\n    role: seoDescription\n    excerpt: firstParagraph\n"),
	}
	if got := schemaFindings(t, files); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestLocalizedKeysMisused(t *testing.T) {
	pages := func(fields string) map[string]string {
		return map[string]string{load.ConfigFile: threeLocales, load.CollectionsDir + "/pages.yml": "label: Pages\npath: content\nfields:\n" + fields}
	}
	perFile := func(fields string) map[string]string {
		return map[string]string{load.ConfigFile: threeLocales, load.CollectionsDir + "/pages.yml": "label: Pages\npath: content\nlocaleFrom: path\nfields:\n" + fields}
	}
	cases := []struct {
		name  string
		files map[string]string
		rule  report.Rule
		says  string
	}{
		{"localized not a boolean", pages(langGroup("t", "text", "    localized: yes please\n")), report.RuleSchemaInvalidLocalized, "true or false"},
		{"a language missing", pages("  - key: t\n    type: group\n    label: T\n    localized: true\n    fields:\n      - {key: cs, type: text, label: C}\n      - {key: en, type: text, label: E}\n"), report.RuleSchemaInvalidLocalized, `no field for "it"`},
		{"a field that is no language", pages(langGroup("t", "text", "    localized: true\n") + "      - {key: de, type: text, label: German}\n"), report.RuleSchemaInvalidLocalized, `"de" is not a language`},
		{"mixed types", pages(strings.Replace(langGroup("t", "text", "    localized: true\n"), "{key: it, type: text", "{key: it, type: paragraph", 1)), report.RuleSchemaInvalidLocalized, "mix text and paragraph"},
		{"a number per language", pages(langGroup("t", "number", "    localized: true\n")), report.RuleSchemaInvalidLocalized, "number field"},
		{"a role on a language", pages(strings.Replace(langGroup("t", "text", "    localized: true\n"), "{key: cs, type: text, label: Czech}", "{key: cs, type: text, label: Czech, role: seoTitle}", 1)), report.RuleSchemaInvalidLocalized, "role of its own"},
		{"fallback without localized", pages(langGroup("t", "text", "    fallback: cs\n")), report.RuleSchemaInvalidLocalized, "not localized"},
		{"fallback to a language the site lacks", pages(langGroup("t", "text", "    localized: true\n    fallback: de\n")), report.RuleSchemaInvalidLocalized, "not in preview.locales"},
		{"localized in a collection of one language per file", perFile(langGroup("t", "text", "    localized: true\n")), report.RuleSchemaInvalidLocalized, "one language"},
		{"a role on a plain group", pages(langGroup("t", "text", "    role: seoTitle\n")), report.RuleSchemaRoleTypeMismatch, "only when it is localized"},
		{"seoTitle on a localized rich text", pages(langGroup("t", "richtext", "    localized: true\n    role: seoTitle\n")), report.RuleSchemaRoleTypeMismatch, "localized group of richtext"},
		{"a template without a role", pages("  - {key: t, type: text, label: T, titleTemplate: '{value}'}\n"), report.RuleSchemaInvalidTitleTemplate, "not role seoTitle"},
		{"a template without its value", pages("  - {key: t, type: text, label: T, role: seoTitle, titleTemplate: 'Home'}\n"), report.RuleSchemaInvalidTitleTemplate, "{value}"},
		{"an unknown placeholder", pages("  - {key: t, type: text, label: T, role: seoTitle, titleTemplate: '{value} – {site}'}\n"), report.RuleSchemaInvalidTitleTemplate, "{site} is not a placeholder"},
		{"a lone brace", pages("  - {key: t, type: text, label: T, role: seoTitle, titleTemplate: '{value} }'}\n"), report.RuleSchemaInvalidTitleTemplate, "lone }"},
		{"a field the page lacks", pages("  - {key: t, type: text, label: T, role: seoTitle, titleTemplate: '{field:kind} {value}'}\n"), report.RuleSchemaInvalidTitleTemplate, "{field:kind}"},
		{"the label of a text", pages("  - {key: k, type: text, label: K}\n  - {key: t, type: text, label: T, role: seoTitle, titleTemplate: '{field:k|label} {value}'}\n"), report.RuleSchemaInvalidTitleTemplate, "not a select"},
		{"rich text description without excerpt", pages("  - {key: d, type: richtext, label: D, role: seoDescription}\n"), report.RuleSchemaInvalidExcerpt, "does not say which part"},
		{"an unknown excerpt", pages("  - {key: d, type: richtext, label: D, role: seoDescription, excerpt: lead}\n"), report.RuleSchemaInvalidExcerpt, "not a value"},
		{"excerpt on a paragraph", pages("  - {key: d, type: paragraph, label: D, role: seoDescription, excerpt: firstParagraph}\n"), report.RuleSchemaInvalidExcerpt, "only a rich text"},
		{"draftWhen on a boolean", pages("  - {key: d, type: boolean, label: D, role: draft, draftWhen: [x]}\n"), report.RuleSchemaInvalidRoleValues, "only a select"},
		{"noindexWhen naming no option", pages("  - {key: r, type: select, label: R, role: noindex, noindexWhen: [hidden], options: [index, noindex]}\n"), report.RuleSchemaInvalidRoleValues, "not one of its options"},
		{"draft select without draftWhen", pages("  - {key: d, type: select, label: D, role: draft, options: [wip, done]}\n"), report.RuleSchemaInvalidRoleValues, "no draftWhen"},
		{"noindexWhen beside role draft", pages("  - {key: d, type: select, label: D, role: draft, draftWhen: [wip], noindexWhen: [wip], options: [wip, done]}\n"), report.RuleSchemaInvalidRoleValues, "only a select with role noindex"},
		{"two drafts", pages("  - {key: a, type: boolean, label: A, role: draft}\n  - {key: b, type: boolean, label: B, role: draft}\n"), report.RuleSchemaDuplicateRole, "already declares"},
		{"composed alt naming a missing field", pages("  - {key: img, type: image, label: I, alt: {from: [a]}}\n"), report.RuleSchemaInvalidAlt, "no field in the same list"},
		{"composed alt naming a number", pages("  - {key: img, type: image, label: I, alt: {from: [n]}}\n  - {key: n, type: number, label: N}\n"), report.RuleSchemaInvalidAlt, "number field"},
		{"composed alt with an unknown key", pages("  - {key: img, type: image, label: I, alt: {from: [a], join: x}}\n  - {key: a, type: text, label: A}\n"), report.RuleSchemaInvalidAlt, "alt.join"},
		{"composed alt with no parts", pages("  - {key: img, type: image, label: I, alt: {from: []}}\n"), report.RuleSchemaInvalidAlt, "1 to 10"},
		{"a plain group as alt text", pages("  - {key: img, type: image, label: I, alt: g}\n  - {key: g, type: group, label: G, fields: [{key: a, type: text, label: A}]}\n"), report.RuleSchemaInvalidAlt, "group field"},
		{"option labels for a language the site lacks", pages("  - key: k\n    type: select\n    label: K\n    options:\n      - {value: a, labels: {de: A}}\n"), report.RuleSchemaInvalidLocalized, `label for "de"`},
		{"translations without by and no localized group", pages("  - {key: t, type: text, label: T}\ntranslations:\n  locales: [cs]\n"), report.RuleSchemaInvalidTranslations, "localized groups"},
	}
	for _, tc := range cases {
		got := schemaFindings(t, tc.files)
		if len(got) != 1 || got[0].Rule != tc.rule || !strings.Contains(got[0].Message+" "+got[0].Hint, tc.says) {
			t.Errorf("%s: got %v, want one %s saying %q", tc.name, got, tc.rule, tc.says)
			continue
		}
		if got[0].Severity != report.SeverityError {
			t.Errorf("%s: want an error, got %v", tc.name, got[0])
		}
	}
}

// A template in a block is read for its syntax only: the fields it names
// are the page's, judged where the block is used.
func TestTemplateInBlock(t *testing.T) {
	files := map[string]string{
		load.ConfigFile:                threeLocales,
		load.BlocksDir + "/seo.yml":    "label: SEO\nfields:\n  - {key: title, type: text, label: T, role: seoTitle, titleTemplate: '{field:kind} {value}'}\n",
		load.CollectionsDir + "/a.yml": "label: A\npath: a\nfields:\n  - {key: kind, type: text, label: Kind}\n  - {key: seo, $ref: seo}\n",
		load.CollectionsDir + "/b.yml": "label: B\npath: b\nfields:\n  - {key: seo, $ref: seo}\n",
	}
	got := schemaFindings(t, files)
	if len(got) != 1 || got[0].File != load.CollectionsDir+"/b.yml" || got[0].Rule != report.RuleSchemaInvalidTitleTemplate {
		t.Fatalf("got %v", got)
	}
}
