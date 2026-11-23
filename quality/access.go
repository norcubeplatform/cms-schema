package quality

import (
	"fmt"
	"strings"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// linkLabelKeys are the keys of the text field beside a link field that
// holds what the link says (a button's label), in normalized form.
var linkLabelKeys = map[string]bool{"label": true, "text": true, "linktext": true, "buttontext": true, "buttonlabel": true, "cta": true, "ctatext": true, "title": true}

// accessibility runs the accessibility rules on one page: every image
// value, every rich text field, the link fields that carry a label, and
// the body. Fields the page hides are skipped. On one language of a page
// that holds every language, a localized group is read in that language
// and what is the same in every language is checked on one view only.
func (r *run) accessibility(p Page) {
	coll := p.Collection
	locale := p.localeOf()
	schema.Walk(r.cfg, coll.Fields, p.Frontmatter, func(v schema.Visit) bool {
		if !v.Item && (v.Field.Hidden || (v.Field.VisibleIf != nil && !schema.Eval(*v.Field.VisibleIf, v.Scope))) {
			return false
		}
		if v.Field.IsLocalized() {
			if child, ok := v.Field.LanguageField(locale); ok && schema.Canonical(child.Type) == schema.TypeRichText {
				m, _ := v.Value.(map[string]any)
				if s, _ := m[locale].(string); strings.TrimSpace(s) != "" {
					r.text(p, v.Path+"."+locale, s, richTextIsHTML(child))
				}
			}
			return false
		}
		s, isString := v.Value.(string)
		if !isString || strings.TrimSpace(s) == "" {
			return true
		}
		switch schema.Canonical(v.Field.Type) {
		case schema.TypeImage:
			r.imageAlt(p, v, s)
		case schema.TypeRichText:
			if p.once() {
				r.text(p, v.Path, s, richTextIsHTML(v.Field))
			}
		case schema.TypeLink:
			if !v.Item {
				r.linkLabel(p, v)
			}
		}
		return true
	})
	if coll.HasBody && !p.IsData && p.once() && strings.TrimSpace(p.Body) != "" {
		r.text(p, resolve.BodyPath, p.Body, false)
	}
}

// altPart is one field an image's alt text is read from, in the page's
// language.
type altPart struct {
	text, path string
	// borrowed is true when the language is empty and the text is the
	// fallback language's.
	borrowed bool
}

// imageAlt checks the alt text of one image value. An alt text that is
// the same in every language is checked on one view of a localized page;
// one that is localized on each.
func (r *run) imageAlt(p Page, v schema.Visit, value string) {
	coll := p.Collection
	if v.Field.Decorative || !r.on(report.RuleQualityImageAltMissing, coll) && !r.on(report.RuleQualityImageAltPoor, coll) {
		return
	}
	var src resolve.AltSource
	siblings := siblingsOf(r.cfg, coll, v)
	if !v.Item {
		src = resolve.AltOf(siblings, v.Field)
		if src.Decorative {
			return
		}
	}
	if !src.Localized && !p.once() {
		return
	}
	base := strings.TrimSuffix(v.Path, v.Field.Key)
	if len(src.Keys) == 0 {
		if r.on(report.RuleQualityImageAltMissing, coll) {
			r.count(report.RuleQualityImageAltMissing)
			r.emit(report.Finding{File: p.Path, Path: v.Path, Rule: report.RuleQualityImageAltMissing,
				Message: "The image has no alt text, and the schema gives it no field for one.",
				Hint:    "Ask a developer to name the field that holds its alt text (`alt:` on the image field) or to mark the image `decorative: true`.",
				Value:   value}, report.ImpactLow)
		}
		return
	}
	locale := p.localeOf()
	var parts []altPart
	for _, key := range src.Keys {
		def := fieldByKey(siblings, key)
		part := altPart{path: base + key}
		if def.IsLocalized() {
			part.path += "." + locale
			part.text, part.borrowed = resolve.LocalizedValue(v.Scope.Self[key], locale, def.Fallback)
		} else {
			s, _ := v.Scope.Self[key].(string)
			part.text = strings.TrimSpace(s)
		}
		parts = append(parts, part)
	}
	var texts []string
	borrowed := true
	for _, part := range parts {
		if part.text != "" {
			texts = append(texts, part.text)
			borrowed = borrowed && part.borrowed
		}
	}
	alt := strings.Join(texts, src.Separator)
	basis := ""
	if src.Composed {
		basis = report.BasisComposedAlt
	}
	altPath := parts[0].path
	if alt == "" {
		if r.on(report.RuleQualityImageAltMissing, coll) {
			r.count(report.RuleQualityImageAltMissing)
			msg := "The image has no alt text."
			switch {
			case src.Composed:
				msg = "The image has no alt text: every field it is made of is empty."
			case !src.Declared:
				msg = "The image has no alt text (the field for it was guessed from field names)."
			}
			r.emit(report.Finding{File: p.Path, Path: altPath, Rule: report.RuleQualityImageAltMissing, Message: msg,
				Hint:  "Describe what the image shows, for people who cannot see it. If it is only decoration, a developer can mark it `decorative: true`.",
				Value: value}, lowered(report.ImpactHigh, !src.Declared))
		}
		return
	}
	if r.on(report.RuleQualityImageAltMissing, coll) {
		r.count(report.RuleQualityImageAltMissing)
	}
	if !borrowed || !src.Localized {
		r.altPoor(p, altPath, alt, value, !src.Declared, basis)
	}
}

// fieldByKey is the field of a list with the key.
func fieldByKey(fields []schema.Field, key string) schema.Field {
	for _, f := range fields {
		if f.Key == key {
			return f
		}
	}
	return schema.Field{}
}

func (r *run) altPoor(p Page, path, alt, src string, guessed bool, basis string) {
	if !r.on(report.RuleQualityImageAltPoor, p.Collection) {
		return
	}
	r.count(report.RuleQualityImageAltPoor)
	if why := PoorAlt(alt, src); why != "" {
		r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityImageAltPoor,
			Message: fmt.Sprintf("The image's alt text %q is %s.", alt, why),
			Hint:    "Say what the image shows or why it is there, in a few words.",
			Value:   alt, Basis: basis}, lowered(report.ImpactLow, guessed))
	}
}

