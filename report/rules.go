package report

// Rule is the frozen identifier of one check. The prefixes group the
// checks by what they read: `schema.` the `.norcube/cms` directory
// alone, `content.` one content file against its collection, `links.`
// the whole site (URLs, assets, references between files). `quality.`
// is the other family: the site works, but a page could serve its
// readers better (package quality). Quality findings are never errors.
type Rule string

const qualityPrefix = "quality."

// Schema rules: the directory loads and makes sense on its own.
const (
	// RuleSchemaParseError: a schema file is not valid YAML.
	RuleSchemaParseError Rule = "schema.parse_error"
	// RuleSchemaNotMapping: a schema file's document is not a mapping.
	RuleSchemaNotMapping Rule = "schema.not_mapping"
	// RuleSchemaUnsupportedVersion: `schemaVersion` names a version this
	// module does not know.
	RuleSchemaUnsupportedVersion Rule = "schema.unsupported_version"
	// RuleSchemaUnresolvedRef: a `$ref` names no block and no input.
	RuleSchemaUnresolvedRef Rule = "schema.unresolved_ref"
	// RuleSchemaRefCycle: `$ref` resolution ran deeper than the limit.
	RuleSchemaRefCycle Rule = "schema.ref_cycle"
	// RuleSchemaInputsGlobUnsupported: an `inputsFromGlob` pattern is not
	// of the supported `<dir>/*.yml` form.
	RuleSchemaInputsGlobUnsupported Rule = "schema.inputs_glob_unsupported"
	// RuleSchemaMissingKey: a field has no `key`.
	RuleSchemaMissingKey Rule = "schema.missing_key"
	// RuleSchemaMissingType: a field has no `type`.
	RuleSchemaMissingType Rule = "schema.missing_type"
	// RuleSchemaMissingLabel: a field has no `label`.
	RuleSchemaMissingLabel Rule = "schema.missing_label"
	// RuleSchemaUnknownType: a field's `type` is not one the editor renders.
	RuleSchemaUnknownType Rule = "schema.unknown_type"
	// RuleSchemaUnknownKnob: a field carries a key no field of its type
	// reads (a typo, or a knob from another type).
	RuleSchemaUnknownKnob Rule = "schema.unknown_knob"
	// RuleSchemaDuplicateKey: two fields in one list share a key.
	RuleSchemaDuplicateKey Rule = "schema.duplicate_key"
	// RuleSchemaEmptyOptions: a select or multichoice field has no options.
	RuleSchemaEmptyOptions Rule = "schema.empty_options"
	// RuleSchemaDefaultNotOption: a select's `default` is not one of its
	// options.
	RuleSchemaDefaultNotOption Rule = "schema.default_not_option"
	// RuleSchemaMinAboveMax: `min` exceeds `max`, or `minLength` exceeds
	// `maxLength`, or `minItems` exceeds `maxItems`.
	RuleSchemaMinAboveMax Rule = "schema.min_above_max"
	// RuleSchemaInvalidRegex: a `regex` or custom validator pattern does
	// not compile. The editor evaluates patterns in JavaScript; this
	// module evaluates them with Go's RE2 engine, so a pattern that only
	// RE2 rejects (lookaround, backreference) is reported with that
	// caveat in the message.
	RuleSchemaInvalidRegex Rule = "schema.invalid_regex"
	// RuleSchemaUnresolvedConditionPath: a `visibleIf` path names a field
	// that does not exist in its scope.
	RuleSchemaUnresolvedConditionPath Rule = "schema.unresolved_condition_path"
	// RuleSchemaUnresolvedComputedPath: a `computed.from` path names a
	// field that does not exist in its scope.
	RuleSchemaUnresolvedComputedPath Rule = "schema.unresolved_computed_path"
	// RuleSchemaUnknownCustomValidator: a `customValidators` entry has a
	// type no handler implements.
	RuleSchemaUnknownCustomValidator Rule = "schema.unknown_custom_validator"
	// RuleSchemaAmbiguousItemShapes: two primitive item shapes of one
	// multi-shape array hold the same value type, so items cannot be
	// told apart.
	RuleSchemaAmbiguousItemShapes Rule = "schema.ambiguous_item_shapes"
	// RuleSchemaAnonymousItemShapes: a group item shape of a multi-shape
	// array has no key, so no item can name it.
	RuleSchemaAnonymousItemShapes Rule = "schema.anonymous_item_shapes"
	// RuleSchemaUnknownStorage: a field's `storage` names no declared
	// storage.
	RuleSchemaUnknownStorage Rule = "schema.unknown_storage"
	// RuleSchemaUnknownDefaultStorage: `defaultStorage` names no declared
	// storage.
	RuleSchemaUnknownDefaultStorage Rule = "schema.unknown_default_storage"
	// RuleSchemaInvalidStorage: a storage entry is malformed (missing
	// type or path, invalid naming, dedup, mode or inline).
	RuleSchemaInvalidStorage Rule = "schema.invalid_storage"
	// RuleSchemaDuplicateCollectionPath: two collections share a `path`,
	// so files under it resolve to one of them arbitrarily.
	RuleSchemaDuplicateCollectionPath Rule = "schema.duplicate_collection_path"
	// RuleSchemaMissingCollectionPath: a collection other than `pages`
	// declares no `path`, so no file can belong to it by directory.
	RuleSchemaMissingCollectionPath Rule = "schema.missing_collection_path"
	// RuleSchemaUnknownPreviewToken: `previewPath` uses a token other than
	// `{locale}`, `{slug}` or `{fields.<key>}`.
	RuleSchemaUnknownPreviewToken Rule = "schema.unknown_preview_token"
	// RuleSchemaInvalidLocaleFrom: `localeFrom` is not `path`, `none` or
	// `field:<key>`.
	RuleSchemaInvalidLocaleFrom Rule = "schema.invalid_locale_from"
	// RuleSchemaInvalidTranslations: a collection's `translations.by` is
	// not `path` or `field:<key>`, names a field the collection does not
	// have at its top level as a required text field, or is `path` in a
	// collection whose locale does not come from the path.
	RuleSchemaInvalidTranslations Rule = "schema.invalid_translations"
	// RuleSchemaTranslatableIgnored: `translatable` is set on a field whose
	// type is never translated (a number, an image, a select…), so it
	// changes nothing.
	RuleSchemaTranslatableIgnored Rule = "schema.translatable_ignored"
	// RuleSchemaInvalidTranslationLocales: a collection's
	// `translations.locales` is not a list of languages, is empty, repeats
	// a language, or names one the site does not declare in
	// `preview.locales`.
	RuleSchemaInvalidTranslationLocales Rule = "schema.invalid_translation_locales"
	// RuleSchemaUnknownRole: a field's `role` is not one of the roles
	// (seoTitle, seoDescription, shareImage, noindex, canonical, alt).
	RuleSchemaUnknownRole Rule = "schema.unknown_role"
	// RuleSchemaDuplicateRole: two fields of one collection or block
	// declare the same page role, or two fields of one fields list declare
	// `role: alt`.
	RuleSchemaDuplicateRole Rule = "schema.duplicate_role"
	// RuleSchemaRoleTypeMismatch: a field's type cannot hold its role
	// (`shareImage` on a text field, `noindex` on a select).
	RuleSchemaRoleTypeMismatch Rule = "schema.role_type_mismatch"
	// RuleSchemaMisplacedRole: a page role sits inside a list, where it
	// would repeat per item, or `role: alt` sits in a fields list that
	// holds no image.
	RuleSchemaMisplacedRole Rule = "schema.misplaced_role"
	// RuleSchemaInvalidAlt: an image's `alt` names no text field in the
	// same fields list, an image is both `decorative` and given alt text,
	// `decorative` is not a boolean, or `role: alt` cannot tell which of
	// several images it describes.
	RuleSchemaInvalidAlt Rule = "schema.invalid_alt"
	// RuleSchemaInvalidQuality: a `quality` block in config.yml or a
	// collection is not a mapping, its `ignore` is not a list, or the list
	// names something that is not a quality rule.
	RuleSchemaInvalidQuality Rule = "schema.invalid_quality"
	// RuleSchemaDefaultLocaleNotDeclared: `preview.defaultLocale` is not
	// in `preview.locales`.
	RuleSchemaDefaultLocaleNotDeclared Rule = "schema.default_locale_not_declared"
	// RuleSchemaUnknownRedirectsFormat: `redirects.format` is not netlify
	// or vercel.
	RuleSchemaUnknownRedirectsFormat Rule = "schema.unknown_redirects_format"
	// RuleSchemaMissingVariablesFile: `variables.file` names a file the
	// site does not have.
	RuleSchemaMissingVariablesFile Rule = "schema.missing_variables_file"
	// RuleSchemaInternalError: loading or checking the schema stopped on
	// a bug in this module (a recovered panic). The checks after it did
	// not run; the finding carries the cause for a bug report. Also the
	// rule of a run that stopped outside any one check.
	RuleSchemaInternalError Rule = "schema.internal_error"
)

