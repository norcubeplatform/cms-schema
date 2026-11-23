package quality

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// languages runs the language rules over the pages that are versions of
// one another: the files of a collection with `translations` that share
// a translation key, and the languages of a file that holds them all in
// localized groups. A draft is not a version: a language whose only
// version is a draft is listed as not published yet.
func (r *run) languages(pages []Page) {
	r.localizedLanguages(pages)
	type group struct {
		coll     string
		key      string
		versions []Page
	}
	groups := map[string]*group{}
	var order []string
	for _, p := range pages {
		if p.Collection.Translations == nil || p.TranslationKey == "" {
			continue
		}
		id := p.Collection.Key + "\x00" + p.TranslationKey
		g, ok := groups[id]
		if !ok {
			g = &group{coll: p.Collection.Key, key: p.TranslationKey}
			groups[id] = g
			order = append(order, id)
		}
		g.versions = append(g.versions, p)
	}
	sort.Strings(order)

	outdatedOn := !r.off[report.RuleQualityTranslationOutdated]
	if outdatedOn && r.opts.LastChanged == nil {
		r.skip(report.RuleQualityTranslationOutdated, "when each file last changed was not given")
		outdatedOn = false
	}
	after := r.opts.Limits.OutdatedAfter
	if after <= 0 {
		after = DefaultOutdatedAfter
	}

	for _, id := range order {
		g := groups[id]
		sort.Slice(g.versions, func(i, j int) bool { return g.versions[i].Path < g.versions[j].Path })
		coll := g.versions[0].Collection
		present := map[string]Page{}
		drafts := map[string]Page{}
		var published []Page
		for _, v := range g.versions {
			if r.drafts[v.Path] {
				if _, dup := drafts[v.Locale]; !dup {
					drafts[v.Locale] = v
				}
				continue
			}
			published = append(published, v)
			if _, dup := present[v.Locale]; !dup {
				present[v.Locale] = v
			}
		}
		if r.on(report.RuleQualityTranslationMissing, coll) {
			for _, locale := range resolve.ExpectedLocales(r.cfg, coll) {
				if _, ok := present[locale]; ok {
					r.count(report.RuleQualityTranslationMissing)
					continue
				}
				if d, ok := drafts[locale]; ok {
					r.count(report.RuleQualityTranslationMissing)
					r.emit(report.Finding{File: d.Path, Rule: report.RuleQualityTranslationMissing,
						Message: fmt.Sprintf("The %q version is a draft, not published yet.", locale),
						Hint:    "Publish it when it is ready (turn its draft field off); until then readers of that language do not see the page.",
						Value:   locale}, report.ImpactInfo)
					continue
				}
				if len(published) == 0 {
					// Every version is a draft: the page is not out yet.
					continue
				}
				r.count(report.RuleQualityTranslationMissing)
				r.emit(report.Finding{File: r.sourceVersion(published).Path, Rule: report.RuleQualityTranslationMissing,
					Message: fmt.Sprintf("The page has no version in %q.", locale),
					Hint:    "Create the version in that language, or, if the collection is not meant to have one, list its languages in `translations.locales`.",
					Value:   locale}, report.ImpactMedium)
			}
		}
		if outdatedOn && r.on(report.RuleQualityTranslationOutdated, coll) && len(published) > 1 {
			r.outdated(published, after)
		}
	}
}

// sourceVersion is the version a missing language is reported on: the
// one in the site's default language, else the first by path.
func (r *run) sourceVersion(versions []Page) Page {
	for _, v := range versions {
		if v.Locale == r.cfg.Preview.DefaultLocale {
			return v
		}
	}
	return versions[0]
}

// outdated reports the dated versions that last changed more than after
// before the newest dated version of the same page.
func (r *run) outdated(versions []Page, after time.Duration) {
	var dated []Page
	var newest Page
	var newestAt time.Time
	for _, v := range versions {
		at, ok := r.opts.LastChanged[v.Path]
		if !ok || at.IsZero() {
			continue
		}
		dated = append(dated, v)
		if at.After(newestAt) {
			newest, newestAt = v, at
		}
	}
	if len(dated) < 2 {
		return
	}
	limit := int64(math.Round(after.Hours() / 24))
	for _, v := range dated {
		r.count(report.RuleQualityTranslationOutdated)
		at := r.opts.LastChanged[v.Path]
		behind := newestAt.Sub(at)
		if v.Path == newest.Path || behind <= after {
			continue
		}
		days := int64(behind.Hours() / 24)
		r.emit(report.Finding{File: v.Path, Rule: report.RuleQualityTranslationOutdated,
			Message: fmt.Sprintf("This %q version last changed %d days before the %q version (%s).", v.Locale, days, newest.Locale, newest.Path),
			Hint:    "Check whether the newer version says something this one does not, and bring it up to date.",
			Value:   newest.Path + "@" + newestAt.UTC().Format(time.RFC3339),
			Measure: &report.Measure{Value: days, Unit: report.UnitDays, Max: &limit}}, report.ImpactMedium)
	}
}

