package resolve_test

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

func loadSchema(t *testing.T, files map[string]string) *schema.Config {
	t.Helper()
	fsys := fstest.MapFS{}
	for p, body := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(body)}
	}
	doc, findings, err := load.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("load: %v", findings)
	}
	return doc.Config
}

// The shape of jetlio.com's schema: an `seo` block pulled into pages by
// `$ref`, logos whose alt text is the client's name, images with a
// `<key>_alt` sibling. Nothing is declared; every role is guessed.
var jetlio = map[string]string{
	load.ConfigFile: "siteUrl: https://jetlio.com\npagesDir: src/content/pages\npreview:\n  defaultLocale: cs\n  locales: [cs, en]\n",
	load.CollectionsDir + "/pages.yml": `label: Pages
path: src/content/pages
previewPath: '/{locale}/{slug}/'
localeFrom: path
translations:
  by: field:translationKey
fields:
  - key: title
    type: text
    label: Title
    required: true
  - key: translationKey
    type: text
    label: Translation key
    required: true
  - $ref: seo
  - key: content_blocks
    type: array
    label: Content blocks
    idKey: _component
    items:
      - $ref: hero-split
      - $ref: logos
preview:
  title:
    - key: title
    - key: seo.page_title
  description:
    - key: seo.page_description
  image:
    - key: seo.featured_image
`,
	load.BlocksDir + "/seo.yml": `label: SEO
fields:
  - key: page_title
    type: text
    label: Meta title
    recommendedLength: { from: 30, to: 60 }
  - key: page_description
    type: paragraph
    label: Meta description
    recommendedLength: { from: 70, to: 160 }
  - key: featured_image
    type: image
    label: Share image
  - key: featured_image_alt
    type: text
    label: Share image description (alt)
  - key: canonical_url
    type: link
    label: Canonical URL
  - key: open_graph_type
    type: select
    label: Open Graph type
    options: [website, article]
  - key: author_twitter_handle
    type: text
    label: Author's X / Twitter handle
  - key: no_index
    type: boolean
    label: Hide from search engines
`,
	load.BlocksDir + "/logos.yml": `label: Client logos
fields:
  - key: text
    type: richtext
    label: Text
  - key: list
    type: array
    label: Logos
    items:
      - key: logo
        type: group
        label: Logo
        fields:
          - key: name
            type: text
            label: Client name
            description: Also the image's alt text.
          - key: img
            type: image
            label: Logo image
          - key: url
            type: link
            label: Client website
`,
	load.BlocksDir + "/hero-split.yml": `label: Hero split
fields:
  - key: heading
    type: text
    label: Heading
  - key: image
    type: image
    label: Image
  - key: image_alt
    type: text
    label: Image alt
  - key: background
    type: image
    label: Background
`,
}

func TestRolesGuessedOnJetlio(t *testing.T) {
	cfg := loadSchema(t, jetlio)
	got := resolve.CollectionRoles(cfg, cfg.Collections["pages"])
	want := []resolve.RoleField{
		{Role: schema.RoleSEOTitle, Path: "seo.page_title", Label: "Meta title"},
		{Role: schema.RoleSEODescription, Path: "seo.page_description", Label: "Meta description"},
		{Role: schema.RoleShareImage, Path: "seo.featured_image", Label: "Share image"},
		{Role: schema.RoleNoindex, Path: "seo.no_index", Label: "Hide from search engines"},
		{Role: schema.RoleCanonical, Path: "seo.canonical_url", Label: "Canonical URL"},
	}
	if !reflect.DeepEqual(got.Fields, want) {
		t.Fatalf("roles:\n got %+v\nwant %+v", got.Fields, want)
	}
	wantImages := []resolve.ImageAlt{
		{Image: "seo.featured_image", Label: "Share image", Alt: "seo.featured_image_alt"},
		{Image: "content_blocks[hero-split].image", Label: "Image", Alt: "content_blocks[hero-split].image_alt"},
		// Two images in one block: only the one named by its key has alt.
		{Image: "content_blocks[hero-split].background", Label: "Background"},
		{Image: "content_blocks[logos].list[].img", Label: "Logo image", Alt: "content_blocks[logos].list[].name"},
	}
	if !reflect.DeepEqual(got.Images, wantImages) || got.Truncated {
		t.Fatalf("images:\n got %+v\nwant %+v", got.Images, wantImages)
	}
	// The same schema answers the same every time.
	for range 20 {
		if again := resolve.CollectionRoles(cfg, cfg.Collections["pages"]); !reflect.DeepEqual(again, got) {
			t.Fatal("the answer changed between calls")
		}
	}
}

