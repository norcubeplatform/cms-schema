package validate

import (
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
	f := func(file, path string, sev report.Severity, rule report.Rule, msg, hint string) {
		out = append(out, report.Finding{File: file, Path: path, Severity: sev, Rule: rule, Message: msg, Hint: hint})
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

const rolesHint = "Roles: seoTitle, seoDescription, shareImage, noindex, canonical, draft, alt, or none (no role, never guessed)."

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
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaUnknownRole, fmt.Sprintf("field %q has role %v, which is not a role", label, raw), rolesHint)
		return
	case role == schema.RoleNone:
		// No role: nothing to count or place.
		return
	case schema.IsGroup(fd.Type) && !fd.IsLocalized():
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaRoleTypeMismatch, fmt.Sprintf("group %q has role %s; a group holds a role only when it is localized", label, role), "When the group's fields are the site's languages, mark it `localized: true` and keep the role on the group; else put the role on the field inside it.")
		return
	case !schema.RoleAccepts(role, fd.ValueType()):
		what := fd.Type + " field"
		if fd.IsLocalized() {
			what = "localized group of " + orMixed(fd.ValueType()) + " fields"
		}
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaRoleTypeMismatch, fmt.Sprintf("field %q has role %s, which a %s cannot hold", label, role, what), fmt.Sprintf("%s goes on a field of type %s.", role, strings.Join(schema.RoleTypes(role), " or ")))
		return
	}
	if schema.IsPageRole(role) {
		if inList {
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaMisplacedRole, fmt.Sprintf("field %q has role %s inside a list, where it repeats per item; a page has one %s", label, role, role), "Declare it on a field at the top level of the collection or inside a group, not inside a list.")
			return
		}
		if prev, dup := rc.seen[role]; dup {
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaDuplicateRole, fmt.Sprintf("field %q has role %s, which %s already declares", label, role, prev), "A page has one field per role; keep the role on one of them.")
			return
		}
		rc.seen[role] = p
		return
	}
	// role: alt describes the one image of its fields list.
	switch {
	case *altRole != "":
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaDuplicateRole, fmt.Sprintf("field %q has role alt, which %s already declares in the same list", label, *altRole), "One field holds an image's alt text.")
		return
	case len(images) == 0:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaMisplacedRole, fmt.Sprintf("field %q has role alt, but its fields list holds no image", label), "role: alt marks the alt text of the image next to it, in the same group or list item.")
	case len(images) > 1:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("field %q has role alt, but its fields list holds %d images", label, len(images)), "Say which image it describes with `alt: "+fd.Key+"` on that image instead.")
	case images[0].Decorative:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("field %q has role alt, but image %q is decorative", label, images[0].Key), "A decorative image has no alt text: remove the role, or `decorative: true` from the image.")
	case images[0].Alt != "" && images[0].Alt != fd.Key:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("field %q has role alt, but image %q names %q as its alt text", label, images[0].Key, images[0].Alt), "Keep one of the two declarations.")
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
	allowed := strings.Join(schema.WhenEmptyRoles(), ", ")
	switch {
	case !ok || !schema.KnownWhenEmpty(v):
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidWhenEmpty, fmt.Sprintf("field %q has whenEmpty %v, which is not a value it takes", label, raw), whenEmptyHint)
	case !hasRole:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidWhenEmpty, fmt.Sprintf("field %q has whenEmpty but no role", label), "whenEmpty says what the site does when a role field is empty; give the field its role ("+allowed+") or remove whenEmpty.")
	case !schema.KnownRole(role):
		// schema.unknown_role says it.
	case !schema.RoleAllowsWhenEmpty(role):
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidWhenEmpty, fmt.Sprintf("field %q has whenEmpty on role %s, whose empty value a site does not fill in code", label, role), "whenEmpty goes on a field with role "+allowed+".")
	}
}

const whenEmptyHint = "whenEmpty is generated (the site makes the value per page in code, such as a share card rendered at build time) or siteDefault (the site falls back to one site-wide value)."

