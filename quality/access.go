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
// the body. Fields the page hides are skipped.
func (r *run) accessibility(p Page) {
	coll := p.Collection
	schema.Walk(r.cfg, coll.Fields, p.Frontmatter, func(v schema.Visit) bool {
		if !v.Item && (v.Field.Hidden || (v.Field.VisibleIf != nil && !schema.Eval(*v.Field.VisibleIf, v.Scope))) {
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
			r.text(p, v.Path, s, richTextIsHTML(v.Field))
		case schema.TypeLink:
			if !v.Item {
				r.linkLabel(p, v)
			}
		}
		return true
	})
	if coll.HasBody && !p.IsData && strings.TrimSpace(p.Body) != "" {
		r.text(p, resolve.BodyPath, p.Body, false)
	}
}

// imageAlt checks the alt text of one image value.
func (r *run) imageAlt(p Page, v schema.Visit, value string) {
	coll := p.Collection
	if v.Field.Decorative || !r.on(report.RuleQualityImageAltMissing, coll) && !r.on(report.RuleQualityImageAltPoor, coll) {
		return
	}
	var altKey string
	declared := false
	if !v.Item {
		var decorative bool
		altKey, decorative, declared = resolve.AltOf(siblingsOf(r.cfg, coll, v), v.Field)
		if decorative {
			return
		}
	}
	base := strings.TrimSuffix(v.Path, v.Field.Key)
	if altKey == "" {
		if r.on(report.RuleQualityImageAltMissing, coll) {
			r.count(report.RuleQualityImageAltMissing)
			r.emit(report.Finding{File: p.Path, Path: v.Path, Rule: report.RuleQualityImageAltMissing,
				Message: "The image has no alt text, and the schema gives it no field for one.",
				Hint:    "Ask a developer to name the field that holds its alt text (`alt:` on the image field) or to mark the image `decorative: true`.",
				Value:   value}, report.ImpactLow)
		}
		return
	}
	alt, _ := v.Scope.Self[altKey].(string)
	alt = strings.TrimSpace(alt)
	altPath := base + altKey
	if alt == "" {
		if r.on(report.RuleQualityImageAltMissing, coll) {
			r.count(report.RuleQualityImageAltMissing)
			msg := "The image has no alt text."
			if !declared {
				msg = "The image has no alt text (the field for it was guessed from field names)."
			}
			r.emit(report.Finding{File: p.Path, Path: altPath, Rule: report.RuleQualityImageAltMissing, Message: msg,
				Hint:  "Describe what the image shows, for people who cannot see it. If it is only decoration, a developer can mark it `decorative: true`.",
				Value: value}, lowered(report.ImpactHigh, !declared))
		}
		return
	}
	if r.on(report.RuleQualityImageAltMissing, coll) {
		r.count(report.RuleQualityImageAltMissing)
	}
	r.altPoor(p, altPath, alt, value, !declared)
}

func (r *run) altPoor(p Page, path, alt, src string, guessed bool) {
	if !r.on(report.RuleQualityImageAltPoor, p.Collection) {
		return
	}
	r.count(report.RuleQualityImageAltPoor)
	if why := PoorAlt(alt, src); why != "" {
		r.emit(report.Finding{File: p.Path, Path: path, Rule: report.RuleQualityImageAltPoor,
			Message: fmt.Sprintf("The image's alt text %q is %s.", alt, why),
			Hint:    "Say what the image shows or why it is there, in a few words.",
			Value:   alt}, lowered(report.ImpactLow, guessed))
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
		t := schema.Canonical(s.Type)
		if s.Key == v.Field.Key || (t != schema.TypeText && t != schema.TypeParagraph) || !linkLabelKeys[normalKey(s.Key)] {
			continue
		}
		if label != nil {
			return
		}
		s := s
		label = &s
	}
	if label == nil {
		return
	}
	text, _ := v.Scope.Self[label.Key].(string)
	if strings.TrimSpace(text) == "" {
		return
	}
	r.count(report.RuleQualityLinkTextGeneric)
	if GenericLinkText(text) {
		r.emit(report.Finding{File: p.Path, Path: strings.TrimSuffix(v.Path, v.Field.Key) + label.Key, Rule: report.RuleQualityLinkTextGeneric,
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
		r.altPoor(p, path, im.alt, im.src, false)
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
