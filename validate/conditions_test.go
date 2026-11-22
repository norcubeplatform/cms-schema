package validate_test

import (
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

// listConditionSchema has conditions whose values are lists and maps,
// the shape that used to crash the validator ("comparing uncomparable
// type []interface {}").
const listConditionSchema = `label: Pages
path: src/content/pages
previewPath: '/{slug}/'
localeFrom: none
fields:
  - key: title
    type: text
    label: Title
  - key: slug
    type: text
    label: Slug
  - key: gallery
    type: array
    label: Gallery
    items:
      - key: image
        type: text
        label: Image
  - key: meta
    type: group
    label: Meta
    fields:
      - key: k
        type: text
        label: K
  - key: whenEmpty
    type: text
    label: When the gallery is the empty list
    required: true
    visibleIf: { field: gallery, op: eq, value: [] }
  - key: whenOld
    type: text
    label: Old spelling
    required: true
    visibleIf: { field: gallery, equals: [] }
  - key: whenOne
    type: text
    label: When the gallery is exactly one image
    required: true
    visibleIf: { field: gallery, op: in, value: [[a.jpg], [b.jpg]] }
  - key: whenMeta
    type: text
    label: When meta is not empty
    required: true
    visibleIf: { field: meta, op: neq, value: {} }
`

func listConditionSite() map[string]string {
	return map[string]string{
		load.ConfigFile:                    "pagesDir: src/content/pages\npreview: {defaultLocale: en, locales: [en]}\n",
		load.CollectionsDir + "/pages.yml": listConditionSchema,
		"src/content/pages/one.md":         "---\ntitle: One\nslug: one\ngallery: [a.jpg]\nmeta: {k: v}\n---\n",
		"src/content/pages/empty.md":       "---\ntitle: Empty\nslug: empty\ngallery: []\nmeta: {}\n---\n",
		"src/content/pages/none.md":        "---\ntitle: None\nslug: none\n---\n",
	}
}

// TestSiteWithListAndMapConditions: list and map condition values are
// compared structurally and the run returns findings instead of
// panicking.
func TestSiteWithListAndMapConditions(t *testing.T) {
	res, err := validate.Site(site(listConditionSite()), validate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := rules(res.Findings)
	want := map[string]int{
		// [a.jpg] is one of the listed lists; meta is not {}.
		"src/content/pages/one.md#whenOne content.required":  1,
		"src/content/pages/one.md#whenMeta content.required": 1,
		// [] equals [] under both spellings.
		"src/content/pages/empty.md#whenEmpty content.required": 1,
		"src/content/pages/empty.md#whenOld content.required":   1,
		// An absent gallery is not the empty list; an absent meta is not {}.
		"src/content/pages/none.md#whenMeta content.required": 1,
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d, want %d", k, got[k], n)
		}
	}
	for k := range got {
		if strings.Contains(k, "internal_error") {
			t.Errorf("unexpected %s", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("findings %v, want exactly %v", got, want)
	}
}

// TestPageWithListAndMapConditions: the same through validate.Page, the
// call the CMS makes per file.
func TestPageWithListAndMapConditions(t *testing.T) {
	res, err := validate.Site(site(listConditionSite()), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	coll := res.Config.Collections["pages"]
	doc, err := content.Parse("src/content/pages/one.md", []byte("---\ngallery: [b.jpg]\nmeta: {k: v}\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := rules(validate.Page(res.Config, coll, doc))
	if got["src/content/pages/one.md#whenOne content.required"] != 1 || got["src/content/pages/one.md#whenMeta content.required"] != 1 || len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

// TestEntryPointsDoNotPanic: a bug in a check (here provoked by inputs
// no loader produces) comes back as the check's internal-error finding.
// Page's recovery is pinned in recover_test.go, as no input is known to
// make it panic, and Links' there too, as its input needs a parsed page.
func TestEntryPointsDoNotPanic(t *testing.T) {
	assertInternal := func(t *testing.T, fs []report.Finding, file string, rule report.Rule) {
		t.Helper()
		if len(fs) != 1 || fs[0].Rule != rule || fs[0].File != file || fs[0].Severity != report.SeverityError || !strings.Contains(fs[0].Message, "internal error") {
			t.Fatalf("got %v, want one %s finding on %q", fs, rule, file)
		}
	}
	t.Run("Schema", func(t *testing.T) {
		assertInternal(t, validate.Schema(nil, nil), load.ConfigFile, report.RuleSchemaInternalError)
	})
}
