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
	// WhenEmpty is what the site does when the field is empty
	// (schema.WhenEmptyGenerated, schema.WhenEmptySiteDefault), as the
	// schema declares it; empty when it says nothing, says something
	// invalid, or the role was guessed.
	WhenEmpty string
	// Type is the canonical type of the value per page: a localized
	// group's language fields' type.
	Type string
	// Localized is true when the field is a localized group: the role
	// holds per language, at `<Path>.<locale>`. Fallback is the language
	// an empty one shows on the site.
	Localized bool
	Fallback  string
	// TitleTemplate is how the site composes the page title from a
	// seoTitle field; the checks measure the composed title.
	TitleTemplate string
	// Excerpt says which part of a rich text seoDescription is the meta
	// description (schema.ExcerptFirstParagraph).
	Excerpt string
	// When lists the values of a select that switch a noindex or draft
	// role on; empty for a boolean (true does). Guessed for a guessed
	// select.
	When []string
}

// ImageAlt is one image field and where its alt text comes from.
type ImageAlt struct {
	// Image is the image field's place in a page. A list item is `[]`
	// when the list has one shape and `[<shape>]` when it has several
	// (`content_blocks[logos].list[].img`).
	Image string
	Label string
	// Alt is the place of the field holding the alt text, in the same
	// notation; empty when the image has none, is decorative, or composes
	// it from several fields (AltFrom).
	Alt string
	// AltFrom are the places of the fields a composed alt text is made
	// of, joined by Separator; empty for an alt text of one field.
	AltFrom   []string `json:"altFrom,omitempty"`
	Separator string   `json:"separator,omitempty"`
	// AltLocalized is true when the alt text is a localized group (or
	// composed of one): it holds per language.
	AltLocalized bool
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
		if schema.IsGroup(fd.Type) && !fd.IsLocalized() {
			collectPageFields(fd.Fields, p, isSEOGroup(fd), depth+1, out)
		}
	}
}

func declaredRole(fields []pageField, role string) (RoleField, bool) {
	for _, pf := range fields {
		fd := pf.field
		if fd.Role != role || !fd.HoldsRole(role) {
			continue
		}
		f := roleField(role, pf.path, fd)
		f.Declared = true
		if schema.RoleAllowsWhenEmpty(role) && schema.KnownWhenEmpty(fd.WhenEmpty) {
			f.WhenEmpty = fd.WhenEmpty
		}
		if role == schema.RoleSEOTitle {
			if _, err := schema.ParseTemplate(fd.TitleTemplate); err == nil {
				f.TitleTemplate = fd.TitleTemplate
			}
		}
		if role == schema.RoleSEODescription && f.Type == schema.TypeRichText {
			f.Excerpt = fd.Excerpt
		}
		f.When = append([]string(nil), fd.WhenValues()...)
		return f, true
	}
	return RoleField{}, false
}

// roleField is what a role answer says about the field itself.
func roleField(role, path string, fd schema.Field) RoleField {
	f := RoleField{Role: role, Path: path, Label: fd.Label, Type: fd.ValueType(), Localized: fd.IsLocalized()}
	if f.Localized {
		f.Fallback = fd.Fallback
	}
	return f
}

func guessedRole(fields []pageField, role string) (RoleField, bool) {
	names := roleNames[role]
	var found *pageField
	var when []string
	for i := range fields {
		pf := &fields[i]
		if pf.field.Role != "" {
			continue
		}
		var w []string
		if schema.Canonical(pf.field.Type) == schema.TypeSelect && schema.RoleWhen(role) != "" {
			// A robots select: the options that say noindex hide the page.
			if w = noindexOptions(pf.field); role != schema.RoleNoindex || len(w) == 0 {
				continue
			}
		} else if !pf.field.HoldsRole(role) {
			continue
		}
		key, label := normalName(pf.field.Key), normalName(pf.field.Label)
		if !names.keys[key] && !names.labels[label] && !(pf.seoGroup && names.inSEOGroup[key]) {
			continue
		}
		if found != nil {
			return RoleField{}, false
		}
		found, when = pf, w
	}
	if found == nil {
		return RoleField{}, false
	}
	f := roleField(role, found.path, found.field)
	f.When = when
	return f, true
}

