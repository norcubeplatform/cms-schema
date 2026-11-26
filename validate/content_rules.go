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
	f := func(path string, rule report.Rule, cause report.Cause, p *report.Params) {
		out = append(out, report.New(doc.Path, path, rule, cause, p))
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
				f("", report.RuleContentExtraKeys, report.CauseStrippedOnSave, &report.Params{Names: extra})
			default:
				f("", report.RuleContentExtraKeys, report.CauseNotInSchema, &report.Params{Names: extra})
			}
		}
	}
	if !coll.HasBody && !doc.IsData && strings.TrimSpace(doc.Body) != "" {
		f("", report.RuleContentUnexpectedBody, "", nil)
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

// pageEmit records a finding of a rule and cause in the page.
type pageEmit func(path string, rule report.Rule, cause report.Cause, p *report.Params)

func checkValue(cfg *schema.Config, v schema.Visit, f pageEmit) {
	fd := v.Field
	label := fd.Label
	if label == "" {
		label = fd.Key
	}
	t := schema.Canonical(fd.Type)

	// required
	if fd.Required && isBlank(v.Value) {
		f(v.Path, report.RuleContentRequired, "", &report.Params{Field: label, CustomMessage: fd.RequiredMessage})
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
				f(v.Path, report.RuleContentRegex, report.CauseCustomValidator, &report.Params{Field: label, Knob: fmt.Sprintf("customValidators[%d]", i), CustomMessage: cv.Message})
			}
		case schema.CustomValidatorExpression:
			cond, ok := cv.Params["condition"].(map[string]any)
			if !ok {
				continue
			}
			if !schema.Eval(parseCond(cond), v.Scope) {
				f(v.Path, report.RuleContentCustom, "", &report.Params{Field: label, Knob: fmt.Sprintf("customValidators[%d]", i), CustomMessage: cv.Message})
			}
		}
	}

	if v.Value == nil {
		return
	}

	// shape
	shape := func(expected report.ValueKind) {
		f(v.Path, report.RuleContentShape, report.CauseWrongType, &report.Params{Field: label, Expected: expected, Actual: report.ValueKindOf(v.Value)})
	}
	switch t {
	case schema.TypeText, schema.TypeParagraph, schema.TypeRichText, schema.TypeLink, schema.TypeDate, schema.TypeImage, schema.TypeFile, schema.TypeColor:
		if _, ok := v.Value.(string); !ok {
			shape(report.ValueKindText)
			return
		}
	case schema.TypeReference:
		if fd.Multiple {
			list, ok := v.Value.([]any)
			if !ok || !allStrings(list) {
				shape(report.ValueKindListOfReferences)
				return
			}
		} else if _, ok := v.Value.(string); !ok {
			shape(report.ValueKindReference)
			return
		}
	case schema.TypeNumber, schema.TypeRange:
		n, ok := numberOf(v.Value)
		if !ok {
			shape(report.ValueKindNumber)
			return
		}
		if fd.Min != nil && n < *fd.Min {
			f(v.Path, report.RuleContentMin, "", &report.Params{Field: label, Min: fd.Min, Current: report.Num(n)})
		}
		if fd.Max != nil && n > *fd.Max {
			f(v.Path, report.RuleContentMax, "", &report.Params{Field: label, Max: fd.Max, Current: report.Num(n)})
		}
	case schema.TypeBoolean:
		if _, ok := v.Value.(bool); !ok {
			shape(report.ValueKindBoolean)
			return
		}
	case schema.TypeSelect:
		s, ok := v.Value.(string)
		if !ok {
			if sv := scalarText(v.Value); sv != "" {
				s = sv
			} else {
				shape(report.ValueKindSelectableValue)
				return
			}
		}
		if s != "" && !contains(schema.OptionValues(fd.Options), s) {
			f(v.Path, report.RuleContentShape, report.CauseOptionGone, &report.Params{Field: label, Value: s})
		}
	case schema.TypeMultiChoice:
		list, ok := v.Value.([]any)
		if !ok {
			shape(report.ValueKindListOfSelectableValues)
			return
		}
		allowed := schema.OptionValues(fd.Options)
		var stray []string
		for _, item := range list {
			s, ok := item.(string)
			if !ok || !contains(allowed, s) {
				stray = append(stray, report.FlowValue(item))
			}
		}
		if len(stray) > 0 {
			f(v.Path, report.RuleContentShape, report.CauseOptionsGone, &report.Params{Field: label, Count: report.Int(int64(len(stray))), Names: stray})
		}
		if fd.Min != nil && float64(len(list)) < *fd.Min {
			f(v.Path, report.RuleContentMinItems, report.CauseSelected, &report.Params{Field: label, Min: fd.Min})
		}
		if fd.Max != nil && float64(len(list)) > *fd.Max {
			f(v.Path, report.RuleContentMaxItems, report.CauseSelected, &report.Params{Field: label, Max: fd.Max})
		}
	case schema.TypeGroup:
		if _, ok := v.Value.(map[string]any); !ok {
			shape(report.ValueKindGroupOfFields)
			return
		}
	case schema.TypeArray:
		list, ok := v.Value.([]any)
		if !ok {
			shape(report.ValueKindList)
			return
		}
		checkArray(fd, label, list, v.Path, f)
	}

	// regex, length
	if s, ok := v.Value.(string); ok {
		if fd.Regex != "" && (t == schema.TypeText || t == schema.TypeParagraph || t == schema.TypeLink) && strings.TrimSpace(s) != "" {
			if re, err := regexp.Compile(fd.Regex); err == nil && !re.MatchString(s) {
				f(v.Path, report.RuleContentRegex, report.CauseRegex, &report.Params{Field: label, CustomMessage: fd.RegexMessage})
			}
		}
		if (t == schema.TypeText || t == schema.TypeParagraph || t == schema.TypeRichText) && s != "" {
			n := len([]rune(s))
			if fd.MinLength != nil && *fd.MinLength > 0 && n < *fd.MinLength {
				f(v.Path, report.RuleContentMinLength, "", &report.Params{Field: label, Min: report.Num(float64(*fd.MinLength)), Current: report.Num(float64(n)), CustomMessage: fd.MinLengthMessage})
			}
			if fd.MaxLength != nil && *fd.MaxLength > 0 && n > *fd.MaxLength {
				f(v.Path, report.RuleContentMaxLength, "", &report.Params{Field: label, Max: report.Num(float64(*fd.MaxLength)), Current: report.Num(float64(n)), CustomMessage: fd.MaxLengthMessage})
			}
		}
	}
}

