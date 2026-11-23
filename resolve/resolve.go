// Package resolve answers the questions a site's layout decides: which
// collection a file belongs to, what URL it renders at, what repository
// path an image value points to, and which values in a text are links
// into the site. It is the Go twin of the editor's resolvers and the
// code the content index runs on.
package resolve

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

// CollectionFor picks the collection a file belongs to: the `_collection`
// stamp when it names a real collection, else the collection whose
// `path` is the longest prefix of the file path, else `pages` when the
// site has it. Collections that share a path are told apart by `match`:
// a literal file name that equals the file's path under the directory
// wins, then a pattern the file matches, then the first by key.
func CollectionFor(cfg *schema.Config, filePath, explicit string) (schema.Collection, bool) {
	if explicit != "" {
		if coll, ok := cfg.Collections[explicit]; ok {
			return coll, true
		}
	}
	var best schema.Collection
	bestLen, bestRank, found := -1, -1, false
	for _, key := range cfg.CollectionKeys {
		coll := cfg.Collections[key]
		if coll.Path == "" || !IsUnder(filePath, coll.Path) {
			continue
		}
		rank := MatchRank(coll, filePath)
		if len(coll.Path) > bestLen || (len(coll.Path) == bestLen && rank > bestRank) {
			best, bestLen, bestRank, found = coll, len(coll.Path), rank, true
		}
	}
	if found {
		return best, true
	}
	if coll, ok := cfg.Collections[schema.DefaultCollection]; ok {
		return coll, true
	}
	return schema.Collection{}, false
}

// MatchRank says how specifically a collection's `match` claims a file
// under its path: 2 for a literal file name that equals it, 1 for a
// pattern it matches, 0 otherwise.
func MatchRank(coll schema.Collection, filePath string) int {
	rel := strings.Trim(filePath, "/")
	if d := strings.Trim(coll.Path, "/"); d != "" && strings.HasPrefix(rel, d+"/") {
		rel = rel[len(d)+1:]
	}
	m := strings.TrimSpace(coll.Match)
	if m == "" {
		m = "**/*.md"
	}
	if IsLiteralMatch(m) {
		if rel == strings.TrimPrefix(m, "./") {
			return 2
		}
		return 0
	}
	if GlobMatch(m, rel) {
		return 1
	}
	return 0
}

// IsLiteralMatch reports whether a `match` names one file rather than a
// pattern.
func IsLiteralMatch(m string) bool {
	return m != "" && !strings.ContainsAny(m, "*?[{")
}

// GlobMatch matches the patterns content loaders use: `*` within one
// segment, `**/` for any number of segments, `{a,b}` alternatives in the
// extension.
func GlobMatch(pattern, rel string) bool {
	for _, alt := range expandBraces(pattern) {
		if globSegments(strings.Split(alt, "/"), strings.Split(rel, "/")) {
			return true
		}
	}
	return false
}

func expandBraces(p string) []string {
	open := strings.Index(p, "{")
	if open < 0 {
		return []string{p}
	}
	close := strings.Index(p[open:], "}")
	if close < 0 {
		return []string{p}
	}
	close += open
	var out []string
	for _, alt := range strings.Split(p[open+1:close], ",") {
		out = append(out, expandBraces(p[:open]+alt+p[close+1:])...)
	}
	return out
}

func globSegments(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globSegments(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
		return false
	}
	return globSegments(pat[1:], segs[1:])
}

// IsUnder reports whether filePath sits inside dir at any depth.
func IsUnder(filePath, dir string) bool {
	f := strings.Trim(filePath, "/")
	d := strings.Trim(dir, "/")
	if d == "" {
		return false
	}
	return f == d || strings.HasPrefix(f, d+"/")
}

// ContentRoots lists the directories content files live in: the pages
// directory and every collection path, sorted. Empty when the config
// declares none, which means the whole repository.
func ContentRoots(cfg *schema.Config) []string {
	seen := map[string]bool{}
	var roots []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			roots = append(roots, p)
		}
	}
	add(cfg.PagesDir)
	for _, key := range cfg.CollectionKeys {
		add(cfg.Collections[key].Path)
	}
	sortStrings(roots)
	return roots
}

// FileID is the content loader's id for a flat collection: the base
// name without extension.
func FileID(filePath string) string {
	base := path.Base(filePath)
	return strings.TrimSuffix(base, path.Ext(base))
}

