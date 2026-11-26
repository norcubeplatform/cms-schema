package report

import "fmt"

const (
	err  = SeverityError
	warn = SeverityWarning
)

// Hints several schema causes share.
const (
	missingKeyHint     = "Every field needs the frontmatter key its value lands at."
	typesHint          = "Types: text, paragraph, richtext, number, boolean, date, color, range, multichoice, link, image, file, reference, select, group, array."
	operatorsHint      = "Operators: eq, neq, in, notIn, truthy, falsy, empty, nonEmpty, gt, lt, matches."
	conditionPathHint  = "Common causes: a typo, a renamed sibling, or a $parent/$root path that walks too far. The field stays hidden while the path does not resolve."
	re2Hint            = "The editor runs patterns in JavaScript, which accepts this; this check uses Go's RE2 and cannot evaluate lookarounds or backreferences, so content checks skip this pattern."
	translationsForms  = "Use `by: path` (the same path under each locale folder) or `by: field:<key>` (pages with the same value of that field)."
	translationLocales = "List the languages this collection's pages are expected in, each one of preview.locales, or leave `locales` out to expect all of them."
	rolesHint          = "Roles: seoTitle, seoDescription, shareImage, noindex, canonical, draft, alt, or none (no role, never guessed)."
	whenEmptyHint      = "whenEmpty is generated (the site makes the value per page in code, such as a share card rendered at build time) or siteDefault (the site falls back to one site-wide value)."
	altHint            = "`alt:` names the field next to the image, in the same fields list, that holds its alt text (a text or paragraph field, or a localized group of them), or composes it: `alt: { from: [title, technique], separator: \" – \" }`."
	localizedHint      = "A localized group has one field per language of preview.locales, keyed by the language (cs, en, …), all text, paragraph or rich text of one type; `fallback: <language>` names the language an empty one shows."
	templateHint       = "titleTemplate is the page title as the site composes it: `{value}` (this field) with literal text, `{field:<key>}` (another field of the page), `{field:<key>|label}` (a select's option label) and `{{variable}}` (a site variable), such as `{value} – {{site_name}}`."
	excerptHint        = "A rich text field with role seoDescription says which part is the meta description: `excerpt: firstParagraph` (the plain text of its first paragraph)."
	qualityHint        = "`quality: { ignore: [quality.share_image_missing] }` turns content quality checks off and, in config.yml, `quality: { limits: { imageKB: 300 } }` sets the image weight limit; the rule ids are listed in the module's README."
	internalHint       = "This is a bug in the validator, not necessarily in the site. Its other findings for this check are missing; report the message to Norcube."
)

// declaredStorages is the hint of a storage name the config does not
// declare: the names it does, or, with none, to declare one or drop the
// line that names it (line: `storage:` on a field, `defaultStorage:` in
// config.yml).
func declaredStorages(line string) func(x in) string {
	return func(x in) string {
		if len(x.Names) == 0 {
			return "This site declares no storages; add a `storages:` block to config.yml or remove the `" + line + ":` line."
		}
		return "Declared storages: " + comma(x.Names) + more(x.Params) + "."
	}
}

// whenVerb says what the values of noindexWhen or draftWhen do.
func whenVerb(role string) string {
	if role == "draft" {
		return "make the page a draft"
	}
	return "hide the page from search engines"
}

func internalError(x in) string { return "the check stopped on an internal error: " + x.Detail }

// storageProblem is the sentence of a storage cause, after the storage's
// name (StorageProblem).
func storageProblem(c Cause, x in) string {
	switch c {
	case CauseTypeMissing:
		return "missing `type:` (the only shipped driver is repo)"
	case CauseTypeUnimplemented:
		return "type \"" + x.Value + "\" is not implemented (only repo is)"
	case CausePathMissing:
		return "missing `path:`"
	case CauseModeInvalid:
		return "invalid mode \"" + x.Value + "\" (allowed: " + comma(x.Allowed) + ")"
	case CauseNamingInvalid:
		return "invalid naming \"" + x.Value + "\" (allowed: " + comma(x.Allowed) + ", or empty for original)"
	case CauseInlineInvalid:
		return "invalid inline \"" + x.Value + "\" (allowed: " + comma(x.Allowed) + ")"
	case CauseDedupInvalid:
		return "invalid dedup \"" + x.Value + "\" (allowed: " + comma(x.Allowed) + ")"
	case CauseDedupNeedsSlugHash:
		return "`dedup: hash` requires `naming: slug+hash`"
	case CauseOptimizedAtBuildNotBoolean:
		return "`optimizedAtBuild` is " + x.Value + ", not true or false"
	case CauseUsedByCodeNotList:
		return "`usedByCode` is not a list of paths"
	case CauseUsedByCodeTooMany:
		return fmt.Sprintf("`usedByCode` lists %s paths, at most %s", integer(x.Count), num(x.Max))
	case CauseUsedByCodeNotPath:
		return "`usedByCode` holds " + x.Value + ", not a path"
	case CauseUsedByCodeTwice:
		return "`usedByCode` lists " + q(x.Path) + " twice"
	}
	return "`usedByCode`: " + patternProblem(c, x)
}

