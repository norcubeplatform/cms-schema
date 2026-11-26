package report

import (
	"strconv"
	"strings"
)

const (
	notice           = SeverityNotice
	linkTextHint     = "People using a screen reader often jump from link to link. Say what they get (\"See our pricing\") instead."
	describeHint     = "Describe what the image shows, for people who cannot see it. If it is only decoration, a developer can mark it `decorative: true`."
	descriptionsHint = "Search engines then pick a snippet from the page. Write one or two sentences that make someone want to open it. If the site writes descriptions in code, say so in the schema with `whenEmpty` on the field."
	poorAltHint      = "Say what the image shows or why it is there, in a few words."
	heavyHint        = "A large image makes the page slow to load, above all on a phone. Save it smaller (about 2000 pixels wide is plenty) or as WebP, and upload it again."
)

// lengthSubject is what a length finding measured: the title or the
// description, or what the site composes of it (Finding.Basis).
func lengthSubject(x in, what string) string {
	switch x.f.Basis {
	case BasisTitleTemplate:
		return "The title as the site composes it"
	case BasisExcerpt:
		return "The description (the first paragraph of the text)"
	}
	return "The " + what
}

func lengthSpecs(rule Rule, what string) []CauseSpec {
	return []CauseSpec{
		spec(rule, CauseTooShort, notice, "current min max", "",
			func(x in) string {
				return lengthSubject(x, what) + " has " + num(x.Current) + " characters, fewer than the recommended " + num(x.Min) + " to " + num(x.Max) + "."
			}, text("A longer "+what+" says more about the page in search results.")),
		spec(rule, CauseTooLong, notice, "current min max", "",
			func(x in) string {
				return lengthSubject(x, what) + " has " + num(x.Current) + " characters, more than the recommended " + num(x.Min) + " to " + num(x.Max) + "."
			}, text("Search results cut a long "+what+" off; put what matters first or shorten it.")),
	}
}

func duplicateSpec(rule Rule, what string) CauseSpec {
	return spec(rule, "", warn, "count names", "more",
		func(x in) string {
			return integer(x.Count) + " pages share this " + what + ": " + comma(x.Names) + more(x.Params) + "."
		}, text("Give each page a "+what+" of its own, so people can tell them apart in search results."))
}

// heavyHintFor adds what the site can say about the image's folder: a
// guess from the framework (Framework set) to confirm, or the knob.
func heavyHintFor(x in) string {
	if x.Framework != "" {
		return heavyHint + " The folder is taken to be served as is, a guess from the site's framework: say `optimizedAtBuild: false` on its storage to confirm it, or `true` if the build resizes its images."
	}
	return heavyHint + " If the site's build resizes the images of this folder, say `optimizedAtBuild: true` on its storage."
}

