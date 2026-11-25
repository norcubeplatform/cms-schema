package validate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Roles checks the field roles and image alt relations of a parsed
// configuration on their own: the same findings Schema reports under
// schema.unknown_role, duplicate_role, role_type_mismatch,
// misplaced_role, invalid_alt and invalid_when_empty, with the files of
// the directory layout. For callers that hold a config but no site, such as a schema
// proposed before it is written.
func Roles(cfg *schema.Config) (out []report.Finding) {
	defer recoverAs(&out, load.ConfigFile, report.RuleSchemaInternalError)
	if cfg == nil {
		return nil
	}
	f := func(file, path string, rule report.Rule, cause report.Cause, p *report.Params) {
		out = append(out, report.New(file, path, rule, cause, p))
	}
	for _, key := range cfg.CollectionKeys {
		checkCollectionRoles(cfg, load.CollectionFile(key), cfg.Collections[key], f)
	}
	for _, key := range cfg.BlockKeys {
		checkRoles(cfg, load.BlockFile(key), "fields", cfg.Blocks[key].Fields, f)
	}
	for _, key := range cfg.InputKeys {
		checkRoles(cfg, load.InputFile(key), "", []schema.Field{cfg.Inputs[key]}, f)
	}
	return out
}

// roleCheck walks one collection, block or input. A page role is
// counted across the whole of it; `role: alt` and `alt:` are judged per
// fields list.
type roleCheck struct {
	cfg  *schema.Config
	file string
	f    emit
	// seen maps a page role to the path of the first field declaring it.
	seen map[string]string
	// coll is the collection checked, nil for a block or an input: the
	// fields a titleTemplate names and the place of a localized group are
	// judged where a page is made of them.
	coll *schema.Collection
}

func checkRoles(cfg *schema.Config, file, base string, fields []schema.Field, f emit) {
	rc := &roleCheck{cfg: cfg, file: file, f: f, seen: map[string]string{}}
	rc.list(base, fields, false, 0)
}

func checkCollectionRoles(cfg *schema.Config, file string, coll schema.Collection, f emit) {
	rc := &roleCheck{cfg: cfg, file: file, f: f, seen: map[string]string{}, coll: &coll}
	rc.list("fields", coll.Fields, false, 0)
}

// list checks one fields list. inList is true below an array, where a
// value repeats per item.
func (rc *roleCheck) list(base string, fields []schema.Field, inList bool, depth int) {
	if depth > schema.MaxWalkDepth {
		return
	}
	var images []schema.Field
	for _, fd := range fields {
		if schema.Canonical(fd.Type) == schema.TypeImage && fd.Key != "" {
			images = append(images, fd)
		}
	}
	altRole := ""
	for i, fd := range fields {
		p := base + "[" + strconvI(i) + "]"
		if fd.Key != "" {
			p = joinPath(base, fd.Key)
		}
		rc.field(p, fd, fields, images, inList, &altRole, depth)
	}
}

// field checks one field at path p, given the list it sits in.
func (rc *roleCheck) field(p string, fd schema.Field, siblings, images []schema.Field, inList bool, altRole *string, depth int) {
	if _, unresolved := fd.Raw[schema.RefKey]; unresolved {
		return
	}
	label := fd.Label
	if label == "" {
		label = fd.Key
	}
	if raw, has := fd.Raw["role"]; has {
		rc.role(p, label, fd, raw, images, inList, altRole)
	}
	if raw, has := fd.Raw["whenEmpty"]; has {
		rc.whenEmpty(p, label, fd, raw)
	}
	rc.roleKnobs(p, label, fd)
	if schema.IsGroup(fd.Type) {
		rc.localized(p, label, fd)
	}
	if t := schema.Canonical(fd.Type); t == schema.TypeSelect || t == schema.TypeMultiChoice {
		rc.optionLabels(p, label, fd)
	}
	if schema.Canonical(fd.Type) == schema.TypeImage {
		rc.alt(p, label, fd, siblings)
	}
	rc.nested(p, fd, inList, depth)
}

