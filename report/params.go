package report

import (
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Params are the facts a finding's sentence is made of, typed, so a
// client can word the finding in any language without parsing English.
// Which keys a rule and cause set is in Catalog(); unset keys are
// omitted. Numbers are numbers (bytes, counts, bounds), lists are lists,
// never a joined or pluralized English text.
//
// The class of each key says what a client does with it:
//
//   - customer: text the site wrote (a field's label, a value, an
//     option). Shown as written, never translated. It may be empty
//     where the site wrote nothing, so a client treats a missing
//     customer key as empty text.
//   - vocabulary: one of a fixed set of words of this module (field
//     types, value kinds, roles, schema keys, frameworks). A client may
//     translate it. Where the set is closed, ParamSpecs lists it (the
//     value kinds of Expected and Actual).
//   - code: a language code.
//   - derived: a repository path, URL or schema path, shown as is.
//   - number: a count, bound, size or position.
//   - technical: a parser's, the regular expression engine's or the
//     runtime's own error text. Shown as technical detail, never
//     translated.
//
// Every string is at most MaxParamRunes characters; Names lists at most MaxParamNames entries, and More
// counts the rest.
type Params struct {
	// Field is the field's label as the schema writes it, else its key.
	Field string `json:"field,omitempty" class:"customer"`
	// Key is another schema key the finding names: an option, a sibling,
	// a block, a storage, a translation key, a discriminator.
	Key string `json:"key,omitempty" class:"customer"`
	// Value is a value as written: text as is, anything else in YAML flow
	// style (`null`, `[a, b]`, `{k: v}`).
	Value string `json:"value,omitempty" class:"customer"`
	// Names lists names: item shapes, keys, collections, tokens, stray
	// options, other files, texts.
	Names []string `json:"names,omitempty" class:"customer"`
	// More is how many names there are beyond Names.
	More *int64 `json:"more,omitempty" class:"number"`
	// SameAs parallels Names on quality.translation_incomplete: the
	// language a text is the same as, "" where it is empty.
	SameAs []string `json:"sameAs,omitempty" class:"code"`
	// Type is a field type (`text`, `richtext`); OtherType is the field
	// type it is compared with.
	Type      string `json:"type,omitempty" class:"vocabulary"`
	OtherType string `json:"otherType,omitempty" class:"vocabulary"`
	// Expected and Actual are value kinds: the kind a value must have and
	// the kind it has (`group_of_fields`, `string`).
	Expected ValueKind `json:"expected,omitempty" class:"vocabulary"`
	Actual   ValueKind `json:"actual,omitempty" class:"vocabulary"`
	// Knob is the schema key concerned (`regex`, `visibleIf`,
	// `noindexWhen`, `customValidators[0]`).
	Knob string `json:"knob,omitempty" class:"vocabulary"`
	// Role is a field role.
	Role string `json:"role,omitempty" class:"vocabulary"`
	// Kind is a fact kind (package fact).
	Kind string `json:"kind,omitempty" class:"vocabulary"`
	// Framework is the build framework a guess came from.
	Framework string `json:"framework,omitempty" class:"vocabulary"`
	// Allowed lists the values that would be accepted.
	Allowed []string `json:"allowed,omitempty" class:"customer"`
	// Locale and Locales are language codes.
	Locale  string   `json:"locale,omitempty" class:"code"`
	Locales []string `json:"locales,omitempty" class:"code"`
	// Path is a schema or repository path, a URL or a link as written.
	Path string `json:"path,omitempty" class:"derived"`
	// Other is the other file, field or place the finding compares with.
	Other string `json:"other,omitempty" class:"derived"`
	// Min, Max and Current are bounds and the measured value.
	Min     *float64 `json:"min,omitempty" class:"number"`
	Max     *float64 `json:"max,omitempty" class:"number"`
	Current *float64 `json:"current,omitempty" class:"number"`
	// Count and Total are counts; Index and OtherIndex 1-based item
	// numbers; Offset a 0-based byte position in a text.
	Count      *int64 `json:"count,omitempty" class:"number"`
	Total      *int64 `json:"total,omitempty" class:"number"`
	Index      *int64 `json:"index,omitempty" class:"number"`
	OtherIndex *int64 `json:"otherIndex,omitempty" class:"number"`
	Offset     *int64 `json:"offset,omitempty" class:"number"`
	// Level and ExpectedLevel are heading levels.
	Level         *int64 `json:"level,omitempty" class:"number"`
	ExpectedLevel *int64 `json:"expectedLevel,omitempty" class:"number"`
	// Bytes and LimitBytes are sizes in bytes.
	Bytes      *int64 `json:"bytes,omitempty" class:"number"`
	LimitBytes *int64 `json:"limitBytes,omitempty" class:"number"`
	// CustomMessage is the schema's own message, which replaces this
	// module's sentence (requiredMessage, regexMessage, a custom
	// validator's message…).
	CustomMessage string `json:"customMessage,omitempty" class:"customer"`
	// Detail is a parser's, the regexp engine's or the runtime's error
	// text.
	Detail string `json:"detail,omitempty" class:"technical"`
}

// ParamKey names one key of Params, as it is spelled in JSON.
type ParamKey string

// Param keys.
const (
	ParamField         ParamKey = "field"
	ParamSchemaKey     ParamKey = "key"
	ParamValue         ParamKey = "value"
	ParamNames         ParamKey = "names"
	ParamMore          ParamKey = "more"
	ParamSameAs        ParamKey = "sameAs"
	ParamType          ParamKey = "type"
	ParamOtherType     ParamKey = "otherType"
	ParamExpected      ParamKey = "expected"
	ParamActual        ParamKey = "actual"
	ParamKnob          ParamKey = "knob"
	ParamRole          ParamKey = "role"
	ParamKind          ParamKey = "kind"
	ParamFramework     ParamKey = "framework"
	ParamAllowed       ParamKey = "allowed"
	ParamLocale        ParamKey = "locale"
	ParamLocales       ParamKey = "locales"
	ParamPath          ParamKey = "path"
	ParamOther         ParamKey = "other"
	ParamMin           ParamKey = "min"
	ParamMax           ParamKey = "max"
	ParamCurrent       ParamKey = "current"
	ParamCount         ParamKey = "count"
	ParamTotal         ParamKey = "total"
	ParamIndex         ParamKey = "index"
	ParamOtherIndex    ParamKey = "otherIndex"
	ParamOffset        ParamKey = "offset"
	ParamLevel         ParamKey = "level"
	ParamExpectedLevel ParamKey = "expectedLevel"
	ParamBytes         ParamKey = "bytes"
	ParamLimitBytes    ParamKey = "limitBytes"
	ParamCustomMessage ParamKey = "customMessage"
	ParamDetail        ParamKey = "detail"
)

// Classes of a param key (see Params).
const (
	ParamClassCustomer   = "customer"
	ParamClassVocabulary = "vocabulary"
	ParamClassCode       = "code"
	ParamClassDerived    = "derived"
	ParamClassNumber     = "number"
	ParamClassTechnical  = "technical"
)

// Types of a param key's value, as JSON Schema names them.
const (
	ParamTypeString     = "string"
	ParamTypeStringList = "string[]"
	ParamTypeNumber     = "number"
	ParamTypeInteger    = "integer"
)

// Bounds of Params.
const (
	// MaxParamRunes bounds each string.
	MaxParamRunes = 300
	// MaxParamNames bounds Names (and SameAs with it); More counts the
	// rest.
	MaxParamNames = 10
	// MaxParamList bounds Allowed and Locales.
	MaxParamList = 50
)

// ParamSpec describes one key of Params.
type ParamSpec struct {
	Key   ParamKey `json:"key"`
	Type  string   `json:"type"`
	Class string   `json:"class"`
	// Values are every value a vocabulary key takes, for a key whose
	// vocabulary is closed (ValueKinds for Expected and Actual); a value
	// outside them breaks the key's declaration (CheckParams).
	Values []string `json:"values,omitempty"`
	// field is the index of the key's field in Params.
	field int
}

// paramSpecs is read from the Params struct itself, so the list of keys
// can never drift from the type, nor a closed vocabulary from its enum.
var paramSpecs = func() []ParamSpec {
	t := reflect.TypeOf(Params{})
	out := make([]ParamSpec, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		var typ string
		var values []string
		switch f.Type {
		case reflect.TypeFor[string]():
			typ = ParamTypeString
		case reflect.TypeFor[ValueKind]():
			typ = ParamTypeString
			for _, k := range ValueKinds() {
				values = append(values, string(k))
			}
		case reflect.TypeFor[[]string]():
			typ = ParamTypeStringList
		case reflect.TypeFor[*float64]():
			typ = ParamTypeNumber
		case reflect.TypeFor[*int64]():
			typ = ParamTypeInteger
		default:
			panic("report: Params." + f.Name + " has an unsupported type " + f.Type.String())
		}
		out = append(out, ParamSpec{Key: ParamKey(name), Type: typ, Class: f.Tag.Get("class"), Values: values, field: i})
	}
	return out
}()

// ParamSpecs lists every key of Params, in the order of the struct.
func ParamSpecs() []ParamSpec {
	out := append([]ParamSpec(nil), paramSpecs...)
	for i := range out {
		out[i].Values = append([]string(nil), out[i].Values...)
	}
	return out
}

func paramSpec(k ParamKey) (ParamSpec, bool) {
	for _, s := range paramSpecs {
		if s.Key == k {
			return s, true
		}
	}
	return ParamSpec{}, false
}

// Keys lists the keys p sets, in the order of ParamSpecs. A string is
// set when it is not empty, a list when it has entries, a number when it
// is there.
func (p *Params) Keys() []ParamKey {
	if p == nil {
		return nil
	}
	v := reflect.ValueOf(p).Elem()
	var out []ParamKey
	for _, s := range paramSpecs {
		f := v.Field(s.field)
		if f.Kind() == reflect.Slice && f.Len() > 0 || f.Kind() != reflect.Slice && !f.IsZero() {
			out = append(out, s.Key)
		}
	}
	return out
}

// Bounded is p with its strings and lists held to the bounds, nil when
// it sets nothing.
func (p *Params) Bounded() *Params {
	if p == nil {
		return nil
	}
	q := *p
	v := reflect.ValueOf(&q).Elem()
	for _, s := range paramSpecs {
		f := v.Field(s.field)
		switch {
		case s.Type == ParamTypeString:
			f.SetString(cutRunes(f.String(), MaxParamRunes))
		case s.Type == ParamTypeStringList && f.Len() > 0:
			list := append([]string(nil), f.Interface().([]string)...)
			for i := range list {
				list[i] = cutRunes(list[i], MaxParamRunes)
			}
			f.Set(reflect.ValueOf(list))
		}
	}
	if n := len(q.Names); n > MaxParamNames {
		q.More = Int(int64(n-MaxParamNames) + deref(q.More))
		q.Names = q.Names[:MaxParamNames]
		if len(q.SameAs) > MaxParamNames {
			q.SameAs = q.SameAs[:MaxParamNames]
		}
	}
	if len(q.Allowed) > MaxParamList {
		q.Allowed = q.Allowed[:MaxParamList]
	}
	if len(q.Locales) > MaxParamList {
		q.Locales = q.Locales[:MaxParamList]
	}
	if len(q.Keys()) == 0 {
		return nil
	}
	return &q
}

func deref(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

// Int is a pointer to n, for the number keys of Params.
func Int(n int64) *int64 { return &n }

// Num is a pointer to n, for Min, Max and Current.
func Num(n float64) *float64 { return &n }

// cutRunes cuts s to n characters, marking a cut with "…".
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// FlowValue renders a YAML value for Params.Value: text as is, null,
// booleans and numbers as YAML writes them, lists and mappings in flow
// style with keys sorted.
func FlowValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return FormatFloat(x)
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = FlowValue(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		return "[" + strings.Join(x, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + FlowValue(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return strconv.Quote(reflect.TypeOf(v).String())
}

// FormatFloat renders a float as Go's %v does (the shortest form that
// reads back the same).
func FormatFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

// FormatNumber renders a number without an exponent: a whole number as
// an integer, else the shortest decimal.
func FormatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