func (rc *roleCheck) alt(p, label string, fd schema.Field, siblings []schema.Field) {
	if raw, has := fd.Raw["decorative"]; has {
		if _, ok := raw.(bool); !ok {
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q has decorative %v; it is true or false", label, raw), "")
		}
	}
	raw, has := fd.Raw[schema.KnobAlt]
	if !has {
		return
	}
	bad := func(msg, hint string) {
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, msg, hint)
	}
	if fd.Decorative {
		bad(fmt.Sprintf("image %q is decorative and also says where its alt text is", label), "A decorative image needs no alt text: keep one of the two.")
		return
	}
	var names []string
	switch v := raw.(type) {
	case string:
		names = []string{strings.TrimSpace(v)}
	case map[string]any:
		for _, k := range sortedMapKeys(v) {
			if k != schema.AltKeyFrom && k != schema.AltKeySeparator {
				bad(fmt.Sprintf("image %q has alt.%s, which a composed alt text does not read", label, k), altHint)
			}
		}
		list, ok := v[schema.AltKeyFrom].([]any)
		if !ok || len(list) == 0 || len(list) > schema.MaxAltParts {
			bad(fmt.Sprintf("image %q composes its alt text from %v; alt.from lists 1 to %d fields", label, v[schema.AltKeyFrom], schema.MaxAltParts), altHint)
			return
		}
		for _, item := range list {
			s, _ := item.(string)
			if slices.Contains(names, strings.TrimSpace(s)) {
				bad(fmt.Sprintf("image %q lists %q twice in alt.from", label, s), altHint)
				return
			}
			names = append(names, strings.TrimSpace(s))
		}
		if sep, has := v[schema.AltKeySeparator]; has {
			if s, ok := sep.(string); !ok || len(s) > schema.MaxSeparatorBytes {
				bad(fmt.Sprintf("image %q has alt.separator %v; it is text of at most %d bytes", label, sep, schema.MaxSeparatorBytes), altHint)
			}
		}
	default:
		bad(fmt.Sprintf("image %q has alt %v, which names no field", label, raw), altHint)
		return
	}
	for _, name := range names {
		switch {
		case name == "":
			bad(fmt.Sprintf("image %q has alt %v, which names no field", label, raw), altHint)
			continue
		case name == fd.Key:
			bad(fmt.Sprintf("image %q names itself as its alt text", label), altHint)
			continue
		}
		found := false
		for _, s := range siblings {
			if s.Key != name {
				continue
			}
			found = true
			if !altText(s) {
				bad(fmt.Sprintf("image %q takes its alt text from %q, which is a %s field", label, name, s.Type), altHint)
			}
			break
		}
		if !found {
			bad(fmt.Sprintf("image %q takes its alt text from %q, but no field in the same list has that key", label, name), altHint)
		}
	}
}

const altHint = "`alt:` names the field next to the image, in the same fields list, that holds its alt text (a text or paragraph field, or a localized group of them), or composes it: `alt: { from: [title, technique], separator: \" – \" }`."

// altText reports whether a field can hold alt text: text, paragraph, or
// a localized group of them.
func altText(f schema.Field) bool {
	t := f.ValueType()
	return (t == schema.TypeText || t == schema.TypeParagraph) && (!schema.IsGroup(f.Type) || f.IsLocalized())
}

