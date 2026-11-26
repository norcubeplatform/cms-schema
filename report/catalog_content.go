package report

const shapeHint = "The schema's type was likely changed after this file was saved. Convert the value or clear it."

var contentCauses = []CauseSpec{
	spec(RuleContentParseError, "", err, "detail", "",
		func(x in) string { return "not valid YAML: " + x.Detail }, nil),
	spec(RuleContentNoCollection, "", warn, "", "",
		text("the file is under the pages directory but no collection claims it"),
		text("Add a collection whose `path` covers it, or stamp the file with `_collection`.")),
	spec(RuleContentRequired, "", err, "field", "customMessage",
		func(x in) string { return custom(x, x.Field+" is required.") },
		text("Add a value to clear this error.")),
	spec(RuleContentShape, CauseWrongType, err, "field expected actual", "",
		func(x in) string {
			return x.Field + " should be a " + x.Expected.English() + " but is a " + x.Actual.English() + "."
		},
		text(shapeHint)),
	spec(RuleContentShape, CauseOptionGone, err, "field value", "",
		func(x in) string { return x.Field + " has a value that's no longer one of the allowed options." },
		func(x in) string {
			return "Current value: " + truncate(x.Value) + ". Pick a valid option to clear this."
		}),
	spec(RuleContentShape, CauseOptionsGone, err, "field count names", "more",
		func(x in) string {
			return x.Field + " has " + integer(x.Count) + " value(s) no longer in the allowed options."
		},
		func(x in) string {
			stray := make([]string, len(x.Names))
			for i, s := range x.Names {
				stray[i] = truncate(s)
			}
			return "Stray: " + comma(stray) + more(x.Params) + ". Re-pick from the available options."
		}),
	spec(RuleContentItemTypeMissing, "", warn, "index field key", "names more",
		func(x in) string {
			return "Item " + integer(x.Index) + " of " + x.Field + " has no `" + x.Key + ":` discriminator."
		},
		func(x in) string {
			if len(x.Names) == 0 {
				return "Set `" + x.Key + ":` to the key of one of the list's shapes."
			}
			return "The editor falls back to the first shape (" + x.Names[0] + "). Set `" + x.Key + ":` to one of: " + comma(x.Names) + more(x.Params) + "."
		}),
	spec(RuleContentItemTypeUnknown, "", err, "index field key value", "names more",
		func(x in) string {
			return "Item " + integer(x.Index) + " of " + x.Field + " has `" + x.Key + ": " + x.Value + "`, which isn't one of the allowed shapes."
		},
		func(x in) string {
			return "Allowed shapes: " + comma(x.Names) + more(x.Params) + ". Fix the discriminator or add the shape to the schema."
		}),
	spec(RuleContentMinItems, CauseItems, err, "field min current", "customMessage",
		func(x in) string {
			m := deref64(x.Min)
			return custom(x, x.Field+" needs at least "+num(x.Min)+" "+plural(m, "item")+".")
		},
		func(x in) string { return "Add " + FormatNumber(*x.Min-*x.Current) + " more." }),
	spec(RuleContentMinItems, CauseSelected, err, "field min", "",
		func(x in) string { return x.Field + " needs at least " + num(x.Min) + " selected." }, nil),
	spec(RuleContentMaxItems, CauseItems, err, "field max current", "customMessage",
		func(x in) string {
			m := deref64(x.Max)
			return custom(x, x.Field+" accepts at most "+num(x.Max)+" "+plural(m, "item")+".")
		},
		func(x in) string { return "Remove " + FormatNumber(*x.Current-*x.Max) + "." }),
	spec(RuleContentMaxItems, CauseSelected, err, "field max", "",
		func(x in) string { return x.Field + " accepts at most " + num(x.Max) + " selected." }, nil),
	spec(RuleContentRegex, CauseRegex, err, "field", "customMessage",
		func(x in) string { return custom(x, x.Field+" doesn't match the expected format.") }, nil),
	spec(RuleContentRegex, CauseCustomValidator, err, "field knob", "customMessage",
		func(x in) string { return custom(x, x.Field+" doesn't match the expected pattern.") },
		func(x in) string { return x.Knob }),
	spec(RuleContentMinLength, "", err, "field min current", "customMessage",
		func(x in) string {
			return custom(x, x.Field+" must be at least "+num(x.Min)+" characters (currently "+num(x.Current)+").")
		}, nil),
	spec(RuleContentMaxLength, "", err, "field max current", "customMessage",
		func(x in) string {
			return custom(x, x.Field+" must be at most "+num(x.Max)+" characters (currently "+num(x.Current)+").")
		}, nil),
	spec(RuleContentMin, "", err, "field min current", "",
		func(x in) string {
			return x.Field + " must be at least " + num(x.Min) + " (currently " + num(x.Current) + ")."
		}, nil),
	spec(RuleContentMax, "", err, "field max current", "",
		func(x in) string {
			return x.Field + " must be at most " + num(x.Max) + " (currently " + num(x.Current) + ")."
		}, nil),
	spec(RuleContentUnique, "", err, "field index otherIndex key", "customMessage",
		func(x in) string {
			return custom(x, x.Field+": item "+integer(x.Index)+" has the same `"+x.Key+"` as item "+integer(x.OtherIndex)+".")
		},
		func(x in) string { return "Each item under " + x.Field + " must have a unique `" + x.Key + "`." }),
	spec(RuleContentCustom, "", err, "field knob", "customMessage",
		func(x in) string { return custom(x, x.Field+" fails the configured rule.") },
		func(x in) string { return x.Knob }),
	spec(RuleContentExtraKeys, CauseStrippedOnSave, err, "names", "more",
		func(x in) string {
			return "keys " + comma(x.Names) + more(x.Params) + " are not in the schema and the collection's extraKeysPolicy is strip: the next save deletes them"
		}, text("Add the fields to the schema or move the data before an editor saves the file.")),
	spec(RuleContentExtraKeys, CauseNotInSchema, warn, "names", "more",
		func(x in) string { return "keys " + comma(x.Names) + more(x.Params) + " are not in the schema" },
		text("They show in the editor's out-of-schema panel. Add them to the schema or remove them.")),
	spec(RuleContentUnexpectedBody, "", warn, "", "",
		text("the file has a Markdown body but the collection has `hasBody: false`"),
		text("The editor does not show the body; set `hasBody: true` or move the text into a field.")),
	spec(RuleContentInternalError, "", err, "detail", "", internalError, text(internalHint)),
}

