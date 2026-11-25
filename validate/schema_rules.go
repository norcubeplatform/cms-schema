package validate

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Schema checks a loaded configuration on its own: every field well
// formed, every reference resolvable, every knob in range, storages and
// the preview block consistent. exists answers whether a repository
// path is present, for the checks that look at files; nil skips them.
// It never panics: a bug in a check comes back as one
// `schema.internal_error` finding.
func Schema(doc *load.Document, exists func(path string) bool) (out []report.Finding) {
	file := load.ConfigFile
	if doc != nil && doc.Legacy {
		file = load.LegacyConfigFile
	}
	defer recoverAs(&out, file, report.RuleSchemaInternalError)
	return checkSchema(doc, exists)
}

func checkSchema(doc *load.Document, exists func(path string) bool) []report.Finding {
	cfg := doc.Config
	var out []report.Finding
	f := func(file, path string, rule report.Rule, cause report.Cause, p *report.Params) {
		out = append(out, report.New(file, path, rule, cause, p))
	}
	configFile := load.ConfigFile
	if doc.Legacy {
		configFile = load.LegacyConfigFile
	}

	// Storages.
	for _, s := range cfg.Storages {
		for _, problem := range resolve.ValidStorage(s) {
			p := report.Params{}
			if problem.Params != nil {
				p = *problem.Params
			}
			p.Key = s.Name
			f(configFile, "storages."+s.Name, report.RuleSchemaInvalidStorage, problem.Cause, &p)
		}
	}
	if cfg.DefaultStorage != "" {
		if _, ok := cfg.Storage(cfg.DefaultStorage); !ok {
			f(configFile, "defaultStorage", report.RuleSchemaUnknownDefaultStorage, "", &report.Params{Key: cfg.DefaultStorage, Names: cfg.StorageNames()})
		}
	}

	// Preview block, redirects, variables.
	if cfg.Preview.DefaultLocale != "" && len(cfg.Preview.Locales) > 0 && !contains(cfg.Preview.Locales, cfg.Preview.DefaultLocale) {
		f(configFile, "preview.defaultLocale", report.RuleSchemaDefaultLocaleNotDeclared, "", &report.Params{Locale: cfg.Preview.DefaultLocale, Locales: cfg.Preview.Locales})
	}
	if cfg.Redirects.File != "" && !cfg.Redirects.Enabled() {
		f(configFile, "redirects.format", report.RuleSchemaUnknownRedirectsFormat, "", &report.Params{Value: cfg.Redirects.Format})
	}
	if cfg.Variables.File != "" && exists != nil && !exists(cfg.Variables.File) {
		f(configFile, "variables.file", report.RuleSchemaMissingVariablesFile, "", &report.Params{Path: cfg.Variables.File})
	}

	for _, problem := range qualityProblems(cfg.Raw, true) {
		f(configFile, "quality", report.RuleSchemaInvalidQuality, problem.Cause, problem.Params)
	}

	// Collections.
	byPath := map[string][]string{}
	for _, key := range cfg.CollectionKeys {
		coll := cfg.Collections[key]
		file := load.CollectionFile(key)
		if doc.Legacy {
			file = configFile
		}
		if coll.Path != "" {
			byPath[coll.Path] = append(byPath[coll.Path], key)
		} else if key != schema.DefaultCollection {
			f(file, "path", report.RuleSchemaMissingCollectionPath, "", nil)
		}
		if unknown := resolve.PreviewTokens(coll.PreviewPath); len(unknown) > 0 {
			f(file, "previewPath", report.RuleSchemaUnknownPreviewToken, "", &report.Params{Value: coll.PreviewPath, Names: unknown})
		}
		switch {
		case coll.LocaleFrom == "", coll.LocaleFrom == schema.LocaleFromPath, coll.LocaleFrom == schema.LocaleFromNone:
		case strings.HasPrefix(coll.LocaleFrom, schema.LocaleFromFieldPrefix) && len(coll.LocaleFrom) > len(schema.LocaleFromFieldPrefix):
		default:
			f(file, "localeFrom", report.RuleSchemaInvalidLocaleFrom, "", &report.Params{Value: coll.LocaleFrom})
		}
		if problem, ok := translationsProblem(coll); ok {
			f(file, "translations.by", report.RuleSchemaInvalidTranslations, problem.Cause, problem.Params)
		}
		for _, problem := range translationLocalesProblems(cfg, coll) {
			f(file, "translations.locales", report.RuleSchemaInvalidTranslationLocales, problem.Cause, problem.Params)
		}
		for _, problem := range qualityProblems(coll.Raw, false) {
			f(file, "quality", report.RuleSchemaInvalidQuality, problem.Cause, problem.Params)
		}
		switch coll.ExtraKeysPolicy {
		case schema.ExtraKeysShow, schema.ExtraKeysHide, schema.ExtraKeysStrip:
		default:
			f(file, "extraKeysPolicy", report.RuleSchemaUnknownKnob, report.CauseExtraKeysPolicy, &report.Params{Value: coll.ExtraKeysPolicy})
		}
		checkFields(cfg, file, "fields", coll.Fields, rootSchemaScope(coll.Fields), f)
		checkCollectionRoles(cfg, file, coll, f)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		keys := ambiguousKeys(cfg, byPath[p])
		if len(keys) == 0 {
			continue
		}
		for _, key := range keys {
			file := load.CollectionFile(key)
			if doc.Legacy {
				file = configFile
			}
			f(file, "path", report.RuleSchemaDuplicateCollectionPath, "", &report.Params{Names: keys, Path: p})
		}
	}

	// Blocks and inputs: their fields are checked where they stand, as
	// well as inlined (a block used by several collections reports once
	// per use site, which is where the fix lands when the use is wrong).
	for _, key := range cfg.BlockKeys {
		b := cfg.Blocks[key]
		file := load.BlockFile(key)
		if doc.Legacy {
			file = configFile
		}
		if b.Label == "" {
			f(file, "label", report.RuleSchemaMissingLabel, report.CauseBlockNoLabel, nil)
		}
		checkFields(cfg, file, "fields", b.Fields, rootSchemaScope(b.Fields), f)
		checkRoles(cfg, file, "fields", b.Fields, f)
	}
	for _, key := range cfg.InputKeys {
		file := load.InputFile(key)
		if doc.Legacy {
			file = configFile
		}
		checkFields(cfg, file, "", []schema.Field{cfg.Inputs[key]}, rootSchemaScope([]schema.Field{cfg.Inputs[key]}), f)
		checkRoles(cfg, file, "", []schema.Field{cfg.Inputs[key]}, f)
	}
	return out
}

