package fact

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

// ErrorKind is the class of a refused fact: ErrInvalid, ErrNotFound,
// ErrAmbiguous or ErrUnplaceable, as a code.
type ErrorKind string

// Kinds of a refused fact.
const (
	ErrorKindInvalid     ErrorKind = "invalid_fact"
	ErrorKindNotFound    ErrorKind = "not_in_schema"
	ErrorKindAmbiguous   ErrorKind = "ambiguous"
	ErrorKindUnplaceable ErrorKind = "cannot_be_placed"
)

// Sentinel is the error a kind matches with errors.Is.
func (k ErrorKind) Sentinel() error {
	switch k {
	case ErrorKindNotFound:
		return ErrNotFound
	case ErrorKindAmbiguous:
		return ErrAmbiguous
	case ErrorKindUnplaceable:
		return ErrUnplaceable
	}
	return ErrInvalid
}

// ErrorCode names why a fact was refused. Frozen like rule ids: never
// renamed or reused. The codes of a titleTemplate that cannot be read
// and of a usedByCode path that is not one are report's causes
// (report.TemplateCauses, report.PatternCauses), so a refused fact and a
// schema finding name the same problem the same way.
type ErrorCode string

// Codes of a refused fact. ErrorCodes() lists each with its kind and
// params.
const (
	// The fact on its own (Fact.Check, Make).
	CodeOwnerNotOne         ErrorCode = "owner_not_one"
	CodeFieldMissing        ErrorCode = "field_missing"
	CodeRoleUnknown         ErrorCode = "role_unknown"
	CodeWhenEmptyMismatch   ErrorCode = "when_empty_mismatch"
	CodeTitleTemplateRole   ErrorCode = "title_template_role"
	CodeExcerptMismatch     ErrorCode = "excerpt_mismatch"
	CodeWhenRole            ErrorCode = "when_role"
	CodeTooMany             ErrorCode = "too_many"
	CodeEntryBlank          ErrorCode = "entry_blank"
	CodeListedTwice         ErrorCode = "listed_twice"
	CodeRoleTakesNoAlt      ErrorCode = "role_takes_no_alt"
	CodeNotLanguageCode     ErrorCode = "not_language_code"
	CodeAltOneOf            ErrorCode = "alt_one_of"
	CodeNotFieldKey         ErrorCode = "not_field_key"
	CodeSeparator           ErrorCode = "separator"
	CodeLocalesOwner        ErrorCode = "locales_owner"
	CodeLocalesCount        ErrorCode = "locales_count"
	CodeOptimizedOwner      ErrorCode = "optimized_owner"
	CodePathsCount          ErrorCode = "paths_count"
	CodeKindUnknown         ErrorCode = "kind_unknown"
	CodeNotName             ErrorCode = "not_name"
	CodeFactsCount          ErrorCode = "facts_count"
	CodeKindNotPlaced       ErrorCode = "kind_not_placed"
	CodeNoLanguageVersions  ErrorCode = "no_language_versions"
	CodeNotSiteLocale       ErrorCode = "not_site_locale"
	CodeStorageKeepsNoFiles ErrorCode = "storage_keeps_no_files"
	CodeRoleCannotHold      ErrorCode = "role_cannot_hold"
	CodeLocalizedCannotHold ErrorCode = "localized_role_cannot_hold"
	CodeWhenNotOption       ErrorCode = "when_not_option"
	CodeNotGroup            ErrorCode = "not_group"
	CodeGroupFieldCount     ErrorCode = "group_field_count"
	CodeGroupFieldNotLocale ErrorCode = "group_field_not_locale"
	CodeNotImage            ErrorCode = "not_image"
	CodeCannotHoldAlt       ErrorCode = "cannot_hold_alt"
	CodeTooDeep             ErrorCode = "too_deep"
	CodePathStartsWithItem  ErrorCode = "path_starts_with_item"
	CodeNotFieldPath        ErrorCode = "not_field_path"
	CodeUsedByCodeTooMany   ErrorCode = "used_by_code_too_many"
	CodeCollectionNotFound  ErrorCode = "collection_not_found"
	CodeBlockNotFound       ErrorCode = "block_not_found"
	CodeStorageNotFound     ErrorCode = "storage_not_found"
	CodeStorageFileNotFound ErrorCode = "storage_file_not_found"
	CodeAltFieldNotFound    ErrorCode = "alt_field_not_found"
	CodeFieldNotFound       ErrorCode = "field_not_found"
	CodeItemOfNonList       ErrorCode = "item_of_non_list"
	CodeListBlockNotFound   ErrorCode = "list_block_not_found"
	CodeShapeNotFound       ErrorCode = "shape_not_found"
	CodeRefNotFound         ErrorCode = "ref_not_found"
	CodeNoFileDeclares      ErrorCode = "no_file_declares"
	CodeNoMappingAt         ErrorCode = "no_mapping_at"
	CodeNoFieldsList        ErrorCode = "no_fields_list"
	CodeNoFieldWithKey      ErrorCode = "no_field_with_key"
	CodeNoItems             ErrorCode = "no_items"
	CodeItemsNotList        ErrorCode = "items_not_list"
	CodeFieldsShareKey      ErrorCode = "fields_share_key"
	CodeFieldIsRef          ErrorCode = "field_is_ref"
	CodeShapesFit           ErrorCode = "shapes_fit"
	CodeShapeIsRef          ErrorCode = "shape_is_ref"
	CodeShapeUnnamed        ErrorCode = "shape_unnamed"
	CodeShapesNamedTwice    ErrorCode = "shapes_named_twice"
	CodeDeclaredTwice       ErrorCode = "declared_twice"
	CodeNoPosition          ErrorCode = "no_position"
	CodeEmptyValue          ErrorCode = "empty_value"
	CodeMultilineValue      ErrorCode = "multiline_value"
	CodeUnterminatedValue   ErrorCode = "unterminated_value"
	CodeNoFlowCollection    ErrorCode = "no_flow_collection"
	CodeUnterminatedFlow    ErrorCode = "unterminated_flow_collection"
	CodeNoColon             ErrorCode = "no_colon"
	CodeEmptyMapping        ErrorCode = "empty_mapping"
	CodeNoLineForKey        ErrorCode = "no_line_for_key"
	CodeSharesLine          ErrorCode = "shares_line"
	CodeFollowedOnLine      ErrorCode = "followed_on_line"
	CodeNotYAML             ErrorCode = "not_yaml"
	CodeNotMapping          ErrorCode = "not_mapping"
	CodeWithinNotMapping    ErrorCode = "within_not_mapping"
)