// patternProblem is the sentence of a usedByCode path pattern cause.
func patternProblem(c Cause, x in) string {
	p := q(x.Path)
	switch c {
	case CausePatternEmpty:
		return p + " is empty"
	case CausePatternTooLong:
		return p + " is longer than " + num(x.Max) + " bytes"
	case CausePatternAbsolute:
		return p + " starts with /: write it relative to the storage's path"
	case CausePatternBackslash:
		return p + " holds a backslash"
	case CausePatternControl:
		return p + " holds a control character"
	case CausePatternEmptyName:
		return p + " has an empty name between slashes"
	case CausePatternDotName:
		return p + " names . or .."
	case CausePatternDoubleStarName:
		return p + " uses ** inside a name: ** stands alone between slashes"
	}
	return p + " is not a valid pattern"
}

// templateProblem is the sentence of a titleTemplate that cannot be
// read, after "invalid titleTemplate: ".
func templateProblem(c Cause, x in) string {
	switch c {
	case CauseTemplateEmpty:
		return "it is empty"
	case CauseTemplateTooLong:
		return "it is longer than " + num(x.Max) + " bytes"
	case CauseTemplateLoneClose:
		return "a lone } at byte " + integer(x.Offset)
	case CauseTemplateUnclosed:
		return "a { at byte " + integer(x.Offset) + " is never closed"
	case CauseTemplateFieldPath:
		return x.Value + " does not name a field (keys joined by dots)"
	case CauseTemplateUnknownPlaceholder:
		return x.Value + " is not a placeholder; use {value}, {field:<key>}, {field:<key>|label} or {{variable}}"
	case CauseTemplateTooManyParts:
		return "more than " + num(x.Max) + " parts"
	}
	return "it does not use {value}, the field's own text"
}

// Problem is a cause with its params where the cause names one problem
// of a value rather than a whole finding: a storage entry, a usedByCode
// path, a titleTemplate. The schema and fact packages return it in
// their errors, so the same problem reads the same in a finding, an
// error and a refused fact.
type Problem struct {
	Cause  Cause
	Params *Params
}

// String is the problem in English, as it follows a prefix
// ("invalid titleTemplate: ", "storage \"images\": ").
func (p Problem) String() string {
	x := in{Params: p.Params, f: &Finding{}}
	if x.Params == nil {
		x.Params = &Params{}
	}
	switch {
	case isTemplateCause(p.Cause):
		return templateProblem(p.Cause, x)
	case isPatternCause(p.Cause):
		return patternProblem(p.Cause, x)
	}
	return storageProblem(p.Cause, x)
}

func isTemplateCause(c Cause) bool {
	switch c {
	case CauseTemplateEmpty, CauseTemplateTooLong, CauseTemplateLoneClose, CauseTemplateUnclosed,
		CauseTemplateFieldPath, CauseTemplateUnknownPlaceholder, CauseTemplateTooManyParts, CauseTemplateNoValue:
		return true
	}
	return false
}

func isPatternCause(c Cause) bool {
	switch c {
	case CausePatternEmpty, CausePatternTooLong, CausePatternAbsolute, CausePatternBackslash, CausePatternControl,
		CausePatternEmptyName, CausePatternDotName, CausePatternDoubleStarName, CausePatternInvalid:
		return true
	}
	return false
}

// TemplateCauses and PatternCauses list the causes of a titleTemplate
// that cannot be read and of a usedByCode path that is not one.
func TemplateCauses() []Cause {
	return []Cause{CauseTemplateEmpty, CauseTemplateTooLong, CauseTemplateLoneClose, CauseTemplateUnclosed,
		CauseTemplateFieldPath, CauseTemplateUnknownPlaceholder, CauseTemplateTooManyParts, CauseTemplateNoValue}
}

func PatternCauses() []Cause {
	return []Cause{CausePatternEmpty, CausePatternTooLong, CausePatternAbsolute, CausePatternBackslash, CausePatternControl,
		CausePatternEmptyName, CausePatternDotName, CausePatternDoubleStarName, CausePatternInvalid}
}

// StorageCauses lists the causes of schema.invalid_storage that are not
// a usedByCode path pattern.
func StorageCauses() []Cause {
	return []Cause{CauseTypeMissing, CauseTypeUnimplemented, CausePathMissing, CauseModeInvalid, CauseNamingInvalid,
		CauseInlineInvalid, CauseDedupInvalid, CauseDedupNeedsSlugHash, CauseOptimizedAtBuildNotBoolean,
		CauseUsedByCodeNotList, CauseUsedByCodeTooMany, CauseUsedByCodeNotPath, CauseUsedByCodeTwice}
}