func (rc *roleCheck) role(p, label string, fd schema.Field, raw any, images []schema.Field, inList bool, altRole *string) {
	role, ok := raw.(string)
	role = strings.TrimSpace(role)
	switch {
	case !ok || !schema.KnownRole(role):
		rc.f(rc.file, p, report.RuleSchemaUnknownRole, "", &report.Params{Field: label, Value: report.FlowValue(raw)})
		return
	case role == schema.RoleNone:
		// No role: nothing to count or place.
		return
	case schema.IsGroup(fd.Type) && !fd.IsLocalized():
		rc.f(rc.file, p, report.RuleSchemaRoleTypeMismatch, report.CauseGroupNotLocalized, &report.Params{Field: label, Role: role})
		return
	case !schema.RoleAccepts(role, fd.ValueType()):
		if fd.IsLocalized() {
			rc.f(rc.file, p, report.RuleSchemaRoleTypeMismatch, report.CauseLocalizedTypeCannotHold, &report.Params{Field: label, Role: role, Type: fd.ValueType(), Allowed: schema.RoleTypes(role)})
		} else {
			rc.f(rc.file, p, report.RuleSchemaRoleTypeMismatch, report.CauseTypeCannotHold, &report.Params{Field: label, Role: role, Type: fd.Type, Allowed: schema.RoleTypes(role)})
		}
		return
	}
	if schema.IsPageRole(role) {
		if inList {
			rc.f(rc.file, p, report.RuleSchemaMisplacedRole, report.CausePageRoleInList, &report.Params{Field: label, Role: role})
			return
		}
		if prev, dup := rc.seen[role]; dup {
			rc.f(rc.file, p, report.RuleSchemaDuplicateRole, report.CausePageRole, &report.Params{Field: label, Role: role, Other: prev})
			return
		}
		rc.seen[role] = p
		return
	}
	// role: alt describes the one image of its fields list.
	switch {
	case *altRole != "":
		rc.f(rc.file, p, report.RuleSchemaDuplicateRole, report.CauseAltRole, &report.Params{Field: label, Other: *altRole})
		return
	case len(images) == 0:
		rc.f(rc.file, p, report.RuleSchemaMisplacedRole, report.CauseAltWithoutImage, &report.Params{Field: label})
	case len(images) > 1:
		rc.f(rc.file, p, report.RuleSchemaInvalidAlt, report.CauseAltRoleManyImages, &report.Params{Field: label, Count: report.Int(int64(len(images))), Key: fd.Key})
	case images[0].Decorative:
		rc.f(rc.file, p, report.RuleSchemaInvalidAlt, report.CauseAltRoleDecorativeImage, &report.Params{Field: label, Key: images[0].Key})
	case images[0].Alt != "" && images[0].Alt != fd.Key:
		rc.f(rc.file, p, report.RuleSchemaInvalidAlt, report.CauseAltRoleAndAltKnob, &report.Params{Field: label, Key: images[0].Key, Other: images[0].Alt})
	}
	*altRole = p
}

// whenEmpty checks a field's `whenEmpty`: a known value, on a field
// whose role a site can fill in code. An unknown role is reported as
// such and not again here.
func (rc *roleCheck) whenEmpty(p, label string, fd schema.Field, raw any) {
	v, ok := raw.(string)
	v = strings.TrimSpace(v)
	rawRole, hasRole := fd.Raw["role"]
	role, _ := rawRole.(string)
	role = strings.TrimSpace(role)
	allowed := schema.WhenEmptyRoles()
	switch {
	case !ok || !schema.KnownWhenEmpty(v):
		rc.f(rc.file, p, report.RuleSchemaInvalidWhenEmpty, report.CauseUnknownValue, &report.Params{Field: label, Value: report.FlowValue(raw)})
	case !hasRole:
		rc.f(rc.file, p, report.RuleSchemaInvalidWhenEmpty, report.CauseNoRole, &report.Params{Field: label, Allowed: allowed})
	case !schema.KnownRole(role):
		// schema.unknown_role says it.
	case !schema.RoleAllowsWhenEmpty(role):
		rc.f(rc.file, p, report.RuleSchemaInvalidWhenEmpty, report.CauseRoleNotFilledInCode, &report.Params{Field: label, Role: role, Allowed: allowed})
	}
}