// siblingsOf is the fields list a visited field sits in: the
// collection's top level, its group or list item shape, or the block of
// a legacy `blocks` item.
func siblingsOf(cfg *schema.Config, coll schema.Collection, v schema.Visit) []schema.Field {
	parent := v.Parent()
	if parent == nil {
		return coll.Fields
	}
	if schema.IsGroup(parent.Type) {
		return parent.Fields
	}
	if name, ok := v.Scope.Self[schema.BlockComponentKey].(string); ok {
		if b, ok := cfg.Blocks[name]; ok {
			return b.Fields
		}
	}
	return nil
}

// linkLabel checks the label beside a link field (a button's text) for a
// generic phrase. A link without such a label is not judged: its text
// may come from code.
func (r *run) linkLabel(p Page, v schema.Visit) {
	if !r.on(report.RuleQualityLinkTextGeneric, p.Collection) {
		return
	}
	var label *schema.Field
	for _, s := range siblingsOf(r.cfg, p.Collection, v) {
		t := s.ValueType()
		if s.Key == v.Field.Key || (t != schema.TypeText && t != schema.TypeParagraph) || (schema.IsGroup(s.Type) && !s.IsLocalized()) || !linkLabelKeys[normalKey(s.Key)] {
			continue
		}
		if label != nil {
			return
		}
		s := s
		label = &s
	}
	if label == nil || (!label.IsLocalized() && !p.once()) {
		return
	}
	path := strings.TrimSuffix(v.Path, v.Field.Key) + label.Key
	text, _ := v.Scope.Self[label.Key].(string)
	if label.IsLocalized() {
		var borrowed bool
		path += "." + p.localeOf()
		if text, borrowed = resolve.LocalizedValue(v.Scope.Self[label.Key], p.localeOf(), label.Fallback); borrowed {
			return
		}
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	r.count(report.RuleQualityLinkTextGeneric)
	if GenericLinkText(text) {
		r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityLinkTextGeneric,
			Message: fmt.Sprintf("The link's text %q does not say where it leads.", strings.TrimSpace(text)),
			Hint:    "People using a screen reader often jump from link to link. Say what they get (\"See our pricing\") instead.",
			Value:   strings.TrimSpace(text)}, report.ImpactLow)
	}
}

