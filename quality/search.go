package quality

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// dupCandidate is one indexable page's title or description.
type dupCandidate struct {
	file, path, locale, value, key string
	guessed                        bool
}

// search runs the search rules on one page: only a page of its own (it
// has a URL), of a collection that holds pages, and not hidden from
// search engines.
func (r *run) search(p Page, hidden map[string]bool) {
	coll := p.Collection
	if p.URL == "" || coll.SiteArea() {
		return
	}
	roles := r.rolesOf(coll)
	read := func(role string) (value, path string, field resolve.RoleField, ok bool) {
		field, ok = roles.Field(role)
		if !ok {
			return "", "", field, false
		}
		if schema.PathHidden(field.Path, hidden) {
			return "", field.Path, field, true
		}
		v := resolve.ReadDotted(p.Frontmatter, field.Path)
		s, _ := v.(string)
		if b, isBool := v.(bool); isBool && b {
			s = "true"
		}
		return strings.TrimSpace(s), field.Path, field, true
	}

	if v, path, field, ok := read(schema.RoleNoindex); ok && v == "true" {
		if r.on(report.RuleQualityNoindex, coll) {
			r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityNoindex,
				Message: "The page is hidden from search engines.", Hint: "That is fine for a thank-you page or a draft; turn off " + labelOr(field.Label, "the no-index field") + " if people should find it in search.",
				Value: p.URL}, report.ImpactInfo)
		}
		return
	}
	canonicalElsewhere := false
	if v, _, _, ok := read(schema.RoleCanonical); ok && v != "" && resolve.NormalizeURLPath(stripOrigin(v)) != resolve.NormalizeURLPath(p.URL) {
		canonicalElsewhere = true
	}

	// Title: the SEO title, else the page title. An empty SEO title the
	// site fills in code (whenEmpty) is the site's, not the page title.
	seoTitle, seoPath, seoField, hasSEO := read(schema.RoleSEOTitle)
	fallback, fallbackPath, hasFallback := pageTitle(coll, p.Frontmatter, seoPath)
	if hasSEO && seoTitle == "" && seoField.WhenEmpty != "" {
		r.fillInCode(schema.RoleSEOTitle, seoField.WhenEmpty)
		if r.on(report.RuleQualityTitleMissing, coll) {
			r.count(report.RuleQualityTitleMissing)
		}
	} else if hasSEO || hasFallback {
		title, titlePath, guessed := seoTitle, seoPath, hasSEO && !seoField.Declared
		if title == "" {
			title, titlePath, guessed = fallback, fallbackPath, false
			if titlePath == "" {
				titlePath = seoPath
			}
		}
		if r.on(report.RuleQualityTitleMissing, coll) {
			r.count(report.RuleQualityTitleMissing)
			if title == "" {
				missingPath := seoPath
				if !hasSEO {
					missingPath = fallbackPath
				}
				r.emit(report.Finding{File: p.Path, Path: missingPath, Rule: report.RuleQualityTitleMissing,
					Message: "The page has no title for search results.", Hint: "Write a title that says what the page is about, in a few words."},
					lowered(report.ImpactHigh, hasSEO && !seoField.Declared))
			}
		}
		if title != "" {
			min, max := window(r.cfg, coll, seoPath, hasSEO && titlePath == seoPath, DefaultTitleMin, DefaultTitleMax)
			r.length(p, coll, report.RuleQualityTitleLength, "title", titlePath, title, min, max)
			if !canonicalElsewhere && r.on(report.RuleQualityTitleDuplicate, coll) {
				r.count(report.RuleQualityTitleDuplicate)
				r.titles = append(r.titles, dupCandidate{file: p.Path, path: titlePath, locale: p.Locale, value: title, key: dupKey(title), guessed: guessed})
			}
		}
	}

	// Description: the SEO description field, when the collection has one.
	if desc, path, field, ok := read(schema.RoleSEODescription); ok {
		guessed := !field.Declared
		fromCode := desc == "" && field.WhenEmpty != ""
		if fromCode {
			r.fillInCode(schema.RoleSEODescription, field.WhenEmpty)
		}
		if r.on(report.RuleQualityDescriptionMissing, coll) {
			r.count(report.RuleQualityDescriptionMissing)
			if desc == "" && !fromCode {
				r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityDescriptionMissing,
					Message: "The page has no meta description.", Hint: "Search engines then pick a snippet from the page. Write one or two sentences that make someone want to open it. If the site writes descriptions in code, say so in the schema with `whenEmpty` on the field."},
					lowered(report.ImpactMedium, guessed))
			}
		}
		if desc != "" {
			min, max := window(r.cfg, coll, path, true, DefaultDescriptionMin, DefaultDescriptionMax)
			r.length(p, coll, report.RuleQualityDescriptionLength, "description", path, desc, min, max)
			if !canonicalElsewhere && r.on(report.RuleQualityDescriptionDuplicate, coll) {
				r.count(report.RuleQualityDescriptionDuplicate)
				r.descriptions = append(r.descriptions, dupCandidate{file: p.Path, path: path, locale: p.Locale, value: desc, key: dupKey(desc), guessed: guessed})
			}
		}
	}

	// Share image.
	if img, path, field, ok := read(schema.RoleShareImage); ok {
		fromCode := img == "" && field.WhenEmpty != ""
		if fromCode {
			r.fillInCode(schema.RoleShareImage, field.WhenEmpty)
		}
		if r.on(report.RuleQualityShareImageMissing, coll) {
			r.count(report.RuleQualityShareImageMissing)
			if img == "" && !fromCode {
				r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityShareImageMissing,
					Message: "The page has no share image.", Hint: "A link to it shared on social networks or in chat shows no picture, unless the site adds one in code. Pick an image of at least 1200 x 630 pixels, or, if the site makes these images itself, say so in the schema with `whenEmpty: generated` on the field."},
					lowered(report.ImpactLow, !field.Declared))
			}
		}
	}
}