// LocaleFromFolder is the file's locale when it sits in a per-locale
// folder directly under the collection root and that folder is a
// declared locale (or any folder when no locales are declared).
func LocaleFromFolder(filePath, collPath string, locales []string) string {
	if collPath == "" {
		return ""
	}
	f := strings.Trim(filePath, "/")
	d := strings.Trim(collPath, "/")
	if d == "" || !strings.HasPrefix(f, d+"/") {
		return ""
	}
	seg := strings.SplitN(f[len(d)+1:], "/", 2)[0]
	if seg == "" {
		return ""
	}
	if len(locales) == 0 {
		return seg
	}
	for _, l := range locales {
		if l == seg {
			return seg
		}
	}
	return ""
}

// Locale is the locale of a file per its collection's `localeFrom` rule,
// falling back to the site default.
func Locale(filePath string, coll schema.Collection, site schema.PreviewSite, fm map[string]any) string {
	locale := site.DefaultLocale
	switch {
	case coll.LocaleFrom == schema.LocaleFromPath:
		if l := LocaleFromFolder(filePath, coll.Path, site.Locales); l != "" {
			locale = l
		}
	case strings.HasPrefix(coll.LocaleFrom, schema.LocaleFromFieldPrefix):
		if v, ok := fm[strings.TrimPrefix(coll.LocaleFrom, schema.LocaleFromFieldPrefix)].(string); ok && v != "" {
			locale = v
		}
	}
	return locale
}

// TranslationKey is what a file shares with its other language versions,
// per its collection's `translations.by`; "" when the collection declares
// no translations or the file has no key. Files of one collection with
// the same key are versions of each other, one per locale.
//
//   - `path`: the file's path under the collection, without its locale
//     folder and its extension, so `cs/pricing.md` and `en/pricing.md`
//     are one page. A file of the default locale that sits outside any
//     locale folder has the same key as its versions inside one.
//   - `field:<key>`: the value of that top-level field.
func TranslationKey(filePath string, coll schema.Collection, site schema.PreviewSite, fm map[string]any) string {
	if coll.Translations == nil {
		return ""
	}
	if field, ok := coll.Translations.Field(); ok {
		v, _ := fm[field].(string)
		return strings.TrimSpace(v)
	}
	if coll.Translations.By != schema.TranslationsByPath {
		return ""
	}
	f := strings.Trim(filePath, "/")
	d := strings.Trim(coll.Path, "/")
	if d == "" || !strings.HasPrefix(f, d+"/") {
		return ""
	}
	rel := f[len(d)+1:]
	if l := LocaleFromFolder(filePath, coll.Path, site.Locales); l != "" && len(site.Locales) > 0 {
		rel = strings.TrimPrefix(rel, l+"/")
	}
	return strings.TrimSuffix(rel, path.Ext(rel))
}

// SlugFallback is the slug a file gets when its frontmatter declares
// none: its path relative to the locale folder (or the collection
// root), without the extension, an `index` file standing for its
// folder. That is the id Astro's content loader gives the file. A file
// outside the collection root keeps the bare file name.
func SlugFallback(filePath, collPath string, locales []string, stripLocale bool) string {
	f := strings.Trim(filePath, "/")
	d := strings.Trim(collPath, "/")
	rel := path.Base(f)
	if d != "" && strings.HasPrefix(f, d+"/") {
		rel = f[len(d)+1:]
		if stripLocale {
			if l := LocaleFromFolder(filePath, collPath, locales); l != "" {
				rel = strings.TrimPrefix(rel, l+"/")
			}
		}
	}
	rel = strings.TrimSuffix(rel, path.Ext(rel))
	if rel == "index" {
		return ""
	}
	return strings.TrimSuffix(rel, "/index")
}

// Slug reads the `slug` value: a string as-is (empty means home), a
// localized map by locale, else the fallback.
func Slug(slug any, locale, fallback string) string {
	switch v := slug.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if s, ok := v[locale].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		return fallback
	}
	return fallback
}

var (
	fieldsTokenRe = regexp.MustCompile(`\{fields\.([^}]+)\}`)
	anyTokenRe    = regexp.MustCompile(`\{([^}]*)\}`)
	multiSlashRe  = regexp.MustCompile(`/{2,}`)
	paginationRe  = regexp.MustCompile(`/[0-9]+$`)
)

