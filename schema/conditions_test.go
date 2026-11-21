package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/norcubeplatform/cms-schema/schema"
)

var allOps = []string{
	schema.OpEq, schema.OpNeq, schema.OpIn, schema.OpNotIn,
	schema.OpTruthy, schema.OpFalsy, schema.OpEmpty, schema.OpNonEmpty,
	schema.OpGt, schema.OpLt, schema.OpMatches, "", "unknown",
}

// evalOn evaluates one condition on a document whose field `v` holds
// actual (absent when actual is the absent marker).
func evalOn(op string, actual, expected any) bool {
	fm := map[string]any{}
	if actual != absent {
		fm["v"] = actual
	}
	return schema.Eval(schema.Condition{Field: "v", Op: op, Value: expected}, schema.RootScope(fm))
}

type absentMarker struct{}

var absent = absentMarker{}

// TestConditionsOnListsAndMaps pins what eq, neq, in and notIn mean when
// either side is a list or a map. Scalars keep their meaning (see
// TestConditions).
func TestConditionsOnListsAndMaps(t *testing.T) {
	gallery := []any{"a.jpg", "b.jpg"}
	cases := []struct {
		name     string
		op       string
		actual   any
		expected any
		want     bool
	}{
		// The reported case: it used to panic.
		{"eq empty list on a list", schema.OpEq, []any{"a.jpg"}, []any{}, false},
		{"eq empty list on an empty list", schema.OpEq, []any{}, []any{}, true},
		{"eq empty map on a map", schema.OpEq, map[string]any{"k": "v"}, map[string]any{}, false},
		{"eq empty map on an empty map", schema.OpEq, map[string]any{}, map[string]any{}, true},

		// eq compares the whole list, in order.
		{"eq same list", schema.OpEq, gallery, []any{"a.jpg", "b.jpg"}, true},
		{"eq other order", schema.OpEq, gallery, []any{"b.jpg", "a.jpg"}, false},
		{"eq prefix", schema.OpEq, gallery, []any{"a.jpg"}, false},
		{"eq numbers across kinds", schema.OpEq, []any{float64(1), float64(2)}, []any{1, int64(2)}, true},
		{"eq []string against []any", schema.OpEq, []any{"a"}, []string{"a"}, true},
		{"eq nested", schema.OpEq, []any{map[string]any{"src": "a.jpg", "n": float64(1)}}, []any{map[string]any{"n": 1, "src": "a.jpg"}}, true},
		{"eq nested differs", schema.OpEq, []any{map[string]any{"src": "a.jpg"}}, []any{map[string]any{"src": "b.jpg"}}, false},
		{"eq map key sets differ", schema.OpEq, map[string]any{"a": nil}, map[string]any{"b": nil}, false},
		{"eq map with a nil value", schema.OpEq, map[string]any{"a": nil}, map[string]any{"a": nil}, true},
		{"neq list", schema.OpNeq, gallery, []any{}, true},
		{"neq same list", schema.OpNeq, gallery, []any{"a.jpg", "b.jpg"}, false},

		// A list never equals a scalar or a map.
		{"eq list vs string", schema.OpEq, []any{"a"}, "a", false},
		{"eq string vs list", schema.OpEq, "a", []any{"a"}, false},
		{"eq list vs map", schema.OpEq, []any{}, map[string]any{}, false},

		// nil (or absent) equals nil only, never an empty list or map.
		{"eq empty list on absent", schema.OpEq, absent, []any{}, false},
		{"eq empty list on null", schema.OpEq, nil, []any{}, false},
		{"eq empty map on absent", schema.OpEq, absent, map[string]any{}, false},
		{"eq null on an empty list", schema.OpEq, []any{}, nil, false},
		{"neq empty list on absent", schema.OpNeq, absent, []any{}, true},

		// in tests the field's whole value against each listed value.
		{"in a list of lists", schema.OpIn, []any{"a"}, []any{[]any{"b"}, []any{"a"}}, true},
		{"in a list of lists, no match", schema.OpIn, []any{"a", "b"}, []any{[]any{"a"}, []any{"b"}}, false},
		{"in on a list field is not overlap", schema.OpIn, []any{"a"}, []any{"a", "b"}, false},
		{"in with list items", schema.OpIn, "a", []any{[]any{"a"}, "a"}, true},
		{"in a list of maps", schema.OpIn, map[string]any{"k": float64(1)}, []any{map[string]any{"k": 1}}, true},
		{"notIn a list of lists", schema.OpNotIn, []any{"a"}, []any{[]any{"a"}}, false},
		{"notIn on a list field", schema.OpNotIn, []any{"a"}, []any{"a"}, true},
		{"in with a map as the list", schema.OpIn, "a", map[string]any{"a": "a"}, false},
		{"notIn with a map as the list", schema.OpNotIn, "a", map[string]any{"a": "a"}, false},

		// Other value types never panic.
		{"eq uncomparable map type", schema.OpEq, map[any]any{"a": []any{}}, map[any]any{"a": []any{}}, true},
		{"eq uncomparable map type differs", schema.OpEq, map[any]any{"a": 1}, map[any]any{"a": 2}, false},
		{"eq func values", schema.OpEq, func() {}, func() {}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalOn(tc.op, tc.actual, tc.expected); got != tc.want {
				t.Errorf("%s %v %v: got %v, want %v", tc.op, tc.actual, tc.expected, got, tc.want)
			}
		})
	}
}

