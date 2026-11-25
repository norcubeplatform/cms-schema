// Package quality checks what a site could do better for its readers,
// as opposed to what is broken: titles and descriptions for search
// results, alt text and link text for people who use a screen reader,
// missing and outdated language versions, heavy and unused media.
//
// Every rule belongs to one area and has an impact. Its findings are
// warnings (high and medium impact) or notices (low impact and
// information), never errors, so a site's CI that fails on errors keeps
// passing. The thresholds below apply when the schema says nothing; a
// field's `recommendedLength` wins.
//
// The module never reads the network or git: what only the caller knows
// (what a language version's source said when the version was last
// current, which media files exist and how large they are) comes in
// Options, and a rule whose data is missing is
// skipped and said so in the Summary.
package quality

import (
	"github.com/norcubeplatform/cms-schema/report"
)

// Area groups the rules by what they are about.
type Area string

const (
	AreaSearch        Area = "search"
	AreaAccessibility Area = "accessibility"
	AreaLanguages     Area = "languages"
	AreaMedia         Area = "media"
)

// Areas lists the areas in the order they are shown.
func Areas() []Area {
	return []Area{AreaSearch, AreaAccessibility, AreaLanguages, AreaMedia}
}

// Data a rule needs from the caller.
const (
	NeedsNothing = ""
	NeedsHistory = "history"
	NeedsMedia   = "media"
)

// RuleSpec is one quality rule: its area, its impact and the data it
// needs.
type RuleSpec struct {
	Rule report.Rule
	Area Area
	// Impact is what a finding of the rule weighs at most. A finding may
	// weigh less (an image over the lower size limit, a check on a field
	// whose role was guessed). ImpactInfo rules are listed, not scored.
	Impact report.Impact
	// Needs names the caller data the rule runs on (Options); empty for
	// none. What a rule holds content against, and where that comes
	// from, is in the README's table of rules.
	Needs string
}

// Scored reports whether findings of the rule count against a score.
func (s RuleSpec) Scored() bool { return s.Impact != report.ImpactInfo }

// Default thresholds, used when the schema declares none.
const (
	DefaultTitleMin       = 30
	DefaultTitleMax       = 60
	DefaultDescriptionMin = 70
	DefaultDescriptionMax = 160
	// DefaultImageHeavyBytes and DefaultImageVeryHeavyBytes are the two
	// size limits of an image content uses: over the first is low impact,
	// over the second medium.
	DefaultImageHeavyBytes     = 200 * 1024
	DefaultImageVeryHeavyBytes = 1024 * 1024
)

// Bounds.
const (
	// MaxMediaFiles bounds the media files one run looks at; the rest
	// are not checked.
	MaxMediaFiles = 50000
	// MaxNamed bounds how many other pages a duplicate finding names.
	MaxNamed = 5
	// maxTextScan bounds the characters of one text scanned for headings
	// and links.
	maxTextScan = 200000
	// maxValueRunes bounds a finding's Value.
	maxValueRunes = 500
	// MaxBaseQueries bounds the language versions one run asks
	// Options.SourceBases about; the rest are not judged.
	MaxBaseQueries = 5000
)

var specs = []RuleSpec{
	{report.RuleQualityTitleMissing, AreaSearch, report.ImpactHigh, NeedsNothing},
	{report.RuleQualityTitleLength, AreaSearch, report.ImpactLow, NeedsNothing},
	{report.RuleQualityTitleDuplicate, AreaSearch, report.ImpactMedium, NeedsNothing},
	{report.RuleQualityDescriptionMissing, AreaSearch, report.ImpactMedium, NeedsNothing},
	{report.RuleQualityDescriptionLength, AreaSearch, report.ImpactLow, NeedsNothing},
	{report.RuleQualityDescriptionDuplicate, AreaSearch, report.ImpactMedium, NeedsNothing},
	{report.RuleQualityShareImageMissing, AreaSearch, report.ImpactLow, NeedsNothing},
	{report.RuleQualityNoindex, AreaSearch, report.ImpactInfo, NeedsNothing},
	{report.RuleQualityImageAltMissing, AreaAccessibility, report.ImpactHigh, NeedsNothing},
	{report.RuleQualityImageAltPoor, AreaAccessibility, report.ImpactLow, NeedsNothing},
	{report.RuleQualityHeadingSkip, AreaAccessibility, report.ImpactLow, NeedsNothing},
	{report.RuleQualityLinkTextEmpty, AreaAccessibility, report.ImpactMedium, NeedsNothing},
	{report.RuleQualityLinkTextGeneric, AreaAccessibility, report.ImpactLow, NeedsNothing},
	{report.RuleQualityTranslationMissing, AreaLanguages, report.ImpactMedium, NeedsNothing},
	{report.RuleQualityTranslationOutdated, AreaLanguages, report.ImpactMedium, NeedsHistory},
	{report.RuleQualityTranslationIncomplete, AreaLanguages, report.ImpactLow, NeedsNothing},
	{report.RuleQualityImageHeavy, AreaMedia, report.ImpactMedium, NeedsMedia},
	{report.RuleQualityMediaUnused, AreaMedia, report.ImpactInfo, NeedsMedia},
}

// Specs lists every quality rule, in the order of report.QualityRules.
func Specs() []RuleSpec { return append([]RuleSpec(nil), specs...) }

// Spec returns the spec of a quality rule.
func Spec(r report.Rule) (RuleSpec, bool) {
	for _, s := range specs {
		if s.Rule == r {
			return s, true
		}
	}
	return RuleSpec{}, false
}
