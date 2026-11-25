package schema

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/norcubeplatform/cms-schema/report"
)

// Field knobs the role and language features read. Every field may carry
// them; the validator reports one that sits where it means nothing.
const (
	// KnobLocalized on a group says its fields are the site's languages,
	// one per locale of preview.locales.
	KnobLocalized = "localized"
	// KnobFallback names the language whose value an empty language of a
	// localized group shows on the site.
	KnobFallback = "fallback"
	// KnobTitleTemplate composes the page title from a seoTitle field.
	KnobTitleTemplate = "titleTemplate"
	// KnobExcerpt says how a rich text seoDescription becomes the meta
	// description.
	KnobExcerpt = "excerpt"
	// KnobNoindexWhen and KnobDraftWhen list the values of a select with
	// role noindex or draft that hide the page or make it a draft.
	KnobNoindexWhen = "noindexWhen"
	KnobDraftWhen   = "draftWhen"
	// KnobAlt on an image names the field holding its alt text, or
	// composes it ({from: [...], separator: "..."}).
	KnobAlt = "alt"
	// AltKeyFrom and AltKeySeparator are the keys of a composed alt.
	AltKeyFrom      = "from"
	AltKeySeparator = "separator"
	// OptionKeyLabels on a select option holds its label per language.
	OptionKeyLabels = "labels"
)

// ExcerptFirstParagraph: the meta description is the plain text of the
// rich text's first paragraph, markup removed.
const ExcerptFirstParagraph = "firstParagraph"

// ExcerptValues lists the values of `excerpt`.
func ExcerptValues() []string { return []string{ExcerptFirstParagraph} }

// DefaultAltSeparator joins the parts of a composed alt text when the
// schema names no separator.
const DefaultAltSeparator = " "

// Bounds of the role and language knobs.
const (
	// MaxAltParts bounds the fields a composed alt text is made of.
	MaxAltParts = 10
	// MaxRoleValues bounds the entries of noindexWhen and draftWhen.
	MaxRoleValues = 100
	// MaxTemplateBytes bounds a titleTemplate.
	MaxTemplateBytes = 300
	// MaxSeparatorBytes bounds an alt separator.
	MaxSeparatorBytes = 20
)

// LocalizedTypes are the canonical types the fields of a localized group
// may have: text a person reads, one type for the whole group.
var localizedTypes = []string{TypeText, TypeParagraph, TypeRichText}

// LocalizedTypes lists the canonical types a localized group's fields
// may have.
func LocalizedTypes() []string { return append([]string(nil), localizedTypes...) }

// IsLocalized reports whether a field is a localized group.
func (f Field) IsLocalized() bool { return f.Localized && IsGroup(f.Type) }

// ValueType is the canonical type of the value a field holds per page:
// for a localized group, the type of its language fields (empty when
// they disagree or there are none), else the field's own type.
func (f Field) ValueType() string {
	if !f.IsLocalized() {
		return Canonical(f.Type)
	}
	t := ""
	for _, c := range f.Fields {
		ct := Canonical(c.Type)
		if t != "" && ct != t {
			return ""
		}
		t = ct
	}
	return t
}

// LanguageField returns the field of a localized group that holds one
// language.
func (f Field) LanguageField(locale string) (Field, bool) {
	for _, c := range f.Fields {
		if c.Key == locale {
			return c, true
		}
	}
	return Field{}, false
}

// RoleWhen is the knob that lists a select's values for a role that a
// boolean holds as true (noindexWhen, draftWhen); empty for any other
// role.
func RoleWhen(role string) string {
	switch role {
	case RoleNoindex:
		return KnobNoindexWhen
	case RoleDraft:
		return KnobDraftWhen
	}
	return ""
}

// WhenValues are the select values that switch the field's role on
// (noindexWhen or draftWhen, by its role).
func (f Field) WhenValues() []string { return f.whenFor(f.Role) }

func (f Field) whenFor(role string) []string {
	switch role {
	case RoleNoindex:
		return f.NoindexWhen
	case RoleDraft:
		return f.DraftWhen
	}
	return nil
}