// TestConditionsOnCyclicValues: a value that contains itself is bounded
// by MaxCompareDepth instead of overflowing the stack.
func TestConditionsOnCyclicValues(t *testing.T) {
	list := []any{nil}
	list[0] = list
	m := map[string]any{}
	m["self"] = m
	for _, v := range []any{list, m} {
		for _, op := range allOps {
			evalOn(op, v, v)
		}
	}
	deep := any("leaf")
	for i := 0; i < schema.MaxCompareDepth+5; i++ {
		deep = []any{deep}
	}
	if evalOn(schema.OpEq, deep, deep) {
		t.Error("values nested deeper than MaxCompareDepth compare unequal")
	}
}

// TestConditionsFromYAML runs the reported schema through ParseField, as
// the loader does.
func TestConditionsFromYAML(t *testing.T) {
	for _, cond := range []map[string]any{
		{"field": "gallery", "op": "eq", "value": []any{}},
		{"field": "gallery", "equals": []any{}},
		{"field": "gallery", "op": "neq", "value": map[string]any{}},
		{"field": "gallery", "op": "in", "value": []any{[]any{}, map[string]any{}}},
	} {
		f := schema.ParseField(map[string]any{"key": "caption", "type": "text", "label": "Caption", "visibleIf": cond})
		if f.VisibleIf == nil {
			t.Fatalf("no condition parsed from %v", cond)
		}
		schema.Eval(*f.VisibleIf, schema.RootScope(map[string]any{"gallery": []any{"a.jpg"}}))
		schema.Eval(*f.VisibleIf, schema.RootScope(map[string]any{"gallery": map[string]any{"a": "b"}}))
	}
}

// FuzzEval: no condition over any JSON values panics, and eq is
// symmetric.
func FuzzEval(f *testing.F) {
	f.Add("eq", `["a.jpg"]`, `[]`)
	f.Add("eq", `{"k":"v"}`, `{}`)
	f.Add("in", `["a"]`, `[["a"],{"k":1}]`)
	f.Add("neq", `[1,[2,{"a":[3]}]]`, `[1.0,[2,{"a":[3]}]]`)
	f.Add("notIn", `null`, `"a, b"`)
	f.Add("matches", `"abc"`, `"^a"`)
	f.Add("gt", `3`, `2`)
	f.Fuzz(evalJSON)
}

// evalJSON is one fuzz input: every operator over the two decoded
// values, both ways round, and through all, any and not.
func evalJSON(t *testing.T, op, actualJSON, expectedJSON string) {
	var actual, expected any
	if json.Unmarshal([]byte(actualJSON), &actual) != nil {
		return
	}
	if json.Unmarshal([]byte(expectedJSON), &expected) != nil {
		return
	}
	ops := append([]string{op}, allOps...)
	for _, o := range ops {
		evalOn(o, actual, expected)
		evalOn(o, expected, actual)
		evalOn(o, actual, actual)
	}
	// Through the combinators and nested scopes as well.
	c := schema.Condition{All: []schema.Condition{
		{Any: []schema.Condition{{Field: "v", Op: op, Value: expected}, {Field: "$root.v.0", Op: schema.OpIn, Value: expected}}},
		{Not: &schema.Condition{Field: "$parent.v", Op: schema.OpEq, Value: actual}},
	}}
	root := map[string]any{"v": actual}
	schema.Eval(c, schema.RootScope(root))
	schema.Eval(c, schema.RootScope(root).Child(map[string]any{"v": expected}))
	if evalOn(schema.OpEq, actual, expected) != evalOn(schema.OpEq, expected, actual) {
		t.Errorf("eq is not symmetric for %s and %s", actualJSON, expectedJSON)
	}
}
