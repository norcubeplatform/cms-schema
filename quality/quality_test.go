package quality_test

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

const (
	configFile = ".norcube/cms/config.yml"
	pagesFile  = ".norcube/cms/collections/pages.yml"
	casesFile  = ".norcube/cms/collections/cases.yml"
	navFile    = ".norcube/cms/collections/nav.yml"
)

const baseConfig = `pagesDir: src/content/pages
preview:
  defaultLocale: cs
  locales: [cs, en]
  prefixDefaultLocale: true
storages:
  images:
    type: repo
    path: public/images
    publicUrl: /images
  shared:
    type: repo
    path: public/shared
    publicUrl: /shared
    mode: read-only
defaultStorage: images
`

const pagesSchema = `label: Pages
path: src/content/pages
match: '**/*.md'
hasBody: true
previewPath: '/{locale}/{slug}/'
localeFrom: path
translations:
  by: path
fields:
  - {key: title, type: text, label: Title}
  - key: seo
    type: group
    label: SEO
    fields:
      - {key: page_title, type: text, label: SEO title, role: seoTitle}
      - {key: page_description, type: paragraph, label: Description, role: seoDescription, recommendedLength: {from: 10, to: 40}}
      - {key: image, type: image, label: Share image, role: shareImage, alt: image_alt}
      - {key: image_alt, type: text, label: Share image alt}
      - {key: no_index, type: boolean, label: Hide, role: noindex}
      - {key: canonical, type: link, label: Canonical, role: canonical}
  - key: blocks
    type: array
    label: Blocks
    items:
      - key: logos
        type: group
        label: Logos
        fields:
          - key: list
            type: array
            label: Logos
            items:
              - type: group
                fields:
                  - {key: img, type: image, label: Logo}
                  - {key: name, type: text, label: Name}
      - key: hero
        type: group
        label: Hero
        fields:
          - {key: photo, type: image, label: Photo, alt: photo_alt}
          - {key: photo_alt, type: text, label: Alt}
          - {key: background, type: image, label: Background, decorative: true}
          - {key: loose, type: image, label: Loose}
          - {key: gallery, type: array, label: Gallery, items: [{type: image}]}
          - {key: text, type: richtext, label: Text}
          - {key: html, type: richtext, label: HTML, format: html}
          - {key: label, type: text, label: Button label}
          - {key: link, type: link, label: Button link}
`

// site builds a site from the base schema and the given content files,
// with extra files (config overrides) on top.
func site(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		configFile: {Data: []byte(baseConfig)},
		pagesFile:  {Data: []byte(pagesSchema)},
	}
	for p, body := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func run(t *testing.T, fsys fstest.MapFS, opts quality.Options) *validate.Result {
	t.Helper()
	res, err := validate.Site(fsys, validate.Options{Only: []string{validate.ScopeQuality}, Quality: opts})
	if err != nil {
		t.Fatal(err)
	}
	if res.Quality == nil {
		t.Fatal("no quality summary")
	}
	for _, f := range res.Findings {
		if f.Severity == report.SeverityError {
			t.Errorf("a quality run gave an error: %v", f)
		}
	}
	return res
}

// keys renders the quality findings as "file#path rule impact".
func keys(res *validate.Result, rules ...report.Rule) []string {
	var out []string
	for _, f := range res.Findings {
		if !f.Rule.IsQuality() || (len(rules) > 0 && !slices.Contains(rules, f.Rule)) {
			continue
		}
		out = append(out, fmt.Sprintf("%s#%s %s %s", f.File, f.Path, f.Rule, f.Impact))
	}
	sort.Strings(out)
	return out
}

func page(fm string) string { return "---\n" + fm + "\n---\n" }

func TestSearchRules(t *testing.T) {
	long := strings.Repeat("x", 70)
	cases := []struct {
		name string
		fm   string
		want []string
	}{
		{"all fine", "title: T\nseo: {page_title: A title of a fitting length here, page_description: Twenty chars long ok, image: /images/a.jpg, image_alt: A}", nil},
		{"seo title falls back to the page title", "title: A page title that is long enough ok\nseo: {page_description: Twenty chars long ok, image: /images/a.jpg}", nil},
		{"no title at all", "seo: {page_description: Twenty chars long ok, image: /images/a.jpg}",
			[]string{"src/content/pages/cs/a.md#seo.page_title quality.title_missing high"}},
		{"short and long", "seo: {page_title: Short, page_description: '" + long + "', image: /images/a.jpg}",
			[]string{"src/content/pages/cs/a.md#seo.page_description quality.description_length low", "src/content/pages/cs/a.md#seo.page_title quality.title_length low"}},
		{"placeholders are not measured", "seo: {page_title: 'Hi {{company}}', page_description: Twenty chars long ok, image: /images/a.jpg}", nil},
		{"description and share image missing", "title: A page title that is long enough ok",
			[]string{"src/content/pages/cs/a.md#seo.image quality.share_image_missing low", "src/content/pages/cs/a.md#seo.page_description quality.description_missing medium"}},
		{"noindex is listed and nothing else", "seo: {no_index: true}",
			[]string{"src/content/pages/cs/a.md#seo.no_index quality.noindex info"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, site(map[string]string{"src/content/pages/cs/a.md": page(c.fm), "src/content/pages/en/a.md": page("seo: {no_index: true}")}), quality.Options{})
			got := keys(res, report.RuleQualityTitleMissing, report.RuleQualityTitleLength, report.RuleQualityDescriptionMissing,
				report.RuleQualityDescriptionLength, report.RuleQualityShareImageMissing, report.RuleQualityNoindex)
			got = slices.DeleteFunc(got, func(s string) bool { return strings.HasPrefix(s, "src/content/pages/en/") })
			if !slices.Equal(got, c.want) {
				t.Errorf("got\n%v\nwant\n%v", got, c.want)
			}
		})
	}
}

