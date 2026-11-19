package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Schema checks a loaded configuration on its own: every field well
// formed, every reference resolvable, every knob in range, storages and
// the preview block consistent. exists answers whether a repository
// path is present, for the checks that look at files; nil skips them.
func Schema(doc *load.Document, exists func(path string) bool) []report.Finding {
	cfg := doc.Config
	var out []report.Finding
	f := func(file, path string, sev report.Severity, rule report.Rule, msg, hint string) {
		out = append(out, report.Finding{File: file, Path: path, Severity: sev, Rule: rule, Message: msg, Hint: hint})
	}
	configFile := load.ConfigFile
	if doc.Legacy {
		configFile = load.LegacyConfigFile
	}

	// Storages.
	for _, s := range cfg.Storages {
		for _, problem := range resolve.ValidStorage(s) {
			f(configFile, "storages."+s.Name, report.SeverityError, report.RuleSchemaInvalidStorage, "storage "+quote(s.Name)+": "+problem, "")
		}
	}
	if cfg.DefaultStorage != "" {
		if _, ok := cfg.Storage(cfg.DefaultStorage); !ok {
			f(configFile, "defaultStorage", report.SeverityError, report.RuleSchemaUnknownDefaultStorage, fmt.Sprintf("defaultStorage %q is not declared under storages", cfg.DefaultStorage), declaredStorages(cfg))
		}
	}

	// Preview block, redirects, variables.
	if cfg.Preview.DefaultLocale != "" && len(cfg.Preview.Locales) > 0 && !contains(cfg.Preview.Locales, cfg.Preview.DefaultLocale) {
		f(configFile, "preview.defaultLocale", report.SeverityError, report.RuleSchemaDefaultLocaleNotDeclared, fmt.Sprintf("defaultLocale %q is not in preview.locales %v", cfg.Preview.DefaultLocale, cfg.Preview.Locales), "")
	}
	if cfg.Redirects.File != "" && !cfg.Redirects.Enabled() {
		f(configFile, "redirects.format", report.SeverityError, report.RuleSchemaUnknownRedirectsFormat, fmt.Sprintf("redirects.format %q is not supported (netlify, vercel)", cfg.Redirects.Format), "")
	}
	if cfg.Variables.File != "" && exists != nil && !exists(cfg.Variables.File) {
		f(configFile, "variables.file", report.SeverityError, report.RuleSchemaMissingVariablesFile, fmt.Sprintf("variables.file %q does not exist", cfg.Variables.File), "Create the variables document, or point `variables.file` at the file that holds the {key, value} list.")
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
			f(file, "path", report.SeverityWarning, report.RuleSchemaMissingCollectionPath, "the collection declares no `path`, so no file resolves to it by directory", "Add `path: <directory>` so files under it use this schema.")
		}
		if unknown := resolve.PreviewTokens(coll.PreviewPath); len(unknown) > 0 {
			f(file, "previewPath", report.SeverityError, report.RuleSchemaUnknownPreviewToken, fmt.Sprintf("previewPath %q uses unknown token(s) %s", coll.PreviewPath, strings.Join(unknown, ", ")), "Tokens are {locale}, {slug} and {fields.<key>}.")
		}
		switch {
		case coll.LocaleFrom == "", coll.LocaleFrom == schema.LocaleFromPath, coll.LocaleFrom == schema.LocaleFromNone:
		case strings.HasPrefix(coll.LocaleFrom, schema.LocaleFromFieldPrefix) && len(coll.LocaleFrom) > len(schema.LocaleFromFieldPrefix):
		default:
			f(file, "localeFrom", report.SeverityError, report.RuleSchemaInvalidLocaleFrom, fmt.Sprintf("localeFrom %q is not path, none or field:<key>", coll.LocaleFrom), "")
		}
		if reason, hint := translationsProblem(coll); reason != "" {
			f(file, "translations.by", report.SeverityError, report.RuleSchemaInvalidTranslations, reason, hint)
		}
		switch coll.ExtraKeysPolicy {
		case schema.ExtraKeysShow, schema.ExtraKeysHide, schema.ExtraKeysStrip:
		default:
			f(file, "extraKeysPolicy", report.SeverityWarning, report.RuleSchemaUnknownKnob, fmt.Sprintf("extraKeysPolicy %q is not show, hide or strip; show is assumed", coll.ExtraKeysPolicy), "")
		}
		checkFields(cfg, file, "fields", coll.Fields, rootSchemaScope(coll.Fields), f)
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
			f(file, "path", report.SeverityError, report.RuleSchemaDuplicateCollectionPath, fmt.Sprintf("collections %s share path %q and their `match` patterns cannot tell them apart; files under it resolve to one of them arbitrarily", strings.Join(keys, ", "), p), "Give each collection its own directory, merge them, or let all but one name a single file with a literal `match`.")
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
			f(file, "label", report.SeverityError, report.RuleSchemaMissingLabel, "the block has no `label`", "")
		}
		checkFields(cfg, file, "fields", b.Fields, rootSchemaScope(b.Fields), f)
	}
	for _, key := range cfg.InputKeys {
		file := load.InputFile(key)
		if doc.Legacy {
			file = configFile
		}
		checkFields(cfg, file, "", []schema.Field{cfg.Inputs[key]}, rootSchemaScope([]schema.Field{cfg.Inputs[key]}), f)
	}
	return out
}

