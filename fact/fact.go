// Package fact writes schema facts into a site's schema files: a field's
// role (with whenEmpty, titleTemplate, excerpt, noindexWhen or
// draftWhen), a localized group, an image's alt text, the languages a
// collection is expected in, and what a storage's images are to the
// build. These are the answers
// a person gives when confirming or rejecting what a check guessed.
//
// An edit changes only the lines it is about: the rest of the file,
// comments, blank lines, quoting and key order included, stays byte for
// byte. Applying the same fact twice changes nothing the second time. A
// path that names more than one field, or a file laid out in a way the
// edit cannot place a line in safely, is refused rather than guessed.
package fact

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
	"github.com/norcubeplatform/cms-schema/validate"
)

// Kinds of fact.
const (
	// KindRole sets `role:` (and optionally `whenEmpty:`) on a field.
	// Role schema.RoleNone records that the field has no role, so it is
	// never guessed again.
	KindRole = "role"
	// KindTranslationLocales sets a collection's `translations.locales`.
	KindTranslationLocales = "translationLocales"
	// KindOptimizedAtBuild sets a storage's `optimizedAtBuild`.
	KindOptimizedAtBuild = "optimizedAtBuild"
	// KindUsedByCode adds paths to a storage's `usedByCode`: files the
	// site's code uses though no content refers to them.
	KindUsedByCode = "usedByCode"
	// KindLocalized marks a group whose fields are the site's languages
	// `localized: true`, with its `fallback` when given.
	KindLocalized = "localized"
	// KindAlt says where an image's alt text is: `alt: <key>`, or
	// composed, `alt: {from: [...], separator: ...}`.
	KindAlt = "alt"
)

// Kinds lists every kind of fact.
func Kinds() []string {
	return []string{KindRole, KindTranslationLocales, KindOptimizedAtBuild, KindUsedByCode, KindLocalized, KindAlt}
}

// Bounds of one fact.
const (
	// MaxFieldPath bounds a field path, in bytes.
	MaxFieldPath = 500
	// MaxLocales bounds the languages of a translationLocales fact.
	MaxLocales = 50
	// MaxFacts bounds the facts of one Plan.
	MaxFacts = 50
	// MaxPaths bounds the paths of one usedByCode fact.
	MaxPaths = 100
)

// Fact is one statement about the site to write into its schema.
type Fact struct {
	Kind string `json:"kind"`
	// Collection names the collection a role or translationLocales fact
	// is about; Block, instead, a block whose own field a role fact sets.
	Collection string `json:"collection,omitempty"`
	Block      string `json:"block,omitempty"`
	// Field is the field's place for a role fact, as resolve names it:
	// keys joined by dots (`seo.page_title`), `[]` for the item of a
	// list with one shape and `[<shape>]` of one with several
	// (`blocks[logos].list[].name`).
	Field string `json:"field,omitempty"`
	// Storage names the storage of an optimizedAtBuild fact.
	Storage string `json:"storage,omitempty"`
	// Role is the role to set (schema.RoleValues), WhenEmpty what the
	// site does when the field is empty, or empty to say nothing.
	Role      string `json:"role,omitempty"`
	WhenEmpty string `json:"whenEmpty,omitempty"`
	// Locales are the languages of a translationLocales fact.
	Locales []string `json:"locales,omitempty"`
	// OptimizedAtBuild is the value of an optimizedAtBuild fact.
	OptimizedAtBuild *bool `json:"optimizedAtBuild,omitempty"`
	// Paths are the entries a usedByCode fact adds, relative to the
	// storage's path (schema.CheckCodePattern). A path without a wildcard
	// must name a file.
	Paths []string `json:"paths,omitempty"`

	// TitleTemplate, on a role fact with role seoTitle, is how the site
	// composes the page title (schema.ParseTemplate).
	TitleTemplate string `json:"titleTemplate,omitempty"`
	// Excerpt, on a role fact with role seoDescription on rich text, is
	// schema.ExcerptFirstParagraph.
	Excerpt string `json:"excerpt,omitempty"`
	// When, on a role fact with role noindex or draft on a select, lists
	// the values that switch the role on (written as noindexWhen or
	// draftWhen).
	When []string `json:"when,omitempty"`
	// Fallback, on a localized fact, is the language an empty language
	// of the group shows; empty to say none.
	Fallback string `json:"fallback,omitempty"`
	// Alt, on an alt fact, names the sibling that holds the image's alt
	// text; AltFrom, instead, the siblings it is composed of, joined by
	// Separator (schema.DefaultAltSeparator when empty).
	Alt       string   `json:"alt,omitempty"`
	AltFrom   []string `json:"altFrom,omitempty"`
	Separator string   `json:"separator,omitempty"`
}