// The measured value and the window travel with a length finding, and the
// field's recommendedLength wins over the default.
func TestLengthMeasure(t *testing.T) {
	res := run(t, site(map[string]string{"src/content/pages/cs/a.md": page("seo: {page_title: A title of a fitting length here, page_description: '" + strings.Repeat("y", 50) + "', image: /images/a.jpg}")}), quality.Options{})
	for _, f := range res.Findings {
		if f.Rule != report.RuleQualityDescriptionLength {
			continue
		}
		if f.Measure == nil || f.Measure.Value != 50 || *f.Measure.Min != 10 || *f.Measure.Max != 40 || f.Measure.Unit != report.UnitCharacters {
			t.Errorf("measure %+v", f.Measure)
		}
		if f.Value != strings.Repeat("y", 50) || f.Severity != report.SeverityNotice {
			t.Errorf("finding %+v", f)
		}
		return
	}
	t.Fatalf("no description_length finding in %v", keys(res))
}

func TestDuplicates(t *testing.T) {
	res := run(t, site(map[string]string{
		"src/content/pages/cs/a.md": page("seo: {page_title: The same title for both pages, page_description: Twenty chars long ok}"),
		"src/content/pages/cs/b.md": page("seo: {page_title: '  the SAME title for both   pages ', page_description: Twenty chars long ok}"),
		// Another language may share it.
		"src/content/pages/en/a.md": page("seo: {page_title: The same title for both pages, page_description: Twenty chars long ok}"),
		// A page that points its canonical elsewhere is a deliberate copy.
		"src/content/pages/cs/c.md": page("seo: {page_title: The same title for both pages, canonical: /cs/a/}"),
	}), quality.Options{})
	want := []string{
		"src/content/pages/cs/a.md#seo.page_description quality.description_duplicate medium",
		"src/content/pages/cs/a.md#seo.page_title quality.title_duplicate medium",
		"src/content/pages/cs/b.md#seo.page_description quality.description_duplicate medium",
		"src/content/pages/cs/b.md#seo.page_title quality.title_duplicate medium",
	}
	if got := keys(res, report.RuleQualityTitleDuplicate, report.RuleQualityDescriptionDuplicate); !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
	for _, f := range res.Findings {
		if f.Rule == report.RuleQualityTitleDuplicate && (f.Measure == nil || f.Measure.Value != 2 || f.Measure.Unit != report.UnitPages) {
			t.Errorf("measure %+v", f.Measure)
		}
	}
}

// A role guessed from a field's key weighs low, whatever the rule.
func TestGuessedRoleWeighsLow(t *testing.T) {
	fsys := site(map[string]string{"src/content/pages/cs/a.md": page("title: A page title that is long enough ok")})
	fsys[pagesFile] = &fstest.MapFile{Data: []byte(`label: Pages
path: src/content/pages
previewPath: '/{locale}/{slug}/'
localeFrom: path
fields:
  - {key: title, type: text, label: Title}
  - {key: meta_description, type: paragraph, label: Description}
`)}
	res := run(t, fsys, quality.Options{})
	want := []string{"src/content/pages/cs/a.md#meta_description quality.description_missing low"}
	if got := keys(res); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAccessibilityRules(t *testing.T) {
	cases := []struct {
		name string
		hero string
		want []string
	}{
		{"declared alt filled", "{_type: hero, photo: /images/p.jpg, photo_alt: Our office}", nil},
		{"declared alt empty", "{_type: hero, photo: /images/p.jpg}",
			[]string{"blocks[0].photo_alt quality.image_alt_missing high"}},
		{"decorative needs none", "{_type: hero, background: /images/b.jpg}", nil},
		{"no field for alt text", "{_type: hero, loose: /images/l.jpg}",
			[]string{"blocks[0].loose quality.image_alt_missing low"}},
		{"an image in a list of images", "{_type: hero, gallery: [/images/g.jpg]}",
			[]string{"blocks[0].gallery[0] quality.image_alt_missing low"}},
		{"alt is a generic word", "{_type: hero, photo: /images/p.jpg, photo_alt: Obrázek}",
			[]string{"blocks[0].photo_alt quality.image_alt_poor low"}},
		{"alt is the file name", "{_type: hero, photo: /images/team-photo-2.jpg, photo_alt: team-photo-2}",
			[]string{"blocks[0].photo_alt quality.image_alt_poor low"}},
		{"heading skip in markdown", "{_type: hero, text: \"## One\\n\\n#### Two\\n\\n## Three\"}",
			[]string{"blocks[0].text quality.heading_skip low"}},
		{"headings in code are not headings", "{_type: hero, text: \"## One\\n\\n```\\n#### code\\n```\\n### Two\"}", nil},
		{"heading skip in HTML", "{_type: hero, html: \"<h2>A</h2><p>x</p><h5>B</h5>\"}",
			[]string{"blocks[0].html quality.heading_skip low"}},
		{"generic and empty links", "{_type: hero, text: \"Read [zde](/cs/a/), [More](/cs/b/) or [](/cs/c/). [Our pricing](/cs/p/)\"}",
			[]string{"blocks[0].text quality.link_text_empty medium", "blocks[0].text quality.link_text_generic low", "blocks[0].text quality.link_text_generic low"}},
		{"an image link speaks its alt text", "{_type: hero, text: \"[![Pricing](/images/p.png)](/cs/p/)\"}", nil},
		{"HTML link with an image or a label", "{_type: hero, html: \"<a href='/a'><img src='/images/x.png' alt='Home'></a> <a href='/b' aria-label='Search'><svg/></a> <a href='/c'> </a>\"}",
			[]string{"blocks[0].html quality.link_text_empty medium"}},
		{"images in text", "{_type: hero, text: \"![](/images/a.jpg) ![A chart](/images/b.jpg)\", html: \"<img src='/images/c.jpg' alt=''><img src='/images/d.jpg'>\"}",
			[]string{"blocks[0].html quality.image_alt_missing high", "blocks[0].text quality.image_alt_missing high"}},
		{"a button's generic label", "{_type: hero, label: Klikněte zde, link: /cs/a/}",
			[]string{"blocks[0].label quality.link_text_generic low"}},
		{"a button's own label", "{_type: hero, label: See the pricing, link: /cs/a/}", nil},
		{"a logo named after its company", "{_type: logos, list: [{img: /images/acme.svg, name: Acme}, {img: /images/b.svg}]}",
			[]string{"blocks[0].list[1].name quality.image_alt_missing low"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, site(map[string]string{"src/content/pages/cs/a.md": page("title: A page title that is long enough ok\nseo: {page_description: Twenty chars long ok, image: /images/a.jpg, image_alt: Our team}\nblocks:\n  - " + c.hero)}), quality.Options{})
			got := keys(res, report.RuleQualityImageAltMissing, report.RuleQualityImageAltPoor, report.RuleQualityHeadingSkip, report.RuleQualityLinkTextEmpty, report.RuleQualityLinkTextGeneric)
			for i := range got {
				got[i] = strings.TrimPrefix(got[i], "src/content/pages/cs/a.md#")
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("got\n%v\nwant\n%v", got, c.want)
			}
		})
	}
}

