package validate_test

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

func site(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for p, body := range files {
		out[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

const pagesSchema = `label: Pages
path: src/content/pages
previewPath: '/{slug}/'
localeFrom: none
fields:
  - key: title
    type: text
    label: Title
    required: true
  - key: slug
    type: text
    label: Slug
  - key: tier
    type: select
    label: Tier
    options: [free, pro]
  - key: seats
    type: number
    label: Seats
    required: true
    visibleIf: { field: tier, op: eq, value: pro }
  - key: secret
    type: text
    label: Secret
    required: true
    hidden: true
`

func base() map[string]string {
	return map[string]string{
		load.ConfigFile:                    "pagesDir: src/content/pages\npreview: {defaultLocale: en, locales: [en]}\n",
		load.CollectionsDir + "/pages.yml": pagesSchema,
		load.CollectionsDir + "/posts.yml": "label: Posts\npath: src/content/posts\nfields:\n  - key: title\n    type: text\n    label: Title\n    required: true\n",
		"src/content/pages/a.md":           "---\ntitle: A\nslug: a\ntier: free\n---\n",
		"src/content/pages/b.md":           "---\ntitle: ''\nslug: b\ntier: pro\n---\n",
		"src/content/posts/p.md":           "---\ntitle: ''\n---\n",
	}
}

func rules(fs []report.Finding) map[string]int {
	out := map[string]int{}
	for _, f := range fs {
		out[f.File+"#"+f.Path+" "+string(f.Rule)]++
	}
	return out
}

func TestVisibilityFilter(t *testing.T) {
	res, err := validate.Site(site(base()), validate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := rules(res.Findings)
	// a.md: tier free hides seats, secret is statically hidden → nothing.
	for k := range got {
		if len(k) > 0 && k[:len("src/content/pages/a.md")] == "src/content/pages/a.md" {
			t.Errorf("unexpected finding on a.md: %s", k)
		}
	}
	// b.md: empty title, and seats required because tier is pro.
	if got["src/content/pages/b.md#title content.required"] != 1 || got["src/content/pages/b.md#seats content.required"] != 1 {
		t.Errorf("b.md findings: %v", got)
	}
	if got["src/content/pages/b.md#secret content.required"] != 0 {
		t.Error("statically hidden fields must not report")
	}
}

func TestOptions(t *testing.T) {
	t.Run("collection filter", func(t *testing.T) {
		res, err := validate.Site(site(base()), validate.Options{Collection: "posts"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Files != 1 || len(res.Findings) != 1 || res.Findings[0].File != "src/content/posts/p.md" {
			t.Fatalf("got %d files, %v", res.Files, res.Findings)
		}
	})
	t.Run("only schema", func(t *testing.T) {
		res, err := validate.Site(site(base()), validate.Options{Only: []string{validate.ScopeSchema}})
		if err != nil {
			t.Fatal(err)
		}
		if res.Files != 0 || len(res.Findings) != 0 {
			t.Fatalf("schema-only run touched content: %d files, %v", res.Files, res.Findings)
		}
	})
	t.Run("strict", func(t *testing.T) {
		files := base()
		files["src/content/pages/a.md"] = "---\ntitle: A\nslug: a\nextra: 1\n---\n"
		res, err := validate.Site(site(files), validate.Options{Strict: true})
		if err != nil {
			t.Fatal(err)
		}
		if res.Warnings != 0 {
			t.Fatalf("strict run kept %d warnings", res.Warnings)
		}
		if rules(res.Findings)["src/content/pages/a.md# content.extra_keys"] != 1 {
			t.Fatalf("extra key not promoted: %v", res.Findings)
		}
	})
	t.Run("not a site", func(t *testing.T) {
		_, err := validate.Site(site(map[string]string{"README.md": "x"}), validate.Options{})
		if !errors.Is(err, load.ErrNotASite) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestNoCollection(t *testing.T) {
	files := map[string]string{
		load.ConfigFile:                    "pagesDir: content\n",
		load.CollectionsDir + "/posts.yml": "label: Posts\npath: content/posts\nfields: []\n",
		"content/stray.md":                 "---\ntitle: x\n---\n",
	}
	res, err := validate.Site(site(files), validate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rules(res.Findings)["content/stray.md# content.no_collection"] != 1 {
		t.Fatalf("got %v", res.Findings)
	}
}

func TestRuleListIsFrozen(t *testing.T) {
	seen := map[report.Rule]bool{}
	for _, r := range report.Rules() {
		if seen[r] {
			t.Errorf("duplicate rule %s", r)
		}
		seen[r] = true
	}
	if len(seen) != 84 {
		t.Errorf("rule count changed to %d; adding a rule is fine, renaming or removing one is not", len(seen))
	}
}

func TestDuplicatePathOnlyWhenAmbiguous(t *testing.T) {
	files := map[string]string{
		load.ConfigFile:                       "pagesDir: content\n",
		load.CollectionsDir + "/pages.yml":    "label: Pages\npath: content\nmatch: '**/*.md'\nfields: []\n",
		load.CollectionsDir + "/blog.yml":     "label: Blog index\npath: content\nmatch: 'blog.md'\nfields: []\n",
		load.CollectionsDir + "/settings.yml": "label: Settings\npath: content\nmatch: 'settings.md'\nfields: []\n",
	}
	res, err := validate.Site(site(files), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("a pattern collection with single-document collections beside it is fine, got %v", res.Findings)
	}
	files[load.CollectionsDir+"/posts.yml"] = "label: Posts\npath: content\nmatch: '*.md'\nfields: []\n"
	res, err = validate.Site(site(files), validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	if n := rules(res.Findings); n[load.CollectionsDir+"/pages.yml#path schema.duplicate_collection_path"] != 1 || n[load.CollectionsDir+"/posts.yml#path schema.duplicate_collection_path"] != 1 || len(res.Findings) != 2 {
		t.Fatalf("two pattern collections on one path must be reported once each, got %v", res.Findings)
	}
}
