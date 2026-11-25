package quality

import (
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
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
	if outdatedOn && r.opts.SourceBases == nil {
		r.skip(report.RuleQualityTranslationOutdated, SkipHistoryNotGiven, nil)
		outdatedOn = false
	}
	var pairs []versionPair

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
					r.emit(report.Finding{File: d.Path, Rule: report.RuleQualityTranslationMissing, Cause: report.CauseDraft,
						Params: &report.Params{Locale: locale},
						Value:  locale}, report.ImpactInfo)
					continue
				}
				if len(published) == 0 {
					// Every version is a draft: the page is not out yet.
					continue
				}
				r.count(report.RuleQualityTranslationMissing)
				r.emit(report.Finding{File: r.sourceVersion(published).Path, Rule: report.RuleQualityTranslationMissing, Cause: report.CauseMissing,
					Params: &report.Params{Locale: locale},
					Value:  locale}, report.ImpactMedium)
			}
		}
		if outdatedOn && r.on(report.RuleQualityTranslationOutdated, coll) && len(published) > 1 {
			pairs = append(pairs, r.versionPairs(published)...)
		}
	}
	if len(pairs) > 0 {
		r.outdated(pairs)
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

// versionPair is a language version and the source it is judged
// against.
type versionPair struct {
	translation, source Page
}

// versionPairs pairs each published version of a page with its source:
// the version in the site's default language. A page with no version in
// the default language has no source and nothing is judged.
func (r *run) versionPairs(published []Page) []versionPair {
	def := r.cfg.Preview.DefaultLocale
	if def == "" {
		return nil
	}
	var source *Page
	for i := range published {
		if published[i].Locale == def {
			source = &published[i]
			break
		}
	}
	if source == nil {
		return nil
	}
	var out []versionPair
	for _, v := range published {
		if v.Path == source.Path || v.Locale == def {
			continue
		}
		out = append(out, versionPair{translation: v, source: *source})
	}
	return out
}

// outdated reports the language versions whose source changed a
// translatable text since the version was last current with it. Its base
// is the source as a mark on the version records it (while the version's
// file is the one marked), else the source as it was when the version
// last changed; Options.SourceBases reads it. A version whose base can't
// be read is not judged, and neither are the versions past
// MaxBaseQueries.
func (r *run) outdated(pairs []versionPair) {
	if len(pairs) > MaxBaseQueries {
		r.skipped = append(r.skipped, newSkipped(report.RuleQualityTranslationOutdated, SkipLanguagesCapped, &report.Params{Max: report.Num(MaxBaseQueries)}))
		pairs = pairs[:MaxBaseQueries]
	}
	queries := make([]BaseQuery, len(pairs))
	for i, p := range pairs {
		queries[i] = QueryFor(p.translation, p.source.Path)
	}
	answers, err := r.opts.SourceBases(queries)
	if err != nil && len(answers) == 0 {
		r.skipped = append(r.skipped, newSkipped(report.RuleQualityTranslationOutdated, SkipHistoryUnreadable, nil))
		return
	}
	for i, p := range pairs {
		if i >= len(answers) || !answers[i].Known {
			continue
		}
		r.count(report.RuleQualityTranslationOutdated)
		changes, ok := SourceChanges(r.cfg, p.source, answers[i])
		if !ok || len(changes) == 0 {
			continue
		}
		r.emit(outdatedFinding(p, changes), report.ImpactMedium)
	}
}

// QueryFor is the BaseQuery of a language version against the source at
// sourcePath: with the blob its mark names while the mark applies (the
// version's file is still the one marked, content.TranslationMark), else
// asking for the source as it was when the version last changed. The
// version's Blob and Mark must be set.
func QueryFor(translation Page, sourcePath string) BaseQuery {
	q := BaseQuery{Translation: translation.Path, Source: sourcePath}
	if translation.Mark.Applies(sourcePath, translation.Blob) {
		q.SourceBlob = translation.Mark.SourceBlob
	}
	return q
}

// SourceChanges are the translatable texts of the source (its current
// Frontmatter and Body, its Collection) that differ from the source at
// its base, as quality.translation_outdated reads them; false when the
// base can't be read as a page.
func SourceChanges(cfg *schema.Config, source Page, base SourceBase) ([]resolve.TextChange, bool) {
	if !base.Known {
		return nil, false
	}
	if base.Same {
		return nil, true
	}
	doc, err := content.Parse(source.Path, base.Content)
	if err != nil {
		return nil, false
	}
	changes, err := resolve.ChangedTexts(cfg, source.Collection, doc.Frontmatter, doc.Body, source.Frontmatter, source.Body)
	if err != nil && len(changes) == 0 {
		return nil, false
	}
	return changes, true
}

// OutdatedValue is the Value of a quality.translation_outdated finding:
// the source's path and the blob id of the source as it was judged, the
// version a "mark up to date" names. ParseOutdatedValue reads it back.
func OutdatedValue(sourcePath, sourceBlob string) string {
	return sourcePath + "@" + sourceBlob
}

// ParseOutdatedValue splits an OutdatedValue; false when it is not one.
func ParseOutdatedValue(v string) (sourcePath, sourceBlob string, ok bool) {
	i := strings.LastIndexByte(v, '@')
	if i <= 0 || i == len(v)-1 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
}

func outdatedFinding(p versionPair, changes []resolve.TextChange) report.Finding {
	named := make([]string, 0, MaxNamed)
	for _, c := range changes {
		if len(named) == MaxNamed {
			break
		}
		named = append(named, c.Label)
	}
	params := &report.Params{Locale: p.source.Locale, Path: p.source.Path, Count: report.Int(int64(len(changes))), Names: named}
	if len(changes) > MaxNamed {
		params.More = report.Int(int64(len(changes) - MaxNamed))
	}
	return report.Finding{File: p.translation.Path, Path: changes[0].Path, Rule: report.RuleQualityTranslationOutdated,
		Params:  params,
		Value:   OutdatedValue(p.source.Path, p.source.Blob),
		Locale:  p.translation.Locale,
		Measure: &report.Measure{Value: int64(len(changes)), Unit: report.UnitChangedTexts}}
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
			var names, sameAs []string
			first := ""
			for _, t := range texts {
				why, ok := untranslated(t, locale)
				if !ok {
					continue
				}
				if first == "" {
					first = t.path + "." + locale
				}
				names = append(names, labelOrKey(t.field))
				sameAs = append(sameAs, why)
			}
			if len(names) == 0 {
				continue
			}
			params := &report.Params{Locale: locale, Count: report.Int(int64(len(names))), Total: report.Int(int64(len(texts))), Names: names, SameAs: sameAs}
			if len(names) > MaxNamed {
				params.More = report.Int(int64(len(names) - MaxNamed))
				params.Names, params.SameAs = names[:MaxNamed], sameAs[:MaxNamed]
			}
			r.emit(report.Finding{File: p.Path, Path: first, Rule: report.RuleQualityTranslationIncomplete, Params: params,
				Value: locale, Locale: locale,
				Measure: &report.Measure{Value: int64(len(names)), Unit: report.UnitFields}}, report.ImpactLow)
		}
	}
}

// untranslated says whether a localized text is not translated into
// locale, and why: "" when it is empty there, else the language it falls
// back to and is the same as. ok is false when it is translated.
func untranslated(t localizedText, locale string) (sameAs string, ok bool) {
	own, _ := t.value[locale].(string)
	if strings.TrimSpace(own) == "" {
		return "", true
	}
	fb := t.field.Fallback
	if fb == "" || fb == locale {
		return "", false
	}
	src, _ := t.value[fb].(string)
	if resolve.HasWords(src) && collapse(src) == collapse(own) {
		return fb, true
	}
	return "", false
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