// Declared roles win over guesses; the rest is still guessed.
func TestDeclaredRolesWin(t *testing.T) {
	files := map[string]string{}
	for k, v := range jetlio {
		files[k] = v
	}
	// The page title is declared as the SEO title, the alt of the logo
	// is declared, the background is decorative.
	files[load.CollectionsDir+"/pages.yml"] = strings.Replace(jetlio[load.CollectionsDir+"/pages.yml"], "    label: Title\n", "    label: Title\n    role: seoTitle\n", 1)
	files[load.BlocksDir+"/logos.yml"] = strings.Replace(jetlio[load.BlocksDir+"/logos.yml"], "    label: Client name\n", "    label: Client name\n            role: alt\n", 1)
	files[load.BlocksDir+"/hero-split.yml"] = strings.Replace(jetlio[load.BlocksDir+"/hero-split.yml"], "    label: Background\n", "    label: Background\n    decorative: true\n", 1)
	cfg := loadSchema(t, files)
	got := resolve.CollectionRoles(cfg, cfg.Collections["pages"])
	title, ok := got.Field(schema.RoleSEOTitle)
	if !ok || title.Path != "title" || !title.Declared {
		t.Fatalf("seoTitle: %+v", title)
	}
	if d, _ := got.Field(schema.RoleSEODescription); d.Path != "seo.page_description" || d.Declared {
		t.Fatalf("seoDescription: %+v", d)
	}
	if title.WhenEmpty != "" {
		t.Fatalf("whenEmpty nobody declared: %+v", title)
	}
	byImage := map[string]resolve.ImageAlt{}
	for _, img := range got.Images {
		byImage[img.Image] = img
	}
	if bg := byImage["content_blocks[hero-split].background"]; !bg.Decorative || !bg.Declared || bg.Alt != "" {
		t.Fatalf("background: %+v", bg)
	}
	if logo := byImage["content_blocks[logos].list[].img"]; logo.Alt != "content_blocks[logos].list[].name" || !logo.Declared {
		t.Fatalf("logo: %+v", logo)
	}
}

// The shape of the CMS template (astro-template): an inline `seo` group
// with generic keys, a robots select (not a boolean, so no noindex), a
// product collection with `image` and `imageAlt`, and a settings
// document.
var template = map[string]string{
	load.ConfigFile: "pagesDir: src/content\npreview:\n  defaultLocale: cs\n  locales: [cs, en]\n",
	load.CollectionsDir + "/pages.yml": `label: Pages
path: src/content/pages
localeFrom: path
translations:
  by: field:translationKey
fields:
  - key: title
    type: text
    label: Title
  - key: translationKey
    type: text
    label: Translation key
    required: true
  - key: seo
    type: group
    label: SEO
    fields:
      - key: title
        type: text
        label: Meta title
      - key: description
        type: paragraph
        label: Meta description
      - key: image
        type: image
        label: Social image
      - key: robots
        type: select
        label: Robots
        options: [index, noindex]
  - key: blocks
    type: array
    label: Content blocks
    idKey: _component
    items:
      - $ref: logos
      - $ref: media-split
`,
	load.CollectionsDir + "/products.yml": `label: Products
path: src/content/products
match: '*.md'
fields:
  - key: title
    type: text
    label: Title
  - key: description
    type: paragraph
    label: Description
  - key: image
    type: image
    label: Photo
  - key: imageUrl
    type: link
    label: Photo URL
  - key: imageAlt
    type: text
    label: Alt text
`,
	load.CollectionsDir + "/globals.yml": `label: Global variables
path: src/content/settings
match: globals.md
area: site
fields:
  - key: variables
    type: array
    label: Variables
    items:
      - key: variable
        type: text
        label: Variable
`,
	load.BlocksDir + "/logos.yml": `label: Logos
fields:
  - key: items
    type: array
    label: Logos
    items:
      - key: logo
        type: group
        label: Logo
        fields:
          - key: image
            type: image
            label: Logo
          - key: imageUrl
            type: link
            label: Logo URL
          - key: href
            type: link
            label: Link
          - key: name
            type: text
            label: Name
`,
	load.BlocksDir + "/media-split.yml": `label: Media split
fields:
  - key: title
    type: text
    label: Title
  - key: image
    type: image
    label: Image
  - key: imageAlt
    type: text
    label: Image alt
`,
}

