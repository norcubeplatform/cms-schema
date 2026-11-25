package quality

import (
	"slices"
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
	// basis is how the value was made (report.Basis*); view the language
	// of a page that holds every language.
	basis, view string
}

// roleValue is what a page holds in one role field, read in the page's
// language.
type roleValue struct {
	// value is the text (a rich text description's excerpt), or "true"
	// for a boolean that is on.
	value string
	// raw is the value as the file holds it.
	raw any
	// path is where to fix it: `<field>.<locale>` in a localized group.
	path  string
	field resolve.RoleField
	// borrowed is true when the language is empty and the site shows the
	// fallback language's text: the fallback's own view measures it.
	borrowed bool
	// shared is true when the value is the same in every language of a
	// localized page, so only one view checks it.
	shared bool
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
	locale := p.localeOf()
	read := func(role string) (roleValue, bool) {
		field, ok := roles.Field(role)
		if !ok {
			return roleValue{}, false
		}
		rv := roleValue{path: field.Path, field: field, shared: p.view != nil && !field.Localized}
		if schema.PathHidden(field.Path, hidden) {
			return rv, true
		}
		rv.raw = resolve.ReadDotted(p.Frontmatter, field.Path)
		switch v := rv.raw.(type) {
		case string:
			rv.value = strings.TrimSpace(v)
		case bool:
			if v {
				rv.value = "true"
			}
		}
		if field.Localized {
			rv.path = field.Path + "." + locale
			rv.value, rv.borrowed = resolve.LocalizedValue(rv.raw, locale, field.Fallback)
		}
		if field.Excerpt == schema.ExcerptFirstParagraph && rv.value != "" {
			rv.value = resolve.FirstParagraph(rv.value, richTextIsHTML(r.valueField(coll, rv.path)))
		}
		return rv, true
	}

	if nx, ok := read(schema.RoleNoindex); ok && !schema.PathHidden(nx.field.Path, hidden) && roleOn(nx.field, nx.raw) {
		if p.once() && r.on(report.RuleQualityNoindex, coll) {
			cause, params := report.CauseFlag, &report.Params{Field: nx.field.Label}
			if len(nx.field.When) > 0 {
				cause, params.Value = report.CauseValue, nx.value
			}
			r.emit(report.Finding{File: p.Path, Path: nx.path, Rule: report.RuleQualityNoindex, Cause: cause, Params: params, Value: p.URL}, report.ImpactInfo)
		}
		return
	}
	canonicalElsewhere := false
	if c, ok := read(schema.RoleCanonical); ok && c.value != "" && resolve.NormalizeURLPath(stripOrigin(c.value)) != resolve.NormalizeURLPath(p.URL) {
		canonicalElsewhere = true
	}

	r.searchTitle(p, read, canonicalElsewhere)

	// Description: the SEO description field, when the collection has one.
	if d, ok := read(schema.RoleSEODescription); ok && (!d.shared || p.once()) {
		guessed := !d.field.Declared
		fromCode := d.value == "" && d.field.WhenEmpty != ""
		if fromCode {
			r.fillInCode(schema.RoleSEODescription, d.field.WhenEmpty)
		}
		if r.on(report.RuleQualityDescriptionMissing, coll) {
			r.count(report.RuleQualityDescriptionMissing)
			if d.value == "" && !fromCode {
				cause := report.CauseEmpty
				if d.field.Excerpt != "" {
					cause = report.CauseExcerptEmpty
				}
				r.emit(report.Finding{File: p.Path, Path: d.path, Rule: report.RuleQualityDescriptionMissing, Cause: cause},
					lowered(report.ImpactMedium, guessed))
			}
		}
		if d.value != "" && !d.borrowed {
			min, max := DefaultDescriptionMin, DefaultDescriptionMax
			basis := ""
			if d.field.Excerpt != "" {
				basis = report.BasisExcerpt
			} else {
				min, max = window(r.cfg, coll, d.path, true, min, max)
			}
			r.length(p, coll, report.RuleQualityDescriptionLength, "description", d.path, d.value, min, max, basis)
			if !canonicalElsewhere && r.on(report.RuleQualityDescriptionDuplicate, coll) {
				r.count(report.RuleQualityDescriptionDuplicate)
				r.descriptions = append(r.descriptions, dupCandidate{file: p.Path, path: d.path, locale: p.Locale, value: d.value, key: dupKey(d.value), guessed: guessed, basis: basis, view: localeOfPath(p, d.path)})
			}
		}
	}

	// Share image.
	if img, ok := read(schema.RoleShareImage); ok && (!img.shared || p.once()) {
		fromCode := img.value == "" && img.field.WhenEmpty != ""
		if fromCode {
			r.fillInCode(schema.RoleShareImage, img.field.WhenEmpty)
		}
		if r.on(report.RuleQualityShareImageMissing, coll) {
			r.count(report.RuleQualityShareImageMissing)
			if img.value == "" && !fromCode {
				r.emit(report.Finding{File: p.Path, Path: img.path, Rule: report.RuleQualityShareImageMissing},
					lowered(report.ImpactLow, !img.field.Declared))
			}
		}
	}
}