type emit func(file, path string, sev report.Severity, rule report.Rule, msg, hint string)

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
				f(file, p, report.SeverityError, report.RuleSchemaDuplicateKey, fmt.Sprintf("key %q is also used by %s in the same list", fd.Key, prev), "Keys must be unique inside one fields list; the editor and the content would not know which field a value belongs to.")
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
		f(file, p, report.SeverityError, report.RuleSchemaMissingKey, "the field has no `key`", "Every field needs the frontmatter key its value lands at.")
	}
	if fd.Type == "" {
		f(file, p, report.SeverityError, report.RuleSchemaMissingType, fmt.Sprintf("field %q has no `type`", label), "")
		return
	}
	if !schema.KnownTypes[fd.Type] {
		f(file, p, report.SeverityError, report.RuleSchemaUnknownType, fmt.Sprintf("field %q has type %q, which the editor does not render", label, fd.Type), "Types: text, paragraph, richtext, number, boolean, date, color, range, multichoice, link, image, file, reference, select, group, array.")
		return
	}
	if fd.Label == "" {
		f(file, p, report.SeverityError, report.RuleSchemaMissingLabel, fmt.Sprintf("field %q has no `label`", fd.Key), "Labels are required; the editor does not fall back to the key.")
	}
	if len(fd.Extra) > 0 {
		keys := make([]string, 0, len(fd.Extra))
		for k := range fd.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hint := ""
			if k == "toolbar" && schema.Canonical(fd.Type) == schema.TypeRichText {
				hint = "richtext takes `options:` (a map of flags such as bold: true, headings: [2, 3]), not a `toolbar:` list."
			}
			f(file, p, report.SeverityWarning, report.RuleSchemaUnknownKnob, fmt.Sprintf("field %q has key %q, which a %s field does not read", label, k, fd.Type), hint)
		}
	}
	t := schema.Canonical(fd.Type)

	// `translatable: true` on a value that is never translated. Saying
	// false there is harmless and is left alone.
	if fd.Translatable != nil && *fd.Translatable && !resolve.TranslatableType(t) && t != schema.TypeGroup && t != schema.TypeArray {
		f(file, p, report.SeverityWarning, report.RuleSchemaTranslatableIgnored, fmt.Sprintf("field %q has `translatable`, but a value of type %s is never translated", label, fd.Type), "Only text, paragraph and richtext are translated; on a group or an array the key covers the fields inside. Remove it here.")
	}

	// Options.
	if t == schema.TypeSelect || t == schema.TypeMultiChoice {
		if len(fd.Options) == 0 {
			f(file, p, report.SeverityError, report.RuleSchemaEmptyOptions, fmt.Sprintf("%s field %q has no options", fd.Type, label), "")
		} else if fd.HasDefault && fd.Default != nil {
			values := schema.OptionValues(fd.Options)
			switch d := fd.Default.(type) {
			case string:
				if t == schema.TypeSelect && !contains(values, d) {
					f(file, p, report.SeverityError, report.RuleSchemaDefaultNotOption, fmt.Sprintf("default %q of %q is not one of its options", d, label), "")
				}
			case []any:
				for _, item := range d {
					if s, ok := item.(string); ok && !contains(values, s) {
						f(file, p, report.SeverityError, report.RuleSchemaDefaultNotOption, fmt.Sprintf("default %q of %q is not one of its options", s, label), "")
					}
				}
			default:
				if t == schema.TypeSelect && !contains(values, fmt.Sprint(d)) {
					f(file, p, report.SeverityError, report.RuleSchemaDefaultNotOption, fmt.Sprintf("default %v of %q is not one of its options", d, label), "")
				}
			}
		}
	}

	// Bounds.
	if fd.Min != nil && fd.Max != nil && *fd.Min > *fd.Max {
		f(file, p, report.SeverityError, report.RuleSchemaMinAboveMax, fmt.Sprintf("%q has min %v above max %v", label, *fd.Min, *fd.Max), "")
	}
	if fd.MinLength != nil && fd.MaxLength != nil && *fd.MaxLength > 0 && *fd.MinLength > *fd.MaxLength {
		f(file, p, report.SeverityError, report.RuleSchemaMinAboveMax, fmt.Sprintf("%q has minLength %d above maxLength %d", label, *fd.MinLength, *fd.MaxLength), "")
	}
	if fd.MinItems != nil && fd.MaxItems != nil && *fd.MinItems > *fd.MaxItems {
		f(file, p, report.SeverityError, report.RuleSchemaMinAboveMax, fmt.Sprintf("%q has minItems %d above maxItems %d", label, *fd.MinItems, *fd.MaxItems), "")
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
				checkConditionPaths(file, p, label, parseCond(cond), scope, report.RuleSchemaUnresolvedConditionPath, "customValidators", f)
			}
		default:
			f(file, p, report.SeverityWarning, report.RuleSchemaUnknownCustomValidator, fmt.Sprintf("%q has a custom validator of type %q, which nothing implements; it is skipped", label, cv.Type), "Built-in types: regex, expression.")
		}
	}

	// Conditions and computed sources.
	if fd.VisibleIf != nil {
		checkConditionPaths(file, p, label, *fd.VisibleIf, scope, report.RuleSchemaUnresolvedConditionPath, "visibleIf", f)
	}
	if fd.Computed != nil && fd.Computed.From != "" {
		if ok, known := scope.pathResolves(fd.Computed.From); known && !ok {
			f(file, p, report.SeverityWarning, report.RuleSchemaUnresolvedComputedPath, fmt.Sprintf("%q is computed from %q, which names no field here", label, fd.Computed.From), "The value never fills in while the path does not resolve.")
		}
	}

	// Storages.
	if (t == schema.TypeImage || t == schema.TypeFile) && fd.Storage != "" {
		if _, ok := cfg.Storage(fd.Storage); !ok {
			f(file, p, report.SeverityWarning, report.RuleSchemaUnknownStorage, fmt.Sprintf("%q uses storage %q, which config.yml does not declare", label, fd.Storage), declaredStorages(cfg))
		}
	}

	// Nesting.
	if t == schema.TypeGroup {
		checkFields(cfg, file, p+".fields", fd.Fields, scope.child(fd.Fields), f)
	}
	if t == schema.TypeArray {
		entries := schema.ItemEntries(fd)
		if len(entries) == 0 && fd.Type != schema.TypeBlocks {
			f(file, p, report.SeverityError, report.RuleSchemaMissingType, fmt.Sprintf("array %q declares no `items`", label), "List one shape for a uniform array or several for a multi-shape array.")
		}
		multi := schema.IsMultiShape(fd)
		buckets := map[string][]string{}
		for _, e := range entries {
			itemPath := p + ".items[" + e.Name + "]"
			if _, unresolved := e.Field.Raw[schema.RefKey]; unresolved {
				continue
			}
			if multi && schema.IsGroup(e.Field.Type) && strings.HasPrefix(e.Name, "__entry_") {
				f(file, itemPath, report.SeverityError, report.RuleSchemaAnonymousItemShapes, fmt.Sprintf("a shape of %q has no key, so no item can name it in its %q", label, schema.ArrayIDKey(fd)), "Give each entry under `items:` a key (hero, cta, …); the editor writes that key as the item's discriminator.")
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
			f(file, p, report.SeverityError, report.RuleSchemaAmbiguousItemShapes, fmt.Sprintf("shapes %s of %q all hold %s values, so items of these shapes cannot be told apart", strings.Join(names, ", "), label, b), "Keep at most one primitive shape per value type, or wrap the others in groups so they carry a discriminator.")
		}
	}
}