// HoldsRole reports whether a field can hold the role as declared: its
// value type accepts the role, a rich text description says how it is
// excerpted, and a select names the values that switch the role on.
// Guesses and readers use it; the validator says why when it is false.
func (f Field) HoldsRole(role string) bool {
	if role == RoleNone {
		return true
	}
	t := f.ValueType()
	if IsGroup(f.Type) && !f.IsLocalized() {
		return false
	}
	if !RoleAccepts(role, t) {
		return false
	}
	switch {
	case role == RoleSEODescription && t == TypeRichText:
		return f.Excerpt == ExcerptFirstParagraph
	case t == TypeSelect:
		return len(f.whenFor(role)) > 0
	}
	return true
}

// OptionLabel is how a select option reads in one language: its label
// for that language (`labels`), else its label, else its value. ok is
// false when the value is not an option.
func OptionLabel(opts []SelectOption, value, locale string) (label string, ok bool) {
	for _, o := range opts {
		if o.Value != value {
			continue
		}
		if l := strings.TrimSpace(o.Labels[locale]); l != "" {
			return l, true
		}
		if l := strings.TrimSpace(o.Label); l != "" {
			return l, true
		}
		return o.Value, true
	}
	return "", false
}

// TemplatePart is one piece of a titleTemplate: literal text, the field's
// own value, another field of the page (or its option label), or a site
// variable.
type TemplatePart struct {
	Kind TemplatePartKind
	// Text is the literal of a TemplateLiteral.
	Text string
	// Path is the field of a TemplateField, keys joined by dots.
	Path string
	// Label is true for `{field:<path>|label}`: the select option's label
	// instead of its value.
	Label bool
	// Variable is the key of a TemplateVariable (`{{site_name}}`).
	Variable string
}

// TemplatePartKind says what a TemplatePart is.
type TemplatePartKind string

// Kinds of TemplatePart.
const (
	TemplateLiteral  TemplatePartKind = "literal"
	TemplateValue    TemplatePartKind = "value"
	TemplateField    TemplatePartKind = "field"
	TemplateVariable TemplatePartKind = "variable"
)

// Placeholders of a titleTemplate.
const (
	TemplatePlaceholderValue  = "{value}"
	TemplateFieldPrefix       = "field:"
	TemplateLabelSuffix       = "|label"
	maxTemplateParts          = 50
	templateFieldPathMaxBytes = 200
)

// ErrTemplate marks a titleTemplate that cannot be read.
var ErrTemplate = errors.New("invalid titleTemplate")

// TemplateError is a titleTemplate that cannot be read: why, as a cause
// of report.TemplateCauses with its params. It matches ErrTemplate.
type TemplateError struct{ report.Problem }

func (e *TemplateError) Error() string { return ErrTemplate.Error() + ": " + e.Problem.String() }
func (e *TemplateError) Unwrap() error { return ErrTemplate }

func templateError(c report.Cause, p *report.Params) error {
	return &TemplateError{report.Problem{Cause: c, Params: p}}
}

var (
	variableToken = regexp.MustCompile(`^\{\{\s*([a-zA-Z][a-zA-Z0-9_]*)\s*\}\}`)
	fieldPathRe   = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
)