// Content rules: one file against its collection.
const (
	// RuleContentParseError: the frontmatter or data file is not valid
	// YAML.
	RuleContentParseError Rule = "content.parse_error"
	// RuleContentNoCollection: a content file under the pages directory
	// belongs to no collection.
	RuleContentNoCollection Rule = "content.no_collection"
	// RuleContentRequired: a required field is empty.
	RuleContentRequired Rule = "content.required"
	// RuleContentShape: a value's type does not match the field's type,
	// or a select value is not one of the options.
	RuleContentShape Rule = "content.shape"
	// RuleContentItemTypeMissing: a multi-shape array item carries no
	// discriminator.
	RuleContentItemTypeMissing Rule = "content.item_type_missing"
	// RuleContentItemTypeUnknown: a multi-shape array item's
	// discriminator names no shape.
	RuleContentItemTypeUnknown Rule = "content.item_type_unknown"
	// RuleContentMinItems: an array has fewer items than `minItems`.
	RuleContentMinItems Rule = "content.min_items"
	// RuleContentMaxItems: an array has more items than `maxItems`.
	RuleContentMaxItems Rule = "content.max_items"
	// RuleContentRegex: a value does not match the field's `regex`.
	RuleContentRegex Rule = "content.regex"
	// RuleContentMinLength: a value is shorter than `minLength`.
	RuleContentMinLength Rule = "content.min_length"
	// RuleContentMaxLength: a value is longer than `maxLength`.
	RuleContentMaxLength Rule = "content.max_length"
	// RuleContentMin: a number is below `min`.
	RuleContentMin Rule = "content.min"
	// RuleContentMax: a number is above `max`.
	RuleContentMax Rule = "content.max"
	// RuleContentUnique: two array items share the value `uniqueOn`
	// points at.
	RuleContentUnique Rule = "content.unique"
	// RuleContentCustom: a `customValidators` entry rejected the value.
	RuleContentCustom Rule = "content.custom"
	// RuleContentExtraKeys: the file has keys the schema does not declare.
	// A warning under `extraKeysPolicy: show`, an error under `strip`
	// (the next save deletes them), silent under `hide`.
	RuleContentExtraKeys Rule = "content.extra_keys"
	// RuleContentUnexpectedBody: a file in a collection without a body
	// has one.
	RuleContentUnexpectedBody Rule = "content.unexpected_body"
	// RuleContentInternalError: checking one content file stopped on a
	// bug in this module (a recovered panic). The file's other checks
	// did not run.
	RuleContentInternalError Rule = "content.internal_error"
)