// emit records a finding of a rule and cause; report.New words it.
type emit func(file, path string, rule report.Rule, cause report.Cause, p *report.Params)

// checkFields checks one field list and recurses into groups and array
// shapes. base is the schema path of the list (`fields`,
// `fields.hero.fields`, `fields.blocks.items[cta].fields`).
func checkFields(cfg *schema.Config, file, base string, fields []schema.Field, scope schemaScope, f emit) {
	seen := map[string]string{}
	for i, fd := range fields {
		p := base + "[" + strconvI(i) + "]"
		if fd.Key != "" {
			p = joinPath(base, fd.Key)
		}
		checkField(cfg, file, p, fd, scope, f)
		if fd.Key != "" {
			if prev, dup := seen[fd.Key]; dup {
				f(file, p, report.RuleSchemaDuplicateKey, "", &report.Params{Key: fd.Key, Other: prev})
			}
			seen[fd.Key] = p
		}
	}
}

func checkField(cfg *schema.Config, file, p string, fd schema.Field, scope schemaScope, f emit) {
	// A `$ref` the loader could not resolve is still a `$ref` mapping
	// here; the loader reported it, so its missing key, type and label
	// are not news.
	if _, unresolved := fd.Raw[schema.RefKey]; unresolved {
		return
	}
	label := fd.Label
	if label == "" {
		label = fd.Key
	}
	if fd.Key == "" {
		f(file, p, report.RuleSchemaMissingKey, "", nil)
	}
	if fd.Type == "" {
		f(file, p, report.RuleSchemaMissingType, report.CauseNoType, &report.Params{Field: label})
		return
	}
	if !schema.KnownTypes[fd.Type] {
		f(file, p, report.RuleSchemaUnknownType, "", &report.Params{Field: label, Type: fd.Type})
		return
	}
	if fd.Label == "" {
		f(file, p, report.RuleSchemaMissingLabel, report.CauseFieldNoLabel, &report.Params{Field: fd.Key})
	}
	if len(fd.Extra) > 0 {
		keys := make([]string, 0, len(fd.Extra))
		for k := range fd.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			cause := report.CauseUnknownKey
			if k == "toolbar" && schema.Canonical(fd.Type) == schema.TypeRichText {
				cause = report.CauseToolbarNotOptions
			}
			if k == "alt" || k == "decorative" {
				cause = report.CauseAltKnobNotOnImage
			}
			f(file, p, report.RuleSchemaUnknownKnob, cause, &report.Params{Field: label, Key: k, Type: fd.Type})
		}
	}
	t := schema.Canonical(fd.Type)

	// `translatable: true` on a value that is never translated. Saying
	// false there is harmless and is left alone.
	if fd.Translatable != nil && *fd.Translatable && !resolve.TranslatableType(t) && t != schema.TypeGroup && t != schema.TypeArray {
		f(file, p, report.RuleSchemaTranslatableIgnored, "", &report.Params{Field: label, Type: fd.Type})
	}

	// Options.
	if t == schema.TypeSelect || t == schema.TypeMultiChoice {
		if len(fd.Options) == 0 {
			f(file, p, report.RuleSchemaEmptyOptions, "", &report.Params{Type: fd.Type, Field: label})
		} else if fd.HasDefault && fd.Default != nil {
			values := schema.OptionValues(fd.Options)
			switch d := fd.Default.(type) {
			case string:
				if t == schema.TypeSelect && !contains(values, d) {
					f(file, p, report.RuleSchemaDefaultNotOption, "", &report.Params{Value: d, Field: label})
				}
			case []any:
				for _, item := range d {
					if s, ok := item.(string); ok && !contains(values, s) {
						f(file, p, report.RuleSchemaDefaultNotOption, "", &report.Params{Value: s, Field: label})
					}
				}
			default:
				if t == schema.TypeSelect && !contains(values, fmt.Sprint(d)) {
					f(file, p, report.RuleSchemaDefaultNotOption, "", &report.Params{Value: report.FlowValue(d), Field: label, Actual: valueKind(d)})
				}
			}
		}
	}

	// Bounds.
	if fd.Min != nil && fd.Max != nil && *fd.Min > *fd.Max {
		f(file, p, report.RuleSchemaMinAboveMax, report.CauseMinMax, &report.Params{Field: label, Min: fd.Min, Max: fd.Max})
	}
	if fd.MinLength != nil && fd.MaxLength != nil && *fd.MaxLength > 0 && *fd.MinLength > *fd.MaxLength {
		f(file, p, report.RuleSchemaMinAboveMax, report.CauseLength, &report.Params{Field: label, Min: report.Num(float64(*fd.MinLength)), Max: report.Num(float64(*fd.MaxLength))})
	}
	if fd.MinItems != nil && fd.MaxItems != nil && *fd.MinItems > *fd.MaxItems {
		f(file, p, report.RuleSchemaMinAboveMax, report.CauseItems, &report.Params{Field: label, Min: report.Num(float64(*fd.MinItems)), Max: report.Num(float64(*fd.MaxItems))})
	}

	// Patterns.
	if fd.Regex != "" {
		checkPattern(file, p, label, "regex", fd.Regex, f)
	}
	for i, cv := range fd.Custom {
		switch cv.Type {
		case schema.CustomValidatorRegex:
			if pat, _ := cv.Params["pattern"].(string); pat != "" {
				checkPattern(file, p, label, fmt.Sprintf("customValidators[%d].pattern", i), pat, f)
			}
		case schema.CustomValidatorExpression:
			if cond, ok := cv.Params["condition"].(map[string]any); ok {
				checkConditionPaths(file, p, label, parseCond(cond), scope, "customValidators", f)
			}
		default:
			f(file, p, report.RuleSchemaUnknownCustomValidator, "", &report.Params{Field: label, Value: cv.Type})
		}
	}

	// Conditions and computed sources.
	if fd.VisibleIf != nil {
		checkConditionPaths(file, p, label, *fd.VisibleIf, scope, "visibleIf", f)
	}
	if fd.Computed != nil && fd.Computed.From != "" {
		if ok, known := scope.pathResolves(fd.Computed.From); known && !ok {
			f(file, p, report.RuleSchemaUnresolvedComputedPath, "", &report.Params{Field: label, Path: fd.Computed.From})
		}
	}

	// Storages.
	if (t == schema.TypeImage || t == schema.TypeFile) && fd.Storage != "" {
		if _, ok := cfg.Storage(fd.Storage); !ok {
			f(file, p, report.RuleSchemaUnknownStorage, "", &report.Params{Field: label, Key: fd.Storage, Names: cfg.StorageNames()})
		}
	}

	// Nesting.
	if t == schema.TypeGroup {
		checkFields(cfg, file, p+".fields", fd.Fields, scope.child(fd.Fields), f)
	}
	if t == schema.TypeArray {
		entries := schema.ItemEntries(fd)
		if len(entries) == 0 && fd.Type != schema.TypeBlocks {
			f(file, p, report.RuleSchemaMissingType, report.CauseNoItems, &report.Params{Field: label})
		}
		multi := schema.IsMultiShape(fd)
		buckets := map[string][]string{}
		for _, e := range entries {
			itemPath := p + ".items[" + e.Name + "]"
			if _, unresolved := e.Field.Raw[schema.RefKey]; unresolved {
				continue
			}
			if multi && schema.IsGroup(e.Field.Type) && strings.HasPrefix(e.Name, "__entry_") {
				f(file, itemPath, report.RuleSchemaAnonymousItemShapes, "", &report.Params{Field: label, Key: schema.ArrayIDKey(fd)})
			}
			if b := schema.PrimitiveBucket(e.Field.Type); b != "" && multi {
				buckets[b] = append(buckets[b], e.Name)
			}
			if schema.IsGroup(e.Field.Type) {
				checkFields(cfg, file, itemPath+".fields", e.Field.Fields, scope.itemScope(e.Field.Fields), f)
			} else {
				checkField(cfg, file, itemPath, e.Field, scope, f)
			}
		}
		bs := make([]string, 0, len(buckets))
		for b := range buckets {
			bs = append(bs, b)
		}
		sort.Strings(bs)
		for _, b := range bs {
			names := buckets[b]
			if len(names) < 2 {
				continue
			}
			f(file, p, report.RuleSchemaAmbiguousItemShapes, "", &report.Params{Names: names, Field: label, Type: b})
		}
	}
}