// URL returns the path the site renders a content file at, or "" when
// the collection declares no previewPath.
func URL(filePath string, coll schema.Collection, site schema.PreviewSite, fm map[string]any) string {
	if filePath == "" || coll.PreviewPath == "" {
		return ""
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return urlFor(filePath, coll, site, fm, Locale(filePath, coll, site, fm))
}

func urlFor(filePath string, coll schema.Collection, site schema.PreviewSite, fm map[string]any, locale string) string {
	localeSeg := ""
	if locale != "" && !(locale == site.DefaultLocale && !site.PrefixDefaultLocale) {
		localeSeg = locale
	}
	slug := Slug(fm["slug"], locale, SlugFallback(filePath, coll.Path, site.Locales, coll.LocaleFrom == schema.LocaleFromPath))
	out := strings.ReplaceAll(coll.PreviewPath, "{locale}", localeSeg)
	out = strings.ReplaceAll(out, "{slug}", slug)
	out = fieldsTokenRe.ReplaceAllStringFunc(out, func(tok string) string {
		key := fieldsTokenRe.FindStringSubmatch(tok)[1]
		switch v := ReadDotted(fm, key).(type) {
		case nil:
			return ""
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return fmt.Sprint(v)
		}
	})
	out = stripOrigin(out)
	out = multiSlashRe.ReplaceAllString(out, "/")
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}

// stripOrigin reduces a full URL a `{fields.<key>}` value may hold
// (`https://example.com/about/`) to its path, so a site that stores
// addresses with the host compares like one that stores paths.
func stripOrigin(u string) string {
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return u
	}
	rest := u[strings.Index(u, "://")+3:]
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[i:]
	}
	return "/"
}

// TemplateRoutes reports whether the collection derives URLs from the
// file (its previewPath has tokens) rather than naming one constant
// preview page.
func TemplateRoutes(coll schema.Collection) bool {
	return strings.Contains(coll.PreviewPath, "{")
}

// PreviewTokens returns the tokens a previewPath uses that the resolver
// does not know.
func PreviewTokens(previewPath string) (unknown []string) {
	for _, m := range anyTokenRe.FindAllStringSubmatch(previewPath, -1) {
		tok := m[1]
		if tok == "locale" || tok == "slug" || strings.HasPrefix(tok, "fields.") {
			continue
		}
		unknown = append(unknown, "{"+tok+"}")
	}
	return unknown
}

// NormalizeURLPath makes two spellings of the same page comparable: no
// query or fragment, no repeated slashes, no trailing slash except for
// the root.
func NormalizeURLPath(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = multiSlashRe.ReplaceAllString(u, "/")
	if len(u) > 1 {
		u = strings.TrimRight(u, "/")
	}
	if u == "" {
		u = "/"
	}
	return u
}

// StripPagination maps a paginated path (`/blog/2`) to the page that
// produces it (`/blog`). Returns "" when there is no page-number suffix.
// Expects a normalized path.
func StripPagination(n string) string {
	if !paginationRe.MatchString(n) {
		return ""
	}
	parent := paginationRe.ReplaceAllString(n, "")
	if parent == "" {
		return "/"
	}
	return parent
}

// IsKnownPath reports whether a site path is declared in `knownPaths`:
// an exact match (trailing slash ignored) or a `prefix*` entry.
func IsKnownPath(cfg *schema.Config, urlPath string) bool {
	n := NormalizeURLPath(urlPath)
	for _, kp := range cfg.KnownPaths {
		if strings.HasSuffix(kp, "*") {
			prefix := NormalizeURLPath(strings.TrimSuffix(kp, "*"))
			if prefix == "/" || n == prefix || strings.HasPrefix(n, prefix+"/") || strings.HasPrefix(n, prefix) {
				return true
			}
			continue
		}
		if NormalizeURLPath(kp) == n {
			return true
		}
	}
	return false
}

// ReadDotted reads a (possibly dotted) key out of a map.
func ReadDotted(obj map[string]any, key string) any {
	if !strings.Contains(key, ".") {
		return obj[key]
	}
	var cur any = obj
	for _, seg := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

// EntryTitle resolves a collection's preview title slots against the
// frontmatter, falling back to a `title` field and then the file name.
func EntryTitle(coll schema.Collection, fm map[string]any, filePath string) string {
	if coll.Preview != nil {
		for _, slot := range coll.Preview.Title {
			if slot.Literal != "" {
				return slot.Literal
			}
			if v, ok := ReadDotted(fm, slot.Key).(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if v, ok := fm["title"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return FileID(filePath)
}

// EntryKeys collects what reference fields can look an entry up by:
// `slug` (a localized map flattens to `slug.<locale>`) and the site's
// reference key fields.
func EntryKeys(cfg *schema.Config, fm map[string]any) map[string]string {
	keys := map[string]string{}
	switch slug := fm["slug"].(type) {
	case string:
		keys["slug"] = strings.TrimSpace(slug)
	case map[string]any:
		for loc, v := range slug {
			if s, ok := v.(string); ok {
				keys["slug."+loc] = strings.TrimSpace(s)
			}
		}
	}
	for _, k := range cfg.ReferenceKeys() {
		if v, ok := ReadDotted(fm, k).(string); ok && v != "" {
			keys[k] = v
		}
	}
	return keys
}

// CleanPath trims surrounding whitespace and slashes.
func CleanPath(p string) string {
	return strings.Trim(strings.TrimSpace(p), "/")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