// Links rules: the whole site.
const (
	// RuleLinksURLCollision: two files whose collections derive URLs from
	// the file resolve to the same URL.
	RuleLinksURLCollision Rule = "links.url_collision"
	// RuleLinksTranslationCollision: two files of one collection and one
	// locale share a translation key, so they claim to be the same page
	// in the same language.
	RuleLinksTranslationCollision Rule = "links.translation_collision"
	// RuleLinksUnresolvedLink: a site link points at no file's URL, no
	// asset and no known path.
	RuleLinksUnresolvedLink Rule = "links.unresolved_link"
	// RuleLinksMissingAsset: an image or file value points at a file the
	// repository does not have.
	RuleLinksMissingAsset Rule = "links.missing_asset"
	// RuleLinksUnresolvedReference: a reference field's value names no
	// entry.
	RuleLinksUnresolvedReference Rule = "links.unresolved_reference"
	// RuleLinksOrphanSidecar: a `.norcube/cms/meta` sidecar belongs to a
	// content file that no longer exists.
	RuleLinksOrphanSidecar Rule = "links.orphan_sidecar"
	// RuleLinksInternalError: checking the links stopped on a bug in
	// this module (a recovered panic). The links checks after it did not
	// run.
	RuleLinksInternalError Rule = "links.internal_error"
)