func checkPattern(file, p, label, knob, pattern string, f emit) {
	if _, err := regexp.Compile(pattern); err != nil {
		cause := report.CauseDoesNotCompile
		if strings.Contains(err.Error(), "invalid or unsupported Perl syntax") {
			cause = report.CauseUnsupportedSyntax
		}
		f(file, p, report.RuleSchemaInvalidRegex, cause, &report.Params{Field: label, Knob: knob, Detail: strings.TrimPrefix(err.Error(), "error parsing regexp: ")})
	}
}

func checkConditionPaths(file, p, label string, c schema.Condition, scope schemaScope, knob string, f emit) {
	switch {
	case len(c.All) > 0:
		for _, sub := range c.All {
			checkConditionPaths(file, p, label, sub, scope, knob, f)
		}
		return
	case len(c.Any) > 0:
		for _, sub := range c.Any {
			checkConditionPaths(file, p, label, sub, scope, knob, f)
		}
		return
	case c.Not != nil:
		checkConditionPaths(file, p, label, *c.Not, scope, knob, f)
		return
	}
	if c.Op != "" && !schema.KnownOps[c.Op] {
		f(file, p, report.RuleSchemaUnknownKnob, report.CauseConditionOperator, &report.Params{Field: label, Knob: knob, Value: c.Op})
	}
	if c.Op == schema.OpMatches {
		if pat, ok := c.Value.(string); ok {
			checkPattern(file, p, label, knob+" pattern", pat, f)
		}
	}
	if c.Field == "" {
		f(file, p, report.RuleSchemaUnresolvedConditionPath, report.CauseNoField, &report.Params{Field: label, Knob: knob})
		return
	}
	if schema.PresenceOp(c.Op) {
		return
	}
	if ok, known := scope.pathResolves(c.Field); known && !ok {
		f(file, p, report.RuleSchemaUnresolvedConditionPath, report.CausePathUnknown, &report.Params{Field: label, Knob: knob, Path: c.Field})
	}
}

