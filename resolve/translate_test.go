package resolve_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

func flag(b bool) *bool { return &b }

// Which fields are translated, one case per type and per rule.
func TestTranslatable(t *testing.T) {
	coll := schema.Collection{Translations: &schema.Translations{By: "field:translationKey"}}
	computed := &schema.Computed{From: "title"}
	cases := []struct {
		name  string
		field schema.Field
		want  bool
	}{
		// By type.
		{"text", schema.Field{Key: "title", Type: schema.TypeText}, true},
		{"paragraph", schema.Field{Key: "intro", Type: schema.TypeParagraph}, true},
		{"richtext", schema.Field{Key: "copy", Type: schema.TypeRichText}, true},
		{"markdown, a legacy name of richtext", schema.Field{Key: "copy", Type: schema.TypeMarkdown}, true},
		{"html, a legacy name of richtext", schema.Field{Key: "copy", Type: schema.TypeHTML}, true},
		{"number", schema.Field{Key: "order", Type: schema.TypeNumber}, false},
		{"boolean", schema.Field{Key: "draft", Type: schema.TypeBoolean}, false},
		{"date", schema.Field{Key: "published", Type: schema.TypeDate}, false},
		{"color", schema.Field{Key: "accent", Type: schema.TypeColor}, false},
		{"range", schema.Field{Key: "opacity", Type: schema.TypeRange}, false},
		{"multichoice", schema.Field{Key: "tags", Type: schema.TypeMultiChoice}, false},
		{"link", schema.Field{Key: "href", Type: schema.TypeLink}, false},
		{"url, a legacy name of link", schema.Field{Key: "href", Type: schema.TypeURL}, false},
		{"image", schema.Field{Key: "hero", Type: schema.TypeImage}, false},
		{"file", schema.Field{Key: "pdf", Type: schema.TypeFile}, false},
		{"reference", schema.Field{Key: "related", Type: schema.TypeReference}, false},
		{"select", schema.Field{Key: "kind", Type: schema.TypeSelect}, false},
		{"group", schema.Field{Key: "seo", Type: schema.TypeGroup}, false},
		{"array", schema.Field{Key: "items", Type: schema.TypeArray}, false},
		{"an unknown type", schema.Field{Key: "x", Type: "string"}, false},

		// Text that is not translated unless it says so.
		{"the slug", schema.Field{Key: "slug", Type: schema.TypeText}, false},
		{"the linking field", schema.Field{Key: "translationKey", Type: schema.TypeText}, false},
		{"text with a regex", schema.Field{Key: "icon", Type: schema.TypeText, Regex: "^[a-z-]+$"}, false},
		{"paragraph with a regex", schema.Field{Key: "note", Type: schema.TypeParagraph, Regex: "."}, true},
		{"computed text", schema.Field{Key: "anchor", Type: schema.TypeText, Computed: computed}, false},
		{"hidden text", schema.Field{Key: "internal", Type: schema.TypeText, Hidden: true}, false},

		// The field's own word wins, for text.
		{"text opting out", schema.Field{Key: "sku", Type: schema.TypeText, Translatable: flag(false)}, false},
		{"slug opting in", schema.Field{Key: "slug", Type: schema.TypeText, Translatable: flag(true)}, true},
		{"linking field opting in", schema.Field{Key: "translationKey", Type: schema.TypeText, Translatable: flag(true)}, true},
		{"regex text opting in", schema.Field{Key: "handle", Type: schema.TypeText, Regex: "^@", Translatable: flag(true)}, true},
		{"computed text opting in", schema.Field{Key: "anchor", Type: schema.TypeText, Computed: computed, Translatable: flag(true)}, true},
		{"hidden text opting in", schema.Field{Key: "internal", Type: schema.TypeText, Hidden: true, Translatable: flag(true)}, true},
		// Never for a value that is not text.
		{"select opting in", schema.Field{Key: "kind", Type: schema.TypeSelect, Translatable: flag(true)}, false},
		{"number opting in", schema.Field{Key: "order", Type: schema.TypeNumber, Translatable: flag(true)}, false},
		{"image opting in", schema.Field{Key: "hero", Type: schema.TypeImage, Translatable: flag(true)}, false},
		// A group or a list answers with what it says.
		{"group opting in", schema.Field{Key: "seo", Type: schema.TypeGroup, Translatable: flag(true)}, true},
		{"array opting out", schema.Field{Key: "items", Type: schema.TypeArray, Translatable: flag(false)}, false},
	}
	for _, c := range cases {
		if got := resolve.Translatable(coll, c.field); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// Without a linking field declared, a field of that name is ordinary text.
	if !resolve.Translatable(schema.Collection{}, schema.Field{Key: "translationKey", Type: schema.TypeText}) {
		t.Error("translationKey is text in a collection that links by nothing")
	}
}

// What a group or a list says covers everything inside it, down to the
// next field that says otherwise.
func TestTranslatableIn(t *testing.T) {
	coll := schema.Collection{Translations: &schema.Translations{By: "field:translationKey"}}
	group := func(tr *bool, hidden bool) schema.Field {
		return schema.Field{Key: "g", Type: schema.TypeGroup, Translatable: tr, Hidden: hidden}
	}
	list := func(tr *bool) schema.Field {
		return schema.Field{Key: "l", Type: schema.TypeArray, Translatable: tr}
	}
	text := schema.Field{Key: "label", Type: schema.TypeText}
	cases := []struct {
		name      string
		ancestors []schema.Field
		field     schema.Field
		want      bool
	}{
		{"text in a group", []schema.Field{group(nil, false)}, text, true},
		{"text in a group that opts out", []schema.Field{group(flag(false), false)}, text, false},
		{"text in a list that opts out", []schema.Field{list(flag(false))}, text, false},
		{"text deep in a group that opts out", []schema.Field{group(flag(false), false), list(nil), group(nil, false)}, text, false},
		{"text opting in inside a group that opts out", []schema.Field{group(flag(false), false)}, schema.Field{Key: "label", Type: schema.TypeText, Translatable: flag(true)}, true},
		{"a nested group opting back in", []schema.Field{group(flag(false), false), group(flag(true), false)}, text, true},
		{"a nested group opting out of a group that opts in", []schema.Field{group(flag(true), false), group(flag(false), false)}, text, false},
		{"text in a hidden group", []schema.Field{group(nil, true)}, text, false},
		{"text in a hidden group that opts in", []schema.Field{group(flag(true), true)}, text, true},
		{"text in a group inside a hidden group", []schema.Field{group(nil, true), group(nil, false)}, text, false},
		// A group opting in hands the decision back to the field's own rules.
		{"number in a group that opts in", []schema.Field{group(flag(true), false)}, schema.Field{Key: "n", Type: schema.TypeNumber}, false},
		{"regex text in a group that opts in", []schema.Field{group(flag(true), false)}, schema.Field{Key: "icon", Type: schema.TypeText, Regex: "^[a-z]+$"}, false},
		{"slug in a group", []schema.Field{group(nil, false)}, schema.Field{Key: "slug", Type: schema.TypeText}, false},
		// The linking field is a top-level field; the same key deeper is text.
		{"a nested field with the linking key", []schema.Field{group(nil, false)}, schema.Field{Key: "translationKey", Type: schema.TypeText}, true},
		{"an item of a list of texts", []schema.Field{list(nil)}, schema.Field{Type: schema.TypeText}, true},
		{"an item of a list of texts that opts out", []schema.Field{list(flag(false))}, schema.Field{Type: schema.TypeText}, false},
		{"an item of a list of images", []schema.Field{list(flag(true))}, schema.Field{Type: schema.TypeImage}, false},
	}
	for _, c := range cases {
		if got := resolve.TranslatableIn(coll, c.ancestors, c.field); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

const textsCollection = `label: Pages
path: content
hasBody: true
translations:
  by: field:translationKey
fields:
  - key: title
    type: text
    label: Title
  - key: slug
    type: text
    label: Slug
  - key: translationKey
    type: text
    label: Translation key
    required: true
  - key: icon
    type: text
    label: Icon
    regex: '^[a-z-]+$'
  - key: handle
    type: text
    label: Handle
    regex: '^@'
    translatable: true
  - key: order
    type: number
    label: Order
  - key: empty
    type: text
    label: Empty
  - key: seo
    type: group
    label: SEO
    fields:
      - key: description
        type: paragraph
        label: Meta description
      - key: image
        type: image
        label: Share image
      - key: keywords
        type: array
        label: Keywords
        items:
          - key: keyword
            type: text
            label: Keyword
  - key: tracking
    type: group
    label: Tracking
    translatable: false
    fields:
      - key: campaign
        type: text
        label: Campaign
      - key: note
        type: text
        label: Note
        translatable: true
      - key: labels
        type: array
        label: Labels
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
            type: richtext
            format: html
            label: Quote
          - key: author
            type: group
            label: Author
            fields:
              - key: name
                type: text
                label: Name
                translatable: false
              - key: role
                type: text
                label: Role
      - key: embed
        type: group
        label: Embed
        translatable: false
        fields:
          - key: caption
            type: text
            label: Caption
  - key: blocks
    type: blocks
    label: Blocks
    components: [hero]
`

const textsHero = `label: Hero
fields:
  - key: id
    type: text
    label: Anchor id
    regex: '^[a-z0-9-]+$'
  - key: heading
    type: text
    label: Heading
  - key: lead
    type: richtext
    label: Lead
  - key: link
    type: link
    label: Link
  - key: bullets
    type: array
    label: Bullets
    items:
      - type: text
`

const textsPage = `---
title: About us
slug: about
translationKey: about
icon: arrow-right
handle: '@norcube'
order: 3
empty: '   '
seo:
  description: Who we are.
  image: /img/share.png
  keywords: [team, '', history]
tracking:
  campaign: spring-2026
  note: Shown to editors
  labels: [internal]
sections:
  - _type: quote
    text: <p>Great.</p>
    author:
      name: Jan Novak
      role: Founder
  - _type: embed
    caption: Not translated
blocks:
  - _component: hero
    id: top
    heading: Welcome
    lead: Read **more** in [the docs](/docs/).
    link: /docs/
    bullets: [Fast, Safe]
---

# About

The body.
`

func textsSite(t *testing.T) (*schema.Config, schema.Collection, content.Document) {
	t.Helper()
	doc, findings, err := load.Load(fstest.MapFS{
		load.ConfigFile:                    {Data: []byte("pagesDir: content\n")},
		load.CollectionsDir + "/pages.yml": {Data: []byte(textsCollection)},
		load.BlocksDir + "/hero.yml":       {Data: []byte(textsHero)},
	})
	if err != nil || len(findings) != 0 {
		t.Fatalf("load: %v %v", err, findings)
	}
	page, err := content.Parse("content/about.md", []byte(textsPage))
	if err != nil {
		t.Fatal(err)
	}
	return doc.Config, doc.Config.Collections["pages"], page
}

// The texts of a page that are sent for translation: these, in this
// order, and nothing else (no key, id, address, number, image or link).
func TestTranslatableTexts(t *testing.T) {
	cfg, coll, page := textsSite(t)
	texts, err := resolve.TranslatableTexts(cfg, coll, page.Frontmatter, page.Body)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range texts {
		value := x.Value
		if x.Body {
			value = strings.TrimSpace(value)
		}
		got = append(got, fmt.Sprintf("%s | %s | %s | %s | %s", x.Path, x.Label, x.Type, x.Format, value))
	}
	want := []string{
		"title | Title | text | plain | About us",
		"handle | Handle | text | plain | @norcube",
		"seo.description | Meta description | paragraph | plain | Who we are.",
		"seo.keywords[0] | Keyword | text | plain | team",
		"seo.keywords[2] | Keyword | text | plain | history",
		"tracking.note | Note | text | plain | Shown to editors",
		"sections[0].text | Quote | richtext | html | <p>Great.</p>",
		"sections[0].author.role | Role | text | plain | Founder",
		"blocks[0].heading | Heading | text | plain | Welcome",
		"blocks[0].lead | Lead | richtext | markdown | Read **more** in [the docs](/docs/).",
		// An item shape without a label of its own carries the list's.
		"blocks[0].bullets[0] | Bullets | text | plain | Fast",
		"blocks[0].bullets[1] | Bullets | text | plain | Safe",
		"body | Body | richtext | markdown | # About\n\nThe body.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if last := texts[len(texts)-1]; !last.Body || last.Path != resolve.BodyPath {
		t.Errorf("the body is the last text: %+v", last)
	}

	again, _ := resolve.TranslatableTexts(cfg, coll, page.Frontmatter, page.Body)
	if !reflect.DeepEqual(texts, again) {
		t.Error("two runs over one page differ")
	}
}

func TestTranslatableTextsBody(t *testing.T) {
	cfg, coll, page := textsSite(t)
	count := func(coll schema.Collection, body string) int {
		texts, err := resolve.TranslatableTexts(cfg, coll, page.Frontmatter, body)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, x := range texts {
			if x.Body {
				n++
			}
		}
		return n
	}
	if count(coll, " \n\t") != 0 {
		t.Error("a blank body is not a text")
	}
	noBody := coll
	noBody.HasBody = false
	if count(noBody, "Text") != 0 {
		t.Error("a collection without a body has no body text")
	}
	// A value that is not a string is left out, whatever the field's type.
	texts, err := resolve.TranslatableTexts(cfg, coll, map[string]any{"title": float64(4), "seo": "not a group"}, "")
	if err != nil || len(texts) != 0 {
		t.Errorf("got %v, %v", texts, err)
	}
}

// A page past the limit is reported, never silently cut.
func TestTranslatableTextsLimit(t *testing.T) {
	coll := schema.Collection{Fields: []schema.Field{{Key: "lines", Type: schema.TypeArray, Items: []schema.Field{{Type: schema.TypeText}}}}}
	lines := func(n int) map[string]any {
		items := make([]any, n)
		for i := range items {
			items[i] = fmt.Sprintf("line %d", i)
		}
		return map[string]any{"lines": items}
	}
	texts, err := resolve.TranslatableTexts(nil, coll, lines(resolve.MaxTranslatableTexts), "")
	if err != nil || len(texts) != resolve.MaxTranslatableTexts {
		t.Fatalf("at the limit: %d texts, %v", len(texts), err)
	}
	texts, err = resolve.TranslatableTexts(nil, coll, lines(resolve.MaxTranslatableTexts+1), "")
	if !errors.Is(err, resolve.ErrTooManyTexts) || len(texts) != resolve.MaxTranslatableTexts {
		t.Fatalf("past the limit: %d texts, %v", len(texts), err)
	}
}

func TestHasWords(t *testing.T) {
	for value, want := range map[string]bool{
		"Kontakt":              true,
		"  ":                   false,
		"+420 735 070 780":     false,
		"2026-11-19":           false,
		"ahoj@jetlio.com":      false,
		"https://jetlio.com/x": false,
		"mailto:a@b.cz":        false,
		"Napište na a@b.cz":    true,
		"12 let zkušeností":    true,
	} {
		if got := resolve.HasWords(value); got != want {
			t.Errorf("HasWords(%q) = %v, want %v", value, got, want)
		}
	}
}
