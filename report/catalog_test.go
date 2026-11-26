package report

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Every rule has catalog entries: one without a cause, or several, each
// with its own.
func TestCatalogCoversEveryRule(t *testing.T) {
	byRule := map[Rule][]CauseSpec{}
	for _, s := range Catalog() {
		byRule[s.Rule] = append(byRule[s.Rule], s)
	}
	for _, r := range Rules() {
		specs := byRule[r]
		switch {
		case len(specs) == 0:
			t.Errorf("%s has no catalog entry", r)
		case len(specs) == 1 && specs[0].Cause != "":
			t.Errorf("%s has one cause, %q; a rule with one cause names none", r, specs[0].Cause)
		case len(specs) > 1:
			for _, s := range specs {
				if s.Cause == "" {
					t.Errorf("%s has several causes and one without a name", r)
				}
			}
		}
		delete(byRule, r)
	}
	for r := range byRule {
		t.Errorf("catalog rule %s is not in Rules()", r)
	}
	if !slices.Equal(CausesOf(RuleSchemaMissingType), []Cause{CauseNoType, CauseNoItems}) || CausesOf(RuleContentMin) != nil {
		t.Errorf("CausesOf: %v, %v", CausesOf(RuleSchemaMissingType), CausesOf(RuleContentMin))
	}
	for _, s := range Catalog() {
		if s.Rule.IsQuality() {
			if s.Severity == SeverityError {
				t.Errorf("%s %q: a quality finding is never an error", s.Rule, s.Cause)
			}
		} else if s.Severity == SeverityNotice {
			t.Errorf("%s %q: only quality findings are notices", s.Rule, s.Cause)
		}
	}
}

// Every Cause constant of causes.go is a cause of the catalog, and every
// cause of the catalog is a constant: the constants are what Swagger
// publishes as the enum.
func TestCauseConstantsAreTheCatalog(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "causes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[Cause]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil || vs.Type.(*ast.Ident).Name != "Cause" {
			return true
		}
		for i, name := range vs.Names {
			v, _ := strconv.Unquote(vs.Values[i].(*ast.BasicLit).Value)
			if prev, dup := consts[Cause(v)]; dup {
				t.Errorf("%s and %s are both %q", prev, name.Name, v)
			}
			consts[Cause(v)] = name.Name
		}
		return true
	})
	used := map[Cause]bool{}
	for _, s := range Catalog() {
		if s.Cause == "" {
			continue
		}
		used[s.Cause] = true
		if _, ok := consts[s.Cause]; !ok {
			t.Errorf("cause %q of %s is not a constant in causes.go", s.Cause, s.Rule)
		}
	}
	for c, name := range consts {
		if !used[c] {
			t.Errorf("%s (%q) is no cause of the catalog", name, c)
		}
	}
}

// sample sets every key of keys to a value of its type, a key with a
// closed vocabulary to its first value (TestValueKindsInEveryCause
// words every value).
func sample(keys []ParamKey) *Params {
	p := &Params{}
	raw := map[string]any{}
	for _, k := range keys {
		s, _ := paramSpec(k)
		switch {
		case len(s.Values) > 0:
			raw[string(k)] = s.Values[0]
		case s.Type == ParamTypeString:
			raw[string(k)] = "s-" + string(k)
		case s.Type == ParamTypeStringList:
			raw[string(k)] = []string{"a-" + string(k), "b-" + string(k)}
		default:
			raw[string(k)] = 7
		}
	}
	b, _ := json.Marshal(raw)
	_ = json.Unmarshal(b, p)
	return p
}

// Every cause has an English sentence that uses its required params and
// makes a finding CheckFinding accepts, with its optional params and
// without them.
func TestEnglishWordsEveryCause(t *testing.T) {
	for _, s := range Catalog() {
		for _, keys := range [][]ParamKey{s.Params, append(slices.Clone(s.Params), s.Optional...)} {
			f := New("a.md", "x", s.Rule, s.Cause, sample(keys))
			if f.Message == "" || strings.Contains(f.Message, "%!") || strings.Contains(f.Hint, "%!") {
				t.Errorf("%s %q: %q / %q", s.Rule, s.Cause, f.Message, f.Hint)
			}
			if f.Severity != s.Severity {
				t.Errorf("%s %q: severity %s", s.Rule, s.Cause, f.Severity)
			}
			if err := CheckFinding(f); err != nil {
				t.Error(err)
			}
			for _, k := range s.Params {
				ps, _ := paramSpec(k)
				// A key with a closed vocabulary shows as its value's
				// words: TestValueKindsInEveryCause holds those.
				if ps.Type != ParamTypeString || len(ps.Values) > 0 || f.Params.CustomMessage != "" {
					continue
				}
				// A required text param shows in the sentence or the
				// hint, unless the schema's own message replaces it.
				if v := "s-" + string(k); !strings.Contains(f.Message+f.Hint, v) {
					t.Errorf("%s %q: %s is not in %q / %q", s.Rule, s.Cause, k, f.Message, f.Hint)
				}
			}
		}
	}
}