func (rc *roleCheck) alt(p, label string, fd schema.Field, siblings []schema.Field) {
	if raw, has := fd.Raw["decorative"]; has {
		if _, ok := raw.(bool); !ok {
			rc.f(rc.file, p, report.RuleSchemaInvalidAlt, report.CauseDecorativeNotBoolean, &report.Params{Field: label, Value: report.FlowValue(raw)})
		}
	}
	raw, has := fd.Raw[schema.KnobAlt]
	if !has {
		return
	}
	bad := func(c report.Cause, params report.Params) {
		params.Field = label
		rc.f(rc.file, p, report.RuleSchemaInvalidAlt, c, &params)
	}
	if fd.Decorative {
		bad(report.CauseDecorativeWithAlt, report.Params{})
		return
	}
	var names []string
	switch v := raw.(type) {
	case string:
		names = []string{strings.TrimSpace(v)}
	case map[string]any:
		for _, k := range sortedMapKeys(v) {
			if k != schema.AltKeyFrom && k != schema.AltKeySeparator {
				bad(report.CauseAltKeyUnknown, report.Params{Key: k})
			}
		}
		list, ok := v[schema.AltKeyFrom].([]any)
		if !ok || len(list) == 0 || len(list) > schema.MaxAltParts {
			bad(report.CauseAltFromCount, report.Params{Value: report.FlowValue(v[schema.AltKeyFrom]), Max: report.Num(schema.MaxAltParts)})
			return
		}
		for _, item := range list {
			s, _ := item.(string)
			if slices.Contains(names, strings.TrimSpace(s)) {
				bad(report.CauseAltFromTwice, report.Params{Key: s})
				return
			}
			names = append(names, strings.TrimSpace(s))
		}
		if sep, has := v[schema.AltKeySeparator]; has {
			if s, ok := sep.(string); !ok || len(s) > schema.MaxSeparatorBytes {
				bad(report.CauseAltSeparatorInvalid, report.Params{Value: report.FlowValue(sep), Max: report.Num(schema.MaxSeparatorBytes)})
			}
		}
	default:
		bad(report.CauseAltNamesNothing, report.Params{Value: report.FlowValue(raw)})
		return
	}
	for _, name := range names {
		switch {
		case name == "":
			bad(report.CauseAltNamesNothing, report.Params{Value: report.FlowValue(raw)})
			continue
		case name == fd.Key:
			bad(report.CauseAltNamesItself, report.Params{})
			continue
		}
		found := false
		for _, s := range siblings {
			if s.Key != name {
				continue
			}
			found = true
			if !altText(s) {
				bad(report.CauseAltFieldNotText, report.Params{Key: name, Type: s.Type})
			}
			break
		}
		if !found {
			bad(report.CauseAltFieldMissing, report.Params{Key: name})
		}
	}
}

// altText reports whether a field can hold alt text: text, paragraph, or
// a localized group of them.
func altText(f schema.Field) bool {
	t := f.ValueType()
	return (t == schema.TypeText || t == schema.TypeParagraph) && (!schema.IsGroup(f.Type) || f.IsLocalized())
}

