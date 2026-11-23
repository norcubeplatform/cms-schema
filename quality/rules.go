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
// (when each file last changed, which media files exist and how large
// they are) comes in Options, and a rule whose data is missing is
// skipped and said so in the Summary.
package quality

import (
	"time"

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
	NeedsNothing     = ""
	NeedsLastChanged = "lastChanged"
	NeedsMedia       = "media"
)

// RuleSpec is one quality rule: its area, its impact, the threshold it
// holds content against and where that comes from.
type RuleSpec struct {
	Rule report.Rule
	Area Area
	// Impact is what a finding of the rule weighs at most. A finding may
	// weigh less (an image over the lower size limit, a check on a field
	// whose role was guessed). ImpactInfo rules are listed, not scored.
	Impact report.Impact
	// Needs names the caller data the rule runs on (Options); empty for
	// none.
	Needs string
	// Threshold and Source are the README's wording.
	Threshold string
	Source    string
}

// Scored reports whether findings of the rule count against a score.
func (s RuleSpec) Scored() bool { return s.Impact != report.ImpactInfo }

// Default thresholds, used when the schema declares none.
const (
	DefaultTitleMin       = 30
	DefaultTitleMax       = 60
	DefaultDescriptionMin = 70
	DefaultDescriptionMax = 160
	// DefaultOutdatedAfter is how much older than the newest version of
	// a page another language version may be before it is outdated.
	DefaultOutdatedAfter = 7 * 24 * time.Hour
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
)

var specs = []RuleSpec{
	{report.RuleQualityTitleMissing, AreaSearch, report.ImpactHigh, NeedsNothing,
		"No SEO title and no page title to fall back to", "Lighthouse document-title"},
	{report.RuleQualityTitleLength, AreaSearch, report.ImpactLow, NeedsNothing,
		"Outside 30 to 60 characters, or the field's recommendedLength", "Google sets no limit and cuts titles to the device width (Search Central, title links); 30 to 60 is the display convention tools use"},
	{report.RuleQualityTitleDuplicate, AreaSearch, report.ImpactMedium, NeedsNothing,
		"The same title on two or more indexable pages of one language, after trimming and ignoring case", "Google Search Central, title links"},
	{report.RuleQualityDescriptionMissing, AreaSearch, report.ImpactMedium, NeedsNothing,
		"An empty meta description", "Lighthouse meta-description"},
	{report.RuleQualityDescriptionLength, AreaSearch, report.ImpactLow, NeedsNothing,
		"Outside 70 to 160 characters, or the field's recommendedLength", "Google sets no limit and cuts snippets to fit (Search Central, snippets); 70 to 160 is the display convention tools use"},
	{report.RuleQualityDescriptionDuplicate, AreaSearch, report.ImpactMedium, NeedsNothing,
		"The same meta description on two or more indexable pages of one language", "Google Search Central, snippets"},
	{report.RuleQualityShareImageMissing, AreaSearch, report.ImpactLow, NeedsNothing,
		"No share image on a page whose collection has a share image field", "Meta, sharing best practices"},
	{report.RuleQualityNoindex, AreaSearch, report.ImpactInfo, NeedsNothing,
		"The page's noindex field is true", ""},
	{report.RuleQualityImageAltMissing, AreaAccessibility, report.ImpactHigh, NeedsNothing,
		"An image that is not decorative with empty alt text", "WCAG 2.2 success criterion 1.1.1 (level A); Lighthouse image-alt"},
	{report.RuleQualityImageAltPoor, AreaAccessibility, report.ImpactLow, NeedsNothing,
		"Alt text that is the file name or a generic word (image, photo, obrázek, Bild…)", "Google Search Central, image SEO"},
	{report.RuleQualityHeadingSkip, AreaAccessibility, report.ImpactLow, NeedsNothing,
		"A heading more than one level below the heading before it, in one text", "axe heading-order (a best practice, not a WCAG requirement)"},
	{report.RuleQualityLinkTextEmpty, AreaAccessibility, report.ImpactMedium, NeedsNothing,
		"A link with no text and no alt text inside it", "WCAG 2.2 success criterion 2.4.4 (level A)"},
	{report.RuleQualityLinkTextGeneric, AreaAccessibility, report.ImpactLow, NeedsNothing,
		"Link text that is only a generic phrase (click here, read more, zde, více, hier…)", "WCAG 2.2 success criterion 2.4.4 (level A); Lighthouse link-text"},
	{report.RuleQualityTranslationMissing, AreaLanguages, report.ImpactMedium, NeedsNothing,
		"A page with no version in a language its collection expects (translations.locales, else every site language)", ""},
	{report.RuleQualityTranslationOutdated, AreaLanguages, report.ImpactMedium, NeedsLastChanged,
		"A version last changed more than 7 days before the newest version of the same page", "A convention"},
	{report.RuleQualityImageHeavy, AreaMedia, report.ImpactMedium, NeedsMedia,
		"A raster image content uses over 200 KB (low impact) or over 1 MB (medium); quality.limits.imageKB moves both. Images in a folder the build optimizes (optimizedAtBuild) are skipped", "Lighthouse uses-optimized-images; crawlers flag 100 KB, which Sitebulb calls relatively arbitrary, so 200 KB flags only clear cases"},
	{report.RuleQualityMediaUnused, AreaMedia, report.ImpactInfo, NeedsMedia,
		"A file in a read-write repository storage that no content and no schema refers to, and that its storage's usedByCode does not name", ""},
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