func orMixed(t string) string {
	if t == "" {
		return "mixed"
	}
	return t
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
	bad := func(rule report.Rule, msg, hint string) {
		rc.f(rc.file, p, report.SeverityError, rule, msg, hint)
	}

	if raw, has := fd.Raw[schema.KnobTitleTemplate]; has {
		s, ok := raw.(string)
		switch {
		case !ok:
			bad(report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q has titleTemplate %v, which is not text", label, raw), templateHint)
		case role != schema.RoleSEOTitle:
			bad(report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q has a titleTemplate but not role seoTitle", label), "titleTemplate says how the site composes the page title from its seoTitle field; give the field `role: seoTitle` or remove it.")
		default:
			parts, err := schema.ParseTemplate(s)
			if err != nil {
				bad(report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q: %v", label, err), templateHint)
				break
			}
			rc.templateFields(p, label, parts)
		}
	}

	if raw, has := fd.Raw[schema.KnobExcerpt]; has {
		s, _ := raw.(string)
		switch {
		case !slices.Contains(schema.ExcerptValues(), strings.TrimSpace(s)):
			bad(report.RuleSchemaInvalidExcerpt, fmt.Sprintf("field %q has excerpt %v, which is not a value it takes", label, raw), excerptHint)
		case role != schema.RoleSEODescription || vt != schema.TypeRichText:
			bad(report.RuleSchemaInvalidExcerpt, fmt.Sprintf("field %q has excerpt, which only a rich text field with role seoDescription reads", label), excerptHint)
		}
	} else if role == schema.RoleSEODescription && vt == schema.TypeRichText {
		bad(report.RuleSchemaInvalidExcerpt, fmt.Sprintf("field %q is rich text with role seoDescription, but does not say which part of it is the description", label), excerptHint)
	}

	for _, r := range []string{schema.RoleNoindex, schema.RoleDraft} {
		knob := schema.RoleWhen(r)
		raw, has := fd.Raw[knob]
		if !has {
			if role == r && vt == schema.TypeSelect {
				bad(report.RuleSchemaInvalidRoleValues, fmt.Sprintf("select %q has role %s but no %s: which of its values %s?", label, r, knob, whenVerb(r)), fmt.Sprintf("List them: `%s: [%s]`.", knob, strings.Join(firstOptions(fd), ", ")))
			}
			continue
		}
		if role != r || vt != schema.TypeSelect {
			bad(report.RuleSchemaInvalidRoleValues, fmt.Sprintf("field %q has %s, which only a select with role %s reads", label, knob, r), fmt.Sprintf("A boolean with role %s needs no list: true %s.", r, whenVerb(r)))
			continue
		}
		list, ok := raw.([]any)
		if !ok || len(list) == 0 || len(list) > schema.MaxRoleValues {
			bad(report.RuleSchemaInvalidRoleValues, fmt.Sprintf("select %q has %s %v; it lists 1 to %d of its values", label, knob, raw, schema.MaxRoleValues), "")
			continue
		}
		values := schema.OptionValues(fd.Options)
		var seen []string
		for _, item := range list {
			v := fmt.Sprint(item)
			switch {
			case slices.Contains(seen, v):
				bad(report.RuleSchemaInvalidRoleValues, fmt.Sprintf("select %q lists %q twice in %s", label, v, knob), "")
			case !slices.Contains(values, v):
				bad(report.RuleSchemaInvalidRoleValues, fmt.Sprintf("select %q lists %q in %s, which is not one of its options", label, v, knob), "Options: "+strings.Join(values, ", ")+".")
			}
			seen = append(seen, v)
		}
	}
}

const templateHint = "titleTemplate is the page title as the site composes it: `{value}` (this field) with literal text, `{field:<key>}` (another field of the page), `{field:<key>|label}` (a select's option label) and `{{variable}}` (a site variable), such as `{value} – {{site_name}}`."

const excerptHint = "A rich text field with role seoDescription says which part is the meta description: `excerpt: firstParagraph` (the plain text of its first paragraph)."

func whenVerb(role string) string {
	if role == schema.RoleDraft {
		return "make the page a draft"
	}
	return "hide the page from search engines"
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
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q has a titleTemplate that names {field:%s}, which is not a field of the page outside lists", label, part.Path), templateHint)
		case part.Label && f.ValueType() != schema.TypeSelect && f.ValueType() != schema.TypeMultiChoice:
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q has a titleTemplate that asks for the label of %q, which is a %s field, not a select", label, part.Path, f.Type), "`|label` reads a select's option label; drop it to use the value.")
		case schema.IsGroup(f.Type) && !f.IsLocalized(), schema.IsArray(f.Type):
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidTitleTemplate, fmt.Sprintf("field %q has a titleTemplate that names %q, a %s, which has no one value", label, part.Path, f.Type), templateHint)
		}
	}
}

