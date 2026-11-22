package schema

import "slices"

// Field roles say what a field means to the site rather than how it is
// edited: which one is the page's title in search results, its share
// image, its alt text. Checks, the editor and Norcube AI read the same
// declaration. The set is closed.
const (
	// RoleSEOTitle is the page's title in search results and the browser
	// tab, when it differs from the page title.
	RoleSEOTitle = "seoTitle"
	// RoleSEODescription is the page's meta description.
	RoleSEODescription = "seoDescription"
	// RoleShareImage is the image shown when the page is shared.
	RoleShareImage = "shareImage"
	// RoleNoindex is a boolean that hides the page from search engines
	// when true.
	RoleNoindex = "noindex"
	// RoleCanonical is the address that should rank instead of the page.
	RoleCanonical = "canonical"
	// RoleAlt is the alt text of the one image in the same fields list
	// (an image object: a group or list item holding an image and its
	// text). It may appear once per such list.
	RoleAlt = "alt"
)

// pageRoles are the roles a page has at most one of, in the order they
// are listed.
var pageRoles = []string{RoleSEOTitle, RoleSEODescription, RoleShareImage, RoleNoindex, RoleCanonical}

// roleTypes are the canonical field types that can hold each role.
var roleTypes = map[string][]string{
	RoleSEOTitle:       {TypeText, TypeParagraph},
	RoleSEODescription: {TypeText, TypeParagraph},
	RoleShareImage:     {TypeImage},
	RoleNoindex:        {TypeBoolean},
	RoleCanonical:      {TypeLink, TypeText},
	RoleAlt:            {TypeText, TypeParagraph},
}

// PageRoles lists the roles a collection has at most one field for, in
// a stable order: seoTitle, seoDescription, shareImage, noindex,
// canonical.
func PageRoles() []string { return append([]string(nil), pageRoles...) }

// Roles lists every role, page roles first, then alt.
func Roles() []string { return append(PageRoles(), RoleAlt) }

// KnownRole reports whether r is a role.
func KnownRole(r string) bool { _, ok := roleTypes[r]; return ok }

// IsPageRole reports whether r is a role a page has at most one of.
func IsPageRole(r string) bool { return KnownRole(r) && r != RoleAlt }

// RoleTypes lists the canonical field types that can hold the role.
func RoleTypes(r string) []string { return append([]string(nil), roleTypes[r]...) }

// RoleAccepts reports whether a field of type t can hold the role.
func RoleAccepts(r, t string) bool {
	for _, want := range roleTypes[r] {
		if Canonical(t) == want {
			return true
		}
	}
	return false
}

// What the site does when a role field is empty (`whenEmpty:`). Saying
// so is the owner's statement about the site's code; it is never
// guessed.
const (
	// WhenEmptyGenerated: the site makes the value per page in code (a
	// share card rendered at build time, a title from the page heading).
	WhenEmptyGenerated = "generated"
	// WhenEmptySiteDefault: the site falls back to one site-wide value.
	WhenEmptySiteDefault = "siteDefault"
)

var whenEmptyValues = []string{WhenEmptyGenerated, WhenEmptySiteDefault}

// whenEmptyRoles are the roles whose empty value a site can fill in
// code. Not alt: alt text describes one image and code cannot write it
// reliably. Not noindex or canonical: empty already means indexable
// and canonical to itself.
var whenEmptyRoles = []string{RoleSEOTitle, RoleSEODescription, RoleShareImage}

// WhenEmptyValues lists the values of `whenEmpty`.
func WhenEmptyValues() []string { return append([]string(nil), whenEmptyValues...) }

// KnownWhenEmpty reports whether v is a value of `whenEmpty`.
func KnownWhenEmpty(v string) bool { return slices.Contains(whenEmptyValues, v) }

// WhenEmptyRoles lists the roles that may declare `whenEmpty`.
func WhenEmptyRoles() []string { return append([]string(nil), whenEmptyRoles...) }

// RoleAllowsWhenEmpty reports whether a field of the role may declare
// `whenEmpty`.
func RoleAllowsWhenEmpty(r string) bool { return slices.Contains(whenEmptyRoles, r) }

// SiteArea reports whether the collection holds site settings
// (navigation, footer, globals) rather than pages. Counts of pages leave
// these documents out.
func (c Collection) SiteArea() bool { return c.Area == AreaSite }
