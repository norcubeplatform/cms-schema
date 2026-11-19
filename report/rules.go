package report

// Rule is the frozen identifier of one check. The three prefixes group
// the checks by what they read: `schema.` the `.norcube/cms` directory
// alone, `content.` one content file against its collection, `links.`
// the whole site (URLs, assets, references between files).
type Rule string

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
	// RuleSchemaDefaultLocaleNotDeclared: `preview.defaultLocale` is not
	// in `preview.locales`.
	RuleSchemaDefaultLocaleNotDeclared Rule = "schema.default_locale_not_declared"
	// RuleSchemaUnknownRedirectsFormat: `redirects.format` is not netlify
	// or vercel.
	RuleSchemaUnknownRedirectsFormat Rule = "schema.unknown_redirects_format"
	// RuleSchemaMissingVariablesFile: `variables.file` names a file the
	// site does not have.
	RuleSchemaMissingVariablesFile Rule = "schema.missing_variables_file"
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
)

// Rules lists every rule id this version knows, in a stable order.
func Rules() []Rule {
	return []Rule{
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
		RuleSchemaDefaultLocaleNotDeclared, RuleSchemaUnknownRedirectsFormat,
		RuleSchemaMissingVariablesFile,
		RuleContentParseError, RuleContentNoCollection, RuleContentRequired,
		RuleContentShape, RuleContentItemTypeMissing, RuleContentItemTypeUnknown,
		RuleContentMinItems, RuleContentMaxItems, RuleContentRegex,
		RuleContentMinLength, RuleContentMaxLength, RuleContentMin, RuleContentMax,
		RuleContentUnique, RuleContentCustom, RuleContentExtraKeys,
		RuleContentUnexpectedBody,
		RuleLinksURLCollision, RuleLinksTranslationCollision, RuleLinksUnresolvedLink, RuleLinksMissingAsset,
		RuleLinksUnresolvedReference, RuleLinksOrphanSidecar,
	}
}