func TestBody(t *testing.T) {
	res := run(t, site(map[string]string{"src/content/pages/cs/a.md": page("title: A page title that is long enough ok") + "# Title\n\n### Skipped\n\n[click here](/x/)\n"}), quality.Options{})
	want := []string{"src/content/pages/cs/a.md#body quality.heading_skip low", "src/content/pages/cs/a.md#body quality.link_text_generic low"}
	if got := keys(res, report.RuleQualityHeadingSkip, report.RuleQualityLinkTextGeneric); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for _, f := range res.Findings {
		if f.Rule == report.RuleQualityHeadingSkip && (f.Measure == nil || f.Measure.Value != 3 || *f.Measure.Max != 2 || f.Value != "Skipped") {
			t.Errorf("heading skip %+v %+v", f, f.Measure)
		}
	}
}

func TestLanguages(t *testing.T) {
	fsys := site(map[string]string{
		"src/content/pages/cs/a.md": page("title: A page title that is long enough ok"),
		"src/content/pages/en/a.md": page("title: A page title that is long enough ok"),
		"src/content/pages/cs/b.md": page("title: A page title that is long enough ok"),
		"src/content/pages/en/c.md": page("title: A page title that is long enough ok"),
		"src/content/cases/one.md":  page("slug: one"),
		"src/content/nav/cs.yml":    "items: []\n",
	})
	fsys[casesFile] = &fstest.MapFile{Data: []byte("label: Cases\npath: src/content/cases\ntranslations: {by: path, locales: [cs]}\nfields: [{key: slug, type: text, label: Slug}]\n")}
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	t.Run("missing", func(t *testing.T) {
		res := run(t, fsys, quality.Options{})
		want := []string{"src/content/pages/cs/b.md# quality.translation_missing medium", "src/content/pages/en/c.md# quality.translation_missing medium"}
		if got := keys(res, report.RuleQualityTranslationMissing, report.RuleQualityTranslationOutdated); !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		// Pages a and b and c each expect cs and en; the case expects cs.
		if n := res.Quality.Checked[report.RuleQualityTranslationMissing]; n != 7 {
			t.Errorf("checked %d expected versions, want 7", n)
		}
		if !slices.ContainsFunc(res.Quality.Skipped, func(s quality.Skipped) bool { return s.Rule == report.RuleQualityTranslationOutdated }) {
			t.Errorf("outdated not skipped without dates: %+v", res.Quality.Skipped)
		}
		for _, f := range res.Findings {
			if f.Rule == report.RuleQualityTranslationMissing && f.File == "src/content/pages/cs/b.md" && f.Value != "en" {
				t.Errorf("value %q", f.Value)
			}
		}
	})
	t.Run("outdated", func(t *testing.T) {
		res := run(t, fsys, quality.Options{LastChanged: map[string]time.Time{
			"src/content/pages/cs/a.md": now,
			"src/content/pages/en/a.md": now.Add(-30 * 24 * time.Hour),
		}})
		want := []string{"src/content/pages/en/a.md# quality.translation_outdated medium"}
		if got := keys(res, report.RuleQualityTranslationOutdated); !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		if n := res.Quality.Checked[report.RuleQualityTranslationOutdated]; n != 2 {
			t.Errorf("checked %d", n)
		}
	})
	t.Run("within the limit or undated", func(t *testing.T) {
		res := run(t, fsys, quality.Options{LastChanged: map[string]time.Time{
			"src/content/pages/cs/a.md": now,
			"src/content/pages/en/a.md": now.Add(-6 * 24 * time.Hour),
			"src/content/pages/cs/b.md": now.Add(-300 * 24 * time.Hour),
		}})
		if got := keys(res, report.RuleQualityTranslationOutdated); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}

func TestMedia(t *testing.T) {
	fsys := site(map[string]string{
		"src/content/pages/cs/a.md": page("title: A page title that is long enough ok\nseo: {image: /images/big.jpg}\nblocks:\n  - {_type: hero, gallery: [/images/listed.png], text: '![x](/images/inline.webp)', photo: '/images/with%20space.jpg', photo_alt: A}"),
	})
	fsys[configFile] = &fstest.MapFile{Data: []byte(baseConfig + "extra: {default: /images/schema-default.png}\n")}
	files := []quality.MediaFile{
		{Path: "public/images/big.jpg", Size: 2 * 1024 * 1024},
		{Path: "public/images/listed.png", Size: 300 * 1024},
		{Path: "public/images/inline.webp", Size: 10},
		{Path: "public/images/with space.jpg", Size: 10},
		{Path: "public/images/schema-default.png", Size: 10},
		{Path: "public/images/logo.svg", Size: 5 * 1024 * 1024},
		{Path: "public/images/unused.jpg", Size: 10},
		{Path: "public/images/.gitkeep", Size: 0},
	}
	var asked []string
	list := func(roots []string) ([]quality.MediaFile, error) { asked = roots; return files, nil }
	res := run(t, fsys, quality.Options{ListMedia: list})
	if !slices.Equal(asked, []string{"public/images"}) {
		t.Errorf("roots %v; a read-only storage is not a media root", asked)
	}
	want := []string{
		"public/images/big.jpg# quality.image_heavy medium",
		"public/images/listed.png# quality.image_heavy low",
		"public/images/logo.svg# quality.media_unused info",
		"public/images/unused.jpg# quality.media_unused info",
	}
	if got := keys(res, report.RuleQualityImageHeavy, report.RuleQualityMediaUnused); !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
	// big, listed, inline, with space and schema-default are raster images
	// in use.
	if n := res.Quality.Checked[report.RuleQualityImageHeavy]; n != 5 {
		t.Errorf("checked %d", n)
	}

	t.Run("no lister skips", func(t *testing.T) {
		res := run(t, fsys, quality.Options{})
		if len(res.Quality.Skipped) != 3 { // both media rules and outdated
			t.Errorf("skipped %+v", res.Quality.Skipped)
		}
	})
	t.Run("a failed listing skips", func(t *testing.T) {
		res := run(t, fsys, quality.Options{ListMedia: func([]string) ([]quality.MediaFile, error) { return nil, errors.New("disk gone") }})
		if got := keys(res, report.RuleQualityImageHeavy, report.RuleQualityMediaUnused); len(got) != 0 {
			t.Errorf("got %v", got)
		}
		if !slices.ContainsFunc(res.Quality.Skipped, func(s quality.Skipped) bool { return strings.Contains(s.Reason, "disk gone") }) {
			t.Errorf("skipped %+v", res.Quality.Skipped)
		}
	})
	t.Run("limits", func(t *testing.T) {
		res := run(t, fsys, quality.Options{ListMedia: list, Limits: quality.Limits{ImageHeavyBytes: 5 * 1024 * 1024, ImageVeryHeavyBytes: 5 * 1024 * 1024}})
		if got := keys(res, report.RuleQualityImageHeavy); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("from a file system", func(t *testing.T) {
		fsys := fstest.MapFS{"public/images/a.jpg": {Data: make([]byte, 7)}, "public/images/.hidden/b.jpg": {Data: []byte("x")}, "public/images/sub/c.png": {Data: []byte("xx")}}
		got, err := quality.FSMedia(fsys)([]string{"public/images", "public/missing"})
		want := []quality.MediaFile{{Path: "public/images/a.jpg", Size: 7}, {Path: "public/images/sub/c.png", Size: 2}}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("got %v %v", got, err)
		}
	})
}

func TestIgnore(t *testing.T) {
	files := map[string]string{"src/content/pages/cs/a.md": page("title: A page title that is long enough ok")}
	t.Run("site-wide", func(t *testing.T) {
		fsys := site(files)
		fsys[configFile] = &fstest.MapFile{Data: []byte(baseConfig + "quality:\n  ignore: [quality.share_image_missing, quality.translation_missing]\n")}
		res := run(t, fsys, quality.Options{})
		if got := keys(res, report.RuleQualityShareImageMissing, report.RuleQualityTranslationMissing); len(got) != 0 {
			t.Errorf("got %v", got)
		}
		if !slices.Equal(res.Quality.Ignored, []report.Rule{report.RuleQualityShareImageMissing, report.RuleQualityTranslationMissing}) {
			t.Errorf("ignored %v", res.Quality.Ignored)
		}
		if _, counted := res.Quality.Checked[report.RuleQualityShareImageMissing]; counted {
			t.Error("an ignored rule is counted as checked")
		}
	})
	t.Run("per collection", func(t *testing.T) {
		fsys := site(files)
		fsys[pagesFile] = &fstest.MapFile{Data: []byte(pagesSchema + "quality: {ignore: [quality.description_missing]}\n")}
		res := run(t, fsys, quality.Options{})
		if got := keys(res, report.RuleQualityDescriptionMissing); len(got) != 0 {
			t.Errorf("got %v", got)
		}
		if got := res.Quality.IgnoredIn["pages"]; !slices.Equal(got, []report.Rule{report.RuleQualityDescriptionMissing}) {
			t.Errorf("ignoredIn %v", res.Quality.IgnoredIn)
		}
	})
	t.Run("the schema check", func(t *testing.T) {
		fsys := site(files)
		fsys[configFile] = &fstest.MapFile{Data: []byte(baseConfig + "quality:\n  ignore: [quality.nope, 3]\n  other: 1\n")}
		res, err := validate.Site(fsys, validate.Options{Only: []string{validate.ScopeSchema}})
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range res.Findings {
			if f.Rule == report.RuleSchemaInvalidQuality {
				n++
				if f.Severity != report.SeverityWarning {
					t.Errorf("severity %s", f.Severity)
				}
			}
		}
		if n != 3 {
			t.Errorf("%d schema.invalid_quality findings, want 3: %v", n, res.Findings)
		}
	})
}

// Quality runs only when asked for, never fails a strict run, and fails
// one only under StrictQuality.
func TestRunsOnlyWhenAsked(t *testing.T) {
	fsys := site(map[string]string{"src/content/pages/cs/a.md": page("seo: {page_title: Short}")})
	res, err := validate.Site(fsys, validate.Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Quality != nil || slices.ContainsFunc(res.Findings, func(f report.Finding) bool { return f.Rule.IsQuality() }) {
		t.Fatal("quality ran by default")
	}
	all := append(validate.Scopes()[:0:0], validate.Scopes()...)
	qualityBy := func(res *validate.Result) map[report.Severity]int {
		out := map[report.Severity]int{}
		for _, f := range res.Findings {
			if f.Rule.IsQuality() {
				out[f.Severity]++
			}
		}
		return out
	}
	res, err = validate.Site(fsys, validate.Options{Only: all, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := qualityBy(res); got[report.SeverityError] != 0 || got[report.SeverityWarning] == 0 || got[report.SeverityNotice] == 0 {
		t.Errorf("strict: %v", got)
	}
	res, err = validate.Site(fsys, validate.Options{Only: all, StrictQuality: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := qualityBy(res); got[report.SeverityError] == 0 || got[report.SeverityWarning] != 0 || got[report.SeverityNotice] == 0 || res.Notices != got[report.SeverityNotice] {
		t.Errorf("strict quality: %v, notices %d", got, res.Notices)
	}
}

func TestSpecs(t *testing.T) {
	var rules []report.Rule
	for _, s := range quality.Specs() {
		rules = append(rules, s.Rule)
		if !s.Rule.IsQuality() || !slices.Contains(quality.Areas(), s.Area) || s.Threshold == "" {
			t.Errorf("spec %+v", s)
		}
		if s.Scored() != (s.Impact != report.ImpactInfo) {
			t.Errorf("%s scored %v", s.Rule, s.Scored())
		}
	}
	if !slices.Equal(rules, report.QualityRules()) {
		t.Errorf("specs list\n%v\nthe rules are\n%v", rules, report.QualityRules())
	}
	for i, want := range map[report.Impact]report.Severity{report.ImpactHigh: report.SeverityWarning, report.ImpactMedium: report.SeverityWarning, report.ImpactLow: report.SeverityNotice, report.ImpactInfo: report.SeverityNotice} {
		if got := report.SeverityOf(i); got != want {
			t.Errorf("SeverityOf(%s) = %s", i, got)
		}
	}
}

func TestWords(t *testing.T) {
	for text, want := range map[string]bool{
		"Click here": true, "  zde ": true, "Více »": true, "Mehr erfahren →": true, "read more...": true,
		"See our pricing": false, "Ceník": false, "": false,
	} {
		if got := quality.GenericLinkText(text); got != want {
			t.Errorf("GenericLinkText(%q) = %v", text, got)
		}
	}
	for _, c := range []struct{ alt, image, want string }{
		{"Photo", "/images/x.jpg", "a generic word"},
		{"obrázek", "/images/x.jpg", "a generic word"},
		{"IMG_2041", "/images/x.jpg", "a camera's file name"},
		{"hero.jpg", "/images/hero.jpg", "the file name"},
		{"hero-final", "/images/hero-final.png?v=2", "the file name"},
		{"Acme", "/images/acme.svg", ""},
		{"Our team at the office", "/images/team.jpg", ""},
	} {
		if got := quality.PoorAlt(c.alt, c.image); got != c.want {
			t.Errorf("PoorAlt(%q, %q) = %q, want %q", c.alt, c.image, got, c.want)
		}
	}
}

// whenEmptySchema is the pages schema with each SEO field saying what
// the site does when it is empty.
func whenEmptySchema() string {
	s := strings.Replace(pagesSchema, "role: seoTitle}", "role: seoTitle, whenEmpty: generated}", 1)
	s = strings.Replace(s, "role: seoDescription,", "role: seoDescription, whenEmpty: siteDefault,", 1)
	return strings.Replace(s, "role: shareImage,", "role: shareImage, whenEmpty: generated,", 1)
}

// An empty role field the site fills in code is not missing: the page
// passes the rule and is counted as filled in code. A value the content
// holds is still measured.
func TestWhenEmpty(t *testing.T) {
	missing := []report.Rule{report.RuleQualityTitleMissing, report.RuleQualityDescriptionMissing, report.RuleQualityShareImageMissing}
	empty := page("title: A page title that is long enough ok")
	without := run(t, site(map[string]string{"src/content/pages/cs/a.md": empty}), quality.Options{})
	if got := keys(without, missing...); len(got) != 2 {
		t.Fatalf("without whenEmpty: %v", got)
	}
	if without.Quality.FilledInCode != nil {
		t.Fatalf("filled in code without whenEmpty: %v", without.Quality.FilledInCode)
	}

	fsys := site(map[string]string{
		"src/content/pages/cs/a.md": empty,
		"src/content/pages/cs/b.md": page("seo: {}"),
		// Filled values are checked as before.
		"src/content/pages/cs/c.md": page("seo: {page_title: Short, page_description: '" + strings.Repeat("y", 50) + "', image: /images/a.jpg}"),
		// A page hidden from search is not counted.
		"src/content/pages/en/a.md": page("seo: {no_index: true}"),
	})
	fsys[pagesFile] = &fstest.MapFile{Data: []byte(whenEmptySchema())}
	res := run(t, fsys, quality.Options{})
	if got := keys(res, missing...); len(got) != 0 {
		t.Errorf("missing reported for fields the site fills: %v", got)
	}
	want := []string{
		"src/content/pages/cs/c.md#seo.page_description quality.description_length low",
		"src/content/pages/cs/c.md#seo.page_title quality.title_length low",
	}
	if got := keys(res, report.RuleQualityTitleLength, report.RuleQualityDescriptionLength); !slices.Equal(got, want) {
		t.Errorf("lengths: got %v, want %v", got, want)
	}
	// The page title is not what the site shows for an empty SEO title
	// it fills, so a.md's page title is not measured.
	if n := res.Quality.Checked[report.RuleQualityTitleLength]; n != 1 {
		t.Errorf("title_length checked %d, want 1", n)
	}
	// Passing, not left out: every indexable page counts.
	for _, r := range missing {
		if n := res.Quality.Checked[r]; n != 3 {
			t.Errorf("%s checked %d, want 3", r, n)
		}
	}
	wantFilled := []quality.FilledInCode{
		{Role: "seoTitle", WhenEmpty: "generated", Pages: 2},
		{Role: "seoDescription", WhenEmpty: "siteDefault", Pages: 2},
		{Role: "shareImage", WhenEmpty: "generated", Pages: 2},
	}
	if !slices.Equal(res.Quality.FilledInCode, wantFilled) {
		t.Errorf("filled in code %+v, want %+v", res.Quality.FilledInCode, wantFilled)
	}
}

// The count is a fact about the pages: it stays when the missing rule is
// turned off.
func TestWhenEmptyCountedWhenRuleIgnored(t *testing.T) {
	fsys := site(map[string]string{"src/content/pages/cs/a.md": page("title: T")})
	fsys[pagesFile] = &fstest.MapFile{Data: []byte(whenEmptySchema())}
	fsys[configFile] = &fstest.MapFile{Data: []byte(baseConfig + "quality:\n  ignore: [quality.share_image_missing]\n")}
	res := run(t, fsys, quality.Options{})
	found := false
	for _, f := range res.Quality.FilledInCode {
		if f.Role == "shareImage" && f.Pages == 1 {
			found = true
		}
	}
	if !found || slices.Contains(keys(res), "src/content/pages/cs/a.md#seo.image quality.share_image_missing low") {
		t.Errorf("filled %+v, findings %v", res.Quality.FilledInCode, keys(res))
	}
}

// buildConfig has a public folder and an Astro assets folder; assets
// is the knob of the assets storage.
func buildConfig(assets string) string {
	return `pagesDir: src/content/pages
preview:
  defaultLocale: cs
  locales: [cs, en]
  prefixDefaultLocale: true
storages:
  images:
    type: repo
    path: public/images
    publicUrl: /images
  assets:
    type: repo
    path: src/assets/images
` + assets + `defaultStorage: images
`
}

func TestOptimizedAtBuild(t *testing.T) {
	const mb = 1024 * 1024
	files := []quality.MediaFile{
		{Path: "public/images/big.jpg", Size: 2 * mb},
		{Path: "src/assets/images/media.jpg", Size: 2 * mb},
		{Path: "src/assets/images/unused.jpg", Size: 2 * mb},
	}
	list := func([]string) ([]quality.MediaFile, error) { return files, nil }
	mk := func(assets string) fstest.MapFS {
		fsys := site(map[string]string{
			"src/content/pages/cs/a.md": page("title: A page title that is long enough ok\nseo: {image: /images/big.jpg, image_alt: A}\nblocks:\n  - {_type: hero, photo: /src/assets/images/media.jpg, photo_alt: B}"),
		})
		fsys[configFile] = &fstest.MapFile{Data: []byte(buildConfig(assets))}
		return fsys
	}
	astro := []string{"package.json", "astro.config.mjs"}
	rules := []report.Rule{report.RuleQualityImageHeavy, report.RuleQualityMediaUnused}

	t.Run("a declared folder is skipped, public is flagged, unused still applies", func(t *testing.T) {
		res := run(t, mk("    optimizedAtBuild: true\n"), quality.Options{ListMedia: list, RootFiles: astro})
		want := []string{
			"public/images/big.jpg# quality.image_heavy low", // public guessed as is
			"src/assets/images/unused.jpg# quality.media_unused info",
		}
		if got := keys(res, rules...); !slices.Equal(got, want) {
			t.Errorf("got\n%v\nwant\n%v", got, want)
		}
		if n := res.Quality.Checked[report.RuleQualityImageHeavy]; n != 1 {
			t.Errorf("checked %d; an optimized image is not checked", n)
		}
		wantFolders := []quality.MediaFolder{
			{Path: "public/images", Storage: "images", Guessed: true, Images: 1},
			{Path: "src/assets/images", Storage: "assets", OptimizedAtBuild: true, Images: 1},
		}
		if !slices.Equal(res.Quality.MediaFolders, wantFolders) {
			t.Errorf("folders %+v", res.Quality.MediaFolders)
		}
		wantGuess := []quality.Guess{{Kind: quality.GuessOptimizedAtBuild, Storage: "images", Path: "public/images", Framework: quality.FrameworkAstro, Basis: "astro.config.mjs"}}
		if !slices.Equal(res.Quality.Guessed, wantGuess) {
			t.Errorf("guessed %+v", res.Quality.Guessed)
		}
	})
	t.Run("without a framework public weighs fully and nothing is guessed", func(t *testing.T) {
		res := run(t, mk("    optimizedAtBuild: true\n"), quality.Options{ListMedia: list, RootFiles: []string{"package.json"}})
		if got := keys(res, report.RuleQualityImageHeavy); !slices.Equal(got, []string{"public/images/big.jpg# quality.image_heavy medium"}) {
			t.Errorf("got %v", got)
		}
		if len(res.Quality.Guessed) != 0 {
			t.Errorf("guessed %+v", res.Quality.Guessed)
		}
	})
	t.Run("an Astro site that declares nothing gets src/assets guessed", func(t *testing.T) {
		res := run(t, mk(""), quality.Options{ListMedia: list, RootFiles: astro})
		if got := keys(res, report.RuleQualityImageHeavy); !slices.Equal(got, []string{"public/images/big.jpg# quality.image_heavy low"}) {
			t.Errorf("got %v", got)
		}
		want := []quality.Guess{
			{Kind: quality.GuessOptimizedAtBuild, Storage: "images", Path: "public/images", Value: false, Framework: quality.FrameworkAstro, Basis: "astro.config.mjs"},
			{Kind: quality.GuessOptimizedAtBuild, Storage: "assets", Path: "src/assets/images", Value: true, Framework: quality.FrameworkAstro, Basis: "astro.config.mjs"},
		}
		if !slices.Equal(res.Quality.Guessed, want) {
			t.Errorf("guessed %+v", res.Quality.Guessed)
		}
	})
	t.Run("the same site without astro.config guesses nothing", func(t *testing.T) {
		res := run(t, mk(""), quality.Options{ListMedia: list})
		want := []string{"public/images/big.jpg# quality.image_heavy medium", "src/assets/images/media.jpg# quality.image_heavy medium"}
		if got := keys(res, report.RuleQualityImageHeavy); !slices.Equal(got, want) {
			t.Errorf("got %v", got)
		}
		if len(res.Quality.Guessed) != 0 {
			t.Errorf("guessed %+v", res.Quality.Guessed)
		}
	})
	t.Run("a declared false wins over the guess", func(t *testing.T) {
		res := run(t, mk("    optimizedAtBuild: false\n"), quality.Options{ListMedia: list, RootFiles: astro})
		if got := keys(res, report.RuleQualityImageHeavy); !slices.Contains(got, "src/assets/images/media.jpg# quality.image_heavy medium") {
			t.Errorf("got %v", got)
		}
		for _, g := range res.Quality.Guessed {
			if g.Storage == "assets" {
				t.Errorf("a declared folder is guessed: %+v", g)
			}
		}
	})
	t.Run("the caller's framework wins over the root files", func(t *testing.T) {
		res := run(t, mk(""), quality.Options{ListMedia: list, Framework: quality.FrameworkAstro})
		if len(res.Quality.Guessed) != 2 || res.Quality.Guessed[0].Basis != "" {
			t.Errorf("guessed %+v", res.Quality.Guessed)
		}
	})
	t.Run("lists", func(t *testing.T) {
		if !slices.Equal(quality.Frameworks(), []string{quality.FrameworkAstro}) || !slices.Equal(quality.GuessKinds(), []string{quality.GuessOptimizedAtBuild}) {
			t.Errorf("%v %v", quality.Frameworks(), quality.GuessKinds())
		}
	})
	t.Run("detect", func(t *testing.T) {
		for _, f := range []string{"astro.config.ts", "./astro.config.mjs", "astro.config.cjs"} {
			if fw, basis := quality.DetectFramework([]string{f}); fw != quality.FrameworkAstro || basis == "" {
				t.Errorf("%s: %q %q", f, fw, basis)
			}
		}
		if fw, _ := quality.DetectFramework([]string{"next.config.js", "src/astro.config.mjs"}); fw != "" {
			t.Errorf("got %q", fw)
		}
		got := quality.FSRootFiles(fstest.MapFS{"astro.config.mjs": {}, "src/a.ts": {}})
		if !slices.Equal(got, []string{"astro.config.mjs"}) {
			t.Errorf("root files %v", got)
		}
	})
}

func TestImageLimit(t *testing.T) {
	const kb = 1024
	files := []quality.MediaFile{
		{Path: "public/images/a.jpg", Size: 150 * kb},
		{Path: "public/images/b.jpg", Size: 600 * kb},
		{Path: "public/images/c.jpg", Size: 2500 * kb},
	}
	list := func([]string) ([]quality.MediaFile, error) { return files, nil }
	mk := func(quality string) fstest.MapFS {
		fsys := site(map[string]string{
			"src/content/pages/cs/a.md": page("title: A page title that is long enough ok\nblocks:\n  - {_type: hero, gallery: [/images/a.jpg, /images/b.jpg, /images/c.jpg]}"),
		})
		fsys[configFile] = &fstest.MapFile{Data: []byte(baseConfig + quality)}
		return fsys
	}
	heavy := func(res *validate.Result) []string { return keys(res, report.RuleQualityImageHeavy) }
	cases := []struct {
		name, quality string
		want          []string
	}{
		{"default", "", []string{"public/images/b.jpg# quality.image_heavy low", "public/images/c.jpg# quality.image_heavy medium"}},
		// 100 KB and 512 KB.
		{"lower", "quality: {limits: {imageKB: 100}}\n", []string{"public/images/a.jpg# quality.image_heavy low", "public/images/b.jpg# quality.image_heavy medium", "public/images/c.jpg# quality.image_heavy medium"}},
		// 3000 KB and 15000 KB.
		{"higher", "quality: {limits: {imageKB: 3000}}\n", nil},
		{"below the bound keeps the default", "quality: {limits: {imageKB: 10}}\n", []string{"public/images/b.jpg# quality.image_heavy low", "public/images/c.jpg# quality.image_heavy medium"}},
		{"above the bound keeps the default", "quality: {limits: {imageKB: 20000}}\n", []string{"public/images/b.jpg# quality.image_heavy low", "public/images/c.jpg# quality.image_heavy medium"}},
		{"a fraction keeps the default", "quality: {limits: {imageKB: 250.5}}\n", []string{"public/images/b.jpg# quality.image_heavy low", "public/images/c.jpg# quality.image_heavy medium"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := heavy(run(t, mk(c.quality), quality.Options{ListMedia: list})); !slices.Equal(got, c.want) {
				t.Errorf("got\n%v\nwant\n%v", got, c.want)
			}
		})
	}
	t.Run("the caller's limits win over the schema", func(t *testing.T) {
		res := run(t, mk("quality: {limits: {imageKB: 100}}\n"), quality.Options{ListMedia: list, Limits: quality.Limits{ImageHeavyBytes: 3000 * kb}})
		if got := heavy(res); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("the edges are allowed", func(t *testing.T) {
		for _, n := range []int{20, 10000} {
			if got := schemaFindings(t, mk(fmt.Sprintf("quality: {limits: {imageKB: %d}}\n", n)), report.RuleSchemaInvalidQuality); len(got) != 0 {
				t.Errorf("%d: %v", n, got)
			}
		}
	})
	t.Run("the schema check", func(t *testing.T) {
		for q, want := range map[string]string{
			"quality: {limits: {imageKB: 10}}\n":        "outside 20 to 10000",
			"quality: {limits: {imageKB: 20000}}\n":     "outside 20 to 10000",
			"quality: {limits: {imageKB: 250.5}}\n":     "not a whole number",
			"quality: {limits: {imageKB: big}}\n":       "not a whole number",
			"quality: {limits: {imageMB: 1}}\n":         `the key "imageMB"`,
			"quality: {limits: 300}\n":                  "not a mapping",
			"quality: {limits: {imageKB: 300}, x: 1}\n": "only `ignore` and `limits`",
		} {
			got := schemaFindings(t, mk(q), report.RuleSchemaInvalidQuality)
			if len(got) != 1 || !strings.Contains(got[0].Message, want) || got[0].Severity != report.SeverityWarning {
				t.Errorf("%q: %v", q, got)
			}
		}
	})
	t.Run("limits in a collection are reported", func(t *testing.T) {
		fsys := mk("")
		fsys[pagesFile] = &fstest.MapFile{Data: []byte(pagesSchema + "quality: {limits: {imageKB: 300}}\n")}
		got := schemaFindings(t, fsys, report.RuleSchemaInvalidQuality)
		if len(got) != 1 || !strings.Contains(got[0].Message, "config.yml only") {
			t.Errorf("got %v", got)
		}
	})
	t.Run("optimizedAtBuild must be a boolean", func(t *testing.T) {
		fsys := mk("")
		fsys[configFile] = &fstest.MapFile{Data: []byte(buildConfig("    optimizedAtBuild: yes please\n"))}
		got := schemaFindings(t, fsys, report.RuleSchemaInvalidStorage)
		if len(got) != 1 || !strings.Contains(got[0].Message, "not true or false") || got[0].Severity != report.SeverityError {
			t.Errorf("got %v", got)
		}
		fsys[configFile] = &fstest.MapFile{Data: []byte(buildConfig("    optimizedAtBuild: true\n"))}
		if got := schemaFindings(t, fsys, report.RuleSchemaInvalidStorage); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}

func schemaFindings(t *testing.T, fsys fstest.MapFS, rule report.Rule) []report.Finding {
	t.Helper()
	res, err := validate.Site(fsys, validate.Options{Only: []string{validate.ScopeSchema}})
	if err != nil {
		t.Fatal(err)
	}
	var out []report.Finding
	for _, f := range res.Findings {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}
