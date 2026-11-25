package report

// Causes. Each is frozen once released: never renamed, never reused for
// another cause. Which rules a cause appears under, with its params, is
// in Catalog().
const (
	// Loading and parsing.
	CauseNotYAML      Cause = "not_yaml"
	CauseUnreadable   Cause = "unreadable"
	CauseNoType       Cause = "no_type"
	CauseNoItems      Cause = "no_items"
	CauseBlockNoLabel Cause = "block_no_label"
	CauseFieldNoLabel Cause = "field_no_label"

	// schema.unknown_knob.
	CauseUnknownKey        Cause = "unknown_key"
	CauseToolbarNotOptions Cause = "toolbar_not_options"
	CauseAltKnobNotOnImage Cause = "alt_knob_not_on_image"
	CauseExtraKeysPolicy   Cause = "extra_keys_policy"
	CauseConditionOperator Cause = "condition_operator"

	// schema.min_above_max, content.min_items and content.max_items.
	CauseMinMax   Cause = "min_max"
	CauseLength   Cause = "length"
	CauseItems    Cause = "items"
	CauseSelected Cause = "selected"

	// schema.invalid_regex.
	CauseUnsupportedSyntax Cause = "unsupported_syntax"
	CauseDoesNotCompile    Cause = "does_not_compile"

	// schema.unresolved_condition_path.
	CauseNoField     Cause = "no_field"
	CausePathUnknown Cause = "path_unknown"

	// schema.invalid_storage.
	CauseTypeMissing                Cause = "type_missing"
	CauseTypeUnimplemented          Cause = "type_unimplemented"
	CausePathMissing                Cause = "path_missing"
	CauseModeInvalid                Cause = "mode_invalid"
	CauseNamingInvalid              Cause = "naming_invalid"
	CauseInlineInvalid              Cause = "inline_invalid"
	CauseDedupInvalid               Cause = "dedup_invalid"
	CauseDedupNeedsSlugHash         Cause = "dedup_needs_slug_hash"
	CauseOptimizedAtBuildNotBoolean Cause = "optimized_at_build_not_boolean"
	CauseUsedByCodeNotList          Cause = "used_by_code_not_list"
	CauseUsedByCodeTooMany          Cause = "used_by_code_too_many"
	CauseUsedByCodeNotPath          Cause = "used_by_code_not_path"
	CauseUsedByCodeTwice            Cause = "used_by_code_twice"

	// A usedByCode path pattern (schema.invalid_storage, and fact).
	CausePatternEmpty          Cause = "pattern_empty"
	CausePatternTooLong        Cause = "pattern_too_long"
	CausePatternAbsolute       Cause = "pattern_absolute"
	CausePatternBackslash      Cause = "pattern_backslash"
	CausePatternControl        Cause = "pattern_control_character"
	CausePatternEmptyName      Cause = "pattern_empty_name"
	CausePatternDotName        Cause = "pattern_dot_name"
	CausePatternDoubleStarName Cause = "pattern_double_star_in_name"
	CausePatternInvalid        Cause = "pattern_invalid"

	// schema.invalid_translations.
	CauseByPathWithoutLocale Cause = "by_path_without_locale"
	CauseByUnknown           Cause = "by_unknown"
	CauseByFieldNotText      Cause = "by_field_not_text"
	CauseByFieldNotRequired  Cause = "by_field_not_required"
	CauseByFieldNotTopLevel  Cause = "by_field_not_top_level"

	// Lists and languages (schema.invalid_translation_locales,
	// schema.invalid_localized).
	CauseNotList         Cause = "not_list"
	CauseEmpty           Cause = "empty"
	CauseNotLanguageCode Cause = "not_language_code"
	CauseListedTwice     Cause = "listed_twice"
	CauseNoSiteLocales   Cause = "no_site_locales"
	CauseNotSiteLocale   Cause = "not_site_locale"

	// Roles.
	CausePageRole                Cause = "page_role"
	CauseAltRole                 Cause = "alt_role"
	CauseGroupNotLocalized       Cause = "group_not_localized"
	CauseTypeCannotHold          Cause = "type_cannot_hold"
	CauseLocalizedTypeCannotHold Cause = "localized_type_cannot_hold"
	CausePageRoleInList          Cause = "page_role_in_list"
	CauseAltWithoutImage         Cause = "alt_without_image"
	CausePageRoleInListedBlock   Cause = "page_role_in_listed_block"

	// schema.invalid_alt.
	CauseAltRoleManyImages      Cause = "alt_role_many_images"
	CauseAltRoleDecorativeImage Cause = "alt_role_decorative_image"
	CauseAltRoleAndAltKnob      Cause = "alt_role_and_alt_knob"
	CauseDecorativeNotBoolean   Cause = "decorative_not_boolean"
	CauseDecorativeWithAlt      Cause = "decorative_with_alt"
	CauseAltKeyUnknown          Cause = "alt_key_unknown"
	CauseAltFromCount           Cause = "alt_from_count"
	CauseAltFromTwice           Cause = "alt_from_twice"
	CauseAltSeparatorInvalid    Cause = "alt_separator_invalid"
	CauseAltNamesNothing        Cause = "alt_names_nothing"
	CauseAltNamesItself         Cause = "alt_names_itself"
	CauseAltFieldNotText        Cause = "alt_field_not_text"
	CauseAltFieldMissing        Cause = "alt_field_missing"

	// schema.invalid_when_empty, schema.invalid_excerpt.
	CauseUnknownValue          Cause = "unknown_value"
	CauseNoRole                Cause = "no_role"
	CauseRoleNotFilledInCode   Cause = "role_not_filled_in_code"
	CauseNotRichSEODescription Cause = "not_rich_seo_description"
	CauseMissing               Cause = "missing"

	// schema.invalid_localized.
	CauseLocalizedNotBoolean    Cause = "localized_not_boolean"
	CauseFallbackNotLocalized   Cause = "fallback_not_localized"
	CauseFallbackNotSiteLocale  Cause = "fallback_not_site_locale"
	CauseOneLanguagePerFile     Cause = "one_language_per_file"
	CauseFieldNotSiteLocale     Cause = "field_not_site_locale"
	CauseFieldType              Cause = "field_type"
	CauseMixedTypes             Cause = "mixed_types"
	CauseLanguageFieldRole      Cause = "language_field_role"
	CauseLocaleMissing          Cause = "locale_missing"
	CauseOptionLabelsNotMapping Cause = "option_labels_not_mapping"
	CauseOptionLabelLocale      Cause = "option_label_locale"

	// schema.invalid_title_template, and a titleTemplate that cannot be
	// read (schema.ParseTemplate).
	CauseNotText                    Cause = "not_text"
	CauseRoleNotSEOTitle            Cause = "role_not_seo_title"
	CauseTemplateEmpty              Cause = "template_empty"
	CauseTemplateTooLong            Cause = "template_too_long"
	CauseTemplateLoneClose          Cause = "template_lone_close"
	CauseTemplateUnclosed           Cause = "template_unclosed"
	CauseTemplateFieldPath          Cause = "template_field_path"
	CauseTemplateUnknownPlaceholder Cause = "template_unknown_placeholder"
	CauseTemplateTooManyParts       Cause = "template_too_many_parts"
	CauseTemplateNoValue            Cause = "template_no_value"
	CauseFieldNotInPage             Cause = "field_not_in_page"
	CauseLabelNotSelect             Cause = "label_not_select"
	CauseFieldHasNoValue            Cause = "field_has_no_value"

	// schema.invalid_role_values.
	CauseWhenMissing   Cause = "when_missing"
	CauseWhenNotRead   Cause = "when_not_read"
	CauseWhenCount     Cause = "when_count"
	CauseWhenTwice     Cause = "when_twice"
	CauseWhenNotOption Cause = "when_not_option"

	// schema.invalid_quality.
	CauseNotMapping           Cause = "not_mapping"
	CauseLimitsNotInSite      Cause = "limits_not_in_site"
	CauseSiteKeyUnknown       Cause = "site_key_unknown"
	CauseCollectionKeyUnknown Cause = "collection_key_unknown"
	CauseLimitsNotMapping     Cause = "limits_not_mapping"
	CauseLimitsKeyUnknown     Cause = "limits_key_unknown"
	CauseImageKBNotNumber     Cause = "image_kb_not_number"
	CauseImageKBOutOfRange    Cause = "image_kb_out_of_range"
	CauseIgnoreNotList        Cause = "ignore_not_list"
	CauseIgnoreNotRuleID      Cause = "ignore_not_rule_id"
	CauseIgnoreUnknownRule    Cause = "ignore_unknown_rule"

	// Content.
	CauseWrongType       Cause = "wrong_type"
	CauseOptionGone      Cause = "option_gone"
	CauseOptionsGone     Cause = "options_gone"
	CauseRegex           Cause = "regex"
	CauseCustomValidator Cause = "custom_validator"
	CauseStrippedOnSave  Cause = "stripped_on_save"
	CauseNotInSchema     Cause = "not_in_schema"

	// Quality.
	CauseTooShort       Cause = "too_short"
	CauseTooLong        Cause = "too_long"
	CauseExcerptEmpty   Cause = "excerpt_empty"
	CauseFlag           Cause = "flag"
	CauseValue          Cause = "value"
	CauseNoAltField     Cause = "no_alt_field"
	CauseComposedEmpty  Cause = "composed_empty"
	CauseGuessedEmpty   Cause = "guessed_empty"
	CauseInText         Cause = "in_text"
	CauseGenericWord    Cause = "generic_word"
	CauseCameraFileName Cause = "camera_file_name"
	CauseFileName       Cause = "file_name"
	CauseLabel          Cause = "label"
	CauseDraft          Cause = "draft"
	CauseOverLowerLimit Cause = "over_lower_limit"
	CauseOverUpperLimit Cause = "over_upper_limit"
)
