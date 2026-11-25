package quality

import (
	"slices"
	"strconv"

	"github.com/norcubeplatform/cms-schema/report"
)

// SkipReason says why a quality rule did not run. Frozen like rule ids:
// never renamed or reused.
type SkipReason string

// Why a rule did not run.
const (
	// SkipHistoryNotGiven: Options.SourceBases is nil, so
	// quality.translation_outdated has nothing to compare with.
	SkipHistoryNotGiven SkipReason = "history_not_given"
	// SkipHistoryUnreadable: Options.SourceBases failed without an answer.
	SkipHistoryUnreadable SkipReason = "history_unreadable"
	// SkipLanguagesCapped: more language versions than MaxBaseQueries
	// (Params.Max); the rest were not judged.
	SkipLanguagesCapped SkipReason = "languages_capped"
	// SkipMediaNotListed: Options.ListMedia is nil.
	SkipMediaNotListed SkipReason = "media_not_listed"
	// SkipMediaUnlistable: Options.ListMedia failed (Params.Detail).
	SkipMediaUnlistable SkipReason = "media_unlistable"
)

// SkipReasonSpec is one reason and the Params keys it sets.
type SkipReasonSpec struct {
	Reason SkipReason        `json:"reason"`
	Params []report.ParamKey `json:"params"`

	english func(p *report.Params) string
}

var skipReasons = []SkipReasonSpec{
	{SkipHistoryNotGiven, []report.ParamKey{}, func(*report.Params) string { return "the history of the content files was not given" }},
	{SkipHistoryUnreadable, []report.ParamKey{}, func(*report.Params) string { return "the history of the content files could not be read" }},
	{SkipLanguagesCapped, []report.ParamKey{report.ParamMax}, func(p *report.Params) string {
		max := 0.0
		if p.Max != nil {
			max = *p.Max
		}
		return "only the first " + report.FormatNumber(max) + " language versions were judged"
	}},
	{SkipMediaNotListed, []report.ParamKey{}, func(*report.Params) string { return "the media files were not listed" }},
	{SkipMediaUnlistable, []report.ParamKey{report.ParamDetail}, func(p *report.Params) string {
		return "the media files could not be listed: " + p.Detail
	}},
}

// SkipReasons lists every reason a rule is skipped, with its params.
func SkipReasons() []SkipReasonSpec { return slices.Clone(skipReasons) }

// Skipped is a rule that did not run, and why: a reason code with its
// params, and Message, the reason in English for terminals and logs. A
// client that words it in another language builds its sentence from
// Reason and Params.
type Skipped struct {
	Rule    report.Rule    `json:"rule"`
	Reason  SkipReason     `json:"reason"`
	Params  *report.Params `json:"params,omitempty"`
	Message string         `json:"message"`
}

// English is the reason in English, made from Reason and Params.
func (s Skipped) English() string {
	p := s.Params
	if p == nil {
		p = &report.Params{}
	}
	for _, spec := range skipReasons {
		if spec.Reason == s.Reason {
			return spec.english(p)
		}
	}
	return strconv.Quote(string(s.Reason))
}

func newSkipped(rule report.Rule, reason SkipReason, p *report.Params) Skipped {
	s := Skipped{Rule: rule, Reason: reason, Params: p.Bounded()}
	s.Message = s.English()
	return s
}