// roleKnobs checks the knobs that go with a role: titleTemplate on a
// seoTitle, excerpt on a rich text seoDescription, noindexWhen and
// draftWhen on a select with role noindex or draft.
func (rc *roleCheck) roleKnobs(p, label string, fd schema.Field) {
	role, _ := fd.Raw["role"].(string)
	role = strings.TrimSpace(role)
	if !schema.KnownRole(role) {
		role = ""
	}
	vt := fd.ValueType()
	bad := func(rule report.Rule, c report.Cause, params report.Params) {
		params.Field = label
		rc.f(rc.file, p, rule, c, &params)
	}

	if raw, has := fd.Raw[schema.KnobTitleTemplate]; has {
		s, ok := raw.(string)
		switch {
		case !ok:
			bad(report.RuleSchemaInvalidTitleTemplate, report.CauseNotText, report.Params{Value: report.FlowValue(raw)})
		case role != schema.RoleSEOTitle:
			bad(report.RuleSchemaInvalidTitleTemplate, report.CauseRoleNotSEOTitle, report.Params{})
		default:
			parts, err := schema.ParseTemplate(s)
			var te *schema.TemplateError
			if errors.As(err, &te) {
				params := report.Params{}
				if te.Params != nil {
					params = *te.Params
				}
				bad(report.RuleSchemaInvalidTitleTemplate, te.Cause, params)
				break
			}
			rc.templateFields(p, label, parts)
		}
	}

	if raw, has := fd.Raw[schema.KnobExcerpt]; has {
		s, _ := raw.(string)
		switch {
		case !slices.Contains(schema.ExcerptValues(), strings.TrimSpace(s)):
			bad(report.RuleSchemaInvalidExcerpt, report.CauseUnknownValue, report.Params{Value: report.FlowValue(raw)})
		case role != schema.RoleSEODescription || vt != schema.TypeRichText:
			bad(report.RuleSchemaInvalidExcerpt, report.CauseNotRichSEODescription, report.Params{})
		}
	} else if role == schema.RoleSEODescription && vt == schema.TypeRichText {
		bad(report.RuleSchemaInvalidExcerpt, report.CauseMissing, report.Params{})
	}

	for _, r := range []string{schema.RoleNoindex, schema.RoleDraft} {
		knob := schema.RoleWhen(r)
		raw, has := fd.Raw[knob]
		if !has {
			if role == r && vt == schema.TypeSelect {
				bad(report.RuleSchemaInvalidRoleValues, report.CauseWhenMissing, report.Params{Role: r, Knob: knob, Allowed: firstOptions(fd)})
			}
			continue
		}
		if role != r || vt != schema.TypeSelect {
			bad(report.RuleSchemaInvalidRoleValues, report.CauseWhenNotRead, report.Params{Knob: knob, Role: r})
			continue
		}
		list, ok := raw.([]any)
		if !ok || len(list) == 0 || len(list) > schema.MaxRoleValues {
			bad(report.RuleSchemaInvalidRoleValues, report.CauseWhenCount, report.Params{Knob: knob, Value: report.FlowValue(raw), Max: report.Num(schema.MaxRoleValues)})
			continue
		}
		values := schema.OptionValues(fd.Options)
		var seen []string
		for _, item := range list {
			v := fmt.Sprint(item)
			switch {
			case slices.Contains(seen, v):
				bad(report.RuleSchemaInvalidRoleValues, report.CauseWhenTwice, report.Params{Value: v, Knob: knob})
			case !slices.Contains(values, v):
				bad(report.RuleSchemaInvalidRoleValues, report.CauseWhenNotOption, report.Params{Value: v, Knob: knob, Allowed: values})
			}
			seen = append(seen, v)
		}
	}
}

func firstOptions(fd schema.Field) []string {
	values := schema.OptionValues(fd.Options)
	if len(values) > 2 {
		values = values[:2]
	}
	return values
}