func deref64(p *float64) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

var linksCauses = []CauseSpec{
	spec(RuleLinksURLCollision, "", err, "path other", "",
		func(x in) string { return "resolves to " + x.Path + ", the same URL as " + x.Other },
		text("Two files cannot render at one address. Change a slug, move a file, or fix the collection's previewPath.")),
	spec(RuleLinksTranslationCollision, "", err, "key locale other", "",
		func(x in) string {
			return "has the translation key " + q(x.Key) + " in " + q(x.Locale) + ", the same as " + x.Other
		}, text("A page has one version per language. Give one of the two files its own key, or move it to the language it is written in.")),
	spec(RuleLinksUnresolvedLink, "", warn, "path", "",
		func(x in) string { return "links to " + x.Path + ", which no content file produces" },
		text("If the site serves this route from code, add it to knownPaths in config.yml; otherwise fix the link.")),
	spec(RuleLinksMissingAsset, "", err, "path other", "",
		func(x in) string {
			return "points at " + x.Path + ", which the repository does not have (looked for " + x.Other + ")"
		}, text("Pick the file again in the media library, or add the file.")),
	// knob is the reference field's `by` (field:<key>) when it names
	// entries by a field; key the collection it names them in.
	spec(RuleLinksUnresolvedReference, "", err, "value", "knob key",
		func(x in) string {
			s := "references " + q(x.Value) + ", which names no entry"
			switch {
			case x.Knob != "":
				s += " by " + x.Knob
			case x.Key != "":
				s += " in collection " + x.Key
			}
			return s
		}, text("Pick the entry again, or fix the value.")),
	spec(RuleLinksOrphanSidecar, "", warn, "", "",
		text("the sidecar belongs to a content file that no longer exists"),
		text("Delete it, or restore the content file; the CMS recovers a renamed file's sidecar through the page's inline _id.")),
	spec(RuleLinksInternalError, "", err, "detail", "", internalError, text(internalHint)),
}
