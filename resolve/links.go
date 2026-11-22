package resolve

import (
	"regexp"
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

var (
	markdownLinkRe = regexp.MustCompile(`\]\(\s*(/[^)\s"']*)`)
	htmlHrefRe     = regexp.MustCompile(`(?i)\b(?:href|src)=["'](/[^"']*)["']`)
)

// IsInternalLink reports whether a link value points into this site: a
// root-relative path. External URLs, protocol-relative URLs, anchors,
// mail and phone links and `{{variable}}` placeholders are not.
func IsInternalLink(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") {
		return false
	}
	return !strings.Contains(v, "{{")
}

// IsAssetValue reports whether an image or file value is something the
// repository can contain, as opposed to an external or data URL.
func IsAssetValue(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.Contains(v, "{{") {
		return false
	}
	lower := strings.ToLower(v)
	return !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "data:") && !strings.HasPrefix(lower, "//")
}

// InternalLinks returns the root-relative targets of Markdown links and
// images and of href/src attributes in a text, each once, in order of
// first appearance.
func InternalLinks(text string) []string {
	if !strings.Contains(text, "](/") && !strings.Contains(text, "=\"/") && !strings.Contains(text, "='/") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if !IsInternalLink(raw) || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}
	for _, m := range markdownLinkRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, m := range htmlHrefRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	return out
}

// Kinds of Reference.
const (
	// RefKindLink is a root-relative link: a link field, or a link or
	// image in a rich text field or the body.
	RefKindLink = "link"
	// RefKindAsset is an image or file field's value that the repository
	// can hold.
	RefKindAsset = "asset"
	// RefKindReference is a reference field's value.
	RefKindReference = "reference"
)

// Reference is one value of a page that points at something else in
// the site, as the content index stores it.
type Reference struct {
	// Path is where the value sits (`blocks[0].image`), or BodyPath.
	Path  string
	Value string
	Kind  string
	// Field is the field holding the value; zero for the body.
	Field schema.Field
}

// References lists what a page points at: link, image and file fields,
// reference fields (each value of a multiple one), links in rich text,
// then links in the body. Items of a list of primitives are not read,
// as the content index does not read them. fields may be nil for a file
// no collection claims; its body is still read.
func References(cfg *schema.Config, fields []schema.Field, frontmatter map[string]any, body string) []Reference {
	var out []Reference
	schema.Walk(cfg, fields, frontmatter, func(v schema.Visit) bool {
		if v.Item || v.Value == nil {
			return true
		}
		f, p := v.Field, v.Path
		switch schema.Canonical(f.Type) {
		case schema.TypeLink:
			if s, ok := v.Value.(string); ok && IsInternalLink(s) {
				out = append(out, Reference{Path: p, Value: strings.TrimSpace(s), Kind: RefKindLink, Field: f})
			}
		case schema.TypeImage, schema.TypeFile:
			if s, ok := v.Value.(string); ok && IsAssetValue(s) {
				out = append(out, Reference{Path: p, Value: strings.TrimSpace(s), Kind: RefKindAsset, Field: f})
			}
		case schema.TypeReference:
			switch val := v.Value.(type) {
			case string:
				if strings.TrimSpace(val) != "" {
					out = append(out, Reference{Path: p, Value: strings.TrimSpace(val), Kind: RefKindReference, Field: f})
				}
			case []any:
				for i, item := range val {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						out = append(out, Reference{Path: schema.IndexPath(p, i), Value: strings.TrimSpace(s), Kind: RefKindReference, Field: f})
					}
				}
			}
		case schema.TypeRichText:
			if s, ok := v.Value.(string); ok {
				for _, raw := range InternalLinks(s) {
					out = append(out, Reference{Path: p, Value: raw, Kind: RefKindLink, Field: f})
				}
			}
		}
		return true
	})
	for _, raw := range InternalLinks(body) {
		out = append(out, Reference{Path: BodyPath, Value: raw, Kind: RefKindLink})
	}
	return out
}