func parseCond(m map[string]any) schema.Condition {
	fd := schema.ParseField(map[string]any{"visibleIf": m})
	if fd.VisibleIf == nil {
		return schema.Condition{}
	}
	return *fd.VisibleIf
}

// valueKind names the kind of a YAML value that is not text.
func valueKind(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case int, int64, uint64, float64:
		return "number"
	case []any:
		return "list"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	}
	return "value"
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func joinPath(base, key string) string { return schema.JoinPath(base, key) }

func strconvI(i int) string { return fmt.Sprintf("%d", i) }

// ambiguousKeys returns the collections among keys (which share one
// path) that `match` cannot tell apart, sorted: every pattern collection
// when there are two or more of them (the resolver would pick one by key
// order), and the literal single-document collections that claim the
// same file name. One pattern collection plus distinct single-document
// collections in the same directory is fine: the literal names win.
func ambiguousKeys(cfg *schema.Config, keys []string) []string {
	var patterns []string
	byLiteral := map[string][]string{}
	for _, key := range keys {
		m := strings.TrimSpace(cfg.Collections[key].Match)
		if m == "" {
			m = "**/*.md"
		}
		if resolve.IsLiteralMatch(m) {
			byLiteral[m] = append(byLiteral[m], key)
			continue
		}
		patterns = append(patterns, key)
	}
	var out []string
	if len(patterns) >= 2 {
		out = append(out, patterns...)
	}
	for _, dup := range byLiteral {
		if len(dup) >= 2 {
			out = append(out, dup...)
		}
	}
	sort.Strings(out)
	return out
}