// localizedText is one localized group of text on a page, with its
// value per language.
type localizedText struct {
	path  string
	field schema.Field
	value map[string]any
}

// localizedLanguages runs quality.translation_incomplete over the pages
// whose file holds every language: per expected language, the localized
// texts left empty while another language has text, or, where the group
// falls back to another language, the same as that language.
func (r *run) localizedLanguages(pages []Page) {
	for _, p := range pages {
		coll := p.Collection
		if !coll.Localized() || r.drafts[p.Path] || coll.SiteArea() || !r.on(report.RuleQualityTranslationIncomplete, coll) {
			continue
		}
		texts := localizedTexts(r.cfg, coll, p.Frontmatter)
		if len(texts) == 0 {
			continue
		}
		for _, locale := range resolve.ExpectedLocales(r.cfg, coll) {
			r.count(report.RuleQualityTranslationIncomplete)
			var missing []string
			first := ""
			for _, t := range texts {
				why := untranslated(t, locale)
				if why == "" {
					continue
				}
				if first == "" {
					first = t.path + "." + locale
				}
				missing = append(missing, fmt.Sprintf("%s (%s)", labelOrKey(t.field), why))
			}
			if len(missing) == 0 {
				continue
			}
			named := missing
			more := ""
			if len(named) > MaxNamed {
				more = fmt.Sprintf(" and %d more", len(named)-MaxNamed)
				named = named[:MaxNamed]
			}
			r.emit(report.Finding{File: p.Path, Path: first, Rule: report.RuleQualityTranslationIncomplete,
				Message: fmt.Sprintf("The %q version leaves %d of %d texts untranslated: %s%s.", locale, len(missing), len(texts), strings.Join(named, ", "), more),
				Hint:    "Write these texts in that language. Until then its readers see them empty or in the fallback language.",
				Value:   locale, Locale: locale,
				Measure: &report.Measure{Value: int64(len(missing)), Unit: report.UnitFields}}, report.ImpactLow)
		}
	}
}

// Reasons a localized text is untranslated in a language.
const (
	untranslatedEmpty = "empty"
	untranslatedCopy  = "same as %s"
)

// untranslated says why a localized text is not translated into locale,
// or "" when it is: empty there, or the same as the language it falls
// back to.
func untranslated(t localizedText, locale string) string {
	own, _ := t.value[locale].(string)
	if strings.TrimSpace(own) == "" {
		return untranslatedEmpty
	}
	fb := t.field.Fallback
	if fb == "" || fb == locale {
		return ""
	}
	src, _ := t.value[fb].(string)
	if resolve.HasWords(src) && collapse(src) == collapse(own) {
		return fmt.Sprintf(untranslatedCopy, fb)
	}
	return ""
}

// localizedTexts lists the localized groups of a page that hold text in
// at least one language, in schema order and list items by index. Hidden
// fields, groups that say `translatable: false` and slugs are left out.
func localizedTexts(cfg *schema.Config, coll schema.Collection, fm map[string]any) []localizedText {
	var out []localizedText
	schema.Walk(cfg, coll.Fields, fm, func(v schema.Visit) bool {
		if !v.Item && (v.Field.Hidden || (v.Field.VisibleIf != nil && !schema.Eval(*v.Field.VisibleIf, v.Scope))) {
			return false
		}
		if !v.Field.IsLocalized() {
			return true
		}
		m, _ := v.Value.(map[string]any)
		if (v.Field.Translatable != nil && !*v.Field.Translatable) || v.Field.Key == "slug" || v.Field.Computed != nil {
			return false
		}
		for _, x := range m {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, localizedText{path: v.Path, field: v.Field, value: m})
				break
			}
		}
		return false
	})
	return out
}

func labelOrKey(f schema.Field) string {
	if strings.TrimSpace(f.Label) != "" {
		return f.Label
	}
	return f.Key
}