// ErrInvalid marks a fact that says something the schema cannot hold.
var ErrInvalid = errors.New("invalid fact")

// ErrNotFound marks a fact about something the schema does not declare.
var ErrNotFound = errors.New("not in the schema")

// ErrAmbiguous marks a fact whose place in the files is not one place.
var ErrAmbiguous = errors.New("ambiguous")

// ErrUnplaceable marks an edit the file's layout leaves no safe place
// for (a value spread over lines where one line goes, say).
var ErrUnplaceable = errors.New("cannot be placed")

var (
	localeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,34}$`)
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	keyRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,127}$`)
)

// checkNames checks a list of 1 to max distinct, non-blank entries of at
// most 200 bytes.
func checkNames(list []string, max int, what string) error {
	if len(list) > max {
		return fmt.Errorf("at most %d entries", max)
	}
	for i, v := range list {
		if strings.TrimSpace(v) == "" || len(v) > 200 {
			return fmt.Errorf("a %s is blank or longer than 200 bytes", what)
		}
		if slices.Contains(list[:i], v) {
			return fmt.Errorf("%q is listed twice", v)
		}
	}
	return nil
}

// Check reports what is wrong with a fact on its own, before any file
// is read.
func (f Fact) Check() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	switch f.Kind {
	case KindRole:
		if (f.Collection == "") == (f.Block == "") {
			return bad("a role fact names a collection or a block, one of them")
		}
		if f.Field == "" || len(f.Field) > MaxFieldPath {
			return bad("a role fact names a field (at most %d bytes)", MaxFieldPath)
		}
		if !schema.KnownRole(f.Role) {
			return bad("role %q is not one of %s", f.Role, strings.Join(schema.RoleValues(), ", "))
		}
		if f.WhenEmpty != "" && (!schema.KnownWhenEmpty(f.WhenEmpty) || !schema.RoleAllowsWhenEmpty(f.Role)) {
			return bad("whenEmpty %q does not go with role %s", f.WhenEmpty, f.Role)
		}
		if f.TitleTemplate != "" {
			if f.Role != schema.RoleSEOTitle {
				return bad("titleTemplate goes with role %s", schema.RoleSEOTitle)
			}
			if _, err := schema.ParseTemplate(f.TitleTemplate); err != nil {
				return bad("%v", err)
			}
		}
		if f.Excerpt != "" && (f.Role != schema.RoleSEODescription || !slices.Contains(schema.ExcerptValues(), f.Excerpt)) {
			return bad("excerpt %q does not go with role %s", f.Excerpt, f.Role)
		}
		if len(f.When) > 0 {
			if schema.RoleWhen(f.Role) == "" {
				return bad("values that switch a role on go with role %s or %s", schema.RoleNoindex, schema.RoleDraft)
			}
			if err := checkNames(f.When, schema.MaxRoleValues, "value"); err != nil {
				return bad("%v", err)
			}
		}
		if f.Fallback != "" || f.Alt != "" || len(f.AltFrom) > 0 || f.Separator != "" {
			return bad("a role fact takes no fallback or alt")
		}
	case KindLocalized:
		if (f.Collection == "") == (f.Block == "") {
			return bad("a localized fact names a collection or a block, one of them")
		}
		if f.Field == "" || len(f.Field) > MaxFieldPath {
			return bad("a localized fact names a group (at most %d bytes)", MaxFieldPath)
		}
		if f.Fallback != "" && !localeRe.MatchString(f.Fallback) {
			return bad("%q is not a language code", f.Fallback)
		}
	case KindAlt:
		if (f.Collection == "") == (f.Block == "") {
			return bad("an alt fact names a collection or a block, one of them")
		}
		if f.Field == "" || len(f.Field) > MaxFieldPath {
			return bad("an alt fact names an image field (at most %d bytes)", MaxFieldPath)
		}
		if (f.Alt == "") == (len(f.AltFrom) == 0) {
			return bad("an alt fact names one field (alt) or the fields it is composed of (altFrom), one of them")
		}
		if f.Alt != "" && !keyRe.MatchString(f.Alt) {
			return bad("%q is not a field key", f.Alt)
		}
		if err := checkNames(f.AltFrom, schema.MaxAltParts, "field"); err != nil {
			return bad("%v", err)
		}
		for _, k := range f.AltFrom {
			if !keyRe.MatchString(k) {
				return bad("%q is not a field key", k)
			}
		}
		if len(f.Separator) > schema.MaxSeparatorBytes || (f.Separator != "" && len(f.AltFrom) == 0) {
			return bad("a separator (at most %d bytes) goes with altFrom", schema.MaxSeparatorBytes)
		}
	case KindTranslationLocales:
		if f.Collection == "" || f.Block != "" {
			return bad("a translationLocales fact names a collection")
		}
		if len(f.Locales) == 0 || len(f.Locales) > MaxLocales {
			return bad("a translationLocales fact lists 1 to %d languages", MaxLocales)
		}
		for i, l := range f.Locales {
			if !localeRe.MatchString(l) {
				return bad("%q is not a language code", l)
			}
			if slices.Contains(f.Locales[:i], l) {
				return bad("%q is listed twice", l)
			}
		}
	case KindOptimizedAtBuild:
		if f.Storage == "" || f.OptimizedAtBuild == nil {
			return bad("an optimizedAtBuild fact names a storage and a value")
		}
	case KindUsedByCode:
		if f.Storage == "" || len(f.Paths) == 0 || len(f.Paths) > MaxPaths {
			return bad("a usedByCode fact names a storage and 1 to %d paths", MaxPaths)
		}
		for i, p := range f.Paths {
			if err := schema.CheckCodePattern(p); err != nil {
				return bad("%s", strings.TrimPrefix(err.Error(), schema.ErrCodePattern.Error()+": "))
			}
			if slices.Contains(f.Paths[:i], p) {
				return bad("%q is listed twice", p)
			}
		}
	default:
		return bad("kind %q is not one of %s", f.Kind, strings.Join(Kinds(), ", "))
	}
	for _, n := range []string{f.Collection, f.Block, f.Storage} {
		if n != "" && !nameRe.MatchString(n) {
			return bad("%q is not a name", n)
		}
	}
	return nil
}

