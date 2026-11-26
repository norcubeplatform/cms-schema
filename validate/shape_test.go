package validate_test

import (
	"testing"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

// shapeSchema has a field of every type a value can have the wrong
// shape for.
const shapeSchema = `label: Pages
path: src/content/pages
previewPath: '/{slug}/'
localeFrom: none
fields:
  - {key: title, type: text, label: Title}
  - {key: body, type: richtext, label: Body}
  - {key: count, type: number, label: Count}
  - {key: score, type: range, label: Score, min: 0, max: 10}
  - {key: flag, type: boolean, label: Flag}
  - {key: tier, type: select, label: Tier, options: [a, b]}
  - {key: tags, type: multichoice, label: Tags, options: [a, b]}
  - {key: partner, type: reference, label: Partner}
  - {key: partners, type: reference, label: Partners, multiple: true}
  - key: meta
    type: group
    label: Meta
    fields:
      - {key: k, type: text, label: K}
  - key: gallery
    type: array
    label: Gallery
    items:
      - {key: image, type: text, label: Image}
`

// Each field type asks for its kind of value and names the kind the
// value has, as codes; the English reads as before the codes.
func TestShapeNamesTheValueKinds(t *testing.T) {
	res, err := validate.Site(site(map[string]string{
		load.ConfigFile:                    "pagesDir: src/content/pages\npreview: {defaultLocale: en, locales: [en]}\n",
		load.CollectionsDir + "/pages.yml": shapeSchema,
		"src/content/pages/one.md": "---\ntitle: [a, b]\nbody: {k: v}\ncount: five\nscore: true\nflag: 3\n" +
			"tier: [a]\ntags: a\npartner: 5\npartners: [1, 2]\nmeta: x\ngallery: x\n---\n",
	}), validate.Options{Only: []string{validate.ScopeContent}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		expected, actual report.ValueKind
		message          string
	}{
		"title":    {report.ValueKindText, report.ValueKindList, "Title should be a text but is a list."},
		"body":     {report.ValueKindText, report.ValueKindObject, "Body should be a text but is a object."},
		"count":    {report.ValueKindNumber, report.ValueKindString, "Count should be a number but is a string."},
		"score":    {report.ValueKindNumber, report.ValueKindBoolean, "Score should be a number but is a boolean."},
		"flag":     {report.ValueKindBoolean, report.ValueKindNumber, "Flag should be a boolean but is a number."},
		"tier":     {report.ValueKindSelectableValue, report.ValueKindList, "Tier should be a selectable value but is a list."},
		"tags":     {report.ValueKindListOfSelectableValues, report.ValueKindString, "Tags should be a list of selectable values but is a string."},
		"partner":  {report.ValueKindReference, report.ValueKindNumber, "Partner should be a reference but is a number."},
		"partners": {report.ValueKindListOfReferences, report.ValueKindList, "Partners should be a list of references but is a list."},
		"meta":     {report.ValueKindGroupOfFields, report.ValueKindString, "Meta should be a group of fields but is a string."},
		"gallery":  {report.ValueKindList, report.ValueKindString, "Gallery should be a list but is a string."},
	}
	for _, f := range res.Findings {
		if f.Rule != report.RuleContentShape || f.Cause != report.CauseWrongType {
			continue
		}
		w, ok := want[f.Path]
		if !ok {
			t.Errorf("unexpected %s", f)
			continue
		}
		delete(want, f.Path)
		if f.Params.Expected != w.expected || f.Params.Actual != w.actual || f.Message != w.message {
			t.Errorf("%s: %q, %q, %q; want %q, %q, %q", f.Path, f.Params.Expected, f.Params.Actual, f.Message, w.expected, w.actual, w.message)
		}
		if err := report.CheckFinding(f); err != nil {
			t.Errorf("%s: %v", f.Path, err)
		}
	}
	for path := range want {
		t.Errorf("no shape finding for %s", path)
	}
}
