// Package report holds the finding type every check in this module
// produces and the frozen list of rule identifiers.
//
// A rule id is a stable, machine-readable name of one check
// (`schema.unresolved_ref`, `content.required`). Scripts and CI
// configurations match on these ids, so an id is never renamed or
// reused for a different check once released; a check that changes
// meaning gets a new id and the old one is retired.
package report

import (
	"fmt"
	"sort"
	"strings"
)

// Severity of a finding.
type Severity string

const (
	// SeverityError marks a finding the site should not ship with: the
	// editor would refuse the value, the build would break, or a link
	// provably points at nothing.
	SeverityError Severity = "error"
	// SeverityWarning marks a finding worth a look that does not break
	// anything by itself.
	SeverityWarning Severity = "warning"
	// SeverityNotice marks a content quality finding of low impact, or
	// one that is information only. Notices never fail a run.
	SeverityNotice Severity = "notice"
)

// Impact says how much a content quality finding matters, for the
// order a site owner works in and for a score. Only `quality.` findings
// carry one.
type Impact string

const (
	ImpactHigh   Impact = "high"
	ImpactMedium Impact = "medium"
	ImpactLow    Impact = "low"
	// ImpactInfo is a fact worth listing that is not a problem (a page
	// hidden from search engines, a file nothing uses).
	ImpactInfo Impact = "info"
)

// SeverityOf is the severity of a quality finding of the impact: high
// and medium are warnings, low and info notices. Quality findings are
// never errors.
func SeverityOf(i Impact) Severity {
	switch i {
	case ImpactHigh, ImpactMedium:
		return SeverityWarning
	}
	return SeverityNotice
}

// Units of a Measure.
const (
	UnitCharacters = "characters"
	UnitBytes      = "bytes"
	UnitDays       = "days"
	UnitPages      = "pages"
	UnitLevel      = "level"
	UnitFields     = "fields"
	// UnitChangedTexts counts the translatable texts a language version's
	// source changed since the version was last current with it.
	UnitChangedTexts = "changedTexts"
)

// Measure is the number a quality check measured, with the window it
// was held against when there is one: a title of 72 characters against
// 30 to 60, an image of 1.4 MB against 200 KB.
type Measure struct {
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
	Min   *int64 `json:"min,omitempty"`
	Max   *int64 `json:"max,omitempty"`
}

// Finding is one problem found in a schema file, a content file or the
// links between them.
type Finding struct {
	// File is the repository-relative path of the file the finding is
	// about. Empty for site-wide findings.
	File string `json:"file,omitempty"`
	// Path is the field path inside the file (`blocks[2].heading`),
	// empty for file-level findings.
	Path string `json:"path,omitempty"`
	// Severity is error or warning.
	Severity Severity `json:"severity"`
	// Rule is the frozen identifier of the check (see rules.go).
	Rule Rule `json:"rule"`
	// Message says what is wrong, in one sentence.
	Message string `json:"message"`
	// Hint says how to fix it, when the check knows.
	Hint string `json:"hint,omitempty"`

	// Impact, Value and Measure are set on `quality.` findings only.
	Impact Impact `json:"impact,omitempty"`
	// Value is what the finding is about as the site holds it: the
	// title, the alt text, the link text, the missing language, the
	// file. When it changes, the finding is a different one.
	Value string `json:"value,omitempty"`
	// Measure is the number the check measured, when it measured one.
	Measure *Measure `json:"measure,omitempty"`
	// Basis says how Value was made from the content when it is not one
	// field's text as written: BasisTitleTemplate, BasisExcerpt or
	// BasisComposedAlt. Value is then what the site shows, not what to
	// write into the field.
	Basis string `json:"basis,omitempty"`
	// Locale is the language a finding is about on a page whose file
	// holds every language (localized groups); empty on other pages,
	// whose file is one language.
	Locale string `json:"locale,omitempty"`
}

// Bases of a quality finding's Value.
const (
	// BasisTitleTemplate: the seoTitle field as its titleTemplate composes
	// the page title.
	BasisTitleTemplate = "titleTemplate"
	// BasisExcerpt: the plain text of a rich text description's first
	// paragraph (excerpt: firstParagraph).
	BasisExcerpt = "excerpt"
	// BasisComposedAlt: an image's alt text joined from several fields
	// (alt: {from: [...]}).
	BasisComposedAlt = "composedAlt"
)

// Bases lists the values of Finding.Basis.
func Bases() []string { return []string{BasisTitleTemplate, BasisExcerpt, BasisComposedAlt} }

// String renders the finding the way a compiler would: file, path,
// severity, rule and message on one line.
func (f Finding) String() string {
	var b strings.Builder
	if f.File != "" {
		b.WriteString(f.File)
		if f.Path != "" {
			b.WriteString("#")
			b.WriteString(f.Path)
		}
		b.WriteString(": ")
	}
	fmt.Fprintf(&b, "%s [%s] %s", f.Severity, f.Rule, f.Message)
	return b.String()
}

// severityRank orders severities, the most severe first.
func severityRank(s Severity) int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	}
	return 2
}

// Sort orders findings by file, path, severity (errors first), rule and
// message, so two runs over the same site print the same list.
func Sort(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Severity != b.Severity {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// Count returns how many findings are errors and how many are warnings.
// Notices are in neither; CountNotices counts them.
func Count(findings []Finding) (errors, warnings int) {
	for _, f := range findings {
		switch f.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		}
	}
	return errors, warnings
}

// CountNotices returns how many findings are notices.
func CountNotices(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if f.Severity == SeverityNotice {
			n++
		}
	}
	return n
}

// IsQuality reports whether a rule is a content quality check.
func (r Rule) IsQuality() bool { return strings.HasPrefix(string(r), qualityPrefix) }
