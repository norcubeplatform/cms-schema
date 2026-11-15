package resolve

import (
	"regexp"
	"strings"
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