// The English of findings no site produces and of what the params
// change: the schema's own message, numbers, names beyond the bound.
func TestEnglishExamples(t *testing.T) {
	cases := []struct {
		f             Finding
		message, hint string
	}{
		{New("", "", RuleSchemaInternalError, "", &Params{Detail: "boom"}), "the check stopped on an internal error: boom", internalHint},
		{New("", "", RuleSchemaParseError, CauseUnreadable, &Params{Detail: "open x.yml: gone"}), "cannot read: open x.yml: gone", ""},
		{New("", "", RuleContentRequired, "", &Params{Field: "Titulek", CustomMessage: "Vyplňte titulek stránky."}), "Vyplňte titulek stránky.", "Add a value to clear this error."},
		{New("", "", RuleContentMaxLength, "", &Params{Field: "Perex", Max: Num(160), Current: Num(214)}), "Perex must be at most 160 characters (currently 214).", ""},
		{New("", "", RuleContentMin, "", &Params{Field: "Price", Min: Num(0.5), Current: Num(-1)}), "Price must be at least 0.5 (currently -1).", ""},
		{New("", "", RuleSchemaMinAboveMax, CauseMinMax, &Params{Field: "Seats", Min: Num(1e6), Max: Num(2)}), `"Seats" has min 1e+06 above max 2`, ""},
		{New("", "", RuleContentMinItems, CauseItems, &Params{Field: "Tags", Min: Num(1), Current: Num(0)}), "Tags needs at least 1 item.", "Add 1 more."},
		{New("", "", RuleQualityImageHeavy, CauseOverUpperLimit, &Params{Bytes: Int(1843200), LimitBytes: Int(400000)}), "The image is 1.8 MB, over 390 KB.",
			heavyHint + " If the site's build resizes the images of this folder, say `optimizedAtBuild: true` on its storage."},
		{New("", "", RuleSchemaInvalidAlt, CauseAltFieldMissing, &Params{Field: "Hero", Key: "heroAlt"}), `image "Hero" takes its alt text from "heroAlt", but no field in the same list has that key`, altHint},
		{New("", "", RuleContentExtraKeys, CauseNotInSchema, &Params{Names: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}}),
			"keys a, b, c, d, e, f, g, h, i, j and 2 more are not in the schema", "They show in the editor's out-of-schema panel. Add them to the schema or remove them."},
		{New("", "", RuleQualityTranslationIncomplete, "", &Params{Locale: "en", Count: Int(2), Total: Int(3), Names: []string{"Name", "About"}, SameAs: []string{"", "cs"}}),
			`The "en" version leaves 2 of 3 texts untranslated: Name (empty), About (same as cs).`, "Write these texts in that language. Until then its readers see them empty or in the fallback language."},
		{New("", "", RuleSchemaUnknownRole, "", &Params{Field: "Kind", Value: FlowValue([]any{"seoTitle", nil})}), `field "Kind" has role [seoTitle, null], which is not a role`, rolesHint},
	}
	for _, c := range cases {
		if c.f.Message != c.message || c.f.Hint != c.hint {
			t.Errorf("%s %q:\n got %q\n     %q\nwant %q\n     %q", c.f.Rule, c.f.Cause, c.f.Message, c.f.Hint, c.message, c.hint)
		}
		if err := CheckFinding(c.f); err != nil {
			t.Error(err)
		}
	}
	// The usedByCode and titleTemplate problems read the same in a
	// finding and on their own.
	p := Problem{Cause: CauseTemplateLoneClose, Params: &Params{Offset: Int(8)}}
	if got := p.String(); got != "a lone } at byte 8" {
		t.Errorf("problem %q", got)
	}
	if got := (Problem{Cause: CausePatternDotName, Params: &Params{Path: "../a"}}).String(); got != `"../a" names . or ..` {
		t.Errorf("problem %q", got)
	}
}

