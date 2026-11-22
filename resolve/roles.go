package resolve

import (
	"strings"
	"unicode"

	"github.com/norcubeplatform/cms-schema/schema"
)

// MaxRoleImages bounds how many image fields CollectionRoles lists for
// one collection. Real schemas have tens.
const MaxRoleImages = 500

// RoleField is the field that holds a page role.
type RoleField struct {
	Role string
	// Path is the field's place in a page, keys joined by dots
	// (`seo.page_title`), as preview slots name it. Page roles are never
	// inside a list.
	Path  string
	Label string
	// Declared is true for a `role:` in the schema, false for a guess
	// from the field's key or label.
	Declared bool
}

// ImageAlt is one image field and where its alt text comes from.
type ImageAlt struct {
	// Image is the image field's place in a page. A list item is `[]`
	// when the list has one shape and `[<shape>]` when it has several
	// (`content_blocks[logos].list[].img`).
	Image string
	Label string
	// Alt is the place of the field holding the alt text, in the same
	// notation; empty when the image has none or is decorative.
	Alt string
	// Decorative is true when the schema says the image needs no alt
	// text.
	Decorative bool
	// Declared is true when Alt or Decorative comes from the schema
	// (`alt:`, `role: alt`, `decorative: true`), false for a guess or
	// for no alt at all.
	Declared bool
}

// Roles are the roles of one collection.
type Roles struct {
	// Fields holds one entry per page role found, in schema.PageRoles
	// order.
	Fields []RoleField
	// Images lists the collection's image fields in schema order, at
	// most MaxRoleImages; Truncated says there were more.
	Images    []ImageAlt
	Truncated bool
}

// Field returns the field of one page role.
func (r Roles) Field(role string) (RoleField, bool) {
	for _, f := range r.Fields {
		if f.Role == role {
			return f, true
		}
	}
	return RoleField{}, false
}

// CollectionRoles answers which field of a collection holds each page
// role and where each image's alt text is.
//
// A role declared in the schema wins. A role nobody declares is guessed
// from field keys and labels (the table in the README): only fields
// outside lists, of a type that can hold the role and declaring no role
// themselves, are candidates, and two candidates for one role guess
// nothing. The answer depends on the schema alone and is the same every
// time.
func CollectionRoles(cfg *schema.Config, coll schema.Collection) Roles {
	var out Roles
	var pageFields []pageField
	collectPageFields(coll.Fields, "", false, 0, &pageFields)
	for _, role := range schema.PageRoles() {
		if f, ok := declaredRole(pageFields, role); ok {
			out.Fields = append(out.Fields, f)
		} else if f, ok := guessedRole(pageFields, role); ok {
			out.Fields = append(out.Fields, f)
		}
	}
	collectImages(cfg, coll.Fields, "", 0, &out)
	return out
}

// pageField is a field outside any list, with its place and the group
// it sits in directly.
type pageField struct {
	field schema.Field
	path  string
	// seoGroup is true when the field sits directly in a group that is
	// about search (key or block `seo`, `meta`, `metadata`).
	seoGroup bool
}

func collectPageFields(fields []schema.Field, base string, seoGroup bool, depth int, out *[]pageField) {
	if depth > schema.MaxWalkDepth {
		return
	}
	for _, fd := range fields {
		if fd.Key == "" {
			continue
		}
		p := schema.JoinPath(base, fd.Key)
		*out = append(*out, pageField{field: fd, path: p, seoGroup: seoGroup})
		if schema.IsGroup(fd.Type) {
			collectPageFields(fd.Fields, p, isSEOGroup(fd), depth+1, out)
		}
	}
}

func declaredRole(fields []pageField, role string) (RoleField, bool) {
	for _, pf := range fields {
		if pf.field.Role == role && schema.RoleAccepts(role, pf.field.Type) {
			return RoleField{Role: role, Path: pf.path, Label: pf.field.Label, Declared: true}, true
		}
	}
	return RoleField{}, false
}

func guessedRole(fields []pageField, role string) (RoleField, bool) {
	names := roleNames[role]
	var found *pageField
	for i := range fields {
		pf := &fields[i]
		if pf.field.Role != "" || !schema.RoleAccepts(role, pf.field.Type) {
			continue
		}
		key, label := normalName(pf.field.Key), normalName(pf.field.Label)
		if !names.keys[key] && !names.labels[label] && !(pf.seoGroup && names.inSEOGroup[key]) {
			continue
		}
		if found != nil {
			return RoleField{}, false
		}
		found = pf
	}
	if found == nil {
		return RoleField{}, false
	}
	return RoleField{Role: role, Path: found.path, Label: found.field.Label}, true
}

// roleNames is the guessing table, in normalized form (lowercase letters
// and digits only, so meta_title, metaTitle and meta-title are one name).
var roleNames = map[string]struct {
	// keys match a field's key anywhere outside a list; labels match its
	// label; inSEOGroup match a key directly inside an seo group.
	keys, labels, inSEOGroup map[string]bool
}{
	schema.RoleSEOTitle: {
		keys:       set("seotitle", "metatitle", "pagetitle"),
		labels:     set("seotitle", "metatitle"),
		inSEOGroup: set("title"),
	},
	schema.RoleSEODescription: {
		keys:       set("seodescription", "metadescription", "pagedescription"),
		labels:     set("seodescription", "metadescription"),
		inSEOGroup: set("description"),
	},
	schema.RoleShareImage: {
		keys:       set("shareimage", "socialimage", "ogimage", "opengraphimage", "featuredimage", "seoimage", "metaimage"),
		labels:     set("shareimage", "socialimage", "ogimage", "opengraphimage"),
		inSEOGroup: set("image"),
	},
	schema.RoleNoindex: {
		keys:       set("noindex", "hidefromsearch", "hidefromsearchengines"),
		labels:     set("noindex", "hidefromsearch", "hidefromsearchengines"),
		inSEOGroup: set(),
	},
	schema.RoleCanonical: {
		keys:       set("canonical", "canonicalurl", "canonicallink"),
		labels:     set("canonical", "canonicalurl", "canonicallink"),
		inSEOGroup: set(),
	},
}

