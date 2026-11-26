package report

//go:generate go run ../internal/catalogjson -o catalog.json

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Cause names which of a rule's causes a finding is. A rule with one
// cause has none (""); a rule with several names each. Like rule ids,
// causes are frozen: never renamed, never reused for another cause. The
// same cause may appear under several rules (`not_list`, `empty`) and
// means there what the rule's catalog entry says.
type Cause string

// CauseSpec is one rule and cause: the severity its findings have
// before --strict (for a quality rule, the most they can have: an
// impact lowered by a guess makes a notice), and the Params keys its
// findings set. Required keys are always set (a customer key may be
// empty text, see Params); optional ones when the run knows them.
type CauseSpec struct {
	Rule     Rule       `json:"rule"`
	Cause    Cause      `json:"cause,omitempty"`
	Severity Severity   `json:"severity"`
	Params   []ParamKey `json:"params"`
	Optional []ParamKey `json:"optional,omitempty"`

	message, hint func(x in) string
}

// in is what a sentence is made of: the finding's Params (never nil) and
// the finding, for the fields outside Params a sentence reads (Basis).
type in struct {
	*Params
	f *Finding
}

// Catalog lists every rule and cause this version reports, in the order
// of Rules() and, within a rule, the order its causes are declared.
func Catalog() []CauseSpec {
	out := make([]CauseSpec, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns the spec of a rule and cause.
func Lookup(rule Rule, cause Cause) (CauseSpec, bool) {
	i, ok := catalogIndex[code{rule, cause}]
	if !ok {
		return CauseSpec{}, false
	}
	return catalog[i], true
}

// CausesOf lists the causes of a rule, nil for a rule with one cause.
func CausesOf(rule Rule) []Cause {
	var out []Cause
	for _, s := range catalog {
		if s.Rule == rule && s.Cause != "" {
			out = append(out, s.Cause)
		}
	}
	return out
}

type code struct {
	rule  Rule
	cause Cause
}

var catalog, catalogIndex = buildCatalog()

func buildCatalog() ([]CauseSpec, map[code]int) {
	var all []CauseSpec
	for _, part := range [][]CauseSpec{schemaCauses, contentCauses, linksCauses, qualityCauses} {
		all = append(all, part...)
	}
	order := map[Rule]int{}
	for i, r := range Rules() {
		order[r] = i
	}
	slices.SortStableFunc(all, func(a, b CauseSpec) int { return order[a.Rule] - order[b.Rule] })
	index := make(map[code]int, len(all))
	for i, s := range all {
		if _, dup := index[code{s.Rule, s.Cause}]; dup {
			panic(fmt.Sprintf("report: %s %q is declared twice", s.Rule, s.Cause))
		}
		index[code{s.Rule, s.Cause}] = i
	}
	return all, index
}

// spec declares one rule and cause. params and optional are the Params
// keys, space-separated.
func spec(rule Rule, cause Cause, sev Severity, params, optional string, message, hint func(x in) string) CauseSpec {
	return CauseSpec{Rule: rule, Cause: cause, Severity: sev, Params: keyList(params), Optional: keyList(optional), message: message, hint: hint}
}

func keyList(s string) []ParamKey {
	out := []ParamKey{}
	for _, k := range strings.Fields(s) {
		if _, ok := paramSpec(ParamKey(k)); !ok {
			panic("report: no Params key " + k)
		}
		out = append(out, ParamKey(k))
	}
	return out
}

// text is a sentence that takes nothing from the finding.
func text(s string) func(in) string { return func(in) string { return s } }

// New makes a finding of a rule and cause: the spec's severity, the
// params held to their bounds, and Message and Hint in English. A rule
// and cause the catalog lacks is a bug in the caller; the finding then
// keeps an empty message, which CheckFinding reports.
func New(file, path string, rule Rule, cause Cause, p *Params) Finding {
	f := Finding{File: file, Path: path, Rule: rule, Cause: cause, Params: p}
	if s, ok := Lookup(rule, cause); ok {
		f.Severity = s.Severity
	}
	return f.Worded()
}

// Worded is f with its params held to their bounds and Message and Hint
// made by English, for a caller that sets fields outside Params (Basis,
// Value, Measure) before the finding is worded.
func (f Finding) Worded() Finding {
	f.Params = f.Params.Bounded()
	f.Message, f.Hint = English(f)
	return f
}

// English is the finding's sentence and hint in English, made from its
// rule, cause and params: what Message and Hint hold. A finding whose
// rule and cause the catalog lacks (one a caller made itself) keeps its
// own Message and Hint.
func English(f Finding) (message, hint string) {
	s, ok := Lookup(f.Rule, f.Cause)
	if !ok {
		return f.Message, f.Hint
	}
	p := f.Params
	if p == nil {
		p = &Params{}
	}
	x := in{Params: p, f: &f}
	message = s.message(x)
	if s.hint != nil {
		hint = s.hint(x)
	}
	return message, hint
}

// CheckFinding reports how a finding breaks its catalog entry: a rule
// or cause the catalog lacks, a Params key the cause does not declare, a
// required key that is not set (customer text excepted: it may be
// empty), a value outside its key's closed vocabulary, strings or lists
// over their bounds, or a Message and Hint other than English(f). Nil
// when it keeps to it. For tests and for readers that store findings.
func CheckFinding(f Finding) error {
	s, ok := Lookup(f.Rule, f.Cause)
	if !ok {
		return fmt.Errorf("%s %q is not in the catalog", f.Rule, f.Cause)
	}
	var errs []error
	if err := CheckParams(f.Params, s.Params, s.Optional); err != nil {
		errs = append(errs, fmt.Errorf("%s %q: %w", f.Rule, f.Cause, err))
	}
	if m, h := English(f); m != f.Message || h != f.Hint {
		errs = append(errs, fmt.Errorf("%s %q: message %q, hint %q are not English(f): %q, %q", f.Rule, f.Cause, f.Message, f.Hint, m, h))
	}
	return errors.Join(errs...)
}

// CheckParams reports how p breaks a declaration of required and
// optional keys: a key set that neither lists, a required key not set
// (customer text excepted: it may be empty), a value outside its key's
// closed vocabulary (ParamSpec.Values), or strings and lists over their
// bounds. Nil when it keeps to it. A reader of params stored by an
// older version learns from it whether this version can word them.
func CheckParams(p *Params, required, optional []ParamKey) error {
	var errs []error
	set := p.Keys()
	for _, k := range set {
		if !slices.Contains(required, k) && !slices.Contains(optional, k) {
			errs = append(errs, fmt.Errorf("sets %s, which it does not declare", k))
		}
		if ps, ok := paramSpec(k); ok && len(ps.Values) > 0 {
			if v := reflect.ValueOf(p).Elem().Field(ps.field).String(); !slices.Contains(ps.Values, v) {
				errs = append(errs, fmt.Errorf("sets %s to %q, which is none of its values", k, v))
			}
		}
	}
	for _, k := range required {
		ps, ok := paramSpec(k)
		if !ok {
			errs = append(errs, fmt.Errorf("declares %s, which Params lacks", k))
			continue
		}
		if !slices.Contains(set, k) && ps.Class != ParamClassCustomer {
			errs = append(errs, fmt.Errorf("does not set %s", k))
		}
	}
	if p != nil {
		if b := p.Bounded(); b == nil || !reflect.DeepEqual(*b, *p) {
			errs = append(errs, errors.New("has params over their bounds, or an empty Params instead of none"))
		}
	}
	return errors.Join(errs...)
}
