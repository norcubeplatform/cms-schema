package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Entry is what the links checks know about one content file: the
// shape the content index stores.
type Entry struct {
	Path       string
	Collection string
	URL        string
	Locale     string
	Title      string
	Keys       map[string]string
	// TranslationKey is what the file shares with its other language
	// versions (resolve.TranslationKey); "" when it has none.
	TranslationKey string
	// Routes is true when the collection derives URLs from the file.
	Routes bool
	doc    content.Document
	coll   schema.Collection
	hasCol bool
}

// Links checks the references between files: URL collisions, site links,
// assets, reference fields and orphaned sidecars. exists reports
// whether a repository path is a file; sidecars lists the sidecar files
// under `.norcube/cms/meta`. It never panics: a bug in a check comes
// back as one `links.internal_error` finding.
func Links(cfg *schema.Config, entries []Entry, exists func(string) bool, sidecars []string) (out []report.Finding) {
	defer recoverAs(&out, "", report.RuleLinksInternalError)
	return checkLinks(cfg, entries, exists, sidecars)
}

func checkLinks(cfg *schema.Config, entries []Entry, exists func(string) bool, sidecars []string) []report.Finding {
	var out []report.Finding
	byPath := map[string]Entry{}
	byURL := map[string]string{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	// URL owners: an entry whose template routes wins over a constant
	// preview page; ties keep the first by path order.
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for _, e := range sorted {
		if e.URL == "" {
			continue
		}
		n := resolve.NormalizeURLPath(e.URL)
		if prev, taken := byURL[n]; taken {
			prevE := byPath[prev]
			if prevE.Routes && e.Routes {
				out = append(out, report.Finding{File: e.Path, Severity: report.SeverityError, Rule: report.RuleLinksURLCollision, Message: fmt.Sprintf("resolves to %s, the same URL as %s", e.URL, prev), Hint: "Two files cannot render at one address. Change a slug, move a file, or fix the collection's previewPath."})
			}
			if prevE.Routes || !e.Routes {
				continue
			}
		}
		byURL[n] = e.Path
	}

	// One page per language: two files of a collection and a locale with
	// one translation key both claim to be that page in that language.
	versions := map[string]string{}
	for _, e := range sorted {
		if e.TranslationKey == "" {
			continue
		}
		id := e.Collection + "\x00" + e.Locale + "\x00" + e.TranslationKey
		if prev, taken := versions[id]; taken {
			out = append(out, report.Finding{File: e.Path, Severity: report.SeverityError, Rule: report.RuleLinksTranslationCollision, Message: fmt.Sprintf("has the translation key %q in %q, the same as %s", e.TranslationKey, e.Locale, prev), Hint: "A page has one version per language. Give one of the two files its own key, or move it to the language it is written in."})
			continue
		}
		versions[id] = e.Path
	}

	for _, e := range sorted {
		var fields []schema.Field
		if e.hasCol {
			fields = e.coll.Fields
		}
		for _, r := range resolve.References(cfg, fields, e.doc.Frontmatter, e.doc.Body) {
			out = append(out, resolveRef(cfg, e, r, byURL, byPath, exists)...)
		}
	}

	for _, sc := range sidecars {
		found := false
		for _, p := range content.ContentPathsForSidecar(sc) {
			if _, ok := byPath[p]; ok || (exists != nil && exists(p)) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, report.Finding{File: sc, Severity: report.SeverityWarning, Rule: report.RuleLinksOrphanSidecar, Message: "the sidecar belongs to a content file that no longer exists", Hint: "Delete it, or restore the content file; the CMS recovers a renamed file's sidecar through the page's inline _id."})
		}
	}
	return out
}

func resolveRef(cfg *schema.Config, from Entry, r resolve.Reference, byURL map[string]string, byPath map[string]Entry, exists func(string) bool) []report.Finding {
	has := func(p string) bool { return p != "" && exists != nil && exists(p) }
	switch r.Kind {
	case resolve.RefKindLink:
		n := resolve.NormalizeURLPath(r.Value)
		if _, ok := byURL[n]; ok {
			return nil
		}
		if parent := resolve.StripPagination(n); parent != "" {
			if _, ok := byURL[parent]; ok {
				return nil
			}
		}
		if has(resolve.AssetRepoPath(cfg, r.Value)) {
			return nil
		}
		if resolve.IsKnownPath(cfg, n) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.Path, Severity: report.SeverityWarning, Rule: report.RuleLinksUnresolvedLink, Message: fmt.Sprintf("links to %s, which no content file produces", r.Value), Hint: "If the site serves this route from code, add it to knownPaths in config.yml; otherwise fix the link."}}
	case resolve.RefKindAsset:
		p := resolve.AssetRepoPath(cfg, r.Value)
		if has(p) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.Path, Severity: report.SeverityError, Rule: report.RuleLinksMissingAsset, Message: fmt.Sprintf("points at %s, which the repository does not have (looked for %s)", r.Value, p), Hint: "Pick the file again in the media library, or add the file."}}
	case resolve.RefKindReference:
		if lookupEntry(byPath, r.Field, r.Value) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.Path, Severity: report.SeverityError, Rule: report.RuleLinksUnresolvedReference, Message: fmt.Sprintf("references %q, which names no entry", r.Value) + byHint(r.Field), Hint: "Pick the entry again, or fix the value."}}
	}
	return nil
}

func byHint(f schema.Field) string {
	if strings.HasPrefix(f.By, schema.ReferenceByFieldPrefix) {
		return fmt.Sprintf(" by %s", f.By)
	}
	if f.Collection != "" {
		return " in collection " + f.Collection
	}
	return ""
}

func lookupEntry(byPath map[string]Entry, f schema.Field, value string) bool {
	if strings.HasPrefix(f.By, schema.ReferenceByFieldPrefix) {
		key := strings.TrimPrefix(f.By, schema.ReferenceByFieldPrefix)
		for _, e := range byPath {
			if f.Collection != "" && e.Collection != f.Collection {
				continue
			}
			if e.Keys[key] == value {
				return true
			}
		}
		return false
	}
	_, ok := byPath[resolve.CleanPath(value)]
	return ok
}
