package resolve

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/norcubeplatform/cms-schema/schema"
)

// Formats of a translatable text.
const (
	FormatPlain    = "plain"
	FormatMarkdown = "markdown"
	FormatHTML     = "html"
)

const (
	// BodyPath is the path of a file's Markdown body in a Text and in a
	// finding.
	BodyPath = "body"
	// MaxTranslatableTexts bounds what TranslatableTexts returns for one
	// page. Real pages hold tens to a few hundred texts.
	MaxTranslatableTexts = 2000

	bodyLabel = "Body"
	slugKey   = "slug"
)

// ErrTooManyTexts is returned by TranslatableTexts for a page with more
// than MaxTranslatableTexts translatable texts.
var ErrTooManyTexts = errors.New("the page has more translatable texts than the limit")

// TranslatableType reports whether values of the field type are text a
// person reads: text, paragraph and rich text, under any of their names.
// No other value is ever translated, whatever the field says.
func TranslatableType(t string) bool {
	switch schema.Canonical(t) {
	case schema.TypeText, schema.TypeParagraph, schema.TypeRichText:
		return true
	}
	return false
}

// Translatable reports whether a field at the top level of the
// collection is translated when a page gets a language version. For a
// field inside a group or a list, use TranslatableIn.
func Translatable(coll schema.Collection, f schema.Field) bool {
	return TranslatableIn(coll, nil, f)
}

// TranslatableIn reports whether the text of a field is translated when
// a page of the collection gets a language version. ancestors are the
// fields around it, outermost first (schema.Visit.Ancestors); none for a
// top-level field.
//
// The rules, the first that applies deciding:
//
//  1. Only text, paragraph and rich text are ever translated.
//  2. The field's own `translatable` wins.
//  3. A computed or hidden field is not translated.
//  4. The nearest group or list around it that says `translatable`, or
//     is hidden, covers it: false or hidden means no; true hands the
//     decision back to the rules below.
//  5. The field that links the versions (`translations.by: field:<key>`)
//     is not translated. Top level only: a nested field with the same
//     key is ordinary.
//  6. A field keyed `slug` is an address and is not translated.
//  7. A text field with a `regex` holds an id, an icon name or a handle
//     and is not translated.
//  8. Everything else is.
//
// For a group or a list itself the answer is what it says (false when
// it says nothing); its fields are judged one by one.
func TranslatableIn(coll schema.Collection, ancestors []schema.Field, f schema.Field) bool {
	container := schema.IsGroup(f.Type) || schema.IsArray(f.Type)
	if !container && !TranslatableType(f.Type) {
		return false
	}
	if f.Translatable != nil {
		return *f.Translatable
	}
	if container || f.Computed != nil || f.Hidden {
		return false
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		a := ancestors[i]
		if a.Translatable != nil {
			if !*a.Translatable {
				return false
			}
			break
		}
		if a.Hidden {
			return false
		}
	}
	if len(ancestors) == 0 {
		if link, ok := coll.Translations.Field(); ok && f.Key == link {
			return false
		}
	}
	if f.Key == slugKey {
		return false
	}
	if schema.Canonical(f.Type) == schema.TypeText && f.Regex != "" {
		return false
	}
	return true
}

// Text is one translatable text of a page.
type Text struct {
	// Path is where the text sits in the frontmatter (`a.b[2].c`), or
	// BodyPath for the body.
	Path string
	// Label is the field's label (its key when it has none); an item of
	// a list of texts carries the list's.
	Label string
	// Type is the canonical field type: text, paragraph or richtext.
	Type string
	// Format is FormatPlain, FormatMarkdown or FormatHTML.
	Format string
	// Value is the text as the file holds it.
	Value string
	// Body is true for the file's body, which a frontmatter field keyed
	// `body` would otherwise share its path with.
	Body bool
}

// TranslatableTexts lists the texts of a page that are translated when
// it gets a language version: every non-blank string value of a field
// TranslatableIn accepts, in schema order with list items by index, and
// then the body when the collection has one. Values that are not
// strings are left out.
//
// A page with more than MaxTranslatableTexts texts returns the first
// MaxTranslatableTexts and ErrTooManyTexts; the list is then incomplete
// and must not be used as the whole page.
func TranslatableTexts(cfg *schema.Config, coll schema.Collection, frontmatter map[string]any, body string) ([]Text, error) {
	var out []Text
	full := false
	add := func(t Text) {
		if len(out) >= MaxTranslatableTexts {
			full = true
			return
		}
		out = append(out, t)
	}
	schema.Walk(cfg, coll.Fields, frontmatter, func(v schema.Visit) bool {
		if full {
			return false
		}
		s, ok := v.Value.(string)
		if !ok || !HasWords(s) || !TranslatableIn(coll, v.Ancestors, v.Field) {
			return true
		}
		add(Text{Path: v.Path, Label: textLabel(v), Type: schema.Canonical(v.Field.Type), Format: textFormat(v.Field), Value: s})
		return true
	})
	if coll.HasBody && HasWords(body) {
		add(Text{Path: BodyPath, Label: bodyLabel, Type: schema.TypeRichText, Format: FormatMarkdown, Value: body, Body: true})
	}
	if full {
		return out, ErrTooManyTexts
	}
	return out, nil
}

// HasWords reports whether a value holds anything to translate. A value
// with no letter (a phone number, a price, a date), or one that is
// nothing but an email address or a web address, is the same in every
// language, whatever kind of field holds it.
func HasWords(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || loneAddress.MatchString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// loneAddress is a value that is one email address or one web address
// and nothing else.
var loneAddress = regexp.MustCompile(`^(?:[^\s@]+@[^\s@]+\.[^\s@]+|(?:https?://|mailto:|tel:|www\.)\S+)$`)

func textLabel(v schema.Visit) string {
	if v.Field.Label != "" {
		return v.Field.Label
	}
	if p := v.Parent(); v.Item && p != nil {
		if p.Label != "" {
			return p.Label
		}
		return p.Key
	}
	return v.Field.Key
}

// textFormat is how a translatable field stores its text: rich text in
// Markdown unless the field or its legacy type name says HTML.
func textFormat(f schema.Field) string {
	if schema.Canonical(f.Type) != schema.TypeRichText {
		return FormatPlain
	}
	if f.Format == schema.RichTextFormatHTML || (f.Format == "" && f.Type == schema.TypeHTML) {
		return FormatHTML
	}
	return FormatMarkdown
}