// translationsProblem says what is wrong with a collection's
// `translations` block; false when it is sound or absent. The block
// decides which pages are one page in several languages, so it has to
// be something every page can answer: a path that carries the locale,
// or a field every page must have.
func translationsProblem(coll schema.Collection) (report.Problem, bool) {
	t := coll.Translations
	if t == nil {
		return report.Problem{}, false
	}
	bad := func(c report.Cause, p *report.Params) (report.Problem, bool) {
		return report.Problem{Cause: c, Params: p}, true
	}
	if t.By == "" && !coll.OneLanguagePerFile() && schema.HasLocalized(coll.Fields) {
		// Each file holds every language in localized groups: there are
		// no versions to link, and `locales` alone says which languages a
		// page is expected in.
		return report.Problem{}, false
	}
	if t.By == schema.TranslationsByPath {
		if coll.LocaleFrom != schema.LocaleFromPath {
			return bad(report.CauseByPathWithoutLocale, nil)
		}
		return report.Problem{}, false
	}
	key, ok := t.Field()
	if !ok {
		return bad(report.CauseByUnknown, &report.Params{Value: t.By})
	}
	for _, f := range coll.Fields {
		if f.Key != key {
			continue
		}
		if schema.Canonical(f.Type) != schema.TypeText {
			return bad(report.CauseByFieldNotText, &report.Params{Key: key, Type: f.Type})
		}
		if !f.Required {
			return bad(report.CauseByFieldNotRequired, &report.Params{Key: key})
		}
		return report.Problem{}, false
	}
	return bad(report.CauseByFieldNotTopLevel, &report.Params{Key: key})
}

