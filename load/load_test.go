package load

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/report"
)

func site(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for p, body := range files {
		out[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func TestLoadDirectoryLayout(t *testing.T) {
	fsys := site(map[string]string{
		ConfigFile:                    "siteUrl: https://x.com\ninputsFromGlob: shared/*.yml\ncollections:\n  inline:\n    label: Inline\n    fields: []\n",
		CollectionsDir + "/pages.yml": "label: Pages\nfields:\n  - $ref: kicker\n  - key: blocks\n    type: array\n    label: Blocks\n    items:\n      - $ref: hero\n",
		BlocksDir + "/hero.yml":       "label: Hero\nfields:\n  - key: title\n    type: text\n    label: Title\n  - $ref: button\n",
		BlocksDir + "/button.yml":     "label: Button\nfields:\n  - key: label\n    type: text\n    label: Label\n",
		InputsDir + "/kicker.yml":     "key: kicker\ntype: text\nlabel: Kicker\n",
		"shared/button.yml":           "key: button\ntype: group\nlabel: Shared button\nfields: []\n",
	})
	doc, findings, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %v", findings)
	}
	cfg := doc.Config
	if cfg.SiteURL != "https://x.com" {
		t.Fatal("root keys")
	}
	if _, ok := cfg.Collections["inline"]; !ok {
		t.Fatal("inline collections survive the merge")
	}
	pages := cfg.Collections["pages"]
	if len(pages.Fields) != 2 || pages.Fields[0].Key != "kicker" || pages.Fields[0].Type != "text" || pages.Fields[0].BlockRef != "kicker" {
		t.Fatalf("input ref inlined into fields: %+v", pages.Fields[0])
	}
	hero := pages.Fields[1].Items[0]
	if hero.Key != "hero" || hero.Type != "group" || hero.BlockRef != "hero" || hero.Label != "Hero" {
		t.Fatalf("block ref inlined as a group item: %+v", hero)
	}
	// The nested $ref inside hero resolves too, and the glob input
	// named `button` overrides the block of the same name.
	if len(hero.Fields) != 2 || hero.Fields[1].Label != "Shared button" || hero.Fields[1].Key != "button" {
		t.Fatalf("nested ref / inputs override: %+v", hero.Fields)
	}
	if len(doc.Files) != 6 {
		t.Fatalf("files: %v", doc.Files)
	}
}

func TestLoadFindings(t *testing.T) {
	fsys := site(map[string]string{
		ConfigFile:                    "inputsFromGlob: ['**/*.yml']\n",
		CollectionsDir + "/pages.yml": "label: Pages\nschemaVersion: 2\nfields:\n  - $ref: ghost\n",
		CollectionsDir + "/bad.yml":   "label: [\n",
		CollectionsDir + "/list.yml":  "- a\n",
		BlocksDir + "/a.yml":          "label: A\nfields:\n  - $ref: b\n",
		BlocksDir + "/b.yml":          "label: B\nfields:\n  - $ref: a\n",
	})
	_, findings, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[report.Rule]int{}
	for _, f := range findings {
		rules[f.Rule]++
	}
	for _, want := range []report.Rule{report.RuleSchemaParseError, report.RuleSchemaNotMapping, report.RuleSchemaUnresolvedRef, report.RuleSchemaRefCycle, report.RuleSchemaUnsupportedVersion, report.RuleSchemaInputsGlobUnsupported} {
		if rules[want] == 0 {
			t.Errorf("missing %s in %v", want, findings)
		}
	}
	for _, f := range findings {
		if f.File == "" {
			t.Errorf("finding without a file: %v", f)
		}
	}
}

func TestLoadLegacyAndMissing(t *testing.T) {
	doc, findings, err := Load(site(map[string]string{LegacyConfigFile: "pagesDir: content\ncollections:\n  pages:\n    label: P\n    fields: []\n"}))
	if err != nil || len(findings) != 0 || !doc.Legacy || doc.Config.PagesDir != "content" {
		t.Fatalf("legacy: %+v %v %v", doc, findings, err)
	}
	if _, _, err := Load(site(map[string]string{"README.md": "x"})); !errors.Is(err, ErrNotASite) {
		t.Fatalf("want ErrNotASite, got %v", err)
	}
}