// Error is a refused fact: its kind, a code and the params its sentence
// is made of. Error() is the English sentence (Make wraps it in a
// FactError with the fact's index); errors.Is matches the kind's
// sentinel (ErrInvalid, ErrNotFound, ErrAmbiguous, ErrUnplaceable).
//
// A code that names the place first prints it before the kind: the
// schema file (Params.Path, "x.yml: ") or else the fact's field path
// (Params.Field, "\"list[].a\": "), when set.
type Error struct {
	Kind   ErrorKind      `json:"kind"`
	Code   ErrorCode      `json:"code"`
	Params *report.Params `json:"params,omitempty"`
}

func (e *Error) Error() string {
	s, ok := errorSpecFor(e.Code)
	p := e.Params
	if p == nil {
		p = &report.Params{}
	}
	if !ok {
		return e.Kind.Sentinel().Error() + ": " + string(e.Code)
	}
	prefix := ""
	switch {
	case !s.prefixed:
	case p.Path != "":
		prefix = p.Path + ": "
	case p.Field != "":
		prefix = strconv.Quote(p.Field) + ": "
	}
	return prefix + e.Kind.Sentinel().Error() + ": " + s.english(p)
}

// Is matches the sentinel of the error's kind.
func (e *Error) Is(target error) bool { return target == e.Kind.Sentinel() }