// localized checks a group's `localized` and `fallback`: a localized
// group has one field per site language and nothing else, of one text
// type, with no roles of their own; it sits only where a file holds
// every language.
func (rc *roleCheck) localized(p, label string, fd schema.Field) {
	bad := func(msg, hint string) {
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidLocalized, msg, hint)
	}
	if raw, has := fd.Raw[schema.KnobLocalized]; has {
		if _, ok := raw.(bool); !ok {
			bad(fmt.Sprintf("group %q has localized %v; it is true or false", label, raw), localizedHint)
			return
		}
	}
	locales := rc.cfg.Preview.Locales
	if raw, has := fd.Raw[schema.KnobFallback]; has {
		s, _ := raw.(string)
		switch {
		case !fd.IsLocalized():
			bad(fmt.Sprintf("group %q has a fallback but is not localized", label), "fallback names the language an empty language of a localized group shows; add `localized: true` or remove it.")
		case !slices.Contains(locales, strings.TrimSpace(s)):
			bad(fmt.Sprintf("group %q falls back to %v, which is not in preview.locales %v", label, raw, locales), localizedHint)
		}
	}
	if !fd.IsLocalized() {
		return
	}
	if rc.coll != nil && rc.coll.OneLanguagePerFile() {
		bad(fmt.Sprintf("group %q is localized, but each file of this collection is one language", label), "A localized group holds every language in one file. In a collection whose files are one language each (localeFrom path or field, translations.by), use a plain field.")
	}
	if len(locales) == 0 {
		bad(fmt.Sprintf("group %q is localized, but the site declares no languages", label), "List the site's languages in preview.locales.")
		return
	}
	t, typeSaid := "", false
	for _, c := range fd.Fields {
		ct := schema.Canonical(c.Type)
		switch {
		case !slices.Contains(locales, c.Key):
			bad(fmt.Sprintf("group %q is localized, but its field %q is not a language of the site %v", label, c.Key, locales), localizedHint)
		case typeSaid:
		case !slices.Contains(schema.LocalizedTypes(), ct):
			bad(fmt.Sprintf("group %q is localized, but its field %q is a %s field", label, c.Key, c.Type), localizedHint)
			typeSaid = true
		case t != "" && ct != t:
			bad(fmt.Sprintf("group %q is localized, but its fields mix %s and %s", label, t, ct), localizedHint)
			typeSaid = true
		}
		if t == "" {
			t = ct
		}
		if r, has := c.Raw["role"]; has && r != schema.RoleNone {
			bad(fmt.Sprintf("field %q of localized group %q has a role of its own", c.Key, label), "Declare the role on the localized group: it then means the field in each language.")
		}
	}
	for _, l := range locales {
		if _, ok := fd.LanguageField(l); !ok {
			bad(fmt.Sprintf("group %q is localized, but has no field for %q", label, l), localizedHint)
		}
	}
}

const localizedHint = "A localized group has one field per language of preview.locales, keyed by the language (cs, en, …), all text, paragraph or rich text of one type; `fallback: <language>` names the language an empty one shows."

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
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidLocalized, fmt.Sprintf("option %v of %q has labels %v; it maps languages to labels", m["value"], label, raw), "")
			continue
		}
		for _, l := range sortedMapKeys(labels) {
			if _, ok := labels[l].(string); !ok || !slices.Contains(rc.cfg.Preview.Locales, l) {
				rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidLocalized, fmt.Sprintf("option %v of %q has a label for %q, which is not a language of the site or not text", m["value"], label, l), "labels maps each language of preview.locales to the option's label in it.")
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
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaMisplacedRole, fmt.Sprintf("block %q, an item of this list, has a field with role %s, which would repeat per item; a page has one %s", block, fd.Role, fd.Role), "Declare page roles at the top level of the collection or inside a group, not inside a list.")
		}
		if schema.IsGroup(fd.Type) {
			rc.pageRolesInList(p, block, fd.Fields, depth+1)
		}
	}
}

// translationLocalesProblems says what is wrong with a collection's
// `translations.locales`: the languages its pages are expected in. Each
// must be a language the site declares.
func translationLocalesProblems(cfg *schema.Config, coll schema.Collection) []string {
	block, _ := coll.Raw["translations"].(map[string]any)
	raw, has := block["locales"]
	if !has {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return []string{fmt.Sprintf("translations.locales is %v, not a list of languages", raw)}
	}
	if len(list) == 0 {
		return []string{"translations.locales lists no language"}
	}
	var out []string
	seen := map[string]bool{}
	for _, item := range list {
		l, ok := item.(string)
		l = strings.TrimSpace(l)
		switch {
		case !ok || l == "":
			out = append(out, fmt.Sprintf("translations.locales has %v, which is not a language code", item))
		case seen[l]:
			out = append(out, fmt.Sprintf("translations.locales lists %q twice", l))
		case len(cfg.Preview.Locales) == 0:
			out = append(out, fmt.Sprintf("translations.locales lists %q, but the site declares no preview.locales", l))
		case !contains(cfg.Preview.Locales, l):
			out = append(out, fmt.Sprintf("translations.locales lists %q, which is not in preview.locales %v", l, cfg.Preview.Locales))
		}
		seen[l] = true
	}
	return out
}
