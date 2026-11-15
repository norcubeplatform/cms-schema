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
)

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
}

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
			return a.Severity == SeverityError
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// Count returns how many findings are errors and how many are warnings.
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