// ParseTemplate reads a titleTemplate into its parts. Placeholders are
// `{value}`, `{field:<path>}`, `{field:<path>|label}` and site variables
// `{{key}}`; every other brace is an error, so a typo never passes as
// literal text. The template must use `{value}`.
func ParseTemplate(s string) ([]TemplatePart, error) {
	if strings.TrimSpace(s) == "" {
		return nil, templateError(report.CauseTemplateEmpty, nil)
	}
	if len(s) > MaxTemplateBytes {
		return nil, templateError(report.CauseTemplateTooLong, &report.Params{Max: report.Num(MaxTemplateBytes)})
	}
	var out []TemplatePart
	lit := strings.Builder{}
	flush := func() {
		if lit.Len() > 0 {
			out = append(out, TemplatePart{Kind: TemplateLiteral, Text: lit.String()})
			lit.Reset()
		}
	}
	hasValue := false
	for i := 0; i < len(s); {
		switch s[i] {
		case '}':
			return nil, templateError(report.CauseTemplateLoneClose, &report.Params{Offset: report.Int(int64(i))})
		case '{':
			if m := variableToken.FindStringSubmatch(s[i:]); m != nil {
				flush()
				out = append(out, TemplatePart{Kind: TemplateVariable, Variable: m[1]})
				i += len(m[0])
				continue
			}
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, templateError(report.CauseTemplateUnclosed, &report.Params{Offset: report.Int(int64(i))})
			}
			tok := s[i : i+end+1]
			inner := tok[1 : len(tok)-1]
			switch {
			case tok == TemplatePlaceholderValue:
				flush()
				out = append(out, TemplatePart{Kind: TemplateValue})
				hasValue = true
			case strings.HasPrefix(inner, TemplateFieldPrefix):
				path := strings.TrimPrefix(inner, TemplateFieldPrefix)
				label := strings.HasSuffix(path, TemplateLabelSuffix)
				path = strings.TrimSuffix(path, TemplateLabelSuffix)
				if len(path) > templateFieldPathMaxBytes || !fieldPathRe.MatchString(path) {
					return nil, templateError(report.CauseTemplateFieldPath, &report.Params{Value: tok})
				}
				flush()
				out = append(out, TemplatePart{Kind: TemplateField, Path: path, Label: label})
			default:
				return nil, templateError(report.CauseTemplateUnknownPlaceholder, &report.Params{Value: tok})
			}
			i += len(tok)
		default:
			lit.WriteByte(s[i])
			i++
		}
		if len(out) > maxTemplateParts {
			return nil, templateError(report.CauseTemplateTooManyParts, &report.Params{Max: report.Num(maxTemplateParts)})
		}
	}
	flush()
	if !hasValue {
		return nil, templateError(report.CauseTemplateNoValue, nil)
	}
	return out, nil
}

// TemplateFields lists the field paths a template reads besides its own
// value, each once, in order.
func TemplateFields(parts []TemplatePart) []string {
	var out []string
	for _, p := range parts {
		if p.Kind == TemplateField && !slices.Contains(out, p.Path) {
			out = append(out, p.Path)
		}
	}
	return out
}

// HasLocalized reports whether a fields list holds a localized group at
// any depth (inside groups and list item shapes).
func HasLocalized(fields []Field) bool { return hasLocalized(fields, 0) }

func hasLocalized(fields []Field, depth int) bool {
	if depth > MaxWalkDepth {
		return false
	}
	for _, f := range fields {
		switch {
		case f.IsLocalized():
			return true
		case IsGroup(f.Type) && hasLocalized(f.Fields, depth+1):
			return true
		case IsArray(f.Type) && hasLocalized(f.Items, depth+1):
			return true
		}
	}
	return false
}

// OneLanguagePerFile reports whether each file of the collection is one
// language version (its locale comes from the path or a field, or its
// versions are linked by translations.by), so a localized group, which
// holds every language in one file, has no place in it.
func (c Collection) OneLanguagePerFile() bool {
	return c.LocaleFrom == LocaleFromPath || strings.HasPrefix(c.LocaleFrom, LocaleFromFieldPrefix) ||
		(c.Translations != nil && c.Translations.By != "")
}

// Localized reports whether the collection's files hold every language
// themselves, in localized groups: each file is one page per language.
func (c Collection) Localized() bool { return !c.OneLanguagePerFile() && HasLocalized(c.Fields) }

// FieldAtPath finds the field at a dotted path outside lists, entering
// groups (a localized group's language fields included). Keys only: a
// path through a list has no one field.
func FieldAtPath(fields []Field, path string) (Field, bool) {
	parts := strings.Split(path, ".")
	for i, key := range parts {
		if i > MaxWalkDepth {
			return Field{}, false
		}
		var found *Field
		for j := range fields {
			if fields[j].Key == key {
				found = &fields[j]
				break
			}
		}
		if found == nil {
			return Field{}, false
		}
		if i == len(parts)-1 {
			return *found, true
		}
		if !IsGroup(found.Type) {
			return Field{}, false
		}
		fields = found.Fields
	}
	return Field{}, false
}