// String is the fact in a line, for a commit message.
func (f Fact) String() string {
	switch f.Kind {
	case KindRole:
		owner := f.Collection
		if owner == "" {
			owner = "block " + f.Block
		}
		s := fmt.Sprintf("%s.%s: role %s", owner, f.Field, f.Role)
		if f.WhenEmpty != "" {
			s += ", whenEmpty " + f.WhenEmpty
		}
		if f.TitleTemplate != "" {
			s += ", titleTemplate " + f.TitleTemplate
		}
		if f.Excerpt != "" {
			s += ", excerpt " + f.Excerpt
		}
		if len(f.When) > 0 {
			s += ", " + schema.RoleWhen(f.Role) + " [" + strings.Join(f.When, ", ") + "]"
		}
		return s
	case KindLocalized:
		s := fmt.Sprintf("%s.%s: localized", f.owner(), f.Field)
		if f.Fallback != "" {
			s += ", fallback " + f.Fallback
		}
		return s
	case KindAlt:
		if f.Alt != "" {
			return fmt.Sprintf("%s.%s: alt %s", f.owner(), f.Field, f.Alt)
		}
		return fmt.Sprintf("%s.%s: alt from %s", f.owner(), f.Field, strings.Join(f.AltFrom, ", "))
	case KindTranslationLocales:
		return fmt.Sprintf("%s: translations.locales [%s]", f.Collection, strings.Join(f.Locales, ", "))
	case KindOptimizedAtBuild:
		v := false
		if f.OptimizedAtBuild != nil {
			v = *f.OptimizedAtBuild
		}
		return fmt.Sprintf("storage %s: optimizedAtBuild %t", f.Storage, v)
	case KindUsedByCode:
		return fmt.Sprintf("storage %s: usedByCode + %s", f.Storage, strings.Join(f.Paths, ", "))
	}
	return f.Kind
}