// storageParams are the params of each storage cause besides the
// storage's name (key).
var storageParams = map[Cause]string{
	CauseTypeUnimplemented: "value", CauseModeInvalid: "value allowed", CauseNamingInvalid: "value allowed",
	CauseInlineInvalid: "value allowed", CauseDedupInvalid: "value allowed", CauseOptimizedAtBuildNotBoolean: "value",
	CauseUsedByCodeTooMany: "count max", CauseUsedByCodeNotPath: "value", CauseUsedByCodeTwice: "path",
	CausePatternTooLong: "path max",
}

func storageSpecs() []CauseSpec {
	var out []CauseSpec
	for _, c := range append(StorageCauses(), PatternCauses()...) {
		c := c
		params := "key " + storageParams[c]
		if isPatternCause(c) && c != CausePatternTooLong && c != CausePatternEmpty {
			// The entry as written; pattern_empty's is empty.
			params = "key path"
		}
		out = append(out, spec(RuleSchemaInvalidStorage, c, err, params, "", func(x in) string {
			return "storage " + q(x.Key) + ": " + storageProblem(c, x)
		}, nil))
	}
	return out
}

func templateSpecs() []CauseSpec {
	params := map[Cause]string{
		CauseTemplateTooLong: "max", CauseTemplateLoneClose: "offset", CauseTemplateUnclosed: "offset",
		CauseTemplateFieldPath: "value", CauseTemplateUnknownPlaceholder: "value", CauseTemplateTooManyParts: "max",
	}
	var out []CauseSpec
	for _, c := range TemplateCauses() {
		c := c
		out = append(out, spec(RuleSchemaInvalidTitleTemplate, c, err, "field "+params[c], "", func(x in) string {
			return "field " + q(x.Field) + ": invalid titleTemplate: " + templateProblem(c, x)
		}, text(templateHint)))
	}
	return out
}