// Quality rules: what a site owner can improve. Package quality holds
// each one's area, impact and threshold.
const (
	// RuleQualityTitleMissing: an indexable page has no title for search
	// results (its SEO title and the page title are both empty).
	RuleQualityTitleMissing Rule = "quality.title_missing"
	// RuleQualityTitleLength: the page's title for search results is
	// shorter or longer than the recommended window.
	RuleQualityTitleLength Rule = "quality.title_length"
	// RuleQualityTitleDuplicate: two or more indexable pages of one
	// language share a title.
	RuleQualityTitleDuplicate Rule = "quality.title_duplicate"
	// RuleQualityDescriptionMissing: an indexable page has no meta
	// description.
	RuleQualityDescriptionMissing Rule = "quality.description_missing"
	// RuleQualityDescriptionLength: the meta description is shorter or
	// longer than the recommended window.
	RuleQualityDescriptionLength Rule = "quality.description_length"
	// RuleQualityDescriptionDuplicate: two or more indexable pages of one
	// language share a meta description.
	RuleQualityDescriptionDuplicate Rule = "quality.description_duplicate"
	// RuleQualityShareImageMissing: an indexable page has no share image.
	RuleQualityShareImageMissing Rule = "quality.share_image_missing"
	// RuleQualityNoindex: the page is hidden from search engines. Listed,
	// not a problem.
	RuleQualityNoindex Rule = "quality.noindex"
	// RuleQualityImageAltMissing: an image that is not decorative has no
	// alt text.
	RuleQualityImageAltMissing Rule = "quality.image_alt_missing"
	// RuleQualityImageAltPoor: an image's alt text is its file name or a
	// generic word ("image", "foto").
	RuleQualityImageAltPoor Rule = "quality.image_alt_poor"
	// RuleQualityHeadingSkip: a text skips a heading level (h2 to h4).
	RuleQualityHeadingSkip Rule = "quality.heading_skip"
	// RuleQualityLinkTextEmpty: a link in a text has no text a screen
	// reader could announce.
	RuleQualityLinkTextEmpty Rule = "quality.link_text_empty"
	// RuleQualityLinkTextGeneric: a link's text says nothing about where
	// it leads ("click here", "zde", "mehr").
	RuleQualityLinkTextGeneric Rule = "quality.link_text_generic"
	// RuleQualityTranslationMissing: a page has no version in a language
	// its collection expects.
	RuleQualityTranslationMissing Rule = "quality.translation_missing"
	// RuleQualityTranslationOutdated: a language version last changed
	// well before the newest version of the same page.
	RuleQualityTranslationOutdated Rule = "quality.translation_outdated"
	// RuleQualityImageHeavy: an image that content uses is a large file.
	RuleQualityImageHeavy Rule = "quality.image_heavy"
	// RuleQualityMediaUnused: a file in a media storage that no content
	// refers to. Listed, not a problem.
	RuleQualityMediaUnused Rule = "quality.media_unused"
)