func (f Fact) owner() string {
	if f.Collection != "" {
		return f.Collection
	}
	return "block " + f.Block
}

// FactError is the error of one fact of a Plan.
type FactError struct {
	// Index is the fact's place in the list given to Plan.
	Index int
	Err   error
}

func (e *FactError) Error() string { return fmt.Sprintf("fact %d: %v", e.Index, e.Err) }
func (e *FactError) Unwrap() error { return e.Err }

// Change is one schema file a Plan changes.
type Change struct {
	// Path is the file's repository path.
	Path          string
	Before, After []byte
}

// Plan is what a list of facts does to a site's schema files.
type Plan struct {
	// Edits holds one edit per fact, in the order given.
	Edits []Edit
	// Changes lists the files whose bytes change, in the order of the
	// first fact that touches each. A fact already true changes nothing.
	Changes []Change
}

// Make locates every fact in the site's schema and applies it to its
// file, the facts on one file in the order given. It reads the schema
// files through fsys and writes nothing.
func Make(fsys fs.FS, facts []Fact) (*Plan, error) {
	if len(facts) == 0 || len(facts) > MaxFacts {
		return nil, fmt.Errorf("%w: 1 to %d facts", ErrInvalid, MaxFacts)
	}
	for i, f := range facts {
		if err := f.Check(); err != nil {
			return nil, &FactError{Index: i, Err: err}
		}
	}
	l, err := relocate(fsys)
	if err != nil {
		return nil, err
	}
	p := &Plan{}
	files := map[string]int{}
	for i, f := range facts {
		e, err := l.locate(f)
		if err != nil {
			return nil, &FactError{Index: i, Err: err}
		}
		p.Edits = append(p.Edits, e)
		idx, seen := files[e.File]
		if !seen {
			raw, err := fs.ReadFile(fsys, e.File)
			if err != nil {
				return nil, &FactError{Index: i, Err: err}
			}
			p.Changes = append(p.Changes, Change{Path: e.File, Before: raw, After: raw})
			idx = len(p.Changes) - 1
			files[e.File] = idx
		}
		out, err := Apply(p.Changes[idx].After, e)
		if err != nil {
			return nil, &FactError{Index: i, Err: err}
		}
		changed := string(out) != string(p.Changes[idx].After)
		p.Changes[idx].After = out
		if changed && i < len(facts)-1 {
			// A later fact is judged on the schema as this one leaves it
			// (a role on a group this plan makes localized).
			if l, err = relocate(p.FS(fsys)); err != nil {
				return nil, &FactError{Index: i, Err: err}
			}
		}
	}
	kept := p.Changes[:0]
	for _, c := range p.Changes {
		if string(c.Before) != string(c.After) {
			kept = append(kept, c)
		}
	}
	p.Changes = kept
	return p, nil
}

// relocate loads the schema of fsys and a locator over it.
func relocate(fsys fs.FS) (*locator, error) {
	doc, _, err := load.Load(fsys)
	if err != nil {
		return nil, err
	}
	return newLocator(fsys, doc)
}

// FS is fsys with the plan's files changed, for a check of the result.
func (p *Plan) FS(fsys fs.FS) fs.FS {
	files := map[string][]byte{}
	for _, c := range p.Changes {
		files[c.Path] = c.After
	}
	return overlayFS{base: fsys, files: files}
}

// NewErrors validates the schema before and after the plan and returns
// the errors the plan would bring: findings at error severity that the
// schema as it is does not have. Errors the site already has are not
// the plan's.
func (p *Plan) NewErrors(fsys fs.FS) ([]report.Finding, error) {
	opts := validate.Options{Only: []string{validate.ScopeSchema}}
	before, err := validate.Site(fsys, opts)
	if err != nil {
		return nil, err
	}
	after, err := validate.Site(p.FS(fsys), opts)
	if err != nil {
		return nil, err
	}
	key := func(f report.Finding) string {
		return f.File + "\x00" + f.Path + "\x00" + string(f.Rule) + "\x00" + f.Message
	}
	had := map[string]bool{}
	for _, f := range before.Findings {
		if f.Severity == report.SeverityError {
			had[key(f)] = true
		}
	}
	var out []report.Finding
	for _, f := range after.Findings {
		if f.Severity == report.SeverityError && !had[key(f)] {
			out = append(out, f)
		}
	}
	return out, nil
}
