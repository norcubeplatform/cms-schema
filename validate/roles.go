package validate

import (
	"fmt"
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
		checkRoles(cfg, load.CollectionFile(key), "fields", cfg.Collections[key].Fields, f)
	}
	for _, key := range cfg.BlockKeys {
		checkRoles(cfg, load.BlockFile(key), "fields", cfg.Blocks[key].Fields, f)
	}
	for _, key := range cfg.InputKeys {
		checkRoles(cfg, load.InputFile(key), "", []schema.Field{cfg.Inputs[key]}, f)
	}
	return out
}

const rolesHint = "Roles: seoTitle, seoDescription, shareImage, noindex, canonical, alt, or none (no role, never guessed)."

// roleCheck walks one collection, block or input. A page role is
// counted across the whole of it; `role: alt` and `alt:` are judged per
// fields list.
type roleCheck struct {
	cfg  *schema.Config
	file string
	f    emit
	// seen maps a page role to the path of the first field declaring it.
	seen map[string]string
}

func checkRoles(cfg *schema.Config, file, base string, fields []schema.Field, f emit) {
	rc := &roleCheck{cfg: cfg, file: file, f: f, seen: map[string]string{}}
	rc.list(base, fields, false, 0)
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
	case !schema.RoleAccepts(role, fd.Type):
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaRoleTypeMismatch, fmt.Sprintf("field %q has role %s, which a %s field cannot hold", label, role, fd.Type), fmt.Sprintf("%s goes on a field of type %s.", role, strings.Join(schema.RoleTypes(role), " or ")))
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
	raw, has := fd.Raw["alt"]
	if !has {
		return
	}
	name, _ := raw.(string)
	name = strings.TrimSpace(name)
	hint := "`alt:` names the field next to the image, in the same fields list, that holds its alt text: a text or paragraph field."
	switch {
	case name == "":
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q has alt %v, which names no field", label, raw), hint)
		return
	case fd.Decorative:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q is decorative and also names %q as its alt text", label, name), "A decorative image needs no alt text: keep one of the two.")
		return
	case name == fd.Key:
		rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q names itself as its alt text", label), hint)
		return
	}
	for _, s := range siblings {
		if s.Key != name {
			continue
		}
		if t := schema.Canonical(s.Type); t != schema.TypeText && t != schema.TypeParagraph {
			rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q names %q as its alt text, which is a %s field", label, name, s.Type), hint)
		}
		return
	}
	rc.f(rc.file, p, report.SeverityError, report.RuleSchemaInvalidAlt, fmt.Sprintf("image %q names %q as its alt text, but no field in the same list has that key", label, name), hint)
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
		if schema.IsPageRole(fd.Role) {
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