// A value as written: text as is, the rest in YAML flow style, never Go
// syntax.
func TestFlowValue(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{nil, "null"}, {"a b", "a b"}, {true, "true"}, {3, "3"}, {1.5, "1.5"},
		{[]any{"a", 2, nil}, "[a, 2, null]"},
		{map[string]any{"b": 1, "a": []any{"x"}}, "{a: [x], b: 1}"},
	} {
		if got := FlowValue(c.v); got != c.want {
			t.Errorf("FlowValue(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestParamsBounds(t *testing.T) {
	long := strings.Repeat("é", MaxParamRunes+5)
	names := make([]string, MaxParamNames+3)
	for i := range names {
		names[i] = strconv.Itoa(i)
	}
	p := (&Params{Value: long, Names: names, More: Int(4)}).Bounded()
	if got := []rune(p.Value); len(got) != MaxParamRunes+1 || got[MaxParamRunes] != '…' {
		t.Errorf("value of %d runes", len(got))
	}
	if len(p.Names) != MaxParamNames || *p.More != 7 {
		t.Errorf("names %d, more %d", len(p.Names), *p.More)
	}
	if (&Params{}).Bounded() != nil || (*Params)(nil).Bounded() != nil {
		t.Error("an empty Params is none")
	}
	// Keys lists what is set, a zero number included.
	if got := (&Params{Field: "a", Min: Num(0), Names: []string{}}).Keys(); !slices.Equal(got, []ParamKey{ParamField, ParamMin}) {
		t.Errorf("keys %v", got)
	}
	for _, s := range ParamSpecs() {
		if !slices.Contains([]string{ParamClassCustomer, ParamClassVocabulary, ParamClassCode, ParamClassDerived, ParamClassNumber, ParamClassTechnical}, s.Class) {
			t.Errorf("%s has class %q", s.Key, s.Class)
		}
	}
}

// Identity is file, path, rule, cause and params; the English takes no
// part, so two findings that differ only in params are two findings.
func TestFindingKey(t *testing.T) {
	a := New("f.md", "title", RuleContentMaxLength, "", &Params{Field: "Title", Max: Num(10), Current: Num(12)})
	b := New("f.md", "title", RuleContentMaxLength, "", &Params{Field: "Title", Max: Num(10), Current: Num(13)})
	if a.Key() == b.Key() {
		t.Error("findings that differ in params share a key")
	}
	c := a
	c.Message, c.Hint, c.Severity = "Titulek je příliš dlouhý.", "", SeverityWarning
	if a.Key() != c.Key() {
		t.Error("the message changed the key")
	}
	// Params set in another order are the same params.
	d := New("f.md", "title", RuleContentMaxLength, "", &Params{Current: Num(12), Max: Num(10), Field: "Title"})
	if a.Key() != d.Key() {
		t.Error("the order params are set in changed the key")
	}
	list := []Finding{b, a, d}
	Sort(list)
	if list[0].Key() != a.Key() || list[2].Key() != b.Key() {
		t.Errorf("sorted %v", list)
	}
	// A finding the catalog does not word is told apart by its text.
	x, y := Finding{Rule: "cli.usage", Message: "one"}, Finding{Rule: "cli.usage", Message: "two"}
	if x.Key() == y.Key() {
		t.Error("unworded findings share a key")
	}
	if m, h := English(x); m != "one" || h != "" {
		t.Errorf("English kept %q %q", m, h)
	}
}

func TestCheckFindingRefuses(t *testing.T) {
	ok := New("f.md", "", RuleContentMax, "", &Params{Field: "Count", Max: Num(5), Current: Num(9)})
	if err := CheckFinding(ok); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]Finding{
		"an unknown cause":  {Rule: RuleContentMax, Cause: "nope"},
		"an undeclared key": New("f.md", "", RuleContentMax, "", &Params{Field: "Count", Max: Num(5), Current: Num(9), Locale: "cs"}),
		"a missing number":  New("f.md", "", RuleContentMax, "", &Params{Field: "Count", Max: Num(5)}),
		"another message":   func() Finding { f := ok; f.Message = "Count is too big."; return f }(),
		"unbounded params": func() Finding {
			f := ok
			f.Params = &Params{Field: strings.Repeat("x", 400), Max: Num(5), Current: Num(9)}
			return f
		}(),
	} {
		if CheckFinding(f) == nil {
			t.Errorf("%s passed", name)
		}
	}
}