func TestRolesGuessedOnTheTemplate(t *testing.T) {
	cfg := loadSchema(t, template)
	pages := resolve.CollectionRoles(cfg, cfg.Collections["pages"])
	want := []resolve.RoleField{
		{Role: schema.RoleSEOTitle, Path: "seo.title", Label: "Meta title"},
		{Role: schema.RoleSEODescription, Path: "seo.description", Label: "Meta description"},
		{Role: schema.RoleShareImage, Path: "seo.image", Label: "Social image"},
	}
	if !reflect.DeepEqual(pages.Fields, want) {
		t.Fatalf("pages roles:\n got %+v\nwant %+v", pages.Fields, want)
	}
	wantImages := []resolve.ImageAlt{
		{Image: "seo.image", Label: "Social image"},
		{Image: "blocks[logos].items[].image", Label: "Logo", Alt: "blocks[logos].items[].name"},
		{Image: "blocks[media-split].image", Label: "Image", Alt: "blocks[media-split].imageAlt"},
	}
	if !reflect.DeepEqual(pages.Images, wantImages) {
		t.Fatalf("pages images:\n got %+v\nwant %+v", pages.Images, wantImages)
	}
	// A product's `title`, `description` and `image` are the product's,
	// not the page's search entries: nothing is guessed.
	products := resolve.CollectionRoles(cfg, cfg.Collections["products"])
	if len(products.Fields) != 0 {
		t.Fatalf("products guessed %+v", products.Fields)
	}
	if !reflect.DeepEqual(products.Images, []resolve.ImageAlt{{Image: "image", Label: "Photo", Alt: "imageAlt"}}) {
		t.Fatalf("products images: %+v", products.Images)
	}
	if !cfg.Collections["globals"].SiteArea() || cfg.Collections["pages"].SiteArea() {
		t.Fatal("area")
	}
}

