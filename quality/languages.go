package quality

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
)

// languages runs the language rules over the pages that are versions of
// one another: the files of a collection with `translations` that share
// a translation key.
func (r *run) languages(pages []Page) {
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
		for _, v := range g.versions {
			if _, dup := present[v.Locale]; !dup {
				present[v.Locale] = v
			}
		}
		if r.on(report.RuleQualityTranslationMissing, coll) {
			source := r.sourceVersion(g.versions)
			for _, locale := range resolve.ExpectedLocales(r.cfg, coll) {
				r.count(report.RuleQualityTranslationMissing)
				if _, ok := present[locale]; ok {
					continue
				}
				r.emit(report.Finding{File: source.Path, Rule: report.RuleQualityTranslationMissing,
					Message: fmt.Sprintf("The page has no version in %q.", locale),
					Hint:    "Create the version in that language, or, if the collection is not meant to have one, list its languages in `translations.locales`.",
					Value:   locale}, report.ImpactMedium)
			}
		}
		if outdatedOn && r.on(report.RuleQualityTranslationOutdated, coll) {
			r.outdated(g.versions, after)
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
