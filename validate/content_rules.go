package validate

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Reserved top-level keys the editor writes that no schema declares.
var reservedKeys = map[string]bool{schema.CollectionKey: true, schema.IDKey: true, schema.ArrayItemTypeKey: true}

// Page checks one content file against its collection: the editor's
// validators (required, shape, item type, array bounds, regex, length,
// number bounds, unique, custom) plus out-of-schema keys and an
// unexpected body. Findings for fields hidden by `hidden` or
// `visibleIf` are dropped, as the editor drops them. It never panics: a
// bug in a check comes back as one `content.internal_error` finding.
func Page(cfg *schema.Config, coll schema.Collection, doc content.Document) (out []report.Finding) {
	defer recoverAs(&out, doc.Path, report.RuleContentInternalError)
	return checkPage(cfg, coll, doc)
}

func checkPage(cfg *schema.Config, coll schema.Collection, doc content.Document) []report.Finding {
	var out []report.Finding
	fm := doc.Frontmatter
	f := func(path string, sev report.Severity, rule report.Rule, msg, hint string) {
		out = append(out, report.Finding{File: doc.Path, Path: path, Severity: sev, Rule: rule, Message: msg, Hint: hint})
	}
	schema.Walk(cfg, coll.Fields, fm, func(v schema.Visit) bool {
		// The rules apply to declared fields; the items of a list of
		// primitives have none of their own.
		if !v.Item {
			checkValue(cfg, v, f)
		}
		return true
	})

	// Out-of-schema keys.
	if len(coll.Fields) > 0 {
		known := map[string]bool{}
		for _, fd := range coll.Fields {
			known[fd.Key] = true
		}
		var extra []string
		for k := range fm {
			if !known[k] && !reservedKeys[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			switch coll.ExtraKeysPolicy {
			case schema.ExtraKeysHide:
			case schema.ExtraKeysStrip:
				f("", report.SeverityError, report.RuleContentExtraKeys, fmt.Sprintf("keys %s are not in the schema and the collection's extraKeysPolicy is strip: the next save deletes them", strings.Join(extra, ", ")), "Add the fields to the schema or move the data before an editor saves the file.")
			default:
				f("", report.SeverityWarning, report.RuleContentExtraKeys, fmt.Sprintf("keys %s are not in the schema", strings.Join(extra, ", ")), "They show in the editor's out-of-schema panel. Add them to the schema or remove them.")
			}
		}
	}
	if !coll.HasBody && !doc.IsData && strings.TrimSpace(doc.Body) != "" {
		f("", report.SeverityWarning, report.RuleContentUnexpectedBody, "the file has a Markdown body but the collection has `hasBody: false`", "The editor does not show the body; set `hasBody: true` or move the text into a field.")
	}

	hidden := schema.HiddenPaths(cfg, coll.Fields, fm)
	if len(hidden) == 0 {
		return out
	}
	kept := out[:0]
	for _, fi := range out {
		if !schema.PathHidden(fi.Path, hidden) {
			kept = append(kept, fi)
		}
	}
	return kept
}

type pageEmit func(path string, sev report.Severity, rule report.Rule, msg, hint string)

func checkValue(cfg *schema.Config, v schema.Visit, f pageEmit) {
	fd := v.Field
	label := fd.Label
	if label == "" {
		label = fd.Key
	}
	t := schema.Canonical(fd.Type)

	// required
	if fd.Required && isBlank(v.Value) {
		msg := fd.RequiredMessage
		if msg == "" {
			msg = label + " is required."
		}
		f(v.Path, report.SeverityError, report.RuleContentRequired, msg, "Add a value to clear this error.")
	}

	// customValidators run on absent values too (an expression may
	// require another field).
	for i, cv := range fd.Custom {
		switch cv.Type {
		case schema.CustomValidatorRegex:
			pat, _ := cv.Params["pattern"].(string)
			s, ok := v.Value.(string)
			if pat == "" || !ok || strings.TrimSpace(s) == "" {
				continue
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			matched := re.MatchString(s)
			if full, _ := cv.Params["fullMatch"].(bool); full && matched {
				loc := re.FindStringIndex(s)
				matched = loc != nil && loc[1]-loc[0] == len(s)
			}
			if !matched {
				msg := cv.Message
				if msg == "" {
					msg = label + " doesn't match the expected pattern."
				}
				f(v.Path, report.SeverityError, report.RuleContentRegex, msg, fmt.Sprintf("customValidators[%d]", i))
			}
		case schema.CustomValidatorExpression:
			cond, ok := cv.Params["condition"].(map[string]any)
			if !ok {
				continue
			}
			if !schema.Eval(parseCond(cond), v.Scope) {
				msg := cv.Message
				if msg == "" {
					msg = label + " fails the configured rule."
				}
				f(v.Path, report.SeverityError, report.RuleContentCustom, msg, fmt.Sprintf("customValidators[%d]", i))
			}
		}
	}

	if v.Value == nil {
		return
	}

	// shape
	actual := describe(v.Value)
	shape := func(expected string) {
		f(v.Path, report.SeverityError, report.RuleContentShape, fmt.Sprintf("%s should be a %s but is a %s.", label, expected, actual), "The schema's type was likely changed after this file was saved. Convert the value or clear it.")
	}
	switch t {
	case schema.TypeText, schema.TypeParagraph, schema.TypeRichText, schema.TypeLink, schema.TypeDate, schema.TypeImage, schema.TypeFile, schema.TypeColor:
		if _, ok := v.Value.(string); !ok {
			shape("text")
			return
		}
	case schema.TypeReference:
		if fd.Multiple {
			list, ok := v.Value.([]any)
			if !ok || !allStrings(list) {
				shape("list of references")
				return
			}
		} else if _, ok := v.Value.(string); !ok {
			shape("reference")
			return
		}
	case schema.TypeNumber, schema.TypeRange:
		n, ok := numberOf(v.Value)
		if !ok {
			shape("number")
			return
		}
		if fd.Min != nil && n < *fd.Min {
			f(v.Path, report.SeverityError, report.RuleContentMin, fmt.Sprintf("%s must be at least %s (currently %s).", label, content.FormatNumber(*fd.Min), content.FormatNumber(n)), "")
		}
		if fd.Max != nil && n > *fd.Max {
			f(v.Path, report.SeverityError, report.RuleContentMax, fmt.Sprintf("%s must be at most %s (currently %s).", label, content.FormatNumber(*fd.Max), content.FormatNumber(n)), "")
		}
	case schema.TypeBoolean:
		if _, ok := v.Value.(bool); !ok {
			shape("boolean")
			return
		}
	case schema.TypeSelect:
		s, ok := v.Value.(string)
		if !ok {
			if sv := scalarText(v.Value); sv != "" {
				s = sv
			} else {
				shape("selectable value")
				return
			}
		}
		if s != "" && !contains(schema.OptionValues(fd.Options), s) {
			f(v.Path, report.SeverityError, report.RuleContentShape, fmt.Sprintf("%s has a value that's no longer one of the allowed options.", label), "Current value: "+truncate(s)+". Pick a valid option to clear this.")
		}
	case schema.TypeMultiChoice:
		list, ok := v.Value.([]any)
		if !ok {
			shape("list of selectable values")
			return
		}
		allowed := schema.OptionValues(fd.Options)
		var stray []string
		for _, item := range list {
			s, ok := item.(string)
			if !ok || !contains(allowed, s) {
				stray = append(stray, truncate(fmt.Sprint(item)))
			}
		}
		if len(stray) > 0 {
			f(v.Path, report.SeverityError, report.RuleContentShape, fmt.Sprintf("%s has %d value(s) no longer in the allowed options.", label, len(stray)), "Stray: "+strings.Join(stray, ", ")+". Re-pick from the available options.")
		}
		if fd.Min != nil && float64(len(list)) < *fd.Min {
			f(v.Path, report.SeverityError, report.RuleContentMinItems, fmt.Sprintf("%s needs at least %s selected.", label, content.FormatNumber(*fd.Min)), "")
		}
		if fd.Max != nil && float64(len(list)) > *fd.Max {
			f(v.Path, report.SeverityError, report.RuleContentMaxItems, fmt.Sprintf("%s accepts at most %s selected.", label, content.FormatNumber(*fd.Max)), "")
		}
	case schema.TypeGroup:
		if _, ok := v.Value.(map[string]any); !ok {
			shape("group of fields")
			return
		}
	case schema.TypeArray:
		list, ok := v.Value.([]any)
		if !ok {
			shape("list")
			return
		}
		checkArray(fd, label, list, v.Path, f)
	}

	// regex, length
	if s, ok := v.Value.(string); ok {
		if fd.Regex != "" && (t == schema.TypeText || t == schema.TypeParagraph || t == schema.TypeLink) && strings.TrimSpace(s) != "" {
			if re, err := regexp.Compile(fd.Regex); err == nil && !re.MatchString(s) {
				msg := fd.RegexMessage
				if msg == "" {
					msg = label + " doesn't match the expected format."
				}
				f(v.Path, report.SeverityError, report.RuleContentRegex, msg, "")
			}
		}
		if (t == schema.TypeText || t == schema.TypeParagraph || t == schema.TypeRichText) && s != "" {
			n := len([]rune(s))
			if fd.MinLength != nil && *fd.MinLength > 0 && n < *fd.MinLength {
				msg := fd.MinLengthMessage
				if msg == "" {
					msg = fmt.Sprintf("%s must be at least %d characters (currently %d).", label, *fd.MinLength, n)
				}
				f(v.Path, report.SeverityError, report.RuleContentMinLength, msg, "")
			}
			if fd.MaxLength != nil && *fd.MaxLength > 0 && n > *fd.MaxLength {
				msg := fd.MaxLengthMessage
				if msg == "" {
					msg = fmt.Sprintf("%s must be at most %d characters (currently %d).", label, *fd.MaxLength, n)
				}
				f(v.Path, report.SeverityError, report.RuleContentMaxLength, msg, "")
			}
		}
	}
}

func checkArray(fd schema.Field, label string, list []any, p string, f pageEmit) {
	if fd.MinItems != nil && len(list) < *fd.MinItems {
		msg := fd.MinItemsMessage
		if msg == "" {
			msg = fmt.Sprintf("%s needs at least %d %s.", label, *fd.MinItems, plural(*fd.MinItems, "item"))
		}
		f(p, report.SeverityError, report.RuleContentMinItems, msg, fmt.Sprintf("Add %d more.", *fd.MinItems-len(list)))
	}
	if fd.MaxItems != nil && len(list) > *fd.MaxItems {
		msg := fd.MaxItemsMessage
		if msg == "" {
			msg = fmt.Sprintf("%s accepts at most %d %s.", label, *fd.MaxItems, plural(*fd.MaxItems, "item"))
		}
		f(p, report.SeverityError, report.RuleContentMaxItems, msg, fmt.Sprintf("Remove %d.", len(list)-*fd.MaxItems))
	}

	// Discriminators of multi-shape arrays.
	if schema.IsMultiShape(fd) {
		entries := schema.ItemEntries(fd)
		var names []string
		anonymous := false
		for _, e := range entries {
			if schema.IsGroup(e.Field.Type) {
				names = append(names, e.Name)
				if strings.HasPrefix(e.Name, "__entry_") {
					anonymous = true
				}
			}
		}
		disc := schema.ArrayIDKey(fd)
		if !anonymous {
			for i, item := range list {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				tag, _ := m[disc].(string)
				itemPath := schema.IndexPath(p, i)
				if tag == "" {
					first := "none"
					if len(names) > 0 {
						first = names[0]
					}
					f(itemPath, report.SeverityWarning, report.RuleContentItemTypeMissing, fmt.Sprintf("Item %d of %s has no `%s:` discriminator.", i+1, label, disc), fmt.Sprintf("The editor falls back to the first shape (%s). Set `%s:` to one of: %s.", first, disc, strings.Join(names, ", ")))
					continue
				}
				if !contains(names, tag) {
					f(itemPath, report.SeverityError, report.RuleContentItemTypeUnknown, fmt.Sprintf("Item %d of %s has `%s: %s`, which isn't one of the allowed shapes.", i+1, label, disc, tag), "Allowed shapes: "+strings.Join(names, ", ")+". Fix the discriminator or add the shape to the schema.")
				}
			}
		}
	}

	// uniqueOn
	if fd.UniqueOn != "" {
		seen := map[string]int{}
		for i, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			val, _ := schema.ResolvePath(fd.UniqueOn, schema.RootScope(m))
			key := valueKey(val)
			if key == "" {
				continue
			}
			if first, dup := seen[key]; dup {
				msg := fd.UniqueOnMessage
				if msg == "" {
					msg = fmt.Sprintf("%s: item %d has the same `%s` as item %d.", label, i+1, fd.UniqueOn, first+1)
				}
				f(schema.IndexPath(p, i), report.SeverityError, report.RuleContentUnique, msg, fmt.Sprintf("Each item under %s must have a unique `%s`.", label, fd.UniqueOn))
				continue
			}
			seen[key] = i
		}
	}
}

// --- value helpers -------------------------------------------------------

func isBlank(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	}
	return false
}

func allStrings(list []any) bool {
	for _, item := range list {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) {
			return 0, false
		}
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func scalarText(v any) string {
	switch x := v.(type) {
	case float64:
		return content.FormatNumber(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}

func describe(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if math.IsNaN(x) {
			return "NaN"
		}
		return "number"
	}
	return fmt.Sprintf("%T", v)
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// valueKey renders a value for uniqueness comparison: typed prefixes
// for scalars, stable JSON for lists and maps, "" for blank.
func valueKey(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if strings.TrimSpace(x) == "" {
			return ""
		}
		return "s:" + x
	case float64:
		return "n:" + content.FormatNumber(x)
	case bool:
		return fmt.Sprintf("b:%v", x)
	}
	b, err := json.Marshal(v) // encoding/json sorts map keys
	if err != nil {
		return ""
	}
	return "j:" + string(b)
}