func TestRoleGuessing(t *testing.T) {
	text := func(key, label string) schema.Field {
		return schema.Field{Key: key, Type: schema.TypeText, Label: label}
	}
	cases := []struct {
		name   string
		fields []schema.Field
		role   string
		want   string // path, "" for none
	}{
		{"meta_title at the top level", []schema.Field{text("title", "Title"), text("meta_title", "Meta")}, schema.RoleSEOTitle, "meta_title"},
		{"metaTitle at the top level", []schema.Field{text("metaTitle", "M")}, schema.RoleSEOTitle, "metaTitle"},
		{"by label", []schema.Field{text("headline", "SEO title")}, schema.RoleSEOTitle, "headline"},
		{"a plain title is the page's", []schema.Field{text("title", "Title")}, schema.RoleSEOTitle, ""},
		{"title inside a meta group", []schema.Field{{Key: "meta", Type: schema.TypeGroup, Fields: []schema.Field{text("title", "T")}}}, schema.RoleSEOTitle, "meta.title"},
		{"title inside a group pulled from the seo block", []schema.Field{{Key: "search", Type: schema.TypeGroup, BlockRef: "seo", Fields: []schema.Field{text("title", "T")}}}, schema.RoleSEOTitle, "search.title"},
		{"title two groups below seo", []schema.Field{{Key: "seo", Type: schema.TypeGroup, Fields: []schema.Field{{Key: "og", Type: schema.TypeGroup, Fields: []schema.Field{text("title", "T")}}}}}, schema.RoleSEOTitle, ""},
		{"two candidates guess nothing", []schema.Field{text("meta_title", "M"), text("seo_title", "S")}, schema.RoleSEOTitle, ""},
		{"a candidate of the wrong type", []schema.Field{{Key: "meta_title", Type: schema.TypeRichText}}, schema.RoleSEOTitle, ""},
		{"a candidate inside a list", []schema.Field{{Key: "items", Type: schema.TypeArray, Items: []schema.Field{{Key: "i", Type: schema.TypeGroup, Fields: []schema.Field{text("meta_title", "M")}}}}}, schema.RoleSEOTitle, ""},
		{"a candidate declaring another role", []schema.Field{{Key: "meta_title", Type: schema.TypeText, Role: schema.RoleSEODescription}}, schema.RoleSEOTitle, ""},
		{"a declaration of the wrong type is ignored", []schema.Field{{Key: "x", Type: schema.TypeNumber, Role: schema.RoleSEOTitle}, text("meta_title", "M")}, schema.RoleSEOTitle, "meta_title"},
		{"meta_description", []schema.Field{{Key: "meta_description", Type: schema.TypeParagraph}}, schema.RoleSEODescription, "meta_description"},
		{"a top-level description is the page's", []schema.Field{{Key: "description", Type: schema.TypeParagraph}}, schema.RoleSEODescription, ""},
		{"og_image", []schema.Field{{Key: "og_image", Type: schema.TypeImage}}, schema.RoleShareImage, "og_image"},
		{"share-image", []schema.Field{{Key: "share-image", Type: schema.TypeImage}}, schema.RoleShareImage, "share-image"},
		{"a top-level image is the page's", []schema.Field{{Key: "image", Type: schema.TypeImage}}, schema.RoleShareImage, ""},
		{"noindex", []schema.Field{{Key: "noindex", Type: schema.TypeBoolean}}, schema.RoleNoindex, "noindex"},
		{"a robots select is not noindex", []schema.Field{{Key: "noindex", Type: schema.TypeSelect}}, schema.RoleNoindex, ""},
		{"canonicalUrl", []schema.Field{{Key: "canonicalUrl", Type: schema.TypeLink}}, schema.RoleCanonical, "canonicalUrl"},
		{"role none is never guessed", []schema.Field{{Key: "meta", Type: schema.TypeGroup, Fields: []schema.Field{{Key: "title", Type: schema.TypeText, Role: schema.RoleNone}}}}, schema.RoleSEOTitle, ""},
		{"role none leaves the other candidate alone", []schema.Field{{Key: "meta_title", Type: schema.TypeText, Role: schema.RoleNone}, text("seo_title", "S")}, schema.RoleSEOTitle, "seo_title"},
	}
	for _, tc := range cases {
		got := resolve.CollectionRoles(&schema.Config{}, schema.Collection{Fields: tc.fields})
		f, ok := got.Field(tc.role)
		switch {
		case tc.want == "" && ok:
			t.Errorf("%s: guessed %+v", tc.name, f)
		case tc.want != "" && (!ok || f.Path != tc.want || f.Declared):
			t.Errorf("%s: got %+v, want a guess at %s", tc.name, f, tc.want)
		}
	}
}

func TestAltOf(t *testing.T) {
	img := schema.Field{Key: "photo", Type: schema.TypeImage}
	other := schema.Field{Key: "cover", Type: schema.TypeImage}
	text := func(key string) schema.Field { return schema.Field{Key: key, Type: schema.TypeText} }
	with := func(f schema.Field, edit func(*schema.Field)) schema.Field { edit(&f); return f }
	cases := []struct {
		name               string
		image              schema.Field
		siblings           []schema.Field
		alt                string
		decorative, declar bool
	}{
		{"decorative", with(img, func(f *schema.Field) { f.Decorative = true }), []schema.Field{text("alt")}, "", true, true},
		{"alt names a sibling", with(img, func(f *schema.Field) { f.Alt = "caption" }), []schema.Field{text("caption"), text("alt")}, "caption", false, true},
		{"alt names nothing usable", with(img, func(f *schema.Field) { f.Alt = "nope" }), []schema.Field{text("alt")}, "", false, false},
		{"role alt", img, []schema.Field{with(text("words"), func(f *schema.Field) { f.Role = schema.RoleAlt })}, "words", false, true},
		{"role alt beside two images guesses on", img, []schema.Field{other, with(text("words"), func(f *schema.Field) { f.Role = schema.RoleAlt })}, "", false, false},
		{"<key>_alt", img, []schema.Field{other, text("photo_alt")}, "photo_alt", false, false},
		{"<key>Alt", img, []schema.Field{text("photoAlt")}, "photoAlt", false, false},
		{"<key>_alt_text", img, []schema.Field{text("photo_alt_text")}, "photo_alt_text", false, false},
		{"alt beside one image", img, []schema.Field{text("alt")}, "alt", false, false},
		{"alt_text beside one image", img, []schema.Field{text("alt_text")}, "alt_text", false, false},
		{"alt beside two images is nobody's", img, []schema.Field{other, text("alt")}, "", false, false},
		{"name beside one image", img, []schema.Field{text("name"), text("url")}, "name", false, false},
		{"alt wins over name", img, []schema.Field{text("name"), text("alt")}, "alt", false, false},
		{"two fits guess nothing", img, []schema.Field{text("alt"), text("altText")}, "", false, false},
		{"a field saying role none is not guessed", img, []schema.Field{with(text("alt"), func(f *schema.Field) { f.Role = schema.RoleNone })}, "", false, false},
		{"a field declaring a role is not guessed", img, []schema.Field{with(text("alt"), func(f *schema.Field) { f.Role = schema.RoleSEOTitle })}, "", false, false},
		{"a number is not alt text", img, []schema.Field{{Key: "alt", Type: schema.TypeNumber}}, "", false, false},
		{"nothing fits", img, []schema.Field{text("caption")}, "", false, false},
	}
	for _, tc := range cases {
		siblings := append([]schema.Field{tc.image}, tc.siblings...)
		alt, decorative, declared := resolve.AltOf(siblings, tc.image)
		if alt != tc.alt || decorative != tc.decorative || declared != tc.declar {
			t.Errorf("%s: got (%q, %v, %v), want (%q, %v, %v)", tc.name, alt, decorative, declared, tc.alt, tc.decorative, tc.declar)
		}
	}
}