var schemaCauses = append(append([]CauseSpec{
	// Loading.
	spec(RuleSchemaParseError, CauseNotYAML, err, "detail", "",
		func(x in) string { return "not valid YAML: " + x.Detail }, nil),
	spec(RuleSchemaParseError, CauseUnreadable, err, "detail", "",
		func(x in) string { return "cannot read: " + x.Detail }, nil),
	spec(RuleSchemaNotMapping, "", err, "", "",
		text("the document must be a mapping (key: value pairs) at the top level"),
		text("A schema file is a mapping of label, fields and preview; a list or a bare value cannot be loaded.")),
	spec(RuleSchemaUnsupportedVersion, "", err, "value", "",
		func(x in) string { return "schemaVersion " + q(x.Value) + " is not supported (supported: 1)" }, nil),
	spec(RuleSchemaUnresolvedRef, "", err, "key path other", "",
		func(x in) string { return "$ref " + q(x.Key) + " names no block and no input" },
		func(x in) string { return "Add " + x.Path + " or " + x.Other + ", or fix the name." }),
	spec(RuleSchemaRefCycle, "", err, "key max", "",
		func(x in) string {
			return "$ref " + q(x.Key) + " nests deeper than " + num(x.Max) + " levels; the blocks reference each other in a cycle"
		}, nil),
	spec(RuleSchemaInputsGlobUnsupported, "", warn, "path", "",
		func(x in) string { return "pattern " + q(x.Path) + " is not supported and is ignored" },
		text("Only `<dir>/*.yml` and `<dir>/*.yaml` patterns are read.")),

	// Fields.
	spec(RuleSchemaMissingKey, "", err, "", "", text("the field has no `key`"), text(missingKeyHint)),
	spec(RuleSchemaMissingType, CauseNoType, err, "field", "",
		func(x in) string { return "field " + q(x.Field) + " has no `type`" }, nil),
	spec(RuleSchemaMissingType, CauseNoItems, err, "field", "",
		func(x in) string { return "array " + q(x.Field) + " declares no `items`" },
		text("List one shape for a uniform array or several for a multi-shape array.")),
	spec(RuleSchemaMissingLabel, CauseBlockNoLabel, err, "", "", text("the block has no `label`"), nil),
	spec(RuleSchemaMissingLabel, CauseFieldNoLabel, err, "field", "",
		func(x in) string { return "field " + q(x.Field) + " has no `label`" },
		text("Labels are required; the editor does not fall back to the key.")),
	spec(RuleSchemaUnknownType, "", err, "field type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has type " + q(x.Type) + ", which the editor does not render"
		},
		text(typesHint)),
	spec(RuleSchemaUnknownKnob, CauseUnknownKey, warn, "field key type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has key " + q(x.Key) + ", which a " + x.Type + " field does not read"
		}, nil),
	spec(RuleSchemaUnknownKnob, CauseToolbarNotOptions, warn, "field key type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has key " + q(x.Key) + ", which a " + x.Type + " field does not read"
		},
		text("richtext takes `options:` (a map of flags such as bold: true, headings: [2, 3]), not a `toolbar:` list.")),
	spec(RuleSchemaUnknownKnob, CauseAltKnobNotOnImage, warn, "field key type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has key " + q(x.Key) + ", which a " + x.Type + " field does not read"
		},
		func(x in) string {
			return "`" + x.Key + "` belongs on an image field. To mark a text field as an image's alt text, give it `role: alt` or name it from the image with `alt: <key>`."
		}),
	spec(RuleSchemaUnknownKnob, CauseExtraKeysPolicy, warn, "value", "",
		func(x in) string {
			return "extraKeysPolicy " + q(x.Value) + " is not show, hide or strip; show is assumed"
		}, nil),
	spec(RuleSchemaUnknownKnob, CauseConditionOperator, err, "field knob value", "",
		func(x in) string {
			return q(x.Field) + " has a " + x.Knob + " with operator " + q(x.Value) + ", which does not exist"
		}, text(operatorsHint)),
	spec(RuleSchemaDuplicateKey, "", err, "key other", "",
		func(x in) string { return "key " + q(x.Key) + " is also used by " + x.Other + " in the same list" },
		text("Keys must be unique inside one fields list; the editor and the content would not know which field a value belongs to.")),
	spec(RuleSchemaEmptyOptions, "", err, "type field", "",
		func(x in) string { return x.Type + " field " + q(x.Field) + " has no options" }, nil),
	spec(RuleSchemaDefaultNotOption, "", err, "value field", "actual",
		func(x in) string {
			v := q(x.Value)
			if x.Actual != "" {
				// A default that is not text is printed as written.
				v = x.Value
			}
			return "default " + v + " of " + q(x.Field) + " is not one of its options"
		}, nil),
	spec(RuleSchemaMinAboveMax, CauseMinMax, err, "field min max", "",
		func(x in) string { return q(x.Field) + " has min " + gnum(x.Min) + " above max " + gnum(x.Max) }, nil),
	spec(RuleSchemaMinAboveMax, CauseLength, err, "field min max", "",
		func(x in) string {
			return q(x.Field) + " has minLength " + num(x.Min) + " above maxLength " + num(x.Max)
		}, nil),
	spec(RuleSchemaMinAboveMax, CauseItems, err, "field min max", "",
		func(x in) string { return q(x.Field) + " has minItems " + num(x.Min) + " above maxItems " + num(x.Max) }, nil),
	spec(RuleSchemaInvalidRegex, CauseUnsupportedSyntax, warn, "field knob detail", "",
		func(x in) string { return q(x.Field) + " has a " + x.Knob + " that does not compile: " + x.Detail },
		text(re2Hint)),
	spec(RuleSchemaInvalidRegex, CauseDoesNotCompile, warn, "field knob detail", "",
		func(x in) string { return q(x.Field) + " has a " + x.Knob + " that does not compile: " + x.Detail }, nil),
	spec(RuleSchemaUnresolvedConditionPath, CauseNoField, err, "field knob", "",
		func(x in) string { return q(x.Field) + " has a " + x.Knob + " without a `field`" }, nil),
	spec(RuleSchemaUnresolvedConditionPath, CausePathUnknown, warn, "field knob path", "",
		func(x in) string {
			return q(x.Field) + " has a " + x.Knob + " on " + q(x.Path) + ", which names no field here"
		},
		text(conditionPathHint)),
	spec(RuleSchemaUnresolvedComputedPath, "", warn, "field path", "",
		func(x in) string {
			return q(x.Field) + " is computed from " + q(x.Path) + ", which names no field here"
		},
		text("The value never fills in while the path does not resolve.")),
	spec(RuleSchemaUnknownCustomValidator, "", warn, "field value", "",
		func(x in) string {
			return q(x.Field) + " has a custom validator of type " + q(x.Value) + ", which nothing implements; it is skipped"
		}, text("Built-in types: regex, expression.")),
	spec(RuleSchemaAmbiguousItemShapes, "", err, "names field type", "more",
		func(x in) string {
			return "shapes " + comma(x.Names) + more(x.Params) + " of " + q(x.Field) + " all hold " + x.Type + " values, so items of these shapes cannot be told apart"
		}, text("Keep at most one primitive shape per value type, or wrap the others in groups so they carry a discriminator.")),
	spec(RuleSchemaAnonymousItemShapes, "", err, "field key", "",
		func(x in) string {
			return "a shape of " + q(x.Field) + " has no key, so no item can name it in its " + q(x.Key)
		},
		text("Give each entry under `items:` a key (hero, cta, …); the editor writes that key as the item's discriminator.")),
	spec(RuleSchemaUnknownStorage, "", warn, "field key", "names more",
		func(x in) string {
			return q(x.Field) + " uses storage " + q(x.Key) + ", which config.yml does not declare"
		},
		declaredStorages("storage")),
	spec(RuleSchemaUnknownDefaultStorage, "", err, "key", "names more",
		func(x in) string { return "defaultStorage " + q(x.Key) + " is not declared under storages" },
		declaredStorages("defaultStorage")),
	spec(RuleSchemaTranslatableIgnored, "", warn, "field type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has `translatable`, but a value of type " + x.Type + " is never translated"
		}, text("Only text, paragraph and richtext are translated; on a group or an array the key covers the fields inside. Remove it here.")),

	// Collections and config.yml.
	spec(RuleSchemaDuplicateCollectionPath, "", err, "names path", "more",
		func(x in) string {
			return "collections " + comma(x.Names) + more(x.Params) + " share path " + q(x.Path) + " and their `match` patterns cannot tell them apart; files under it resolve to one of them arbitrarily"
		}, text("Give each collection its own directory, merge them, or let all but one name a single file with a literal `match`.")),
	spec(RuleSchemaMissingCollectionPath, "", warn, "", "",
		text("the collection declares no `path`, so no file resolves to it by directory"),
		text("Add `path: <directory>` so files under it use this schema.")),
	spec(RuleSchemaUnknownPreviewToken, "", err, "value names", "more",
		func(x in) string {
			return "previewPath " + q(x.Value) + " uses unknown token(s) " + comma(x.Names) + more(x.Params)
		}, text("Tokens are {locale}, {slug} and {fields.<key>}.")),
	spec(RuleSchemaInvalidLocaleFrom, "", err, "value", "",
		func(x in) string { return "localeFrom " + q(x.Value) + " is not path, none or field:<key>" }, nil),
	spec(RuleSchemaInvalidTranslations, CauseByPathWithoutLocale, err, "", "",
		text("translations.by is path, but the collection's locale does not come from the path"),
		text("Add `localeFrom: path`, or link the versions by a field: `by: field:<key>`.")),
	spec(RuleSchemaInvalidTranslations, CauseByUnknown, err, "value", "",
		func(x in) string { return "translations.by " + q(x.Value) + " is not path or field:<key>" },
		text(translationsForms+" A collection whose files hold every language in localized groups may list `locales` alone.")),
	spec(RuleSchemaInvalidTranslations, CauseByFieldNotText, err, "key type", "",
		func(x in) string {
			return "translations.by names " + q(x.Key) + ", which is a " + x.Type + " field; the linking field must be text"
		}, text("Link the versions by a text field whose value they share.")),
	spec(RuleSchemaInvalidTranslations, CauseByFieldNotRequired, err, "key", "",
		func(x in) string {
			return "translations.by names " + q(x.Key) + ", which is not required, so a page could have no key"
		}, text("Mark the field `required: true`.")),
	spec(RuleSchemaInvalidTranslations, CauseByFieldNotTopLevel, err, "key", "",
		func(x in) string {
			return "translations.by names " + q(x.Key) + ", which is not a top-level field of the collection"
		}, text("Add the field to the collection, or name one it has.")),
	spec(RuleSchemaInvalidTranslationLocales, CauseNotList, err, "value", "",
		func(x in) string { return "translations.locales is " + x.Value + ", not a list of languages" },
		text(translationLocales)),
	spec(RuleSchemaInvalidTranslationLocales, CauseEmpty, err, "", "",
		text("translations.locales lists no language"), text(translationLocales)),
	spec(RuleSchemaInvalidTranslationLocales, CauseNotLanguageCode, err, "value", "",
		func(x in) string { return "translations.locales has " + x.Value + ", which is not a language code" },
		text(translationLocales)),
	spec(RuleSchemaInvalidTranslationLocales, CauseListedTwice, err, "locale", "",
		func(x in) string { return "translations.locales lists " + q(x.Locale) + " twice" }, text(translationLocales)),
	spec(RuleSchemaInvalidTranslationLocales, CauseNoSiteLocales, err, "locale", "",
		func(x in) string {
			return "translations.locales lists " + q(x.Locale) + ", but the site declares no preview.locales"
		}, text(translationLocales)),
	spec(RuleSchemaInvalidTranslationLocales, CauseNotSiteLocale, err, "locale locales", "",
		func(x in) string {
			return "translations.locales lists " + q(x.Locale) + ", which is not in preview.locales " + list(x.Locales)
		}, text(translationLocales)),

	// Roles.
	spec(RuleSchemaUnknownRole, "", err, "field value", "",
		func(x in) string { return "field " + q(x.Field) + " has role " + x.Value + ", which is not a role" },
		text(rolesHint)),
	spec(RuleSchemaDuplicateRole, CausePageRole, err, "field role other", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role " + x.Role + ", which " + x.Other + " already declares"
		},
		text("A page has one field per role; keep the role on one of them.")),
	spec(RuleSchemaDuplicateRole, CauseAltRole, err, "field other", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role alt, which " + x.Other + " already declares in the same list"
		}, text("One field holds an image's alt text.")),
	spec(RuleSchemaRoleTypeMismatch, CauseGroupNotLocalized, err, "field role", "",
		func(x in) string {
			return "group " + q(x.Field) + " has role " + x.Role + "; a group holds a role only when it is localized"
		}, text("When the group's fields are the site's languages, mark it `localized: true` and keep the role on the group; else put the role on the field inside it.")),
	spec(RuleSchemaRoleTypeMismatch, CauseTypeCannotHold, err, "field role type allowed", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role " + x.Role + ", which a " + x.Type + " field cannot hold"
		}, roleTypesHint),
	spec(RuleSchemaRoleTypeMismatch, CauseLocalizedTypeCannotHold, err, "field role allowed", "type",
		func(x in) string {
			return "field " + q(x.Field) + " has role " + x.Role + ", which a localized group of " + or(x.Type, "mixed") + " fields cannot hold"
		}, roleTypesHint),
	spec(RuleSchemaMisplacedRole, CausePageRoleInList, err, "field role", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role " + x.Role + " inside a list, where it repeats per item; a page has one " + x.Role
		}, text("Declare it on a field at the top level of the collection or inside a group, not inside a list.")),
	spec(RuleSchemaMisplacedRole, CauseAltWithoutImage, err, "field", "",
		func(x in) string { return "field " + q(x.Field) + " has role alt, but its fields list holds no image" },
		text("role: alt marks the alt text of the image next to it, in the same group or list item.")),
	spec(RuleSchemaMisplacedRole, CausePageRoleInListedBlock, err, "key role", "",
		func(x in) string {
			return "block " + q(x.Key) + ", an item of this list, has a field with role " + x.Role + ", which would repeat per item; a page has one " + x.Role
		}, text("Declare page roles at the top level of the collection or inside a group, not inside a list.")),
	spec(RuleSchemaInvalidAlt, CauseAltRoleManyImages, err, "field count key", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role alt, but its fields list holds " + integer(x.Count) + " images"
		},
		func(x in) string {
			return "Say which image it describes with `alt: " + x.Key + "` on that image instead."
		}),
	spec(RuleSchemaInvalidAlt, CauseAltRoleDecorativeImage, err, "field key", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role alt, but image " + q(x.Key) + " is decorative"
		},
		text("A decorative image has no alt text: remove the role, or `decorative: true` from the image.")),
	spec(RuleSchemaInvalidAlt, CauseAltRoleAndAltKnob, err, "field key other", "",
		func(x in) string {
			return "field " + q(x.Field) + " has role alt, but image " + q(x.Key) + " names " + q(x.Other) + " as its alt text"
		}, text("Keep one of the two declarations.")),
	spec(RuleSchemaInvalidAlt, CauseDecorativeNotBoolean, err, "field value", "",
		func(x in) string {
			return "image " + q(x.Field) + " has decorative " + x.Value + "; it is true or false"
		}, nil),
	spec(RuleSchemaInvalidAlt, CauseDecorativeWithAlt, err, "field", "",
		func(x in) string { return "image " + q(x.Field) + " is decorative and also says where its alt text is" },
		text("A decorative image needs no alt text: keep one of the two.")),
	spec(RuleSchemaInvalidAlt, CauseAltKeyUnknown, err, "field key", "",
		func(x in) string {
			return "image " + q(x.Field) + " has alt." + x.Key + ", which a composed alt text does not read"
		},
		text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltFromCount, err, "field value max", "",
		func(x in) string {
			return "image " + q(x.Field) + " composes its alt text from " + x.Value + "; alt.from lists 1 to " + num(x.Max) + " fields"
		}, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltFromTwice, err, "field key", "",
		func(x in) string { return "image " + q(x.Field) + " lists " + q(x.Key) + " twice in alt.from" }, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltSeparatorInvalid, err, "field value max", "",
		func(x in) string {
			return "image " + q(x.Field) + " has alt.separator " + x.Value + "; it is text of at most " + num(x.Max) + " bytes"
		}, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltNamesNothing, err, "field value", "",
		func(x in) string { return "image " + q(x.Field) + " has alt " + x.Value + ", which names no field" }, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltNamesItself, err, "field", "",
		func(x in) string { return "image " + q(x.Field) + " names itself as its alt text" }, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltFieldNotText, err, "field key type", "",
		func(x in) string {
			return "image " + q(x.Field) + " takes its alt text from " + q(x.Key) + ", which is a " + x.Type + " field"
		}, text(altHint)),
	spec(RuleSchemaInvalidAlt, CauseAltFieldMissing, err, "field key", "",
		func(x in) string {
			return "image " + q(x.Field) + " takes its alt text from " + q(x.Key) + ", but no field in the same list has that key"
		}, text(altHint)),
	spec(RuleSchemaInvalidWhenEmpty, CauseUnknownValue, err, "field value", "",
		func(x in) string {
			return "field " + q(x.Field) + " has whenEmpty " + x.Value + ", which is not a value it takes"
		},
		text(whenEmptyHint)),
	spec(RuleSchemaInvalidWhenEmpty, CauseNoRole, err, "field allowed", "",
		func(x in) string { return "field " + q(x.Field) + " has whenEmpty but no role" },
		func(x in) string {
			return "whenEmpty says what the site does when a role field is empty; give the field its role (" + comma(x.Allowed) + ") or remove whenEmpty."
		}),
	spec(RuleSchemaInvalidWhenEmpty, CauseRoleNotFilledInCode, err, "field role allowed", "",
		func(x in) string {
			return "field " + q(x.Field) + " has whenEmpty on role " + x.Role + ", whose empty value a site does not fill in code"
		}, func(x in) string { return "whenEmpty goes on a field with role " + comma(x.Allowed) + "." }),

	// Localized groups.
	spec(RuleSchemaInvalidLocalized, CauseLocalizedNotBoolean, err, "field value", "",
		func(x in) string {
			return "group " + q(x.Field) + " has localized " + x.Value + "; it is true or false"
		},
		text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseFallbackNotLocalized, err, "field", "",
		func(x in) string { return "group " + q(x.Field) + " has a fallback but is not localized" },
		text("fallback names the language an empty language of a localized group shows; add `localized: true` or remove it.")),
	spec(RuleSchemaInvalidLocalized, CauseFallbackNotSiteLocale, err, "field value", "locales",
		func(x in) string {
			return "group " + q(x.Field) + " falls back to " + x.Value + ", which is not in preview.locales " + list(x.Locales)
		}, text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseOneLanguagePerFile, err, "field", "",
		func(x in) string {
			return "group " + q(x.Field) + " is localized, but each file of this collection is one language"
		},
		text("A localized group holds every language in one file. In a collection whose files are one language each (localeFrom path or field, translations.by), use a plain field.")),
	spec(RuleSchemaInvalidLocalized, CauseNoSiteLocales, err, "field", "",
		func(x in) string { return "group " + q(x.Field) + " is localized, but the site declares no languages" },
		text("List the site's languages in preview.locales.")),
	spec(RuleSchemaInvalidLocalized, CauseFieldNotSiteLocale, err, "field key locales", "",
		func(x in) string {
			return "group " + q(x.Field) + " is localized, but its field " + q(x.Key) + " is not a language of the site " + list(x.Locales)
		}, text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseFieldType, err, "field key type", "",
		func(x in) string {
			return "group " + q(x.Field) + " is localized, but its field " + q(x.Key) + " is a " + x.Type + " field"
		}, text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseMixedTypes, err, "field type otherType", "",
		func(x in) string {
			return "group " + q(x.Field) + " is localized, but its fields mix " + x.Type + " and " + x.OtherType
		}, text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseLanguageFieldRole, err, "key field", "",
		func(x in) string {
			return "field " + q(x.Key) + " of localized group " + q(x.Field) + " has a role of its own"
		},
		text("Declare the role on the localized group: it then means the field in each language.")),
	spec(RuleSchemaInvalidLocalized, CauseLocaleMissing, err, "field locale", "",
		func(x in) string { return "group " + q(x.Field) + " is localized, but has no field for " + q(x.Locale) },
		text(localizedHint)),
	spec(RuleSchemaInvalidLocalized, CauseOptionLabelsNotMapping, err, "key field value", "",
		func(x in) string {
			return "option " + x.Key + " of " + q(x.Field) + " has labels " + x.Value + "; it maps languages to labels"
		}, nil),
	spec(RuleSchemaInvalidLocalized, CauseOptionLabelLocale, err, "key field locale", "",
		func(x in) string {
			return "option " + x.Key + " of " + q(x.Field) + " has a label for " + q(x.Locale) + ", which is not a language of the site or not text"
		}, text("labels maps each language of preview.locales to the option's label in it.")),

	// Title templates.
	spec(RuleSchemaInvalidTitleTemplate, CauseNotText, err, "field value", "",
		func(x in) string {
			return "field " + q(x.Field) + " has titleTemplate " + x.Value + ", which is not text"
		},
		text(templateHint)),
	spec(RuleSchemaInvalidTitleTemplate, CauseRoleNotSEOTitle, err, "field", "",
		func(x in) string { return "field " + q(x.Field) + " has a titleTemplate but not role seoTitle" },
		text("titleTemplate says how the site composes the page title from its seoTitle field; give the field `role: seoTitle` or remove it.")),
}, templateSpecs()...), append([]CauseSpec{
	spec(RuleSchemaInvalidTitleTemplate, CauseFieldNotInPage, err, "field path", "",
		func(x in) string {
			return "field " + q(x.Field) + " has a titleTemplate that names {field:" + x.Path + "}, which is not a field of the page outside lists"
		}, text(templateHint)),
	spec(RuleSchemaInvalidTitleTemplate, CauseLabelNotSelect, err, "field path type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has a titleTemplate that asks for the label of " + q(x.Path) + ", which is a " + x.Type + " field, not a select"
		}, text("`|label` reads a select's option label; drop it to use the value.")),
	spec(RuleSchemaInvalidTitleTemplate, CauseFieldHasNoValue, err, "field path type", "",
		func(x in) string {
			return "field " + q(x.Field) + " has a titleTemplate that names " + q(x.Path) + ", a " + x.Type + ", which has no one value"
		}, text(templateHint)),

	// Role values and excerpts.
	spec(RuleSchemaInvalidRoleValues, CauseWhenMissing, err, "field role knob", "allowed",
		func(x in) string {
			return "select " + q(x.Field) + " has role " + x.Role + " but no " + x.Knob + ": which of its values " + whenVerb(x.Role) + "?"
		}, func(x in) string { return "List them: `" + x.Knob + ": [" + comma(x.Allowed) + "]`." }),
	spec(RuleSchemaInvalidRoleValues, CauseWhenNotRead, err, "field knob role", "",
		func(x in) string {
			return "field " + q(x.Field) + " has " + x.Knob + ", which only a select with role " + x.Role + " reads"
		}, func(x in) string {
			return "A boolean with role " + x.Role + " needs no list: true " + whenVerb(x.Role) + "."
		}),
	spec(RuleSchemaInvalidRoleValues, CauseWhenCount, err, "field knob value max", "",
		func(x in) string {
			return "select " + q(x.Field) + " has " + x.Knob + " " + x.Value + "; it lists 1 to " + num(x.Max) + " of its values"
		}, nil),
	spec(RuleSchemaInvalidRoleValues, CauseWhenTwice, err, "field value knob", "",
		func(x in) string { return "select " + q(x.Field) + " lists " + q(x.Value) + " twice in " + x.Knob }, nil),
	spec(RuleSchemaInvalidRoleValues, CauseWhenNotOption, err, "field value knob", "allowed",
		func(x in) string {
			return "select " + q(x.Field) + " lists " + q(x.Value) + " in " + x.Knob + ", which is not one of its options"
		}, func(x in) string { return "Options: " + comma(x.Allowed) + "." }),
	spec(RuleSchemaInvalidExcerpt, CauseUnknownValue, err, "field value", "",
		func(x in) string {
			return "field " + q(x.Field) + " has excerpt " + x.Value + ", which is not a value it takes"
		},
		text(excerptHint)),
	spec(RuleSchemaInvalidExcerpt, CauseNotRichSEODescription, err, "field", "",
		func(x in) string {
			return "field " + q(x.Field) + " has excerpt, which only a rich text field with role seoDescription reads"
		}, text(excerptHint)),
	spec(RuleSchemaInvalidExcerpt, CauseMissing, err, "field", "",
		func(x in) string {
			return "field " + q(x.Field) + " is rich text with role seoDescription, but does not say which part of it is the description"
		}, text(excerptHint)),

	// The quality block.
	spec(RuleSchemaInvalidQuality, CauseNotMapping, warn, "value", "",
		func(x in) string { return "quality is " + x.Value + ", not a mapping" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseLimitsNotInSite, warn, "", "",
		text("quality.limits is read in config.yml only: an image is checked once per file, not per page"), text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseSiteKeyUnknown, warn, "key", "",
		func(x in) string { return "quality has the key " + q(x.Key) + "; only `ignore` and `limits` are read" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseCollectionKeyUnknown, warn, "key", "",
		func(x in) string { return "quality has the key " + q(x.Key) + "; only `ignore` is read" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseLimitsNotMapping, warn, "value", "",
		func(x in) string { return "quality.limits is " + x.Value + ", not a mapping" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseLimitsKeyUnknown, warn, "key knob", "",
		func(x in) string { return "quality.limits has the key " + q(x.Key) + "; only `" + x.Knob + "` is read" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseImageKBNotNumber, warn, "value", "",
		func(x in) string { return "quality.limits.imageKB is " + x.Value + ", not a whole number of kilobytes" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseImageKBOutOfRange, warn, "current min max", "",
		func(x in) string {
			return "quality.limits.imageKB is " + num(x.Current) + ", outside " + num(x.Min) + " to " + num(x.Max)
		}, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseIgnoreNotList, warn, "value", "",
		func(x in) string { return "quality.ignore is " + x.Value + ", not a list of rule ids" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseIgnoreNotRuleID, warn, "value", "",
		func(x in) string { return "quality.ignore has " + x.Value + ", which is not a rule id" }, text(qualityHint)),
	spec(RuleSchemaInvalidQuality, CauseIgnoreUnknownRule, warn, "value", "",
		func(x in) string {
			return "quality.ignore lists " + q(x.Value) + ", which is not a content quality rule"
		}, text(qualityHint)),

	// The preview block, redirects, variables.
	spec(RuleSchemaDefaultLocaleNotDeclared, "", err, "locale locales", "",
		func(x in) string {
			return "defaultLocale " + q(x.Locale) + " is not in preview.locales " + list(x.Locales)
		}, nil),
	spec(RuleSchemaUnknownRedirectsFormat, "", err, "value", "",
		func(x in) string { return "redirects.format " + q(x.Value) + " is not supported (netlify, vercel)" }, nil),
	spec(RuleSchemaMissingVariablesFile, "", err, "path", "",
		func(x in) string { return "variables.file " + q(x.Path) + " does not exist" },
		text("Create the variables document, or point `variables.file` at the file that holds the {key, value} list.")),
	spec(RuleSchemaInternalError, "", err, "detail", "", internalError, text(internalHint)),
}, storageSpecs()...)...)

func roleTypesHint(x in) string {
	return x.Role + " goes on a field of type " + joinOr(x.Allowed) + "."
}

// joinOr joins with " or ".
func joinOr(xs []string) string {
	out := ""
	for i, s := range xs {
		if i > 0 {
			out += " or "
		}
		out += s
	}
	return out
}