// noindexOptions are a select's values that carry the robots directive
// noindex (or none, which means noindex, nofollow).
func noindexOptions(fd schema.Field) []string {
	var out []string
	for _, v := range schema.OptionValues(fd.Options) {
		for _, tok := range strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
			if tok == "noindex" || tok == "none" {
				out = append(out, v)
				break
			}
		}
	}
	return out
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
		keys:       set("noindex", "hidefromsearch", "hidefromsearchengines", "robots", "metarobots"),
		labels:     set("noindex", "hidefromsearch", "hidefromsearchengines", "robots", "metarobots"),
		inSEOGroup: set(),
	},
	schema.RoleCanonical: {
		keys:       set("canonical", "canonicalurl", "canonicallink"),
		labels:     set("canonical", "canonicalurl", "canonicallink"),
		inSEOGroup: set(),
	},
	schema.RoleDraft: {
		keys:       set("draft", "isdraft"),
		labels:     set("draft"),
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
			src := AltOf(fields, fd)
			img := ImageAlt{Image: p, Label: fd.Label, Decorative: src.Decorative, Declared: src.Declared, AltLocalized: src.Localized}
			switch {
			case src.Composed:
				img.Separator = src.Separator
				for _, k := range src.Keys {
					img.AltFrom = append(img.AltFrom, schema.JoinPath(base, k))
				}
			case len(src.Keys) == 1:
				img.Alt = schema.JoinPath(base, src.Keys[0])
			}
			out.Images = append(out.Images, img)
		case schema.IsGroup(fd.Type) && !fd.IsLocalized():
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

// AltSource is where an image's alt text comes from.
type AltSource struct {
	// Keys are the siblings that hold it: one field, or the parts of a
	// composed alt text in order. Empty when the image has none or is
	// decorative.
	Keys []string
	// Separator joins the parts of a composed alt text.
	Separator string
	// Decorative is true when the schema says the image needs none.
	Decorative bool
	// Declared is true when the answer comes from the schema (`alt:`,
	// `role: alt`, `decorative: true`), false for a guess or no answer.
	Declared bool
	// Localized is true when a part is a localized group: the alt text
	// holds per language.
	Localized bool
	// Composed is true for `alt: {from: [...]}`: Keys joined by Separator,
	// empty parts left out.
	Composed bool
}

// AltOf says where an image field's alt text comes from, given the
// fields list it sits in.
//
// Declared, in order: `decorative: true`; `alt: {from: [...]}` composing
// it from text, paragraph or localized text siblings; `alt: <key>`
// naming one; a sibling with `role: alt` when the image is the only one
// in the list. Guessed otherwise, from a text or paragraph sibling that
// declares no role: `<key>_alt` or `<key>Alt` (also `<key>_alt_text`);
// when the image is the only one in the list, `alt`, `alt_text` or
// `alternative_text`, and failing those `name`. Two siblings that fit
// one step guess nothing.
func AltOf(siblings []schema.Field, image schema.Field) AltSource {
	if image.Decorative {
		return AltSource{Decorative: true, Declared: true}
	}
	byKey := func(k string) (schema.Field, bool) {
		for _, s := range siblings {
			if s.Key == k && altText(s, image) {
				return s, true
			}
		}
		return schema.Field{}, false
	}
	if len(image.AltFrom) > 0 {
		out := AltSource{Separator: image.AltSeparator, Declared: true, Composed: true}
		for _, k := range image.AltFrom {
			s, ok := byKey(k)
			if !ok {
				return AltSource{}
			}
			out.Keys = append(out.Keys, k)
			out.Localized = out.Localized || s.IsLocalized()
		}
		return out
	}
	one := func(s schema.Field, declared bool) AltSource {
		return AltSource{Keys: []string{s.Key}, Declared: declared, Localized: s.IsLocalized()}
	}
	images := 0
	for _, s := range siblings {
		if schema.Canonical(s.Type) == schema.TypeImage && s.Key != "" {
			images++
		}
	}
	if image.Alt != "" {
		if s, ok := byKey(image.Alt); ok {
			return one(s, true)
		}
		return AltSource{}
	}
	if images == 1 {
		for _, s := range siblings {
			if s.Role == schema.RoleAlt && altText(s, image) {
				return one(s, true)
			}
		}
	}
	pick := func(names map[string]bool) (schema.Field, bool, bool) {
		var found *schema.Field
		for i, s := range siblings {
			if !altText(s, image) || s.Role != "" || !names[normalName(s.Key)] {
				continue
			}
			if found != nil {
				return schema.Field{}, false, true
			}
			found = &siblings[i]
		}
		if found == nil {
			return schema.Field{}, false, false
		}
		return *found, true, true
	}
	own := normalName(image.Key)
	if s, ok, hit := pick(set(own+"alt", own+"alttext")); hit {
		if ok {
			return one(s, false)
		}
		return AltSource{}
	}
	if images != 1 {
		return AltSource{}
	}
	if s, ok, hit := pick(set("alt", "alttext", "alternativetext")); hit {
		if ok {
			return one(s, false)
		}
		return AltSource{}
	}
	if s, ok, _ := pick(set("name")); ok {
		return one(s, false)
	}
	return AltSource{}
}

// altText reports whether a sibling can hold an image's alt text: text,
// paragraph, or a localized group of them.
func altText(f, image schema.Field) bool {
	if f.Key == "" || f.Key == image.Key {
		return false
	}
	if schema.IsGroup(f.Type) && !f.IsLocalized() {
		return false
	}
	t := f.ValueType()
	return t == schema.TypeText || t == schema.TypeParagraph
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