// QualityRules lists the quality rules in a stable order.
func QualityRules() []Rule {
	return []Rule{
		RuleQualityTitleMissing, RuleQualityTitleLength, RuleQualityTitleDuplicate,
		RuleQualityDescriptionMissing, RuleQualityDescriptionLength, RuleQualityDescriptionDuplicate,
		RuleQualityShareImageMissing, RuleQualityNoindex,
		RuleQualityImageAltMissing, RuleQualityImageAltPoor, RuleQualityHeadingSkip,
		RuleQualityLinkTextEmpty, RuleQualityLinkTextGeneric,
		RuleQualityTranslationMissing, RuleQualityTranslationOutdated,
		RuleQualityImageHeavy, RuleQualityMediaUnused,
	}
}

// Rules lists every rule id this version knows, in a stable order.
func Rules() []Rule {
	return append([]Rule{
		RuleSchemaParseError, RuleSchemaNotMapping, RuleSchemaUnsupportedVersion,
		RuleSchemaUnresolvedRef, RuleSchemaRefCycle, RuleSchemaInputsGlobUnsupported,
		RuleSchemaMissingKey, RuleSchemaMissingType, RuleSchemaMissingLabel,
		RuleSchemaUnknownType, RuleSchemaUnknownKnob, RuleSchemaDuplicateKey,
		RuleSchemaEmptyOptions, RuleSchemaDefaultNotOption, RuleSchemaMinAboveMax,
		RuleSchemaInvalidRegex, RuleSchemaUnresolvedConditionPath,
		RuleSchemaUnresolvedComputedPath, RuleSchemaUnknownCustomValidator,
		RuleSchemaAmbiguousItemShapes, RuleSchemaAnonymousItemShapes,
		RuleSchemaUnknownStorage, RuleSchemaUnknownDefaultStorage, RuleSchemaInvalidStorage,
		RuleSchemaDuplicateCollectionPath, RuleSchemaMissingCollectionPath,
		RuleSchemaUnknownPreviewToken, RuleSchemaInvalidLocaleFrom, RuleSchemaInvalidTranslations,
		RuleSchemaTranslatableIgnored, RuleSchemaInvalidTranslationLocales,
		RuleSchemaUnknownRole, RuleSchemaDuplicateRole, RuleSchemaRoleTypeMismatch,
		RuleSchemaMisplacedRole, RuleSchemaInvalidAlt, RuleSchemaInvalidQuality,
		RuleSchemaDefaultLocaleNotDeclared, RuleSchemaUnknownRedirectsFormat,
		RuleSchemaMissingVariablesFile, RuleSchemaInternalError,
		RuleContentParseError, RuleContentNoCollection, RuleContentRequired,
		RuleContentShape, RuleContentItemTypeMissing, RuleContentItemTypeUnknown,
		RuleContentMinItems, RuleContentMaxItems, RuleContentRegex,
		RuleContentMinLength, RuleContentMaxLength, RuleContentMin, RuleContentMax,
		RuleContentUnique, RuleContentCustom, RuleContentExtraKeys,
		RuleContentUnexpectedBody, RuleContentInternalError,
		RuleLinksURLCollision, RuleLinksTranslationCollision, RuleLinksUnresolvedLink, RuleLinksMissingAsset,
		RuleLinksUnresolvedReference, RuleLinksOrphanSidecar, RuleLinksInternalError,
	}, QualityRules()...)
}