// CheckError reports how a refusal breaks its code's entry in
// ErrorCodes(): a code it lacks, another kind, or params the code does
// not declare (report.CheckParams). Nil when it keeps to it.
func CheckError(e *Error) error {
	s, ok := errorSpecFor(e.Code)
	switch {
	case !ok:
		return fmt.Errorf("fact: code %q is not in ErrorCodes", e.Code)
	case s.Kind != e.Kind:
		return fmt.Errorf("fact: code %q is of kind %s, not %s", e.Code, s.Kind, e.Kind)
	}
	if err := report.CheckParams(e.Params, s.Params, s.Optional); err != nil {
		return fmt.Errorf("fact: code %q: %w", e.Code, err)
	}
	return nil
}

// ErrorSpec is one code: its kind and the params it sets.
type ErrorSpec struct {
	Kind     ErrorKind         `json:"kind"`
	Code     ErrorCode         `json:"code"`
	Params   []report.ParamKey `json:"params"`
	Optional []report.ParamKey `json:"optional,omitempty"`

	prefixed bool
	english  func(p *report.Params) string
}

// Where a code prints the place: in its sentence (prefixNone), or
// before the kind (prefixed: Params.Path, else Params.Field).
const (
	prefixNone  = false
	prefixed    = true
	prefixPath  = prefixed
	prefixField = prefixed
)

// ErrorCodes lists every code a refused fact can have, with its kind and
// params.
func ErrorCodes() []ErrorSpec { return slices.Clone(errorSpecs) }

func errorSpecFor(c ErrorCode) (ErrorSpec, bool) {
	i := slices.IndexFunc(errorSpecs, func(s ErrorSpec) bool { return s.Code == c })
	if i < 0 {
		return ErrorSpec{}, false
	}
	return errorSpecs[i], true
}

// refuse makes a refusal of a code; its kind comes from ErrorCodes.
func refuse(c ErrorCode, p *report.Params) error {
	s, ok := errorSpecFor(c)
	if !ok {
		panic("fact: no error code " + string(c))
	}
	return &Error{Kind: s.Kind, Code: c, Params: p.Bounded()}
}

// inFile names the schema file a refusal is about. A refusal that
// already names one, or an error that is not a refusal (I/O), is
// wrapped as "<file>: <error>".
func inFile(file string, err error) error {
	var fe *Error
	if errors.As(err, &fe) && err == error(fe) {
		if s, _ := errorSpecFor(fe.Code); s.prefixed && (fe.Params == nil || fe.Params.Path == "") {
			p := report.Params{}
			if fe.Params != nil {
				p = *fe.Params
			}
			p.Path = file
			return &Error{Kind: fe.Kind, Code: fe.Code, Params: &p}
		}
	}
	return fmt.Errorf("%s: %w", file, err)
}

// problemError is a refused fact for a report.Problem: a titleTemplate
// that cannot be read or a usedByCode path that is not one.
func problemError(pr report.Problem) error {
	return &Error{Kind: ErrorKindInvalid, Code: ErrorCode(pr.Cause), Params: pr.Params.Bounded()}
}

// article is "a" or "an" before an English word.
func article(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}

func q(s string) string { return strconv.Quote(s) }

func num(p *float64) string {
	if p == nil {
		return "0"
	}
	return report.FormatNumber(*p)
}

func count(p *int64) string {
	if p == nil {
		return "0"
	}
	return strconv.FormatInt(*p, 10)
}

// ownerNoun is what a fact of a kind names: a field, a group, an image.
func ownerNoun(kind string) string {
	switch kind {
	case KindLocalized:
		return "a group"
	case KindAlt:
		return "an image field"
	}
	return "a field"
}