func checkPattern(file, p, label, knob, pattern string, f emit) {
	if _, err := regexp.Compile(pattern); err != nil {
		msg := fmt.Sprintf("%q has a %s that does not compile: %s", label, knob, strings.TrimPrefix(err.Error(), "error parsing regexp: "))
		hint := ""
		if strings.Contains(err.Error(), "invalid or unsupported Perl syntax") {
			hint = "The editor runs patterns in JavaScript, which accepts this; this check uses Go's RE2 and cannot evaluate lookarounds or backreferences, so content checks skip this pattern."
		}
		f(file, p, report.SeverityWarning, report.RuleSchemaInvalidRegex, msg, hint)
	}
}

func checkConditionPaths(file, p, label string, c schema.Condition, scope schemaScope, rule report.Rule, knob string, f emit) {
	switch {
	case len(c.All) > 0:
		for _, sub := range c.All {
			checkConditionPaths(file, p, label, sub, scope, rule, knob, f)
		}
		return
	case len(c.Any) > 0:
		for _, sub := range c.Any {
			checkConditionPaths(file, p, label, sub, scope, rule, knob, f)
		}
		return
	case c.Not != nil:
		checkConditionPaths(file, p, label, *c.Not, scope, rule, knob, f)
		return
	}
	if c.Op != "" && !schema.KnownOps[c.Op] {
		f(file, p, report.SeverityError, report.RuleSchemaUnknownKnob, fmt.Sprintf("%q has a %s with operator %q, which does not exist", label, knob, c.Op), "Operators: eq, neq, in, notIn, truthy, falsy, empty, nonEmpty, gt, lt, matches.")
	}
	if c.Op == schema.OpMatches {
		if pat, ok := c.Value.(string); ok {
			checkPattern(file, p, label, knob+" pattern", pat, f)
		}
	}
	if c.Field == "" {
		f(file, p, report.SeverityError, report.RuleSchemaUnresolvedConditionPath, fmt.Sprintf("%q has a %s without a `field`", label, knob), "")
		return
	}
	if schema.PresenceOp(c.Op) {
		return
	}
	if ok, known := scope.pathResolves(c.Field); known && !ok {
		f(file, p, report.SeverityWarning, rule, fmt.Sprintf("%q has a %s on %q, which names no field here", label, knob, c.Field), "Common causes: a typo, a renamed sibling, or a $parent/$root path that walks too far. The field stays hidden while the path does not resolve.")
	}
}

