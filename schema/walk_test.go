package schema_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/schema"
)

const walkCollection = `label: Pages
path: content
fields:
  - key: title
    type: text
    label: Title
  - key: seo
    type: group
    label: SEO
    fields:
      - key: description
        type: paragraph
        label: Description
      - key: keywords
        type: array
        label: Keywords
        items:
          - type: text
  - key: sections
    type: array
    label: Sections
    items:
      - key: quote
        type: group
        label: Quote
        fields:
          - key: text
            type: text
            label: Text
      - key: gallery
        type: group
        label: Gallery
        fields:
          - key: images
            type: array
            label: Images
            items:
              - type: image
  - key: blocks
    type: blocks
    label: Blocks
    components: [hero]
  - key: order
    type: number
    label: Order
`

const walkHero = `label: Hero
fields:
  - key: heading
    type: text
    label: Heading
`

const walkPage = `title: Home
seo:
  description: About us
  keywords: [one, two]
sections:
  - _type: quote
    text: Hello
  - _type: gallery
    images: [/a.jpg]
  - _type: unknown
    text: Skipped
  - just a string
blocks:
  - _component: hero
    heading: Welcome
  - _component: missing
    heading: Skipped
`

func walkSite(t *testing.T) (*schema.Config, schema.Collection, map[string]any) {
	t.Helper()
	doc, findings, err := load.Load(fstest.MapFS{
		load.ConfigFile:                    {Data: []byte("pagesDir: content\n")},
		load.CollectionsDir + "/pages.yml": {Data: []byte(walkCollection)},
		load.BlocksDir + "/hero.yml":       {Data: []byte(walkHero)},
	})
	if err != nil || len(findings) != 0 {
		t.Fatalf("load: %v %v", err, findings)
	}
	fm, err := content.ParseYAMLMap([]byte(walkPage))
	if err != nil {
		t.Fatal(err)
	}
	return doc.Config, doc.Config.Collections["pages"], fm
}

// The walk is the one reading of a document against its schema: these
// are the positions it visits, in this order, with these paths.
func TestWalkOrderAndPaths(t *testing.T) {
	cfg, coll, fm := walkSite(t)
	var got []string
	schema.Walk(cfg, coll.Fields, fm, func(v schema.Visit) bool {
		var around []string
		for _, a := range v.Ancestors {
			around = append(around, a.Key)
		}
		line := fmt.Sprintf("%s depth=%d in=%s", v.Path, v.Depth, strings.Join(around, ">"))
		if v.Item {
			line += " item"
		}
		if !v.Has {
			line += " absent"
		}
		got = append(got, line)
		return true
	})
	want := []string{
		"title depth=0 in=",
		"seo depth=0 in=",
		"seo.description depth=1 in=seo",
		"seo.keywords depth=1 in=seo",
		"seo.keywords[0] depth=2 in=seo>keywords item",
		"seo.keywords[1] depth=2 in=seo>keywords item",
		"sections depth=0 in=",
		"sections[0].text depth=1 in=sections>quote",
		"sections[1].images depth=1 in=sections>gallery",
		"sections[1].images[0] depth=2 in=sections>gallery>images item",
		// An item naming no shape falls back to the first group shape; a
		// string in a list of groups has no shape and is not visited.
		"sections[2].text depth=1 in=sections>quote",
		"blocks depth=0 in=",
		"blocks[0].heading depth=1 in=blocks",
		"order depth=0 in= absent",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestWalkVisitCarriesFieldValueAndParent(t *testing.T) {
	cfg, coll, fm := walkSite(t)
	seen := map[string]schema.Visit{}
	schema.Walk(cfg, coll.Fields, fm, func(v schema.Visit) bool {
		seen[v.Path] = v
		return true
	})
	if v := seen["title"]; v.Value != "Home" || v.Parent() != nil || v.Field.Label != "Title" {
		t.Errorf("title: %+v", v)
	}
	if v := seen["seo.keywords[1]"]; v.Value != "two" || v.Field.Type != schema.TypeText || v.Parent() == nil || v.Parent().Key != "keywords" {
		t.Errorf("keywords[1]: %+v", v)
	}
	if v := seen["sections[0].text"]; v.Scope.Item == nil || v.Scope.Item["text"] != "Hello" || v.Scope.Root["title"] != "Home" {
		t.Errorf("sections[0].text scope: %+v", v.Scope)
	}
}

func TestWalkSkipsWhatAVisitDeclines(t *testing.T) {
	cfg, coll, fm := walkSite(t)
	var got []string
	schema.Walk(cfg, coll.Fields, fm, func(v schema.Visit) bool {
		got = append(got, v.Path)
		return v.Path != "seo" && v.Path != "sections"
	})
	want := []string{"title", "seo", "sections", "blocks", "blocks[0].heading", "order"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A document nested deeper than the bound is walked down to it and no
// further.
func TestWalkIsBoundedInDepth(t *testing.T) {
	const levels = schema.MaxWalkDepth + 10
	field := schema.Field{Key: "leaf", Type: schema.TypeText}
	var value any = "x"
	key := "leaf"
	for i := 0; i < levels; i++ {
		value = map[string]any{key: value}
		field = schema.Field{Key: "g", Type: schema.TypeGroup, Fields: []schema.Field{field}}
		key = "g"
	}
	doc := map[string]any{"g": value}
	deepest, visits := 0, 0
	schema.Walk(nil, []schema.Field{field}, doc, func(v schema.Visit) bool {
		visits++
		if v.Depth > deepest {
			deepest = v.Depth
		}
		return true
	})
	if deepest != schema.MaxWalkDepth || visits != schema.MaxWalkDepth+1 {
		t.Errorf("deepest visit at %d after %d visits, want %d and %d", deepest, visits, schema.MaxWalkDepth, schema.MaxWalkDepth+1)
	}
}

func TestPaths(t *testing.T) {
	if got := schema.IndexPath(schema.JoinPath(schema.JoinPath("", "a"), "b"), 2); schema.JoinPath(got, "c") != "a.b[2].c" {
		t.Errorf("got %q", schema.JoinPath(got, "c"))
	}
}