var qualityCauses = append(append([]CauseSpec{
	spec(RuleQualityTitleMissing, "", warn, "", "",
		text("The page has no title for search results."),
		text("Write a title that says what the page is about, in a few words.")),
	duplicateSpec(RuleQualityTitleDuplicate, "title"),
	spec(RuleQualityDescriptionMissing, CauseEmpty, warn, "", "",
		text("The page has no meta description."), text(descriptionsHint)),
	spec(RuleQualityDescriptionMissing, CauseExcerptEmpty, warn, "", "",
		text("The page has no meta description: the first paragraph of its text is empty."), text(descriptionsHint)),
	duplicateSpec(RuleQualityDescriptionDuplicate, "description"),
	spec(RuleQualityShareImageMissing, "", notice, "", "",
		text("The page has no share image."),
		text("A link to it shared on social networks or in chat shows no picture, unless the site adds one in code. Pick an image of at least 1200 x 630 pixels, or, if the site makes these images itself, say so in the schema with `whenEmpty: generated` on the field.")),
	// field is the noindex field's label, when it has one.
	spec(RuleQualityNoindex, CauseFlag, notice, "", "field",
		text("The page is hidden from search engines."),
		func(x in) string {
			return "That is fine for a thank-you page or a draft; turn off " + quotedLabel(x.Field, "the no-index field") + " if people should find it in search."
		}),
	spec(RuleQualityNoindex, CauseValue, notice, "value", "field",
		func(x in) string { return "The page is hidden from search engines (" + x.Value + ")." },
		func(x in) string {
			return "That is fine for a thank-you page; pick a value of " + quotedLabel(x.Field, "the indexing field") + " that lets search engines index it if people should find it in search."
		}),
	spec(RuleQualityImageAltMissing, CauseNoAltField, notice, "", "",
		text("The image has no alt text, and the schema gives it no field for one."),
		text("Ask a developer to name the field that holds its alt text (`alt:` on the image field) or to mark the image `decorative: true`.")),
	spec(RuleQualityImageAltMissing, CauseEmpty, warn, "", "",
		text("The image has no alt text."), text(describeHint)),
	spec(RuleQualityImageAltMissing, CauseComposedEmpty, warn, "", "",
		text("The image has no alt text: every field it is made of is empty."), text(describeHint)),
	spec(RuleQualityImageAltMissing, CauseGuessedEmpty, notice, "", "",
		text("The image has no alt text (the field for it was guessed from field names)."), text(describeHint)),
	// path is the image's address as the text writes it, when it has one.
	spec(RuleQualityImageAltMissing, CauseInText, warn, "", "path",
		func(x in) string {
			if x.Path == "" {
				return "An image in the text has no address and no alt text."
			}
			return "The image " + x.Path + " in the text has no alt text."
		},
		text("Describe what the image shows between the square brackets: ![a team at work](...).")),
	spec(RuleQualityImageAltPoor, CauseGenericWord, notice, "value", "",
		func(x in) string { return "The image's alt text " + q(x.Value) + " is a generic word." }, text(poorAltHint)),
	spec(RuleQualityImageAltPoor, CauseCameraFileName, notice, "value", "",
		func(x in) string { return "The image's alt text " + q(x.Value) + " is a camera's file name." }, text(poorAltHint)),
	spec(RuleQualityImageAltPoor, CauseFileName, notice, "value", "",
		func(x in) string { return "The image's alt text " + q(x.Value) + " is the file name." }, text(poorAltHint)),
	spec(RuleQualityHeadingSkip, "", notice, "value level expectedLevel", "",
		func(x in) string {
			return "The heading " + q(x.Value) + " is level " + integer(x.Level) + " right after a level " + integer(Int(deref(x.ExpectedLevel)-1)) + " heading."
		},
		func(x in) string {
			return "Make it a level " + integer(x.ExpectedLevel) + " heading, so people who move through a page by its headings do not miss a level."
		}),
	// path is the link's address, when it has one.
	spec(RuleQualityLinkTextEmpty, "", warn, "", "path",
		func(x in) string { return "A link to " + orNowhere(x.Path) + " has no text." },
		text("A screen reader announces such a link by its address, or not at all. Give it words that say where it leads.")),
	spec(RuleQualityLinkTextGeneric, CauseLabel, notice, "value", "",
		func(x in) string { return "The link's text " + q(x.Value) + " does not say where it leads." }, text(linkTextHint)),
	spec(RuleQualityLinkTextGeneric, CauseInText, notice, "value", "path",
		func(x in) string {
			return "The link " + q(x.Value) + " to " + orNowhere(x.Path) + " does not say where it leads."
		},
		text(linkTextHint)),
	spec(RuleQualityTranslationMissing, CauseDraft, notice, "locale", "",
		func(x in) string { return "The " + q(x.Locale) + " version is a draft, not published yet." },
		text("Publish it when it is ready (turn its draft field off); until then readers of that language do not see the page.")),
	spec(RuleQualityTranslationMissing, CauseMissing, warn, "locale", "",
		func(x in) string { return "The page has no version in " + q(x.Locale) + "." },
		text("Create the version in that language, or, if the collection is not meant to have one, list its languages in `translations.locales`.")),
	// locale and path are the source's: the version in the site's
	// default language the finding's file was translated from.
	spec(RuleQualityTranslationOutdated, "", warn, "locale path count names", "more",
		func(x in) string {
			return "The " + q(x.Locale) + " version (" + x.Path + ") changed " + integer(x.Count) + " " + plural(deref(x.Count), "text") + " since this version was last current with it: " + comma(x.Names) + more(x.Params) + "."
		}, text("Check whether this version still says what the source says now. If it does, mark it up to date; if not, update the texts that changed.")),
	// names are the texts' labels; sameAs, entry for entry, the language
	// a text is the same as, "" where it is empty.
	spec(RuleQualityTranslationIncomplete, "", notice, "locale count total names sameAs", "more",
		func(x in) string {
			named := make([]string, len(x.Names))
			for i, n := range x.Names {
				why := "empty"
				if i < len(x.SameAs) && x.SameAs[i] != "" {
					why = "same as " + x.SameAs[i]
				}
				named[i] = n + " (" + why + ")"
			}
			return "The " + q(x.Locale) + " version leaves " + integer(x.Count) + " of " + integer(x.Total) + " texts untranslated: " + comma(named) + more(x.Params) + "."
		}, text("Write these texts in that language. Until then its readers see them empty or in the fallback language.")),
	// framework names the build framework the folder's handling was
	// guessed from, when it was guessed.
	spec(RuleQualityImageHeavy, CauseOverLowerLimit, notice, "bytes limitBytes", "framework",
		heavyMessage, heavyHintFor),
	spec(RuleQualityImageHeavy, CauseOverUpperLimit, warn, "bytes limitBytes", "framework",
		heavyMessage, heavyHintFor),
	spec(RuleQualityMediaUnused, "", notice, "", "",
		text("No content and no schema refers to this file."),
		text("If the site's code does not use it either, delete it to keep the media library tidy. If the code does use it, list it in the storage's `usedByCode`.")),
}, lengthSpecs(RuleQualityTitleLength, "title")...), lengthSpecs(RuleQualityDescriptionLength, "description")...)

func heavyMessage(x in) string {
	return "The image is " + HumanBytes(deref(x.Bytes)) + ", over " + HumanBytes(deref(x.LimitBytes)) + "."
}

// HumanBytes renders a size in English, the way the image weight
// findings print it: 1.8 MB, 300 KB, 900 bytes.
func HumanBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return strconv.FormatFloat(float64(n)/(1024*1024), 'f', 1, 64) + " MB"
	case n >= 1024:
		return integer(Int(n/1024)) + " KB"
	}
	return integer(Int(n)) + " bytes"
}

// quotedLabel is a field's label in typographic quotes, else def.
func quotedLabel(label, def string) string {
	if strings.TrimSpace(label) == "" {
		return def
	}
	return "“" + label + "”"
}