func parseCond(m map[string]any) schema.Condition {
	fd := schema.ParseField(map[string]any{"visibleIf": m})
	if fd.VisibleIf == nil {
		return schema.Condition{}
	}
	return *fd.VisibleIf
}

func declaredStorages(cfg *schema.Config) string {
	if len(cfg.Storages) == 0 {
		return "This site declares no storages; add a `storages:` block to config.yml or remove the `storage:` line."
	}
	return "Declared storages: " + strings.Join(cfg.StorageNames(), ", ") + "."
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

func quote(s string) string { return fmt.Sprintf("%q", s) }

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
// `translations` block, with a hint, or "" when it is sound or absent.
// The block decides which pages are one page in several languages, so
// it has to be something every page can answer: a path that carries the
// locale, or a field every page must have.
func translationsProblem(coll schema.Collection) (reason, hint string) {
	t := coll.Translations
	if t == nil {
		return "", ""
	}
	const forms = "Use `by: path` (the same path under each locale folder) or `by: field:<key>` (pages with the same value of that field)."
	if t.By == schema.TranslationsByPath {
		if coll.LocaleFrom != schema.LocaleFromPath {
			return "translations.by is path, but the collection's locale does not come from the path", "Add `localeFrom: path`, or link the versions by a field: `by: field:<key>`."
		}
		return "", ""
	}
	key, ok := t.Field()
	if !ok {
		return fmt.Sprintf("translations.by %q is not path or field:<key>", t.By), forms
	}
	for _, f := range coll.Fields {
		if f.Key != key {
			continue
		}
		if schema.Canonical(f.Type) != schema.TypeText {
			return fmt.Sprintf("translations.by names %q, which is a %s field; the linking field must be text", key, f.Type), "Link the versions by a text field whose value they share."
		}
		if !f.Required {
			return fmt.Sprintf("translations.by names %q, which is not required, so a page could have no key", key), "Mark the field `required: true`."
		}
		return "", ""
	}
	return fmt.Sprintf("translations.by names %q, which is not a top-level field of the collection", key), "Add the field to the collection, or name one it has."
}