// fillInCode counts a page whose role field is empty for the site to
// fill in code.
func (r *run) fillInCode(role, whenEmpty string) { r.filled[[2]string{role, whenEmpty}]++ }

// length checks a title or a description against its window. A value
// with a `{{placeholder}}` is not measured: its length on the page is not
// known.
func (r *run) length(p Page, coll schema.Collection, rule report.Rule, what, path, value string, min, max int) {
	if !r.on(rule, coll) || strings.Contains(value, "{{") {
		return
	}
	r.count(rule)
	n := utf8.RuneCountInString(value)
	if n >= min && n <= max {
		return
	}
	lo, hi := int64(min), int64(max)
	m := &report.Measure{Value: int64(n), Unit: report.UnitCharacters, Min: &lo, Max: &hi}
	msg, hint := "", ""
	if n < min {
		msg = fmt.Sprintf("The %s has %d characters, fewer than the recommended %d to %d.", what, n, min, max)
		hint = fmt.Sprintf("A longer %s says more about the page in search results.", what)
	} else {
		msg = fmt.Sprintf("The %s has %d characters, more than the recommended %d to %d.", what, n, min, max)
		hint = fmt.Sprintf("Search results cut a long %s off; put what matters first or shorten it.", what)
	}
	r.emit(report.Finding{File: p.Path, Path: path, Rule: rule, Message: msg, Hint: hint, Value: value, Measure: m}, report.ImpactLow)
}

// duplicates reports the titles and the descriptions shared by pages of
// one language.
func (r *run) duplicates() {
	r.dupes(r.titles, report.RuleQualityTitleDuplicate, "title")
	r.dupes(r.descriptions, report.RuleQualityDescriptionDuplicate, "description")
}

