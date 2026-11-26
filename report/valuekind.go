package report

import "math"

// ValueKind is a kind of value a finding names: the kind a field's value
// must have (Params.Expected) and the kind the value has
// (Params.Actual). Its values are codes, frozen like causes: never
// renamed, never reused for another kind. English gives the words this
// module's sentences use; a client that words findings itself words
// each kind (ValueKinds lists them, and catalog.json lists them as the
// values of the params expected and actual).
type ValueKind string

// Value kinds.
const (
	// The kinds a field's value must have. ValueKindText is the value of
	// the text-like fields (text, paragraph, richtext, link, date, image,
	// file, color); the rest name their field type's value.
	ValueKindText                   ValueKind = "text"
	ValueKindReference              ValueKind = "reference"
	ValueKindListOfReferences       ValueKind = "list_of_references"
	ValueKindNumber                 ValueKind = "number"
	ValueKindBoolean                ValueKind = "boolean"
	ValueKindSelectableValue        ValueKind = "selectable_value"
	ValueKindListOfSelectableValues ValueKind = "list_of_selectable_values"
	ValueKindGroupOfFields          ValueKind = "group_of_fields"
	ValueKindList                   ValueKind = "list"

	// The other kinds a value has (ValueKindOf).
	ValueKindString ValueKind = "string"
	ValueKindObject ValueKind = "object"
	ValueKindNull   ValueKind = "null"
	// ValueKindNaN is a float that is not a number. Only values a caller
	// builds itself hold one: a file's `.nan` is read as text.
	ValueKindNaN ValueKind = "nan"
	// ValueKindOther is a value of a type no YAML or JSON document
	// produces (a caller's own Go value).
	ValueKindOther ValueKind = "other"
)

// valueKinds is every kind with the words this module's sentences use
// for it, in the order ValueKinds lists them: the one place a kind is
// declared.
var valueKinds = []struct {
	kind    ValueKind
	english string
}{
	{ValueKindText, "text"},
	{ValueKindReference, "reference"},
	{ValueKindListOfReferences, "list of references"},
	{ValueKindNumber, "number"},
	{ValueKindBoolean, "boolean"},
	{ValueKindSelectableValue, "selectable value"},
	{ValueKindListOfSelectableValues, "list of selectable values"},
	{ValueKindGroupOfFields, "group of fields"},
	{ValueKindList, "list"},
	{ValueKindString, "string"},
	{ValueKindObject, "object"},
	{ValueKindNull, "null"},
	{ValueKindNaN, "NaN"},
	{ValueKindOther, "value"},
}

// ValueKinds lists every value kind.
func ValueKinds() []ValueKind {
	out := make([]ValueKind, len(valueKinds))
	for i, k := range valueKinds {
		out[i] = k.kind
	}
	return out
}

// English is the kind in this module's sentences: "group of fields". A
// value that is no kind of this version is returned as it is, so a
// finding stored by an older version, which held these words, reads as
// it did.
func (k ValueKind) English() string {
	for _, v := range valueKinds {
		if v.kind == k {
			return v.english
		}
	}
	return string(k)
}

// ValueKindOf is the kind of a value as content.ParseYAMLMap and
// encoding/json give them: text, a number, a boolean, null, a list, a
// mapping. Numbers of any Go type are ValueKindNumber; a type neither
// produces is ValueKindOther.
func ValueKindOf(v any) ValueKind {
	switch x := v.(type) {
	case nil:
		return ValueKindNull
	case string:
		return ValueKindString
	case bool:
		return ValueKindBoolean
	case float64:
		if math.IsNaN(x) {
			return ValueKindNaN
		}
		return ValueKindNumber
	case float32:
		if math.IsNaN(float64(x)) {
			return ValueKindNaN
		}
		return ValueKindNumber
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return ValueKindNumber
	case []any:
		return ValueKindList
	case map[string]any:
		return ValueKindObject
	}
	return ValueKindOther
}
