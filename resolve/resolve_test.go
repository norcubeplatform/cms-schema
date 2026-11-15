package resolve

import (
	"reflect"
	"testing"

	"github.com/norcubeplatform/cms-schema/schema"
)

// The URL cases are the ones the CMS service and the dashboard share;
// every implementation must agree on each file's URL.
var urlSite = schema.PreviewSite{DefaultLocale: "cs", Locales: []string{"cs", "en", "it"}}

var urlCollections = map[string]schema.Collection{
	"pages":  {Key: "pages", Path: "src/content/pages", PreviewPath: "/{locale}/{slug}/", LocaleFrom: "path"},
	"works":  {Key: "works", Path: "src/content/works", PreviewPath: "/{locale}/portfolio/{slug}/", LocaleFrom: "none"},
	"events": {Key: "events", Path: "src/content/events", PreviewPath: "/{locale}/vystavy/{slug}/", LocaleFrom: "none"},
}

func TestURL(t *testing.T) {
	cases := []struct {
		name, path, coll string
		fm               map[string]any
		want             string
	}{
		{"default-locale page is unprefixed", "src/content/pages/cs/kontakt.md", "pages", map[string]any{"slug": "kontakt"}, "/kontakt/"},
		{"non-default locale is prefixed", "src/content/pages/en/contact.md", "pages", map[string]any{"slug": "contact"}, "/en/contact/"},
		{"italian page", "src/content/pages/it/contatti.md", "pages", map[string]any{"slug": "contatti"}, "/it/contatti/"},
		{"default-locale home is the root", "src/content/pages/cs/index.md", "pages", map[string]any{"slug": ""}, "/"},
		{"non-default home keeps the prefix", "src/content/pages/en/index.md", "pages", map[string]any{"slug": ""}, "/en/"},
		{"empty slug map falls back to the filename", "src/content/works/ticho-ii.md", "works", map[string]any{"slug": map[string]any{}}, "/portfolio/ticho-ii/"},
		{"default-locale slug entry", "src/content/works/ticho-ii.md", "works", map[string]any{"slug": map[string]any{"cs": "ticho-ii-cs", "en": "silence-ii"}}, "/portfolio/ticho-ii-cs/"},
		{"events parent segment", "src/content/events/vernisaz-ticho.md", "events", map[string]any{"slug": map[string]any{}}, "/vystavy/vernisaz-ticho/"},
		{"missing slug falls back to the filename", "src/content/events/monochrom.md", "events", map[string]any{}, "/vystavy/monochrom/"},
		{"stray file under the root uses the default locale", "src/content/pages/stray.md", "pages", map[string]any{"slug": "stray"}, "/stray/"},
		{"nested page without a slug keeps its folder", "src/content/pages/cs/reseni/hosting.md", "pages", map[string]any{}, "/reseni/hosting/"},
		{"nested page in a prefixed locale", "src/content/pages/en/solutions/hosting.md", "pages", map[string]any{}, "/en/solutions/hosting/"},
		{"folder index without a slug is the folder", "src/content/pages/cs/technologie/index.md", "pages", map[string]any{}, "/technologie/"},
		{"locale index without a slug is the locale home", "src/content/pages/en/index.md", "pages", map[string]any{}, "/en/"},
		{"default-locale index without a slug is the root", "src/content/pages/cs/index.md", "pages", map[string]any{}, "/"},
		{"file outside the collection root keeps the file name", "content/elsewhere/deep/thing.md", "pages", map[string]any{}, "/thing/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := URL(tc.path, urlCollections[tc.coll], urlSite, tc.fm); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("no previewPath means no URL", func(t *testing.T) {
		coll := schema.Collection{Key: "x", Path: "src/content/pages"}
		if got := URL("src/content/pages/cs/kontakt.md", coll, urlSite, map[string]any{"slug": "kontakt"}); got != "" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("prefixed default locale", func(t *testing.T) {
		site := schema.PreviewSite{DefaultLocale: "cs", Locales: []string{"cs", "en"}, PrefixDefaultLocale: true}
		if got := URL("src/content/pages/cs/kontakt.md", urlCollections["pages"], site, map[string]any{"slug": "kontakt"}); got != "/cs/kontakt/" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("fields token and locale from a field", func(t *testing.T) {
		coll := schema.Collection{Key: "posts", Path: "src/content/posts", PreviewPath: "/blog/{fields.category}/{slug}/", LocaleFrom: "none"}
		if got := URL("src/content/posts/hello.md", coll, urlSite, map[string]any{"slug": "hello", "category": "news"}); got != "/blog/news/hello/" {
			t.Fatalf("got %q", got)
		}
		coll = schema.Collection{Key: "pages", Path: "src/content/pages", PreviewPath: "/{locale}/{slug}/", LocaleFrom: "field:lang"}
		if got := URL("src/content/pages/contact.md", coll, urlSite, map[string]any{"slug": "contact", "lang": "en"}); got != "/en/contact/" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestNormalizeAndPagination(t *testing.T) {
	for in, want := range map[string]string{"/sluzby/": "/sluzby", "/sluzby?x=1#top": "/sluzby", "//a//b/": "/a/b", "": "/", "/": "/"} {
		if got := NormalizeURLPath(in); got != want {
			t.Errorf("NormalizeURLPath(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"/blog/2": "/blog", "/2": "/", "/blog": "", "/en/blog/10": "/en/blog"} {
		if got := StripPagination(in); got != want {
			t.Errorf("StripPagination(%q) = %q, want %q", in, got, want)
		}
	}
	cfg := &schema.Config{KnownPaths: []string{"/404/", "/api/*"}}
	for in, want := range map[string]bool{"/404": true, "/404/": true, "/api/health": true, "/api": true, "/apix": true, "/other/": false} {
		if got := IsKnownPath(cfg, in); got != want {
			t.Errorf("IsKnownPath(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCollectionFor(t *testing.T) {
	cfg := &schema.Config{
		Collections: map[string]schema.Collection{
			"pages":   {Key: "pages", Path: "src/content/pages", Match: "**/*.md"},
			"studies": {Key: "studies", Path: "src/content/pages/cs/studie", Match: "*.md"},
			"globals": {Key: "globals", Path: "src/content/settings", Match: "globals.md"},
			"theme":   {Key: "theme", Path: "src/content/settings", Match: "theme.md"},
		},
		CollectionKeys: []string{"globals", "pages", "studies", "theme"},
	}
	cases := map[string]string{
		"src/content/pages/cs/index.md":    "pages",
		"src/content/pages/cs/studie/a.md": "studies",
		"src/content/settings/theme.md":    "theme",
		"src/content/settings/globals.md":  "globals",
		"src/content/settings/other.md":    "globals", // tie, first by key
		"src/content/pagesx/a.md":          "pages",   // fallback to pages
	}
	for p, want := range cases {
		coll, ok := CollectionFor(cfg, p, "")
		if !ok || coll.Key != want {
			t.Errorf("CollectionFor(%q) = %q (%v), want %q", p, coll.Key, ok, want)
		}
	}
	if coll, _ := CollectionFor(cfg, "src/content/pages/cs/index.md", "theme"); coll.Key != "theme" {
		t.Errorf("explicit collection should win, got %q", coll.Key)
	}
	if coll, _ := CollectionFor(cfg, "src/content/pages/cs/index.md", "ghost"); coll.Key != "pages" {
		t.Errorf("unknown explicit collection should be ignored, got %q", coll.Key)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"*.md", "a.md", true},
		{"*.md", "cs/a.md", false},
		{"**/*.md", "cs/a.md", true},
		{"**/*.md", "a.md", true},
		{"**/*.{md,mdx}", "cs/deep/a.mdx", true},
		{"*.yml", "en.yml", true},
		{"*.yml", "en.yaml", false},
	}
	for _, tc := range cases {
		if got := GlobMatch(tc.pattern, tc.rel); got != tc.want {
			t.Errorf("GlobMatch(%q, %q) = %v, want %v", tc.pattern, tc.rel, got, tc.want)
		}
	}
}

func TestStorageValues(t *testing.T) {
	uploads := schema.Storage{Name: "uploads", Type: "repo", Path: "public/uploads", PublicURL: "/uploads"}
	cdn := schema.Storage{Name: "cdn", Type: "repo", Path: "assets", PublicURL: "https://cdn.example.com"}
	icons := schema.Storage{Name: "icons", Type: "repo", Path: "src/assets/icons"}
	if got := PublicValue(uploads, "hero.jpg"); got != "/uploads/hero.jpg" {
		t.Errorf("uploads: %q", got)
	}
	if got := PublicValue(cdn, "hero.jpg"); got != "https://cdn.example.com/hero.jpg" {
		t.Errorf("cdn: %q", got)
	}
	if got := PublicValue(icons, "check.svg"); got != "/src/assets/icons/check.svg" {
		t.Errorf("icons: %q", got)
	}
	for _, s := range []schema.Storage{uploads, cdn, icons} {
		v := PublicValue(s, "deep/file.png")
		key, ok := KeyFromValue(s, v)
		if !ok || key != "deep/file.png" {
			t.Errorf("%s: KeyFromValue(%q) = %q, %v", s.Name, v, key, ok)
		}
	}
	if _, ok := KeyFromValue(uploads, "https://elsewhere.com/x.png"); ok {
		t.Error("external URL must not match a root-relative storage")
	}
	cfg := &schema.Config{PublicDir: "public", Storages: []schema.Storage{uploads, icons}}
	for in, want := range map[string]string{
		"/uploads/a.jpg?v=2":      "public/uploads/a.jpg",
		"/src/assets/icons/x.svg": "src/assets/icons/x.svg",
		"/images/logo.png":        "public/images/logo.png",
		"docs/file.pdf":           "docs/file.pdf",
	} {
		if got := AssetRepoPath(cfg, in); got != want {
			t.Errorf("AssetRepoPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInternalLinks(t *testing.T) {
	text := "See [a](/a/) and [b]( /b/ ) and [ext](https://x.com/) and <a href='/c/'>c</a> <img src=\"/d.png\"> and [a again](/a/) and {{var}}/x"
	got := InternalLinks(text)
	want := []string{"/a/", "/b/", "/c/", "/d.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for in, want := range map[string]bool{"/x": true, "//cdn": false, "mailto:a@b": false, "#top": false, "/{{slug}}/": false, "": false} {
		if IsInternalLink(in) != want {
			t.Errorf("IsInternalLink(%q) != %v", in, want)
		}
	}
	for in, want := range map[string]bool{"/images/a.png": true, "https://x/a.png": false, "data:image/png;base64,x": false, "a.png": true} {
		if IsAssetValue(in) != want {
			t.Errorf("IsAssetValue(%q) != %v", in, want)
		}
	}
}

func TestEntryKeysAndTitle(t *testing.T) {
	cfg := &schema.Config{Collections: map[string]schema.Collection{"nav": {Fields: []schema.Field{{Key: "page", Type: "reference", By: "field:translationKey"}}}}}
	fm := map[string]any{"slug": map[string]any{"cs": "a", "en": "b"}, "translationKey": "home", "title": "Home"}
	keys := EntryKeys(cfg, fm)
	if keys["slug.cs"] != "a" || keys["slug.en"] != "b" || keys["translationKey"] != "home" {
		t.Fatalf("keys: %v", keys)
	}
	coll := schema.Collection{Preview: &schema.Preview{Title: []schema.Resolver{{Key: "seo.title"}, {Key: "title"}}}}
	if got := EntryTitle(coll, fm, "x/y.md"); got != "Home" {
		t.Errorf("title: %q", got)
	}
	if got := EntryTitle(schema.Collection{}, map[string]any{}, "x/y.md"); got != "y" {
		t.Errorf("fallback title: %q", got)
	}
}