// searchTitle runs the title rules: the SEO title, else the page title.
// An empty SEO title the site fills in code (whenEmpty) is the site's,
// not the page title. A title template composes what is measured.
func (r *run) searchTitle(p Page, read func(string) (roleValue, bool), canonicalElsewhere bool) {
	coll := p.Collection
	seo, hasSEO := read(schema.RoleSEOTitle)
	if hasSEO && seo.shared && !p.once() {
		return
	}
	fallback, fallbackPath, hasFallback := pageTitle(r.cfg, coll, p.Frontmatter, seo.field.Path, p.localeOf())
	if !p.once() && hasFallback && !hasSEO && !inLocalized(coll, fallbackPath) {
		// A page title that is the same in every language is checked once.
		return
	}
	if hasSEO && seo.value == "" && seo.field.WhenEmpty != "" {
		r.fillInCode(schema.RoleSEOTitle, seo.field.WhenEmpty)
		if r.on(report.RuleQualityTitleMissing, coll) {
			r.count(report.RuleQualityTitleMissing)
		}
		return
	}
	if !hasSEO && !hasFallback {
		return
	}
	title, titlePath, guessed, borrowed := seo.value, seo.path, hasSEO && !seo.field.Declared, seo.borrowed
	fromSEO := title != ""
	if title == "" {
		title, titlePath, guessed, borrowed = fallback, fallbackPath, false, false
		if titlePath == "" {
			titlePath = seo.path
		}
	}
	if r.on(report.RuleQualityTitleMissing, coll) {
		r.count(report.RuleQualityTitleMissing)
		if title == "" {
			missingPath := seo.path
			if !hasSEO {
				missingPath = fallbackPath
			}
			r.emit(report.Finding{File: p.Path, Path: missingPath, Rule: report.RuleQualityTitleMissing},
				lowered(report.ImpactHigh, hasSEO && !seo.field.Declared))
		}
	}
	if title == "" || borrowed {
		return
	}
	measured, basis := title, ""
	min, max := DefaultTitleMin, DefaultTitleMax
	if fromSEO && seo.field.TitleTemplate != "" {
		composed, ok := resolve.ComposeTitle(seo.field.TitleTemplate, title, resolve.TitleContext{
			Fields: coll.Fields, Frontmatter: p.Frontmatter, Locale: p.localeOf(), DefaultLocale: r.cfg.Preview.DefaultLocale, Variables: r.variables,
		})
		if !ok {
			return
		}
		measured, basis = composed, report.BasisTitleTemplate
	} else {
		min, max = window(r.cfg, coll, titlePath, fromSEO, min, max)
	}
	r.length(p, coll, report.RuleQualityTitleLength, "title", titlePath, measured, min, max, basis)
	if !canonicalElsewhere && r.on(report.RuleQualityTitleDuplicate, coll) {
		r.count(report.RuleQualityTitleDuplicate)
		r.titles = append(r.titles, dupCandidate{file: p.Path, path: titlePath, locale: p.Locale, value: measured, key: dupKey(measured), guessed: guessed, basis: basis, view: localeOfPath(p, titlePath)})
	}
}