// seoGroupNames are the normalized keys and block names of a group about
// search.
var seoGroupNames = set("seo", "meta", "metadata")

func isSEOGroup(fd schema.Field) bool {
	return seoGroupNames[normalName(fd.Key)] || (fd.BlockRef != "" && seoGroupNames[normalName(fd.BlockRef)])
}

// collectImages lists every image field with its alt source, entering
// groups, list item shapes and the blocks of a legacy `blocks` list.
func collectImages(cfg *schema.Config, fields []schema.Field, base string, depth int, out *Roles) {
	if depth > schema.MaxWalkDepth {
		return
	}
	for _, fd := range fields {
		if fd.Key == "" {
			continue
		}
		p := schema.JoinPath(base, fd.Key)
		switch {
		case schema.Canonical(fd.Type) == schema.TypeImage:
			if len(out.Images) == MaxRoleImages {
				out.Truncated = true
				return
			}
			alt, decorative, declared := AltOf(fields, fd)
			img := ImageAlt{Image: p, Label: fd.Label, Decorative: decorative, Declared: declared}
			if alt != "" {
				img.Alt = schema.JoinPath(base, alt)
			}
			out.Images = append(out.Images, img)
		case schema.IsGroup(fd.Type):
			collectImages(cfg, fd.Fields, p, depth+1, out)
		case fd.Type == schema.TypeBlocks || len(fd.Components) > 0:
			for _, name := range fd.Components {
				if cfg == nil {
					break
				}
				if b, ok := cfg.Blocks[name]; ok {
					collectImages(cfg, b.Fields, p+"["+name+"]", depth+1, out)
				}
			}
		case schema.IsArray(fd.Type):
			multi := schema.IsMultiShape(fd)
			for _, e := range schema.ItemEntries(fd) {
				ip := p + "[]"
				if multi {
					ip = p + "[" + e.Name + "]"
				}
				if schema.IsGroup(e.Field.Type) {
					collectImages(cfg, e.Field.Fields, ip, depth+1, out)
				} else if schema.Canonical(e.Field.Type) == schema.TypeImage {
					if len(out.Images) == MaxRoleImages {
						out.Truncated = true
						return
					}
					// An image alone in a list has no sibling to take
					// alt text from.
					out.Images = append(out.Images, ImageAlt{Image: ip, Label: fd.Label, Decorative: e.Field.Decorative, Declared: e.Field.Decorative})
				}
			}
		}
		if out.Truncated {
			return
		}
	}
}

// AltOf says where an image field's alt text comes from, given the
// fields list it sits in: the key of the sibling that holds it, whether
// the image is decorative, and whether the answer is declared in the
// schema.
//
// Declared, in order: `decorative: true`; `alt: <key>` naming a text or
// paragraph sibling; a sibling with `role: alt` when the image is the
// only one in the list. Guessed otherwise, from a text or paragraph
// sibling that declares no role: `<key>_alt` or `<key>Alt` (also
// `<key>_alt_text`); when the image is the only one in the list, `alt`,
// `alt_text` or `alternative_text`, and failing those `name`. Two
// siblings that fit one step guess nothing.
func AltOf(siblings []schema.Field, image schema.Field) (alt string, decorative, declared bool) {
	if image.Decorative {
		return "", true, true
	}
	text := func(f schema.Field) bool {
		t := schema.Canonical(f.Type)
		return f.Key != "" && f.Key != image.Key && (t == schema.TypeText || t == schema.TypeParagraph)
	}
	images := 0
	for _, s := range siblings {
		if schema.Canonical(s.Type) == schema.TypeImage && s.Key != "" {
			images++
		}
	}
	if image.Alt != "" {
		for _, s := range siblings {
			if s.Key == image.Alt && text(s) {
				return s.Key, false, true
			}
		}
		return "", false, false
	}
	if images == 1 {
		for _, s := range siblings {
			if s.Role == schema.RoleAlt && text(s) {
				return s.Key, false, true
			}
		}
	}
	pick := func(names map[string]bool) (string, bool) {
		found := ""
		for _, s := range siblings {
			if !text(s) || s.Role != "" || !names[normalName(s.Key)] {
				continue
			}
			if found != "" {
				return "", true
			}
			found = s.Key
		}
		return found, found != ""
	}
	own := normalName(image.Key)
	if key, hit := pick(set(own+"alt", own+"alttext")); hit {
		return key, false, false
	}
	if images != 1 {
		return "", false, false
	}
	if key, hit := pick(set("alt", "alttext", "alternativetext")); hit {
		return key, false, false
	}
	key, _ := pick(set("name"))
	return key, false, false
}

// ExpectedLocales are the languages a collection's pages are expected
// in: its `translations.locales`, else every language of the site
// (`preview.locales`). A listed language the site does not declare is
// left out (the validator reports it), so a collection whose list names
// none of them expects nothing. Only collections that declare
// `translations` have language versions to compare.
func ExpectedLocales(cfg *schema.Config, coll schema.Collection) []string {
	var site []string
	if cfg != nil {
		site = cfg.Preview.Locales
	}
	if coll.Translations == nil || len(coll.Translations.Locales) == 0 {
		return append([]string(nil), site...)
	}
	out := []string{}
	for _, l := range coll.Translations.Locales {
		if containsString(site, l) && !containsString(out, l) {
			out = append(out, l)
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// normalName keeps the lowercase letters and digits of a key or label.
func normalName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}