// declarationKind is the kind of declaration a refusal names, in
// English. A refusal stored by 0.12.1 named it in English in kind, and
// reads as it did (CheckError refuses it).
func declarationKind(p *report.Params) string {
	if p.DeclarationKind == "" {
		return p.Kind
	}
	return p.DeclarationKind.English()
}

// entryNoun is what one entry of a fact's list is.
func entryNoun(knob string) string {
	if knob == "altFrom" {
		return "field"
	}
	return "value"
}

func spec(kind ErrorKind, c ErrorCode, params, optional string, prefix bool, english func(p *report.Params) string) ErrorSpec {
	return ErrorSpec{Kind: kind, Code: c, Params: keys(params), Optional: keys(optional), prefixed: prefix, english: english}
}

func keys(s string) []report.ParamKey {
	out := []report.ParamKey{}
	for _, k := range strings.Fields(s) {
		out = append(out, report.ParamKey(k))
	}
	return out
}

func says(s string) func(*report.Params) string { return func(*report.Params) string { return s } }

const (
	inv  = ErrorKindInvalid
	nf   = ErrorKindNotFound
	amb  = ErrorKindAmbiguous
	unpl = ErrorKindUnplaceable
)

var errorSpecs = append(append([]ErrorSpec{
	spec(inv, CodeOwnerNotOne, "kind", "", prefixNone, func(p *report.Params) string {
		return article(p.Kind) + " " + p.Kind + " fact names a collection or a block, one of them"
	}),
	spec(inv, CodeFieldMissing, "kind max", "", prefixNone, func(p *report.Params) string {
		return article(p.Kind) + " " + p.Kind + " fact names " + ownerNoun(p.Kind) + " (at most " + num(p.Max) + " bytes)"
	}),
	spec(inv, CodeRoleUnknown, "value allowed", "", prefixNone, func(p *report.Params) string {
		return "role " + q(p.Value) + " is not one of " + strings.Join(p.Allowed, ", ")
	}),
	spec(inv, CodeWhenEmptyMismatch, "value role", "", prefixNone, func(p *report.Params) string {
		return "whenEmpty " + q(p.Value) + " does not go with role " + p.Role
	}),
	spec(inv, CodeTitleTemplateRole, "role", "", prefixNone, func(p *report.Params) string {
		return "titleTemplate goes with role " + p.Role
	}),
	spec(inv, CodeExcerptMismatch, "value role", "", prefixNone, func(p *report.Params) string {
		return "excerpt " + q(p.Value) + " does not go with role " + p.Role
	}),
	spec(inv, CodeWhenRole, "allowed", "", prefixNone, func(p *report.Params) string {
		return "values that switch a role on go with role " + strings.Join(p.Allowed, " or ")
	}),
	spec(inv, CodeTooMany, "knob max", "", prefixNone, func(p *report.Params) string {
		return "at most " + num(p.Max) + " entries"
	}),
	spec(inv, CodeEntryBlank, "knob max", "", prefixNone, func(p *report.Params) string {
		return "a " + entryNoun(p.Knob) + " is blank or longer than " + num(p.Max) + " bytes"
	}),
	spec(inv, CodeListedTwice, "value", "knob", prefixNone, func(p *report.Params) string {
		return q(p.Value) + " is listed twice"
	}),
	spec(inv, CodeRoleTakesNoAlt, "", "", prefixNone, says("a role fact takes no fallback or alt")),
	spec(inv, CodeNotLanguageCode, "value", "", prefixNone, func(p *report.Params) string {
		return q(p.Value) + " is not a language code"
	}),
	spec(inv, CodeAltOneOf, "", "", prefixNone,
		says("an alt fact names one field (alt) or the fields it is composed of (altFrom), one of them")),
	spec(inv, CodeNotFieldKey, "value", "", prefixNone, func(p *report.Params) string {
		return q(p.Value) + " is not a field key"
	}),
	spec(inv, CodeSeparator, "max", "", prefixNone, func(p *report.Params) string {
		return "a separator (at most " + num(p.Max) + " bytes) goes with altFrom"
	}),
	spec(inv, CodeLocalesOwner, "", "", prefixNone, says("a translationLocales fact names a collection")),
	spec(inv, CodeLocalesCount, "max", "", prefixNone, func(p *report.Params) string {
		return "a translationLocales fact lists 1 to " + num(p.Max) + " languages"
	}),
	spec(inv, CodeOptimizedOwner, "", "", prefixNone, says("an optimizedAtBuild fact names a storage and a value")),
	spec(inv, CodePathsCount, "max", "", prefixNone, func(p *report.Params) string {
		return "a usedByCode fact names a storage and 1 to " + num(p.Max) + " paths"
	}),
	spec(inv, CodeKindUnknown, "value allowed", "", prefixNone, func(p *report.Params) string {
		return "kind " + q(p.Value) + " is not one of " + strings.Join(p.Allowed, ", ")
	}),
	spec(inv, CodeNotName, "value", "", prefixNone, func(p *report.Params) string {
		return q(p.Value) + " is not a name"
	}),
	spec(inv, CodeFactsCount, "max", "", prefixNone, func(p *report.Params) string {
		return "1 to " + num(p.Max) + " facts"
	}),
	spec(inv, CodeKindNotPlaced, "value", "", prefixNone, func(p *report.Params) string {
		return "kind " + q(p.Value)
	}),
	spec(inv, CodeNoLanguageVersions, "key", "", prefixNone, func(p *report.Params) string {
		return "collection " + q(p.Key) + " has no language versions (no translations)"
	}),
	spec(inv, CodeNotSiteLocale, "locale", "", prefixNone, func(p *report.Params) string {
		return q(p.Locale) + " is not a language of the site (preview.locales)"
	}),
	spec(inv, CodeStorageKeepsNoFiles, "key", "", prefixNone, func(p *report.Params) string {
		return "storage " + q(p.Key) + " keeps no files in the repository"
	}),
	spec(inv, CodeRoleCannotHold, "type role", "", prefixNone, func(p *report.Params) string {
		return "a " + p.Type + " field cannot hold role " + p.Role + " as the fact says it"
	}),
	spec(inv, CodeLocalizedCannotHold, "role", "type", prefixNone, func(p *report.Params) string {
		return "a localized group of " + p.Type + " fields cannot hold role " + p.Role + " as the fact says it"
	}),
	spec(inv, CodeWhenNotOption, "value field", "", prefixNone, func(p *report.Params) string {
		return q(p.Value) + " is not an option of " + q(p.Field)
	}),
	spec(inv, CodeNotGroup, "field type", "", prefixNone, func(p *report.Params) string {
		return q(p.Field) + " is a " + p.Type + " field, not a group"
	}),
	spec(inv, CodeGroupFieldCount, "field count total", "", prefixNone, func(p *report.Params) string {
		return "group " + q(p.Field) + " has " + count(p.Count) + " fields, the site " + count(p.Total) + " languages"
	}),
	spec(inv, CodeGroupFieldNotLocale, "key field", "", prefixNone, func(p *report.Params) string {
		return "field " + q(p.Key) + " of " + q(p.Field) + " is not a language of the site"
	}),
	spec(inv, CodeNotImage, "field type", "", prefixNone, func(p *report.Params) string {
		return q(p.Field) + " is a " + p.Type + " field, not an image"
	}),
	spec(inv, CodeCannotHoldAlt, "key", "", prefixNone, func(p *report.Params) string {
		return "field " + q(p.Key) + " cannot hold alt text"
	}),
	spec(inv, CodeTooDeep, "field max", "", prefixNone, func(p *report.Params) string {
		return q(p.Field) + " nests deeper than " + num(p.Max)
	}),
	spec(inv, CodePathStartsWithItem, "field", "", prefixNone, func(p *report.Params) string {
		return "field path " + q(p.Field) + " starts with a list item"
	}),
	spec(inv, CodeNotFieldPath, "field", "", prefixNone, func(p *report.Params) string {
		return q(p.Field) + " is not a field path (keys joined by dots, [] or [shape] for a list item, ending at a field)"
	}),
	spec(inv, CodeUsedByCodeTooMany, "key count max", "", prefixNone, func(p *report.Params) string {
		return "storage " + q(p.Key) + " would list " + count(p.Count) + " paths in usedByCode, at most " + num(p.Max)
	}),

	spec(nf, CodeCollectionNotFound, "key", "", prefixNone, func(p *report.Params) string { return "collection " + q(p.Key) }),
	spec(nf, CodeBlockNotFound, "key", "", prefixNone, func(p *report.Params) string { return "block " + q(p.Key) }),
	spec(nf, CodeStorageNotFound, "key", "", prefixNone, func(p *report.Params) string { return "storage " + q(p.Key) }),
	spec(nf, CodeStorageFileNotFound, "path key", "", prefixNone, func(p *report.Params) string {
		return "no file " + p.Path + " in storage " + q(p.Key)
	}),
	spec(nf, CodeAltFieldNotFound, "key field", "", prefixNone, func(p *report.Params) string {
		return "no field " + q(p.Key) + " beside image " + q(p.Field)
	}),
	spec(nf, CodeFieldNotFound, "key field", "", prefixNone, func(p *report.Params) string {
		return "field " + q(p.Key) + " of " + q(p.Field)
	}),
	spec(nf, CodeItemOfNonList, "field", "", prefixNone, func(p *report.Params) string {
		return q(p.Field) + " steps into a list item of a field that is not a list"
	}),
	spec(nf, CodeListBlockNotFound, "key field", "", prefixNone, func(p *report.Params) string {
		return "block " + q(p.Key) + " in " + q(p.Field)
	}),
	// The fact's field (Field) or the file (Path) leads the sentence.
	spec(nf, CodeShapeNotFound, "key", "field path", prefixField, func(p *report.Params) string {
		return "item shape " + q(p.Key)
	}),
	spec(nf, CodeRefNotFound, "key", "", prefixNone, func(p *report.Params) string { return "block or input " + q(p.Key) }),
	spec(nf, CodeNoFileDeclares, "declarationKind key", "", prefixNone, func(p *report.Params) string {
		return "no file declares " + declarationKind(p) + " " + q(p.Key)
	}),
	spec(nf, CodeNoMappingAt, "path knob", "", prefixNone, func(p *report.Params) string {
		return p.Path + " has no mapping at " + p.Knob
	}),
	spec(nf, CodeNoFieldsList, "key", "path", prefixPath, func(p *report.Params) string {
		return "no fields list holds " + q(p.Key)
	}),
	spec(nf, CodeNoFieldWithKey, "key", "path", prefixPath, func(p *report.Params) string {
		return "no field has the key " + q(p.Key)
	}),
	spec(nf, CodeNoItems, "", "path", prefixPath, says("the list declares no items")),
	spec(nf, CodeItemsNotList, "", "path", prefixPath, says("items is not a list")),

	spec(amb, CodeFieldsShareKey, "count key", "path", prefixPath, func(p *report.Params) string {
		return count(p.Count) + " fields have the key " + q(p.Key)
	}),
	spec(amb, CodeFieldIsRef, "key", "path", prefixPath, func(p *report.Params) string {
		return "field " + q(p.Key) + " is a $ref"
	}),
	spec(amb, CodeShapesFit, "count key", "path", prefixPath, func(p *report.Params) string {
		return count(p.Count) + " item shapes fit " + q(p.Key)
	}),
	spec(amb, CodeShapeIsRef, "key", "path", prefixPath, func(p *report.Params) string {
		return "item shape " + q(p.Key) + " is a $ref"
	}),
	spec(amb, CodeShapeUnnamed, "count", "field", prefixField, func(p *report.Params) string {
		return "the list has " + count(p.Count) + " item shapes; name one"
	}),
	spec(amb, CodeShapesNamedTwice, "count key", "field", prefixField, func(p *report.Params) string {
		return count(p.Count) + " item shapes are named " + q(p.Key)
	}),
	spec(amb, CodeDeclaredTwice, "declarationKind key names", "", prefixNone, func(p *report.Params) string {
		return declarationKind(p) + " " + q(p.Key) + " is declared in " + strings.Join(p.Names, " and ")
	}),

	spec(unpl, CodeNoPosition, "", "path", prefixPath, says("no position")),
	spec(unpl, CodeEmptyValue, "", "path", prefixPath, says("an empty value")),
	spec(unpl, CodeMultilineValue, "", "path", prefixPath, says("a value over several lines")),
	spec(unpl, CodeUnterminatedValue, "", "path", prefixPath, says("an unterminated value")),
	spec(unpl, CodeNoFlowCollection, "", "path", prefixPath, says("no flow collection")),
	spec(unpl, CodeUnterminatedFlow, "", "path", prefixPath, says("an unterminated flow collection")),
	spec(unpl, CodeNoColon, "knob", "path", prefixPath, func(p *report.Params) string { return "no colon after " + p.Knob }),
	spec(unpl, CodeEmptyMapping, "", "path", prefixPath, says("an empty mapping")),
	spec(unpl, CodeNoLineForKey, "knob", "path", prefixPath, func(p *report.Params) string {
		return "no key on a line of its own to put " + p.Knob + " after"
	}),
	spec(unpl, CodeSharesLine, "knob", "path", prefixPath, func(p *report.Params) string {
		return p.Knob + " shares its line with another key"
	}),
	spec(unpl, CodeFollowedOnLine, "knob", "path", prefixPath, func(p *report.Params) string {
		return "something follows " + p.Knob + " on its line"
	}),
	spec(unpl, CodeNotYAML, "detail", "path", prefixPath, func(p *report.Params) string { return p.Detail }),
	spec(unpl, CodeNotMapping, "", "path", prefixPath, says("the file is not a mapping")),
	spec(unpl, CodeWithinNotMapping, "path knob", "", prefixNone, func(p *report.Params) string {
		return p.Path + " has no " + p.Knob + " mapping there"
	}),
}, problemSpecs(report.TemplateCauses(), "invalid titleTemplate: ")...), problemSpecs(report.PatternCauses(), "")...)