// templateFields checks the fields a titleTemplate names: each a field of
// the page outside lists, a select or multichoice where it asks for the
// option label. Judged in a collection only; a block's fields are named
// from the page that uses it.
func (rc *roleCheck) templateFields(p, label string, parts []schema.TemplatePart) {
	if rc.coll == nil {
		return
	}
	for _, part := range parts {
		if part.Kind != schema.TemplateField {
			continue
		}
		f, ok := schema.FieldAtPath(rc.coll.Fields, part.Path)
		switch {
		case !ok:
			rc.f(rc.file, p, report.RuleSchemaInvalidTitleTemplate, report.CauseFieldNotInPage, &report.Params{Field: label, Path: part.Path})
		case part.Label && f.ValueType() != schema.TypeSelect && f.ValueType() != schema.TypeMultiChoice:
			rc.f(rc.file, p, report.RuleSchemaInvalidTitleTemplate, report.CauseLabelNotSelect, &report.Params{Field: label, Path: part.Path, Type: f.Type})
		case schema.IsGroup(f.Type) && !f.IsLocalized(), schema.IsArray(f.Type):
			rc.f(rc.file, p, report.RuleSchemaInvalidTitleTemplate, report.CauseFieldHasNoValue, &report.Params{Field: label, Path: part.Path, Type: f.Type})
		}
	}
}

// localized checks a group's `localized` and `fallback`: a localized
// group has one field per site language and nothing else, of one text
// type, with no roles of their own; it sits only where a file holds
// every language.
func (rc *roleCheck) localized(p, label string, fd schema.Field) {
	bad := func(c report.Cause, params report.Params) {
		params.Field = label
		rc.f(rc.file, p, report.RuleSchemaInvalidLocalized, c, &params)
	}
	if raw, has := fd.Raw[schema.KnobLocalized]; has {
		if _, ok := raw.(bool); !ok {
			bad(report.CauseLocalizedNotBoolean, report.Params{Value: report.FlowValue(raw)})
			return
		}
	}
	locales := rc.cfg.Preview.Locales
	if raw, has := fd.Raw[schema.KnobFallback]; has {
		s, _ := raw.(string)
		switch {
		case !fd.IsLocalized():
			bad(report.CauseFallbackNotLocalized, report.Params{})
		case !slices.Contains(locales, strings.TrimSpace(s)):
			bad(report.CauseFallbackNotSiteLocale, report.Params{Value: report.FlowValue(raw), Locales: locales})
		}
	}
	if !fd.IsLocalized() {
		return
	}
	if rc.coll != nil && rc.coll.OneLanguagePerFile() {
		bad(report.CauseOneLanguagePerFile, report.Params{})
	}
	if len(locales) == 0 {
		bad(report.CauseNoSiteLocales, report.Params{})
		return
	}
	t, typeSaid := "", false
	for _, c := range fd.Fields {
		ct := schema.Canonical(c.Type)
		switch {
		case !slices.Contains(locales, c.Key):
			bad(report.CauseFieldNotSiteLocale, report.Params{Key: c.Key, Locales: locales})
		case typeSaid:
		case !slices.Contains(schema.LocalizedTypes(), ct):
			bad(report.CauseFieldType, report.Params{Key: c.Key, Type: c.Type})
			typeSaid = true
		case t != "" && ct != t:
			bad(report.CauseMixedTypes, report.Params{Expected: t, Actual: ct})
			typeSaid = true
		}
		if t == "" {
			t = ct
		}
		if r, has := c.Raw["role"]; has && r != schema.RoleNone {
			bad(report.CauseLanguageFieldRole, report.Params{Key: c.Key})
		}
	}
	for _, l := range locales {
		if _, ok := fd.LanguageField(l); !ok {
			bad(report.CauseLocaleMissing, report.Params{Locale: l})
		}
	}
}

