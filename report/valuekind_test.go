package report

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Every ValueKind constant of valuekind.go is in the table, once, with
// a code a client can key on and English no other kind has.
func TestValueKindsAreTheConstants(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "valuekind.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[ValueKind]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil || vs.Type.(*ast.Ident).Name != "ValueKind" {
			return true
		}
		for i, name := range vs.Names {
			v, _ := strconv.Unquote(vs.Values[i].(*ast.BasicLit).Value)
			if prev, dup := consts[ValueKind(v)]; dup {
				t.Errorf("%s and %s are both %q", prev, name.Name, v)
			}
			consts[ValueKind(v)] = name.Name
		}
		return true
	})
	code := regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	english := map[string]ValueKind{}
	for _, k := range ValueKinds() {
		if _, ok := consts[k]; !ok {
			t.Errorf("value kind %q is not a constant in valuekind.go", k)
		}
		delete(consts, k)
		if !code.MatchString(string(k)) {
			t.Errorf("value kind %q is not a snake_case code", k)
		}
		w := k.English()
		if strings.TrimSpace(w) == "" {
			t.Errorf("value kind %q has no English", k)
		}
		if prev, dup := english[w]; dup {
			t.Errorf("%q and %q both read %q", prev, k, w)
		}
		english[w] = k
	}
	for k, name := range consts {
		t.Errorf("%s (%q) is not in the table of value kinds", name, k)
	}
}

// The English is the words 0.12.0 sent as the value itself, so a
// sentence reads as it did; a value that is no kind (stored by 0.12.0)
// is its own English.
func TestValueKindEnglish(t *testing.T) {
	for k, want := range map[ValueKind]string{
		ValueKindText:                   "text",
		ValueKindListOfReferences:       "list of references",
		ValueKindListOfSelectableValues: "list of selectable values",
		ValueKindGroupOfFields:          "group of fields",
		ValueKindNaN:                    "NaN",
		ValueKindOther:                  "value",
		"group of fields":               "group of fields",
		"":                              "",
	} {
		if got := k.English(); got != want {
			t.Errorf("%q reads %q, want %q", k, got, want)
		}
	}
	f := New("a.md", "title", RuleContentShape, CauseWrongType, &Params{Field: "Title", Expected: ValueKindGroupOfFields, Actual: ValueKindString})
	if f.Message != "Title should be a group of fields but is a string." {
		t.Errorf("message %q", f.Message)
	}
	old := f
	old.Params = &Params{Field: "Title", Expected: "group of fields", Actual: "string"}
	if m, _ := English(old); m != f.Message {
		t.Errorf("a finding stored by 0.12.0 reads %q", m)
	}
	if err := CheckFinding(old); err == nil || !strings.Contains(err.Error(), "none of its values") {
		t.Errorf("a stored finding with words for a kind keeps to its entry: %v", err)
	}
}

// The kind of every value a document can hold, and of the ones only a
// caller's own Go values hold.
func TestValueKindOf(t *testing.T) {
	cases := []struct {
		v    any
		want ValueKind
	}{
		{nil, ValueKindNull},
		{"", ValueKindString},
		{"12", ValueKindString},
		{true, ValueKindBoolean},
		{12.5, ValueKindNumber},
		{math.Inf(1), ValueKindNumber},
		{math.NaN(), ValueKindNaN},
		{float32(math.NaN()), ValueKindNaN},
		{float32(1), ValueKindNumber},
		{3, ValueKindNumber},
		{int64(3), ValueKindNumber},
		{uint8(3), ValueKindNumber},
		{[]any{"a"}, ValueKindList},
		{[]any{}, ValueKindList},
		{map[string]any{"k": 1.0}, ValueKindObject},
		{[]string{"a"}, ValueKindOther},
		{map[any]any{}, ValueKindOther},
		{struct{}{}, ValueKindOther},
	}
	for _, c := range cases {
		if got := ValueKindOf(c.v); got != c.want {
			t.Errorf("ValueKindOf(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}

// The params expected and actual, and no others, have a closed
// vocabulary: every value kind, in the order of ValueKinds, in
// ParamSpecs and in catalog.json, which clients are generated from.
func TestCatalogListsTheValueKinds(t *testing.T) {
	var kinds []string
	for _, k := range ValueKinds() {
		kinds = append(kinds, string(k))
	}
	// What a caller does with the list it gets changes nothing here.
	for _, s := range ParamSpecs() {
		if s.Key == ParamExpected {
			s.Values[0] = "changed"
		}
	}
	for _, s := range ParamSpecs() {
		switch s.Key {
		case ParamExpected, ParamActual:
			if !slices.Equal(s.Values, kinds) || s.Class != ParamClassVocabulary || s.Type != ParamTypeString {
				t.Errorf("%s: %s %s %v, want a vocabulary string of %v", s.Key, s.Class, s.Type, s.Values, kinds)
			}
		default:
			if s.Values != nil {
				t.Errorf("%s has values %v", s.Key, s.Values)
			}
		}
	}
	raw, err := os.ReadFile("catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Params []struct {
			Key    string   `json:"key"`
			Values []string `json:"values"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, p := range file.Params {
		if p.Values == nil {
			continue
		}
		listed++
		if (p.Key != string(ParamExpected) && p.Key != string(ParamActual)) || !slices.Equal(p.Values, kinds) {
			t.Errorf("catalog.json lists %s: %v, want the value kinds %v", p.Key, p.Values, kinds)
		}
	}
	if listed != 2 {
		t.Errorf("catalog.json lists the values of %d params, want expected and actual", listed)
	}
}

// Every cause that names value kinds words each kind, and refuses a
// value that is none.
func TestValueKindsInEveryCause(t *testing.T) {
	seen := 0
	for _, s := range Catalog() {
		all := append(slices.Clone(s.Params), s.Optional...)
		var keys []ParamKey
		for _, k := range all {
			if k == ParamExpected || k == ParamActual {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			continue
		}
		seen++
		for _, k := range ValueKinds() {
			p := sample(all)
			for _, key := range keys {
				if key == ParamExpected {
					p.Expected = k
				} else {
					p.Actual = k
				}
			}
			f := New("a.md", "x", s.Rule, s.Cause, p)
			if err := CheckFinding(f); err != nil {
				t.Errorf("%s %q with %q: %v", s.Rule, s.Cause, k, err)
			}
			for _, key := range keys {
				if slices.Contains(s.Params, key) && !strings.Contains(f.Message, " "+k.English()+" ") && !strings.Contains(f.Message, " "+k.English()+".") {
					t.Errorf("%s %q: %s %q is not in %q", s.Rule, s.Cause, key, k.English(), f.Message)
				}
			}
		}
		p := sample(all)
		p.Actual = "s-actual"
		if err := CheckFinding(New("a.md", "x", s.Rule, s.Cause, p)); err == nil {
			t.Errorf("%s %q: a value that is no kind keeps to the entry", s.Rule, s.Cause)
		}
	}
	if seen != 2 {
		t.Errorf("%d causes name value kinds, want content.shape wrong_type and schema.default_not_option", seen)
	}
}