// problemSpecs are the codes of a titleTemplate that cannot be read and
// of a usedByCode path that is not one: report's causes, worded by
// report.Problem.
func problemSpecs(causes []report.Cause, lead string) []ErrorSpec {
	var out []ErrorSpec
	for _, c := range causes {
		c := c
		params := map[report.Cause]string{
			report.CauseTemplateTooLong: "max", report.CauseTemplateLoneClose: "offset", report.CauseTemplateUnclosed: "offset",
			report.CauseTemplateFieldPath: "value", report.CauseTemplateUnknownPlaceholder: "value", report.CauseTemplateTooManyParts: "max",
			report.CausePatternTooLong: "path max",
		}[c]
		if params == "" && slices.Contains(report.PatternCauses(), c) && c != report.CausePatternEmpty {
			params = "path"
		}
		out = append(out, spec(inv, ErrorCode(c), params, "", prefixNone, func(p *report.Params) string {
			return lead + report.Problem{Cause: c, Params: p}.String()
		}))
	}
	return out
}

// templateRefusal is a fact's titleTemplate that cannot be read.
func templateRefusal(tmpl string) error {
	_, err := schema.ParseTemplate(tmpl)
	var te *schema.TemplateError
	if errors.As(err, &te) {
		return problemError(te.Problem)
	}
	return err
}