// qualityProblems says what is wrong with the `quality` block of a
// config (site) or collection mapping. Limits are read in config.yml
// only: an image is checked once per file, not per page.
func qualityProblems(raw map[string]any, site bool) []report.Problem {
	v, has := raw["quality"]
	if !has || v == nil {
		return nil
	}
	problem := func(c report.Cause, p *report.Params) report.Problem { return report.Problem{Cause: c, Params: p} }
	m, ok := v.(map[string]any)
	if !ok {
		return []report.Problem{problem(report.CauseNotMapping, &report.Params{Value: report.FlowValue(v)})}
	}
	var out []report.Problem
	for _, k := range sortedMapKeys(m) {
		switch {
		case k == schema.QualityKeyIgnore:
		case k == schema.QualityKeyLimits && site:
		case k == schema.QualityKeyLimits:
			out = append(out, problem(report.CauseLimitsNotInSite, nil))
		case site:
			out = append(out, problem(report.CauseSiteKeyUnknown, &report.Params{Key: k}))
		default:
			out = append(out, problem(report.CauseCollectionKeyUnknown, &report.Params{Key: k}))
		}
	}
	if site {
		out = append(out, qualityLimitsProblems(m[schema.QualityKeyLimits])...)
	}
	ignore, has := m[schema.QualityKeyIgnore]
	if !has || ignore == nil {
		return out
	}
	list, ok := ignore.([]any)
	if !ok {
		return append(out, problem(report.CauseIgnoreNotList, &report.Params{Value: report.FlowValue(ignore)}))
	}
	for _, item := range list {
		id, ok := item.(string)
		if !ok {
			out = append(out, problem(report.CauseIgnoreNotRuleID, &report.Params{Value: report.FlowValue(item)}))
			continue
		}
		if _, known := quality.Spec(report.Rule(strings.TrimSpace(id))); !known {
			out = append(out, problem(report.CauseIgnoreUnknownRule, &report.Params{Value: id}))
		}
	}
	return out
}

// qualityLimitsProblems says what is wrong with a site's
// `quality.limits`. A limit it reports is not applied.
func qualityLimitsProblems(v any) []report.Problem {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return []report.Problem{{Cause: report.CauseLimitsNotMapping, Params: &report.Params{Value: report.FlowValue(v)}}}
	}
	var out []report.Problem
	for _, k := range sortedMapKeys(m) {
		if k != schema.QualityLimitImageKB {
			out = append(out, report.Problem{Cause: report.CauseLimitsKeyUnknown, Params: &report.Params{Key: k, Knob: schema.QualityLimitImageKB}})
		}
	}
	kb, has := m[schema.QualityLimitImageKB]
	if !has || kb == nil {
		return out
	}
	n, ok := wholeNumber(kb)
	switch {
	case !ok:
		out = append(out, report.Problem{Cause: report.CauseImageKBNotNumber, Params: &report.Params{Value: report.FlowValue(kb)}})
	case n < schema.QualityImageKBMin || n > schema.QualityImageKBMax:
		out = append(out, report.Problem{Cause: report.CauseImageKBOutOfRange, Params: &report.Params{Current: report.Num(float64(n)), Min: report.Num(schema.QualityImageKBMin), Max: report.Num(schema.QualityImageKBMax)}})
	}
	return out
}

// wholeNumber reads a YAML number that has no fraction.
func wholeNumber(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == math.Trunc(n) {
			return int(math.Max(-1e9, math.Min(n, 1e9))), true
		}
	}
	return 0, false
}

func sortedMapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
