package resolve

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

// LocalizedValue reads one language of a localized group's value (a
// mapping of languages to texts): the text in locale, else, when the
// group falls back to another language, that language's text, which the
// site then shows. borrowed is true when the value is the fallback's.
// A value that is not a mapping holds no language.
func LocalizedValue(v any, locale, fallback string) (value string, borrowed bool) {
	m, _ := v.(map[string]any)
	if s, _ := m[locale].(string); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s), false
	}
	if fallback != "" && fallback != locale {
		if s, _ := m[fallback].(string); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), true
		}
	}
	return "", false
}

// URLIn is URL for one language of a file that holds every language
// (localized groups): `{locale}` and a localized `slug` read locale
// instead of the file's own.
func URLIn(filePath string, coll schema.Collection, site schema.PreviewSite, fm map[string]any, locale string) string {
	if filePath == "" || coll.PreviewPath == "" {
		return ""
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return urlFor(filePath, coll, site, fm, locale)
}

// SiteVariable is one `{{key}}` placeholder of the site's variables
// document: a value, and per language where it is localized.
type SiteVariable struct {
	Key          string
	Value        string
	Translations map[string]string
}

// MaxSiteVariables bounds the variables read from the document.
const MaxSiteVariables = 1000

// SiteVariables reads the `{key, value}` list of the variables document
// (`variables.file` and `.field`): a value, or `translations` (or a
// mapping `value`) per language. Entries without a key are left out.
func SiteVariables(fm map[string]any, field string) []SiteVariable {
	list, _ := fm[field].([]any)
	var out []SiteVariable
	for _, item := range list {
		if len(out) == MaxSiteVariables {
			break
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["key"].(string)
		if strings.TrimSpace(key) == "" {
			continue
		}
		v := SiteVariable{Key: strings.TrimSpace(key)}
		v.Value, _ = m["value"].(string)
		v.Translations = stringMap(m["translations"])
		if v.Translations == nil {
			v.Translations = stringMap(m["value"])
		}
		out = append(out, v)
	}
	return out
}

func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, x := range m {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LookupVariable is a variable's value in a language, as a site build
// resolves it: its translation in locale, else in the default locale,
// else its value. Keys compare without case. ok is false for a key the
// list does not have.
func LookupVariable(vars []SiteVariable, key, locale, defaultLocale string) (string, bool) {
	for _, v := range vars {
		if !strings.EqualFold(v.Key, key) {
			continue
		}
		if s, ok := v.Translations[locale]; ok {
			return s, true
		}
		if s, ok := v.Translations[defaultLocale]; ok {
			return s, true
		}
		return v.Value, true
	}
	return "", false
}

// TitleContext is what composing a title needs besides the template.
type TitleContext struct {
	// Fields and Frontmatter are the page's schema and values.
	Fields      []schema.Field
	Frontmatter map[string]any
	// Locale is the language of the page; DefaultLocale the site's.
	Locale, DefaultLocale string
	// Variables are the site's `{{key}}` values.
	Variables []SiteVariable
}

// ComposeTitle is the page title a titleTemplate makes of value. ok is
// false when it cannot be known: the template does not parse, or it
// (or a value it takes in) holds a `{{variable}}` the site does not
// define. A field the page leaves empty reads as nothing.
func ComposeTitle(template, value string, c TitleContext) (string, bool) {
	parts, err := schema.ParseTemplate(template)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		switch p.Kind {
		case schema.TemplateLiteral:
			b.WriteString(p.Text)
		case schema.TemplateValue:
			b.WriteString(value)
		case schema.TemplateVariable:
			v, ok := LookupVariable(c.Variables, p.Variable, c.Locale, c.DefaultLocale)
			if !ok {
				return "", false
			}
			b.WriteString(v)
		case schema.TemplateField:
			b.WriteString(templateField(p, c))
		}
	}
	out, ok := ExpandVariables(b.String(), c.Variables, c.Locale, c.DefaultLocale)
	if !ok {
		return "", false
	}
	return strings.Join(strings.Fields(out), " "), true
}

// templateField is the text of a `{field:<path>}` part in the page's
// language.
func templateField(p schema.TemplatePart, c TitleContext) string {
	fd, ok := schema.FieldAtPath(c.Fields, p.Path)
	if !ok {
		return ""
	}
	v := ReadDotted(c.Frontmatter, p.Path)
	if fd.IsLocalized() {
		s, _ := LocalizedValue(v, c.Locale, fd.Fallback)
		return s
	}
	var values []string
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			values = append(values, scalarText(item))
		}
	default:
		values = []string{scalarText(x)}
	}
	if p.Label {
		for i, s := range values {
			if l, ok := schema.OptionLabel(fd.Options, s, c.Locale); ok {
				values[i] = l
			}
		}
	}
	var kept []string
	for _, s := range values {
		if strings.TrimSpace(s) != "" {
			kept = append(kept, strings.TrimSpace(s))
		}
	}
	return strings.Join(kept, ", ")
}