// optionLabels checks the `labels` of a select's options: a label per
// language of the site.
func (rc *roleCheck) optionLabels(p, label string, fd schema.Field) {
	list, _ := fd.Raw["options"].([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		raw, has := m[schema.OptionKeyLabels]
		if !has {
			continue
		}
		labels, ok := raw.(map[string]any)
		if !ok {
			rc.f(rc.file, p, report.RuleSchemaInvalidLocalized, report.CauseOptionLabelsNotMapping, &report.Params{Key: report.FlowValue(m["value"]), Field: label, Value: report.FlowValue(raw)})
			continue
		}
		for _, l := range sortedMapKeys(labels) {
			if _, ok := labels[l].(string); !ok || !slices.Contains(rc.cfg.Preview.Locales, l) {
				rc.f(rc.file, p, report.RuleSchemaInvalidLocalized, report.CauseOptionLabelLocale, &report.Params{Key: report.FlowValue(m["value"]), Field: label, Locale: l})
			}
		}
	}
}

// nested enters groups and the item shapes of a list. The blocks of a
// legacy `blocks` list are checked in their own files; here only a page
// role inside one is reported, since it repeats per item.
func (rc *roleCheck) nested(p string, fd schema.Field, inList bool, depth int) {
	switch {
	case schema.IsGroup(fd.Type):
		rc.list(p+".fields", fd.Fields, inList, depth+1)
	case fd.Type == schema.TypeBlocks || len(fd.Components) > 0:
		for _, name := range fd.Components {
			if b, ok := rc.cfg.Blocks[name]; ok {
				rc.pageRolesInList(p, name, b.Fields, depth+1)
			}
		}
	case schema.IsArray(fd.Type):
		for _, e := range schema.ItemEntries(fd) {
			itemPath := p + ".items[" + e.Name + "]"
			if schema.IsGroup(e.Field.Type) {
				rc.list(itemPath+".fields", e.Field.Fields, true, depth+1)
			} else if depth < schema.MaxWalkDepth {
				// A primitive item has no siblings.
				alone := []schema.Field{e.Field}
				var images []schema.Field
				if schema.Canonical(e.Field.Type) == schema.TypeImage {
					images = alone
				}
				altRole := ""
				rc.field(itemPath, e.Field, alone, images, true, &altRole, depth+1)
			}
		}
	}
}

// pageRolesInList reports page roles inside a block used as an item of
// a legacy `blocks` list, at the list's path.
func (rc *roleCheck) pageRolesInList(p, block string, fields []schema.Field, depth int) {
	if depth > schema.MaxWalkDepth {
		return
	}
	for _, fd := range fields {
		if schema.IsPageRole(fd.Role) && fd.HoldsRole(fd.Role) {
			rc.f(rc.file, p, report.RuleSchemaMisplacedRole, report.CausePageRoleInListedBlock, &report.Params{Key: block, Role: fd.Role})
		}
		if schema.IsGroup(fd.Type) {
			rc.pageRolesInList(p, block, fd.Fields, depth+1)
		}
	}
}

// translationLocalesProblems says what is wrong with a collection's
// `translations.locales`: the languages its pages are expected in. Each
// must be a language the site declares.
func translationLocalesProblems(cfg *schema.Config, coll schema.Collection) []report.Problem {
	block, _ := coll.Raw["translations"].(map[string]any)
	raw, has := block["locales"]
	if !has {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return []report.Problem{{Cause: report.CauseNotList, Params: &report.Params{Value: report.FlowValue(raw)}}}
	}
	if len(list) == 0 {
		return []report.Problem{{Cause: report.CauseEmpty}}
	}
	var out []report.Problem
	add := func(c report.Cause, p *report.Params) { out = append(out, report.Problem{Cause: c, Params: p}) }
	seen := map[string]bool{}
	for _, item := range list {
		l, ok := item.(string)
		l = strings.TrimSpace(l)
		switch {
		case !ok || l == "":
			add(report.CauseNotLanguageCode, &report.Params{Value: report.FlowValue(item)})
		case seen[l]:
			add(report.CauseListedTwice, &report.Params{Locale: l})
		case len(cfg.Preview.Locales) == 0:
			add(report.CauseNoSiteLocales, &report.Params{Locale: l})
		case !contains(cfg.Preview.Locales, l):
			add(report.CauseNotSiteLocale, &report.Params{Locale: l, Locales: cfg.Preview.Locales})
		}
		seen[l] = true
	}
	return out
}