func TestImagesAreBounded(t *testing.T) {
	var fields []schema.Field
	for i := range resolve.MaxRoleImages + 3 {
		fields = append(fields, schema.Field{Key: "i" + strings.Repeat("x", i), Type: schema.TypeImage})
	}
	got := resolve.CollectionRoles(&schema.Config{}, schema.Collection{Fields: fields})
	if len(got.Images) != resolve.MaxRoleImages || !got.Truncated {
		t.Fatalf("%d images, truncated %v", len(got.Images), got.Truncated)
	}
}

func TestExpectedLocales(t *testing.T) {
	cfg := &schema.Config{Preview: schema.PreviewSite{Locales: []string{"cs", "en", "de"}}}
	cases := []struct {
		name string
		tr   *schema.Translations
		want []string
	}{
		{"no translations", nil, []string{"cs", "en", "de"}},
		{"no locales declared", &schema.Translations{By: "path"}, []string{"cs", "en", "de"}},
		{"a subset", &schema.Translations{By: "path", Locales: []string{"en", "cs"}}, []string{"en", "cs"}},
		{"a language the site lacks is left out", &schema.Translations{By: "path", Locales: []string{"cs", "fr"}}, []string{"cs"}},
		{"none of the site's", &schema.Translations{By: "path", Locales: []string{"fr"}}, []string{}},
	}
	for _, tc := range cases {
		if got := resolve.ExpectedLocales(cfg, schema.Collection{Translations: tc.tr}); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := resolve.ExpectedLocales(nil, schema.Collection{}); len(got) != 0 {
		t.Errorf("no config: %v", got)
	}
}

// whenEmpty travels with a declared role; a value the module does not
// know, or one on a role that cannot take it, is left out.
func TestWhenEmptyOnRoles(t *testing.T) {
	cfg, err := schema.Parse(map[string]any{"collections": map[string]any{"pages": map[string]any{"fields": []any{
		map[string]any{"key": "og", "type": "image", "label": "OG", "role": "shareImage", "whenEmpty": "generated"},
		map[string]any{"key": "desc", "type": "paragraph", "label": "D", "role": "seoDescription", "whenEmpty": "fallback"},
		map[string]any{"key": "canonical", "type": "link", "label": "C", "role": "canonical", "whenEmpty": "siteDefault"},
		map[string]any{"key": "meta_title", "type": "text", "label": "T", "whenEmpty": "generated"},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	got := resolve.CollectionRoles(cfg, cfg.Collections["pages"])
	for role, want := range map[string]string{schema.RoleShareImage: schema.WhenEmptyGenerated, schema.RoleSEODescription: "", schema.RoleCanonical: "", schema.RoleSEOTitle: ""} {
		f, ok := got.Field(role)
		if !ok || f.WhenEmpty != want {
			t.Errorf("%s: %+v, want whenEmpty %q", role, f, want)
		}
	}
}