func checkArray(fd schema.Field, label string, list []any, p string, f pageEmit) {
	if fd.MinItems != nil && len(list) < *fd.MinItems {
		f(p, report.RuleContentMinItems, report.CauseItems, &report.Params{Field: label, Min: report.Num(float64(*fd.MinItems)), Current: report.Num(float64(len(list))), CustomMessage: fd.MinItemsMessage})
	}
	if fd.MaxItems != nil && len(list) > *fd.MaxItems {
		f(p, report.RuleContentMaxItems, report.CauseItems, &report.Params{Field: label, Max: report.Num(float64(*fd.MaxItems)), Current: report.Num(float64(len(list))), CustomMessage: fd.MaxItemsMessage})
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
					f(itemPath, report.RuleContentItemTypeMissing, "", &report.Params{Index: report.Int(int64(i + 1)), Field: label, Key: disc, Names: names})
					continue
				}
				if !contains(names, tag) {
					f(itemPath, report.RuleContentItemTypeUnknown, "", &report.Params{Index: report.Int(int64(i + 1)), Field: label, Key: disc, Value: tag, Names: names})
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
				f(schema.IndexPath(p, i), report.RuleContentUnique, "", &report.Params{Field: label, Index: report.Int(int64(i + 1)), OtherIndex: report.Int(int64(first + 1)), Key: fd.UniqueOn, CustomMessage: fd.UniqueOnMessage})
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
