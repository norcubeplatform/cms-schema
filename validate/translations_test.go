package validate_test

import (
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

const twoLocales = "preview:\n  defaultLocale: cs\n  locales: [cs, en]\n"

func pagesCollection(extra string) string {
	return "label: Pages\npath: content\nlocaleFrom: path\n" + extra +
		"fields:\n  - key: title\n    type: text\n    label: Title\n  - key: translationKey\n    type: text\n    label: Key\n    required: true\n  - key: order\n    type: number\n    label: Order\n  - key: note\n    type: text\n    label: Note\n"
}

// A collection says how its language versions are linked, in a way every
// page can answer. Anything else is an error on the declaration itself.
func TestTranslationsDeclaration(t *testing.T) {
	cases := []struct {
		name, collection string
		problem          string
	}{
		{"none declared", pagesCollection(""), ""},
		{"by a required text field", pagesCollection("translations:\n  by: field:translationKey\n"), ""},
		{"by path, with the locale in the path", pagesCollection("translations:\n  by: path\n"), ""},
		{"by path, without the locale in the path", strings.Replace(pagesCollection("translations:\n  by: path\n"), "localeFrom: path\n", "", 1), "does not come from the path"},
		{"an unknown way", pagesCollection("translations:\n  by: slug\n"), "is not path or field:<key>"},
		{"no way at all", pagesCollection("translations: {}\n"), "is not path or field:<key>"},
		{"a field with no key", pagesCollection("translations:\n  by: 'field:'\n"), "is not path or field:<key>"},
		{"a field the collection lacks", pagesCollection("translations:\n  by: field:group\n"), "not a top-level field"},
		{"a field that is not text", pagesCollection("translations:\n  by: field:order\n"), "must be text"},
		{"a field that is optional", pagesCollection("translations:\n  by: field:note\n"), "not required"},
	}
	for _, tc := range cases {
		files := map[string]string{load.ConfigFile: twoLocales, load.CollectionsDir + "/pages.yml": tc.collection}
		res, err := validate.Site(site(files), validate.Options{Only: []string{validate.ScopeSchema}})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got []report.Finding
		for _, f := range res.Findings {
			if f.Rule == report.RuleSchemaInvalidTranslations {
				got = append(got, f)
			}
		}
		switch {
		case tc.problem == "" && len(got) != 0:
			t.Errorf("%s: reported %v", tc.name, got)
		case tc.problem != "" && (len(got) != 1 || !strings.Contains(got[0].Message, tc.problem) || got[0].Path != "translations.by" || got[0].Severity != report.SeverityError || got[0].Hint == ""):
			t.Errorf("%s: got %v, want one error at translations.by saying %q, with a hint", tc.name, got, tc.problem)
		}
	}
}

// `translatable` is a key every field may carry: it is not an unknown one.
func TestTranslatableIsAKnownKey(t *testing.T) {
	collection := strings.Replace(pagesCollection(""), "    label: Title\n", "    label: Title\n    translatable: false\n", 1)
	res, err := validate.Site(site(map[string]string{load.ConfigFile: twoLocales, load.CollectionsDir + "/pages.yml": collection}), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.Rule == report.RuleSchemaUnknownKnob {
			t.Errorf("translatable was reported as unknown: %v", f)
		}
	}
}

// A page has one version per language. Two files of a collection and a
// locale with one key are reported, on the second by path; the same key
// in two languages is what the key is for.
func TestTranslationCollision(t *testing.T) {
	page := func(key string) string { return "---\ntitle: T\ntranslationKey: " + key + "\n---\n" }
	files := map[string]string{
		load.ConfigFile:                    twoLocales,
		load.CollectionsDir + "/pages.yml": pagesCollection("translations:\n  by: field:translationKey\n"),
		"content/cs/cenik.md":              page("pricing"),
		"content/en/pricing.md":            page("pricing"),
		"content/cs/kontakt.md":            page("contact"),
		"content/cs/kontakty.md":           page("contact"),
		"content/en/about.md":              page("about"),
	}
	res, err := validate.Site(site(files), validate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []report.Finding
	for _, f := range res.Findings {
		if f.Rule == report.RuleLinksTranslationCollision {
			got = append(got, f)
		}
	}
	if len(got) != 1 || got[0].File != "content/cs/kontakty.md" || !strings.Contains(got[0].Message, "content/cs/kontakt.md") || got[0].Severity != report.SeverityError {
		t.Fatalf("got %v, want one error on content/cs/kontakty.md naming content/cs/kontakt.md", got)
	}

	// By path, the files above are all different pages: nothing collides.
	// The same path with another extension is the same page twice.
	collisions := func() int {
		res, err := validate.Site(site(files), validate.Options{})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range res.Findings {
			if f.Rule == report.RuleLinksTranslationCollision {
				n++
			}
		}
		return n
	}
	files[load.CollectionsDir+"/pages.yml"] = pagesCollection("translations:\n  by: path\n")
	if n := collisions(); n != 0 {
		t.Errorf("by path: %d collisions among files with different paths", n)
	}
	files["content/cs/cenik.mdx"] = page("x")
	if n := collisions(); n != 1 {
		t.Errorf("by path: %d collisions for one page in two files, want 1", n)
	}
}

// `translatable` means something on text and on the groups and lists
// that hold it. Anywhere else it changes nothing, and the schema says so.
func TestTranslatableOnAValueThatIsNeverTranslated(t *testing.T) {
	const fields = `  - key: seo
    type: group
    label: SEO
    translatable: false
    fields:
      - key: description
        type: paragraph
        label: Description
        translatable: true
      - key: priority
        type: number
        label: Priority
        translatable: false
  - key: tags
    type: array
    label: Tags
    translatable: true
    items:
      - key: tag
        type: text
        label: Tag
  - key: hero
    type: image
    label: Hero
    translatable: true
  - key: kind
    type: select
    label: Kind
    options: [a, b]
    translatable: true
`
	res, err := validate.Site(site(map[string]string{load.ConfigFile: twoLocales, load.CollectionsDir + "/pages.yml": pagesCollection("") + fields}), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Findings {
		if f.Rule != report.RuleSchemaTranslatableIgnored {
			t.Errorf("unexpected finding: %v", f)
			continue
		}
		if f.Severity != report.SeverityWarning || f.Hint == "" {
			t.Errorf("want a warning with a hint: %v", f)
		}
		got = append(got, f.Path)
	}
	// Saying false on such a field is harmless and is not reported.
	want := []string{"fields.hero", "fields.kind"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("reported at %v, want %v", got, want)
	}
}