func scalarText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool, int, int64:
		return fmt.Sprint(x)
	}
	return ""
}

var variableRe = regexp.MustCompile(`\{\{\s*([a-zA-Z][a-zA-Z0-9_]*)\s*\}\}`)

// ExpandVariables replaces the `{{key}}` placeholders of a text with the
// site's values in a language. ok is false when one names a variable the
// site does not define, or a brace pair is left that is not one.
func ExpandVariables(s string, vars []SiteVariable, locale, defaultLocale string) (string, bool) {
	if !strings.Contains(s, "{{") {
		return s, true
	}
	ok := true
	out := variableRe.ReplaceAllStringFunc(s, func(tok string) string {
		key := variableRe.FindStringSubmatch(tok)[1]
		v, found := LookupVariable(vars, key, locale, defaultLocale)
		if !found {
			ok = false
		}
		return v
	})
	return out, ok && !strings.Contains(out, "{{")
}

var (
	mdHeadingLine = regexp.MustCompile(`^ {0,3}#{1,6}(\s|$)`)
	mdSetextLine  = regexp.MustCompile(`^ {0,3}(=+|-+)\s*$`)
	mdFence       = regexp.MustCompile("^ {0,3}(```|~~~)")
	mdImageRe     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLinkRe      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdRefLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	mdMarks       = strings.NewReplacer("**", "", "__", "", "~~", "", "`", "")
	mdEmphasis    = regexp.MustCompile(`(^|[^\w*])[*_]([^*_\s][^*_]*?)[*_]([^\w*]|$)`)
	htmlPara      = regexp.MustCompile(`(?is)<p\b[^>]*>(.*?)</p\s*>`)
	htmlTagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	blockLead     = regexp.MustCompile(`^ {0,3}(>\s?|[-*+]\s+|\d+[.)]\s+)`)
)

// maxExcerptScan bounds the characters of a text read for its first
// paragraph.
const maxExcerptScan = 100000

// FirstParagraph is the plain text of a rich text's first paragraph, as
// `excerpt: firstParagraph` reads it: Markdown is split at blank lines,
// headings and fenced code are skipped, and the first other block is
// taken with its markup removed (links and images keep their text);
// HTML takes the first `<p>`. White space is collapsed.
func FirstParagraph(text string, isHTML bool) string {
	if len(text) > maxExcerptScan {
		text = text[:maxExcerptScan]
	}
	if isHTML {
		if m := htmlPara.FindStringSubmatch(text); m != nil {
			return plainHTML(m[1])
		}
		return plainHTML(text)
	}
	var block []string
	inFence, fence := false, ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if m := mdFence.FindStringSubmatch(line); m != nil {
			switch {
			case !inFence:
				inFence, fence = true, m[1]
			case m[1] == fence:
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		blank := strings.TrimSpace(line) == ""
		switch {
		case blank && len(block) > 0:
			return plainMarkdown(block)
		case blank, mdHeadingLine.MatchString(line):
			continue
		case len(block) == 1 && mdSetextLine.MatchString(line):
			// The line before was a heading.
			block = nil
			continue
		}
		block = append(block, line)
	}
	return plainMarkdown(block)
}

func plainMarkdown(lines []string) string {
	for i, l := range lines {
		lines[i] = blockLead.ReplaceAllString(l, "")
	}
	s := strings.Join(lines, " ")
	s = mdImageRe.ReplaceAllString(s, "$1")
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = mdRefLinkRe.ReplaceAllString(s, "$1")
	s = mdMarks.Replace(s)
	for range 3 {
		s = mdEmphasis.ReplaceAllString(s, "$1$2$3")
	}
	return plainHTML(s)
}

func plainHTML(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTagRe.ReplaceAllString(s, " "))), " ")
}