// inLocalized reports whether a page path is a language field of a
// localized group (`name.en`).
func inLocalized(coll schema.Collection, path string) bool {
	i := strings.LastIndex(path, ".")
	if i <= 0 {
		return false
	}
	f, ok := schema.FieldAtPath(coll.Fields, path[:i])
	return ok && f.IsLocalized()
}

// valueField is the definition of the field at a page path outside
// lists (a localized group's language field at `<group>.<locale>`).
func (r *run) valueField(coll schema.Collection, path string) schema.Field {
	f, _ := schema.FieldAtPath(coll.Fields, path)
	return f
}

// localeOfPath is the language a finding at path is about: the view's,
// when the path is a language field of a localized group; empty for a
// value that is the same in every language and on any other page.
func localeOfPath(p Page, path string) string {
	if p.view == nil || !strings.HasSuffix(path, "."+p.view.locale) {
		return ""
	}
	// Paths through a list (`gallery[0].caption.en`) are only made for
	// localized groups; others are checked against the schema.
	if !strings.Contains(path, "[") && !inLocalized(p.Collection, path) {
		return ""
	}
	return p.view.locale
}

// fillInCode counts a page whose role field is empty for the site to
// fill in code.
func (r *run) fillInCode(role, whenEmpty string) { r.filled[[2]string{role, whenEmpty}]++ }

// length checks a title or a description against its window. A value
// with a `{{placeholder}}` is not measured: its length on the page is not
// known.
func (r *run) length(p Page, coll schema.Collection, rule report.Rule, what, path, value string, min, max int, basis string) {
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
	cause := report.CauseTooLong
	if n < min {
		cause = report.CauseTooShort
	}
	params := &report.Params{Current: report.Num(float64(n)), Min: report.Num(float64(min)), Max: report.Num(float64(max))}
	r.emit(report.Finding{File: p.Path, Path: path, Rule: rule, Cause: cause, Params: params, Value: value, Measure: m, Basis: basis}, report.ImpactLow)
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
			params := &report.Params{Count: report.Int(int64(len(g))), Names: others}
			if rest := len(g) - 1 - len(others); rest > 0 {
				params.More = report.Int(int64(rest))
			}
			r.emit(report.Finding{File: c.file, Path: c.path, Rule: rule, Params: params,
				Value: c.value, Measure: &report.Measure{Value: int64(len(g)), Unit: report.UnitPages}, Basis: c.basis, Locale: c.view},
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
//
// On a page that holds every language, a slot naming one language of a
// localized group (`name.cs`) reads the page's language instead.
func pageTitle(cfg *schema.Config, coll schema.Collection, fm map[string]any, seoPath, locale string) (value, path string, has bool) {
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
			key := inLanguage(cfg, coll, slot.Key, locale)
			if key == "" || key == seoPath || slot.Key == seoPath {
				continue
			}
			if v, ok := consider(key); ok {
				return v, key, true
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

// inLanguage turns a path into a localized group (the group, or one of
// its languages) into the path of the locale's language field.
func inLanguage(cfg *schema.Config, coll schema.Collection, key, locale string) string {
	if key == "" || locale == "" || !coll.Localized() {
		return key
	}
	if f, ok := schema.FieldAtPath(coll.Fields, key); ok && f.IsLocalized() {
		return key + "." + locale
	}
	if i := strings.LastIndex(key, "."); i > 0 {
		if f, ok := schema.FieldAtPath(coll.Fields, key[:i]); ok && f.IsLocalized() && slices.Contains(cfg.Preview.Locales, key[i+1:]) {
			return key[:i] + "." + locale
		}
	}
	return key
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
