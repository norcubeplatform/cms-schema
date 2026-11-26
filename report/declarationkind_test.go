package report

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// codePattern is what an enum value of this package looks like: a
// lowercase snake_case code a client can key on.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// enumConstants reads the constants of type typeName declared in file:
// each value with its constant's name. Two constants with one value are
// an error.
func enumConstants(t *testing.T, file, typeName string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			return true
		}
		if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != typeName {
			return true
		}
		for i, name := range vs.Names {
			v, _ := strconv.Unquote(vs.Values[i].(*ast.BasicLit).Value)
			if prev, dup := consts[v]; dup {
				t.Errorf("%s and %s are both %q", prev, name.Name, v)
			}
			consts[v] = name.Name
		}
		return true
	})
	return consts
}

// Every DeclarationKind constant of declarationkind.go is in the table,
// once, with a code a client can key on and English no other kind has.
func TestDeclarationKindsAreTheConstants(t *testing.T) {
	consts := enumConstants(t, "declarationkind.go", "DeclarationKind")
	if len(consts) == 0 {
		t.Fatal("no DeclarationKind constants")
	}
	english := map[string]DeclarationKind{}
	for _, k := range DeclarationKinds() {
		if _, ok := consts[string(k)]; !ok {
			t.Errorf("declaration kind %q is not a constant in declarationkind.go", k)
		}
		delete(consts, string(k))
		if !codePattern.MatchString(string(k)) {
			t.Errorf("declaration kind %q is not a snake_case code", k)
		}
		w := k.English()
		if strings.TrimSpace(w) == "" {
			t.Errorf("declaration kind %q has no English", k)
		}
		if prev, dup := english[w]; dup {
			t.Errorf("%q and %q both read %q", prev, k, w)
		}
		english[w] = k
	}
	for k, name := range consts {
		t.Errorf("%s (%q) is not in the table of declaration kinds", name, k)
	}
}

// The English is the words 0.12.1 sent in kind, so a sentence reads as
// it did; a value that is no kind is its own English.
func TestDeclarationKindEnglish(t *testing.T) {
	for k, want := range map[DeclarationKind]string{
		DeclarationKindCollection: "collection",
		DeclarationKindBlock:      "block",
		DeclarationKindInput:      "input",
		"storage":                 "storage",
		"":                        "",
	} {
		if got := k.English(); got != want {
			t.Errorf("%q reads %q, want %q", k, got, want)
		}
	}
}

// A declarationKind is one of the kinds: CheckParams takes each and
// refuses any other value, the 0.12.1 English words of an unknown one
// included.
func TestCheckParamsDeclarationKind(t *testing.T) {
	required := []ParamKey{ParamDeclarationKind, ParamSchemaKey}
	for _, k := range DeclarationKinds() {
		if err := CheckParams(&Params{DeclarationKind: k, Key: "pages"}, required, nil); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
	for _, v := range []DeclarationKind{"storage", "Collection", "collections"} {
		err := CheckParams(&Params{DeclarationKind: v, Key: "pages"}, required, nil)
		if err == nil || !strings.Contains(err.Error(), "none of its values") {
			t.Errorf("%q keeps to the declaration: %v", v, err)
		}
	}
	// A refusal stored by 0.12.1 named the kind in kind.
	if err := CheckParams(&Params{Kind: "collection", Key: "pages"}, required, nil); err == nil {
		t.Error("params with the kind in kind keep to the declaration")
	}
}

// The keys with a closed vocabulary, and no others, list it: the value
// kinds for expected and actual, the declaration kinds for
// declarationKind, in their enum's order, in ParamSpecs and in
// catalog.json, which clients are generated from.
func TestCatalogListsTheClosedVocabularies(t *testing.T) {
	want := map[ParamKey][]string{
		ParamExpected:        codes(ValueKinds()),
		ParamActual:          codes(ValueKinds()),
		ParamDeclarationKind: codes(DeclarationKinds()),
	}
	// What a caller does with the list it gets changes nothing here.
	for _, s := range ParamSpecs() {
		if s.Key == ParamExpected {
			s.Values[0] = "changed"
		}
	}
	for _, s := range ParamSpecs() {
		values, closed := want[s.Key]
		switch {
		case closed:
			if !slices.Equal(s.Values, values) || s.Class != ParamClassVocabulary || s.Type != ParamTypeString {
				t.Errorf("%s: %s %s %v, want a vocabulary string of %v", s.Key, s.Class, s.Type, s.Values, values)
			}
		case s.Values != nil:
			t.Errorf("%s has values %v", s.Key, s.Values)
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
	listed := map[ParamKey]bool{}
	for _, p := range file.Params {
		if p.Values == nil {
			continue
		}
		listed[ParamKey(p.Key)] = true
		if values, ok := want[ParamKey(p.Key)]; !ok || !slices.Equal(p.Values, values) {
			t.Errorf("catalog.json lists %s: %v, want %v", p.Key, p.Values, values)
		}
	}
	for k := range want {
		if !listed[k] {
			t.Errorf("catalog.json lists no values of %s", k)
		}
	}
}
