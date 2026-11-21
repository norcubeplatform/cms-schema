package validate

import (
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/report"
)

// TestRecoverAs pins what an entry point returns when a check panics:
// one error finding with the check's rule, on the file, quoting a
// bounded cause, instead of the findings gathered so far.
func TestRecoverAs(t *testing.T) {
	check := func() (out []report.Finding) {
		defer recoverAs(&out, "p.md", report.RuleContentInternalError)
		out = append(out, report.Finding{Rule: report.RuleContentRequired})
		var m map[string]any
		_ = m["x"].([]any)[strings.Count("", "")+5]
		return out
	}
	got := check()
	if len(got) != 1 || got[0].Rule != report.RuleContentInternalError || got[0].File != "p.md" || got[0].Severity != report.SeverityError {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(got[0].Message, "interface conversion") {
		t.Errorf("message does not carry the cause: %q", got[0].Message)
	}
	long := internalError("", report.RuleLinksInternalError, strings.Repeat("x", 10*maxPanicText))
	if n := len([]rune(long.Message)); n > maxPanicText+100 {
		t.Errorf("message of %d runes is not bounded", n)
	}
}

// TestRecoverAsWithoutPanic: a check that does not panic keeps its
// findings.
func TestRecoverAsWithoutPanic(t *testing.T) {
	ok := func() (out []report.Finding) {
		defer recoverAs(&out, "", report.RuleLinksInternalError)
		return []report.Finding{{Rule: report.RuleLinksMissingAsset}}
	}
	if got := ok(); len(got) != 1 || got[0].Rule != report.RuleLinksMissingAsset {
		t.Fatalf("got %v", got)
	}
}