func (r *run) dupes(cands []dupCandidate, rule report.Rule, what string) {
	groups := map[string][]dupCandidate{}
	for _, c := range cands {
		k := c.locale + "\x00" + c.key
		groups[k] = append(groups[k], c)
	}
	for _, k := range sortedKeys(groups) {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		for _, c := range g {
			var others []string
			for _, o := range g {
				if o.file != c.file && len(others) < MaxNamed {
					others = append(others, o.file)
				}
			}
			more := ""
			if rest := len(g) - 1 - len(others); rest > 0 {
				more = fmt.Sprintf(" and %d more", rest)
			}
			r.emit(report.Finding{File: c.file, Path: c.path, Rule: rule,
				Message: fmt.Sprintf("%d pages share this %s: %s%s.", len(g), what, strings.Join(others, ", "), more),
				Hint:    fmt.Sprintf("Give each page a %s of its own, so people can tell them apart in search results.", what),
				Value:   c.value, Measure: &report.Measure{Value: int64(len(g)), Unit: report.UnitPages}},
				lowered(report.ImpactMedium, c.guessed))
		}
	}
}

// pageTitle is the title a page falls back to when its SEO title is
// empty: the first of the collection's preview title slots that has a
// value (other than the SEO title itself), else a top-level `title`.
// path is where the value is, or, when there is none, the first place a
// title could be written. has is false when the collection names no such
// field, so a missing title cannot be told.
func pageTitle(coll schema.Collection, fm map[string]any, seoPath string) (value, path string, has bool) {
	first := ""
	consider := func(key string) (string, bool) {
		has = true
		if first == "" {
			first = key
		}
		v, ok := resolve.ReadDotted(fm, key).(string)
		return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
	}
	if coll.Preview != nil {
		for _, slot := range coll.Preview.Title {
			if slot.Literal != "" {
				return strings.TrimSpace(slot.Literal), "", true
			}
			if slot.Key == "" || slot.Key == seoPath {
				continue
			}
			if v, ok := consider(slot.Key); ok {
				return v, slot.Key, true
			}
		}
	}
	for _, f := range coll.Fields {
		if f.Key == "title" && resolve.TranslatableType(f.Type) {
			if v, ok := consider("title"); ok {
				return v, "title", true
			}
		}
	}
	return "", first, has
}

// window is the length window of a role field: its recommendedLength
// where the schema sets one (either end alone), else the defaults.
// fromField is false when the value checked is not the field's own (the
// page title a missing SEO title falls back to).
func window(cfg *schema.Config, coll schema.Collection, path string, fromField bool, min, max int) (int, int) {
	if !fromField {
		return min, max
	}
	if f, ok := fieldAt(cfg, coll.Fields, path); ok && f.RecommendedLength != nil {
		if f.RecommendedLength.From != nil && *f.RecommendedLength.From >= 0 {
			min = *f.RecommendedLength.From
		}
		if f.RecommendedLength.To != nil && *f.RecommendedLength.To > 0 {
			max = *f.RecommendedLength.To
		}
	}
	return min, max
}

// fieldAt finds the field at a dotted path outside lists.
func fieldAt(cfg *schema.Config, fields []schema.Field, path string) (schema.Field, bool) {
	parts := strings.Split(path, ".")
	for i, key := range parts {
		var found *schema.Field
		for j := range fields {
			if fields[j].Key == key {
				found = &fields[j]
				break
			}
		}
		if found == nil {
			return schema.Field{}, false
		}
		if i == len(parts)-1 {
			return *found, true
		}
		if !schema.IsGroup(found.Type) {
			return schema.Field{}, false
		}
		fields = found.Fields
	}
	return schema.Field{}, false
}

// dupKey is what two titles or descriptions share when they are the
// same: the text with its white space collapsed, in lowercase.
func dupKey(s string) string { return strings.ToLower(collapse(s)) }

func labelOr(label, fallback string) string {
	if strings.TrimSpace(label) == "" {
		return fallback
	}
	return "“" + label + "”"
}

// stripOrigin turns an absolute address into its path.
func stripOrigin(s string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(s), scheme) {
			rest := s[len(scheme):]
			if i := strings.Index(rest, "/"); i >= 0 {
				return rest[i:]
			}
			return "/"
		}
	}
	return s
}
