package validate

import (
	"fmt"

	"github.com/norcubeplatform/cms-schema/report"
)

// maxPanicText bounds the cause quoted in an internal-error finding.
const maxPanicText = 300

// recoverAs turns a panic in one check into a single finding with the
// check's internal-error rule, so a bug in this module degrades to a
// finding instead of ending the caller's process. It must be deferred
// directly by an exported entry point; the findings that check had
// gathered before the panic are replaced by that one finding.
func recoverAs(out *[]report.Finding, file string, rule report.Rule) {
	r := recover()
	if r == nil {
		return
	}
	*out = []report.Finding{internalError(file, rule, r)}
}

func internalError(file string, rule report.Rule, cause any) report.Finding {
	text := []rune(fmt.Sprint(cause))
	if len(text) > maxPanicText {
		text = append(text[:maxPanicText], '…')
	}
	return report.Finding{
		File:     file,
		Severity: report.SeverityError,
		Rule:     rule,
		Message:  "the check stopped on an internal error: " + string(text),
		Hint:     "This is a bug in the validator, not necessarily in the site. Its other findings for this check are missing; report the message to Norcube.",
	}
}
