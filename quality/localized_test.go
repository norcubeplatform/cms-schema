package quality_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
)

// A site shaped like a portfolio: works and events hold cs, en and it in
// one file (localized groups falling back to cs), pages are one language
// per file with a draft field and a robots select.
const localizedConfig = `pagesDir: src/content
preview:
  defaultLocale: cs
  locales: [cs, en, it]
  prefixDefaultLocale: false
storages:
  assets:
    type: repo
    path: src/assets
variables:
  file: src/content/settings/globals.md
  field: variables
`

const worksSchema = `label: Works
path: src/content/works
match: '*.md'
previewPath: /{locale}/portfolio/{slug}/
localeFrom: none
fields:
  - key: image
    type: image
    label: Photo
    alt: {from: [title, technique], separator: ' – '}
  - key: title
    type: group
    label: Title
    localized: true
    fallback: cs
    role: seoTitle
    titleTemplate: '{value} – {{site_name}}'
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
      - {key: it, type: text, label: Italian}
  - key: technique
    type: group
    label: Technique
    localized: true
    fallback: cs
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
      - {key: it, type: text, label: Italian}
  - key: description
    type: group
    label: Description
    localized: true
    role: seoDescription
    fields:
      - {key: cs, type: paragraph, label: Czech}
      - {key: en, type: paragraph, label: English}
      - {key: it, type: paragraph, label: Italian}
  - key: slug
    type: group
    label: Slug
    localized: true
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
      - {key: it, type: text, label: Italian}
`

const eventsSchema = `label: Events
path: src/content/events
match: '*.md'
previewPath: /{locale}/vystavy/{slug}/
localeFrom: none
translations:
  locales: [cs, en]
fields:
  - key: kind
    type: select
    label: Kind
    options:
      - {value: solo, label: Samostatná výstava, labels: {cs: Samostatná výstava, en: Solo exhibition}}
  - key: name
    type: group
    label: Name
    localized: true
    fallback: cs
    role: seoTitle
    titleTemplate: '{field:kind|label} – {value} – Adéla'
    fields:
      - {key: cs, type: text, label: Czech}
      - {key: en, type: text, label: English}
      - {key: it, type: text, label: Italian}
  - key: description
    type: group
    label: Description
    localized: true
    fallback: cs
    role: seoDescription
    excerpt: firstParagraph
    fields:
      - {key: cs, type: richtext, label: Czech}
      - {key: en, type: richtext, label: English}
      - {key: it, type: richtext, label: Italian}
`

const localizedPagesSchema = `label: Pages
path: src/content/pages
match: '**/*.md'
previewPath: /{locale}/{slug}/
localeFrom: path
translations:
  by: field:key
fields:
  - {key: key, type: text, label: Key, required: true}
  - {key: draft, type: boolean, label: Draft, role: draft}
  - key: seo
    type: group
    label: SEO
    fields:
      - {key: title, type: text, label: Title, role: seoTitle}
      - {key: description, type: paragraph, label: Description, role: seoDescription}
      - key: robots
        type: select
        label: Robots
        role: noindex
        noindexWhen: ['noindex,follow']
        options: ['index,follow', 'noindex,follow']
`

const globalsSchema = `label: Globals
path: src/content/settings
match: globals.md
area: site
fields:
  - key: variables
    type: array
    label: Variables
    items:
      - type: group
        key: v
        label: V
        fields:
          - {key: key, type: text, label: Key}
          - {key: value, type: text, label: Value}
`

func localizedSite(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		configFile:                             {Data: []byte(localizedConfig)},
		".norcube/cms/collections/works.yml":   {Data: []byte(worksSchema)},
		".norcube/cms/collections/events.yml":  {Data: []byte(eventsSchema)},
		".norcube/cms/collections/pages.yml":   {Data: []byte(localizedPagesSchema)},
		".norcube/cms/collections/globals.yml": {Data: []byte(globalsSchema)},
		"src/content/settings/globals.md":      {Data: []byte(page("variables: [{key: site_name, value: Adéla Maršálová}]"))},
	}
	for p, body := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

const goodDescription = "A painting of a forest clearing near Ochoz, in black and white acrylic on canvas."

// render shows the quality findings of some rules as
// "file#path [locale] rule impact".
func render(fs []report.Finding, rules ...report.Rule) []string {
	var out []string
	for _, f := range fs {
		if !f.Rule.IsQuality() || (len(rules) > 0 && !slices.Contains(rules, f.Rule)) {
			continue
		}
		out = append(out, f.File+"#"+f.Path+" ["+f.Locale+"] "+string(f.Rule)+" "+string(f.Impact))
	}
	slices.Sort(out)
	return out
}