// text checks one rich text value or the body: heading levels, link
// text, and the alt text of images written into it.
func (r *run) text(p Page, path, value string, isHTML bool) {
	coll := p.Collection
	heads, links, images := scanText(value, isHTML)
	if len(heads) >= 2 && r.on(report.RuleQualityHeadingSkip, coll) {
		r.count(report.RuleQualityHeadingSkip)
		for i := 1; i < len(heads); i++ {
			prev, cur := heads[i-1], heads[i]
			if cur.level <= prev.level+1 {
				continue
			}
			want := int64(prev.level + 1)
			r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityHeadingSkip,
				Message: fmt.Sprintf("The heading %q is level %d right after a level %d heading.", cutRunes(cur.text, 80), cur.level, prev.level),
				Hint:    fmt.Sprintf("Make it a level %d heading, so people who move through a page by its headings do not miss a level.", want),
				Value:   cur.text, Measure: &report.Measure{Value: int64(cur.level), Unit: report.UnitLevel, Max: &want}}, report.ImpactLow)
			break
		}
	}
	for _, l := range links {
		if strings.TrimSpace(l.text) == "" {
			if r.on(report.RuleQualityLinkTextEmpty, coll) {
				r.count(report.RuleQualityLinkTextEmpty)
				r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityLinkTextEmpty,
					Message: fmt.Sprintf("A link to %s has no text.", orNothing(l.href)),
					Hint:    "A screen reader announces such a link by its address, or not at all. Give it words that say where it leads.",
					Value:   l.href}, report.ImpactMedium)
			}
			continue
		}
		if r.on(report.RuleQualityLinkTextEmpty, coll) {
			r.count(report.RuleQualityLinkTextEmpty)
		}
		if r.on(report.RuleQualityLinkTextGeneric, coll) {
			r.count(report.RuleQualityLinkTextGeneric)
			if GenericLinkText(l.text) {
				r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityLinkTextGeneric,
					Message: fmt.Sprintf("The link %q to %s does not say where it leads.", l.text, orNothing(l.href)),
					Hint:    "People using a screen reader often jump from link to link. Say what they get (\"See our pricing\") instead.",
					Value:   l.text}, report.ImpactLow)
			}
		}
	}
	for _, im := range images {
		if !r.on(report.RuleQualityImageAltMissing, coll) && !r.on(report.RuleQualityImageAltPoor, coll) {
			break
		}
		// alt="" in HTML marks an image as decorative on purpose.
		if im.html && im.hasAlt && im.alt == "" {
			continue
		}
		if im.alt == "" {
			if r.on(report.RuleQualityImageAltMissing, coll) {
				r.count(report.RuleQualityImageAltMissing)
				r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityImageAltMissing,
					Message: fmt.Sprintf("The image %s in the text has no alt text.", orNothing(im.src)),
					Hint:    "Describe what the image shows between the square brackets: ![a team at work](...).",
					Value:   im.src}, report.ImpactHigh)
			}
			continue
		}
		if r.on(report.RuleQualityImageAltMissing, coll) {
			r.count(report.RuleQualityImageAltMissing)
		}
		r.altPoor(p, path, im.alt, im.src, false, "")
	}
}

func richTextIsHTML(f schema.Field) bool {
	return f.Format == schema.RichTextFormatHTML || (f.Format == "" && f.Type == schema.TypeHTML)
}

func orNothing(s string) string {
	if strings.TrimSpace(s) == "" {
		return "nowhere"
	}
	return s
}

// normalKey keeps the lowercase letters and digits of a key.
func normalKey(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	return b.String()
}
