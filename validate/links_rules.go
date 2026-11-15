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
	// Routes is true when the collection derives URLs from the file.
	Routes bool
	doc    content.Document
	coll   schema.Collection
	hasCol bool
}

// Reference kinds.
const (
	refLink      = "link"
	refAsset     = "asset"
	refReference = "reference"
)

type rawRef struct {
	fieldPath string
	raw       string
	kind      string
	field     schema.Field
}

// Links checks the references between files: URL collisions, site links,
// assets, reference fields and orphaned sidecars. exists reports
// whether a repository path is a file; sidecars lists the sidecar files
// under `.norcube/cms/meta`.
func Links(cfg *schema.Config, entries []Entry, exists func(string) bool, sidecars []string) []report.Finding {
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

	refKeys := cfg.ReferenceKeys()
	_ = refKeys
	for _, e := range sorted {
		if !e.hasCol {
			for _, raw := range resolve.InternalLinks(e.doc.Body) {
				out = append(out, resolveRef(cfg, e, rawRef{fieldPath: "body", raw: raw, kind: refLink}, byURL, byPath, exists)...)
			}
			continue
		}
		var refs []rawRef
		refs = walkRefs(cfg, e.coll.Fields, e.doc.Frontmatter, "", refs)
		for _, raw := range resolve.InternalLinks(e.doc.Body) {
			refs = append(refs, rawRef{fieldPath: "body", raw: raw, kind: refLink})
		}
		for _, r := range refs {
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

func resolveRef(cfg *schema.Config, from Entry, r rawRef, byURL map[string]string, byPath map[string]Entry, exists func(string) bool) []report.Finding {
	has := func(p string) bool { return p != "" && exists != nil && exists(p) }
	switch r.kind {
	case refLink:
		n := resolve.NormalizeURLPath(r.raw)
		if _, ok := byURL[n]; ok {
			return nil
		}
		if parent := resolve.StripPagination(n); parent != "" {
			if _, ok := byURL[parent]; ok {
				return nil
			}
		}
		if has(resolve.AssetRepoPath(cfg, r.raw)) {
			return nil
		}
		if resolve.IsKnownPath(cfg, n) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.fieldPath, Severity: report.SeverityWarning, Rule: report.RuleLinksUnresolvedLink, Message: fmt.Sprintf("links to %s, which no content file produces", r.raw), Hint: "If the site serves this route from code, add it to knownPaths in config.yml; otherwise fix the link."}}
	case refAsset:
		p := resolve.AssetRepoPath(cfg, r.raw)
		if has(p) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.fieldPath, Severity: report.SeverityError, Rule: report.RuleLinksMissingAsset, Message: fmt.Sprintf("points at %s, which the repository does not have (looked for %s)", r.raw, p), Hint: "Pick the file again in the media library, or add the file."}}
	case refReference:
		if lookupEntry(byPath, r.field, r.raw) {
			return nil
		}
		return []report.Finding{{File: from.Path, Path: r.fieldPath, Severity: report.SeverityError, Rule: report.RuleLinksUnresolvedReference, Message: fmt.Sprintf("references %q, which names no entry", r.raw) + byHint(r.field), Hint: "Pick the entry again, or fix the value."}}
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

// walkRefs extracts every candidate reference from the frontmatter
// against the collection's fields, as the content index does.
func walkRefs(cfg *schema.Config, fields []schema.Field, obj map[string]any, prefix string, out []rawRef) []rawRef {
	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		val, present := obj[f.Key]
		if !present || val == nil {
			continue
		}
		p := joinPath(prefix, f.Key)
		switch schema.Canonical(f.Type) {
		case schema.TypeLink:
			if s, ok := val.(string); ok && resolve.IsInternalLink(s) {
				out = append(out, rawRef{fieldPath: p, raw: strings.TrimSpace(s), kind: refLink})
			}
		case schema.TypeImage, schema.TypeFile:
			if s, ok := val.(string); ok && resolve.IsAssetValue(s) {
				out = append(out, rawRef{fieldPath: p, raw: strings.TrimSpace(s), kind: refAsset})
			}
		case schema.TypeReference:
			switch v := val.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					out = append(out, rawRef{fieldPath: p, raw: strings.TrimSpace(v), kind: refReference, field: f})
				}
			case []any:
				for i, item := range v {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						out = append(out, rawRef{fieldPath: fmt.Sprintf("%s[%d]", p, i), raw: strings.TrimSpace(s), kind: refReference, field: f})
					}
				}
			}
		case schema.TypeRichText:
			if s, ok := val.(string); ok {
				for _, raw := range resolve.InternalLinks(s) {
					out = append(out, rawRef{fieldPath: p, raw: raw, kind: refLink})
				}
			}
		case schema.TypeGroup:
			if m, ok := val.(map[string]any); ok {
				out = walkRefs(cfg, f.Fields, m, p, out)
			}
		case schema.TypeArray:
			items, ok := val.([]any)
			if !ok {
				continue
			}
			for i, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				sub := schema.ItemFields(cfg, f, m)
				if sub == nil {
					continue
				}
				out = walkRefs(cfg, sub, m, fmt.Sprintf("%s[%d]", p, i), out)
			}
		}
	}
	return out
}