// Each language of a localized page is a page of its own for the search
// rules, measured in its language; a language left empty that falls back
// shows the fallback's text and is not measured again.
func TestLocalizedSearchPerLanguage(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/works/a.md": page(`image: /src/assets/a.jpg
title: {cs: Lesní mýtina, en: Forest clearing}
technique: {cs: akryl, en: acrylic, it: acrilico}
description: {cs: '` + goodDescription + `', en: short}`),
	}), quality.Options{})
	got := render(res.Findings, report.RuleQualityTitleMissing, report.RuleQualityTitleLength, report.RuleQualityDescriptionMissing, report.RuleQualityDescriptionLength)
	want := []string{
		// No fallback on the description: it is empty in Italian.
		"src/content/works/a.md#description.en [en] quality.description_length low",
		"src/content/works/a.md#description.it [it] quality.description_missing medium",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%v\nwant\n%v", got, want)
	}
	if n := res.Quality.Checked[report.RuleQualityTitleMissing]; n != 3 {
		t.Errorf("titles checked %d, want one per language", n)
	}
}

// The title is measured as the template composes it, with site variables
// and a select's label in the page's language.
func TestTitleTemplate(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/works/a.md": page(`title: {cs: Les, en: A forest clearing near the village of Ochoz u Brna in spring}
technique: {cs: akryl}
description: {cs: '` + goodDescription + `', en: '` + goodDescription + `', it: '` + goodDescription + `'}`),
		"src/content/events/e.md": page(`kind: solo
name: {cs: BLURRED, en: BLURRED}
description: {cs: 'První odstavec, který je dost dlouhý na popis výstavy v kavárně Votruba kafe v Ochozi.', en: 'The *first* paragraph of [the text](/x/), long enough to describe the show in a café.'}`),
	}), quality.Options{})
	var cs, en, ev []report.Finding
	for _, f := range res.Findings {
		switch {
		case f.Rule == report.RuleQualityTitleLength && f.File == "src/content/works/a.md" && f.Locale == "cs":
			cs = append(cs, f)
		case f.Rule == report.RuleQualityTitleLength && f.File == "src/content/works/a.md" && f.Locale == "en":
			en = append(en, f)
		case f.File == "src/content/events/e.md" && f.Rule.IsQuality():
			ev = append(ev, f)
		}
	}
	if len(cs) != 1 || cs[0].Value != "Les – Adéla Maršálová" || cs[0].Basis != report.BasisTitleTemplate || cs[0].Measure.Value != 21 || cs[0].Path != "title.cs" {
		t.Errorf("cs title: %+v", cs)
	}
	if len(en) != 1 || !strings.HasSuffix(en[0].Value, " – Adéla Maršálová") || !strings.Contains(en[0].Message, "as the site composes it") {
		t.Errorf("en title: %+v", en)
	}
	// "Samostatná výstava – BLURRED – Adéla" is 36 characters, "Solo
	// exhibition – BLURRED – Adéla" 33: both fit. Only the description
	// findings are left, and Italian is not expected for events.
	for _, f := range ev {
		if f.Rule == report.RuleQualityTitleLength || f.Locale == "it" {
			t.Errorf("event: %+v", f)
		}
	}
	// Italian borrows the Czech title: measured in cs only.
	if n := res.Quality.Checked[report.RuleQualityTitleLength]; n != 2+2 {
		t.Errorf("title lengths checked %d", n)
	}
}

// A rich text description is measured as the plain text of its first
// paragraph.
func TestExcerpt(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/events/e.md": page(`kind: solo
name: {cs: Výstava v kavárně, en: A show in a café}
description: {cs: "# Nadpis\n\nKrátký *text*.\n\nDruhý odstavec, který se nepočítá, i když je dlouhý a má mnoho slov v sobě.", en: "Short [one](/x/).\n\nA second paragraph that is long enough to pass the window if it were counted at all."}`),
	}), quality.Options{})
	var got []report.Finding
	for _, f := range res.Findings {
		if f.Rule == report.RuleQualityDescriptionLength {
			got = append(got, f)
		}
	}
	slices.SortFunc(got, func(a, b report.Finding) int { return strings.Compare(a.Locale, b.Locale) })
	if len(got) != 2 || got[0].Value != "Krátký text." || got[1].Value != "Short one." || got[0].Basis != report.BasisExcerpt || got[0].Path != "description.cs" {
		t.Fatalf("got %+v", got)
	}
}

// A composed alt text joins its parts in the page's language; empty when
// every part is.
func TestComposedAlt(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/works/a.md": page("image: /src/assets/a.jpg\ntitle: {cs: Les, en: Forest}\ntechnique: {cs: akryl}\ndescription: {cs: '" + goodDescription + "'}"),
		"src/content/works/b.md": page("image: /src/assets/b.jpg\ndescription: {cs: '" + goodDescription + "'}"),
		"src/content/works/c.md": page("image: /src/assets/c.jpg\ntitle: {cs: obrázek}\ndescription: {cs: '" + goodDescription + "'}"),
	}), quality.Options{})
	got := render(res.Findings, report.RuleQualityImageAltMissing, report.RuleQualityImageAltPoor)
	want := []string{
		"src/content/works/b.md#title.cs [cs] quality.image_alt_missing high",
		"src/content/works/b.md#title.en [en] quality.image_alt_missing high",
		"src/content/works/b.md#title.it [it] quality.image_alt_missing high",
		// The en and it views borrow cs: judged once, in cs.
		"src/content/works/c.md#title.cs [cs] quality.image_alt_poor low",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%v\nwant\n%v", got, want)
	}
	if n := res.Quality.Checked[report.RuleQualityImageAltMissing]; n != 9 {
		t.Errorf("images checked %d, want three per work", n)
	}
}

// A language left empty, or the same as the language it falls back to,
// is untranslated; an empty field in every language is not.
func TestTranslationIncomplete(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/works/a.md": page(`title: {cs: Lesní mýtina, en: Lesní mýtina, it: Radura}
technique: {cs: akryl, en: acrylic}
description: {cs: '` + goodDescription + `', en: '` + goodDescription + `', it: Un dipinto.}
slug: {cs: les}`),
	}), quality.Options{})
	var got []report.Finding
	for _, f := range res.Findings {
		if f.Rule == report.RuleQualityTranslationIncomplete {
			got = append(got, f)
		}
	}
	slices.SortFunc(got, func(a, b report.Finding) int { return strings.Compare(a.Locale, b.Locale) })
	// en: the title is cs's copy; the description has no fallback, so the
	// same text is not judged a copy. it: technique is empty.
	if len(got) != 2 || got[0].Locale != "en" || got[0].Path != "title.en" || !strings.Contains(got[0].Message, "Title (same as cs)") ||
		got[1].Locale != "it" || got[1].Path != "technique.it" || !strings.Contains(got[1].Message, "Technique (empty)") ||
		got[0].Impact != report.ImpactLow || got[0].Measure.Value != 1 || got[0].Measure.Unit != report.UnitFields {
		t.Fatalf("got %+v", got)
	}
	if n := res.Quality.Checked[report.RuleQualityTranslationIncomplete]; n != 3 {
		t.Errorf("checked %d, want one per expected language", n)
	}
}

// A draft is skipped by the page rules and is not a language version: a
// language whose only version is a draft is listed as not published yet.
// A robots select hides a page by the values noindexWhen lists.
func TestDraftsAndNoindexSelect(t *testing.T) {
	res := run(t, localizedSite(map[string]string{
		"src/content/pages/cs/a.md": page("key: a\nseo: {title: Short}"),
		"src/content/pages/en/a.md": page("key: a\ndraft: true\nseo: {title: Short}"),
		"src/content/pages/cs/b.md": page("key: b\nseo: {title: Short, robots: 'noindex,follow'}"),
		"src/content/pages/en/b.md": page("key: b\nseo: {title: Short, robots: 'index,follow'}"),
		"src/content/pages/cs/c.md": page("key: c\ndraft: true\nseo: {title: Short}"),
	}), quality.Options{})
	got := render(res.Findings, report.RuleQualityTitleLength, report.RuleQualityNoindex, report.RuleQualityTranslationMissing)
	want := []string{
		"src/content/pages/cs/a.md# [] quality.translation_missing medium",
		"src/content/pages/cs/a.md#seo.title [] quality.title_length low",
		"src/content/pages/cs/b.md# [] quality.translation_missing medium",
		"src/content/pages/cs/b.md#seo.robots [] quality.noindex info",
		"src/content/pages/cs/c.md# [] quality.translation_missing info",
		"src/content/pages/en/a.md# [] quality.translation_missing info",
		"src/content/pages/en/b.md#seo.title [] quality.title_length low",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%v\nwant\n%v", got, want)
	}
	if res.Quality.Drafts != 2 {
		t.Errorf("drafts %d", res.Quality.Drafts)
	}
}

// A localized page is one file: translation_outdated has no versions to
// compare there, and its languages are never missing as files.
func TestLocalizedHasNoFileVersions(t *testing.T) {
	asked := 0
	res := run(t, localizedSite(map[string]string{
		"src/content/works/a.md": page("title: {cs: Les, en: Forest, it: Foresta}\ndescription: {cs: '" + goodDescription + "'}"),
	}), quality.Options{SourceBases: func(q []quality.BaseQuery) ([]quality.SourceBase, error) {
		asked += len(q)
		return make([]quality.SourceBase, len(q)), nil
	}})
	if asked != 0 {
		t.Errorf("asked about %d versions", asked)
	}
	for _, f := range res.Findings {
		if f.Rule == report.RuleQualityTranslationMissing || f.Rule == report.RuleQualityTranslationOutdated {
			t.Errorf("unexpected %v", f)
		}
	}
}
