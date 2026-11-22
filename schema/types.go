// Package schema is the typed model of a Norcube CMS site configuration:
// the root config, its collections, blocks, inputs and storages, and the
// field definitions they are made of. It parses the merged document the
// loader assembles (see package load) and offers the helpers every
// consumer needs: array item shapes, condition evaluation, select
// options.
//
// The model mirrors the editor's schema module field for field. A knob
// the editor reads has a place here; a key it does not read lands in
// Field.Extra so a validator can report it.
package schema

import "strings"

// Field types the editor renders. The three legacy spellings are the
// names fields carried before a rename and are still accepted in
// content written against them.
const (
	TypeText        = "text"
	TypeParagraph   = "paragraph"
	TypeRichText    = "richtext"
	TypeNumber      = "number"
	TypeBoolean     = "boolean"
	TypeDate        = "date"
	TypeColor       = "color"
	TypeRange       = "range"
	TypeMultiChoice = "multichoice"
	TypeLink        = "link"
	TypeImage       = "image"
	TypeFile        = "file"
	TypeReference   = "reference"
	TypeSelect      = "select"
	TypeGroup       = "group"
	TypeArray       = "array"

	// Legacy spellings.
	TypeURL      = "url"      // link
	TypeMarkdown = "markdown" // richtext
	TypeHTML     = "html"     // richtext
	TypeObject   = "object"   // group
	TypeBlocks   = "blocks"   // array of named components
)

// KnownTypes lists every field type this module understands, legacy
// spellings included.
var KnownTypes = map[string]bool{
	TypeText: true, TypeParagraph: true, TypeRichText: true, TypeNumber: true,
	TypeBoolean: true, TypeDate: true, TypeColor: true, TypeRange: true,
	TypeMultiChoice: true, TypeLink: true, TypeImage: true, TypeFile: true,
	TypeReference: true, TypeSelect: true, TypeGroup: true, TypeArray: true,
	TypeURL: true, TypeMarkdown: true, TypeHTML: true, TypeObject: true, TypeBlocks: true,
}

// Canonical maps a legacy spelling to the current type name and returns
// any other name unchanged.
func Canonical(t string) string {
	switch t {
	case TypeURL:
		return TypeLink
	case TypeMarkdown, TypeHTML:
		return TypeRichText
	case TypeObject:
		return TypeGroup
	case TypeBlocks:
		return TypeArray
	}
	return t
}

// IsGroup reports whether a field type nests fields.
func IsGroup(t string) bool { return Canonical(t) == TypeGroup }

// IsArray reports whether a field type holds a list of items.
func IsArray(t string) bool { return Canonical(t) == TypeArray }

// IsStringLike reports whether values of the type are strings.
func IsStringLike(t string) bool {
	switch Canonical(t) {
	case TypeText, TypeParagraph, TypeRichText, TypeLink, TypeDate, TypeImage,
		TypeFile, TypeReference, TypeSelect, TypeColor:
		return true
	}
	return false
}

// Keys with a fixed meaning inside content files.
const (
	// ArrayItemTypeKey is the default discriminator of a multi-shape
	// array item; a field's `idKey` overrides it.
	ArrayItemTypeKey = "_type"
	// BlockComponentKey discriminates items of a legacy `blocks` field.
	BlockComponentKey = "_component"
	// CollectionKey stamps a file with the collection that owns it.
	CollectionKey = "_collection"
	// IDKey is the page-level identity the editor writes inline.
	IDKey = "_id"
	// RefKey marks a mapping that stands for a block or input.
	RefKey = "$ref"
	// BlockRefKey is the annotation the loader stamps on a resolved `$ref`.
	BlockRefKey = "_blockRef"
)

// Values of `localeFrom`, `by` and `redirects.format`.
const (
	LocaleFromPath        = "path"
	LocaleFromNone        = "none"
	LocaleFromFieldPrefix = "field:"

	ReferenceByPath        = "path"
	ReferenceByFieldPrefix = "field:"

	TranslationsByPath        = "path"
	TranslationsByFieldPrefix = "field:"

	RedirectsFormatNetlify = "netlify"
	RedirectsFormatVercel  = "vercel"

	ExtraKeysShow  = "show"
	ExtraKeysHide  = "hide"
	ExtraKeysStrip = "strip"

	AreaContent = "content"
	AreaSite    = "site"

	// Storage formats of a rich text field; markdown when it does not say.
	RichTextFormatMarkdown = "markdown"
	RichTextFormatHTML     = "html"

	WhenHiddenKeep = "keep"
	WhenHiddenDrop = "drop"

	DefaultCollection = "pages"
	CurrentVersion    = 1
)

// SupportedVersions lists the schema-language versions this module reads.
var SupportedVersions = map[int]bool{1: true}

// Condition is a `visibleIf` tree: a leaf (Field, Op, Value) or one of
// All, Any, Not.
type Condition struct {
	Field string
	Op    string
	Value any

	All []Condition
	Any []Condition
	Not *Condition
}

// IsLeaf reports whether the condition compares one field.
func (c Condition) IsLeaf() bool { return len(c.All) == 0 && len(c.Any) == 0 && c.Not == nil }

// Condition operators.
const (
	OpEq       = "eq"
	OpNeq      = "neq"
	OpIn       = "in"
	OpNotIn    = "notIn"
	OpTruthy   = "truthy"
	OpFalsy    = "falsy"
	OpEmpty    = "empty"
	OpNonEmpty = "nonEmpty"
	OpGt       = "gt"
	OpLt       = "lt"
	OpMatches  = "matches"
)

// KnownOps lists the condition operators.
var KnownOps = map[string]bool{
	OpEq: true, OpNeq: true, OpIn: true, OpNotIn: true, OpTruthy: true, OpFalsy: true,
	OpEmpty: true, OpNonEmpty: true, OpGt: true, OpLt: true, OpMatches: true,
}

// PresenceOp reports whether the operator only asks whether a value is
// there, so a path that resolves to nothing is a legitimate answer.
func PresenceOp(op string) bool {
	switch op {
	case OpTruthy, OpFalsy, OpEmpty, OpNonEmpty:
		return true
	}
	return false
}

// Computed derives a field's value from another field.
type Computed struct {
	From      string
	Transform string
	Mode      string
}

// CustomValidator is one `customValidators` entry; Params holds every
// key but `type` and `message`.
type CustomValidator struct {
	Type    string
	Message string
	Params  map[string]any
}

// Built-in custom validator types.
const (
	CustomValidatorRegex      = "regex"
	CustomValidatorExpression = "expression"
)

// SelectOption is one option of a select or multichoice field.
type SelectOption struct {
	Value       string
	Label       string
	Description string
}

// RecommendedLength is the soft length window of a text field.
type RecommendedLength struct {
	From *int
	To   *int
}

// Resolver is one entry of a preview slot: a field key or a literal.
type Resolver struct {
	Key     string
	Literal string
}

// Preview says how an entry renders as a summary row.
type Preview struct {
	Title       []Resolver
	Description []Resolver
	Image       []Resolver
	Badges      [][]Resolver
}

// Field is one field definition. Every type-specific knob has a place;
// a nil pointer or zero value means the knob is absent.
type Field struct {
	Key         string
	Type        string
	Label       string
	Description string
	// Name is the discriminator of an array item shape when it differs
	// from Key.
	Name string
	// BlockRef is the block or input a `$ref` resolved from.
	BlockRef string

	Required        bool
	RequiredMessage string
	Nullable        bool
	Hidden          bool
	VisibleIf       *Condition
	WhenHidden      string
	Computed        *Computed
	Custom          []CustomValidator
	// Default is the declared default; HasDefault distinguishes an
	// explicit null from no default.
	Default    any
	HasDefault bool
	// Variables is false when the field opts out of the placeholder
	// paster; nil when unset.
	Variables *bool

	// text, paragraph, richtext, link
	Regex             string
	RegexMessage      string
	MinLength         *int
	MinLengthMessage  string
	MaxLength         *int
	MaxLengthMessage  string
	RecommendedLength *RecommendedLength
	Rows              *int
	// richtext and color
	Format string
	// richtext
	RichTextOptions map[string]any

	// number, range, multichoice
	Min  *float64
	Max  *float64
	Step *float64

	// select, multichoice
	Options           []SelectOption
	Widget            string
	Searchable        bool
	SearchPlaceholder string

	// image, file
	Accept  string
	MaxSize int64
	Storage string
	Dir     string
	LockDir bool
	// image: Alt names the sibling field that holds the image's alt text;
	// Decorative says the image needs none.
	Alt        string
	Decorative bool

	// reference
	Collection string
	By         string
	Multiple   bool

	// group
	Fields  []Field
	Preview *Preview
	Inline  bool

	// array
	Items           []Field
	IDKey           string
	MinItems        *int
	MinItemsMessage string
	MaxItems        *int
	MaxItemsMessage string
	UniqueOn        string
	UniqueOnMessage string
	// Components backs the legacy `blocks` type.
	Components []string

	// Translatable says whether the field's text is translated when a
	// page gets a language version; nil when the field does not say, and
	// the default for its type applies (resolve.Translatable).
	Translatable *bool

	// Role is what the field means to the site (schema.RoleSEOTitle…);
	// empty when it declares none or the value is not a string.
	Role string

	// Extra holds keys the editor does not read for this field type.
	Extra map[string]any
	// Raw is the field as parsed, for consumers that need a knob this
	// model does not name.
	Raw map[string]any
}

// Collection is one content type.
type Collection struct {
	Key             string
	Label           string
	Description     string
	Path            string
	Match           string
	HasBody         bool
	ExtraKeysPolicy string
	Area            string
	Preview         *Preview
	PreviewPath     string
	LocaleFrom      string
	// Translations says how the collection's language versions are linked;
	// nil when the collection declares none.
	Translations  *Translations
	SchemaVersion *int
	Fields        []Field
	Raw           map[string]any
}

// Translations is a collection's `translations` block: how the pages that
// are versions of one another in different languages are told apart from
// the rest. Nothing is inferred: a collection without it has no language
// versions.
type Translations struct {
	// By is `path` (the same path under each locale folder) or
	// `field:<key>` (pages with the same value of that field).
	By string
	// Locales are the languages the collection's pages are expected in;
	// empty means every site language (resolve.ExpectedLocales).
	Locales []string
}

// Field returns the key of the linking field for `by: field:<key>`, and
// whether By has that form.
func (t *Translations) Field() (string, bool) {
	if t == nil || !strings.HasPrefix(t.By, TranslationsByFieldPrefix) {
		return "", false
	}
	key := strings.TrimPrefix(t.By, TranslationsByFieldPrefix)
	return key, key != ""
}

// Block is a reusable group of fields.
type Block struct {
	Key         string
	Label       string
	Description string
	Fields      []Field
	Preview     *Preview
	Raw         map[string]any
}

// Storage is one upload destination.
type Storage struct {
	Name      string
	Type      string
	Mode      string
	Accept    string
	Naming    string
	Inline    string
	Dedup     string
	MaxSize   int64
	Path      string
	PublicURL string
	Raw       map[string]any
}

// Storage knob values.
const (
	StorageTypeRepo = "repo"

	StorageModeReadOnly  = "read-only"
	StorageModeReadWrite = "read-write"

	StorageNamingOriginal = ""
	StorageNamingSlug     = "slug"
	StorageNamingSlugHash = "slug+hash"
	StorageNamingUUID     = "uuid"

	StorageInlineRef = "ref"
	StorageInlineSVG = "svg"

	StorageDedupHash = "hash"
)

// PreviewSite holds the site-level i18n facts of the `preview` block.
type PreviewSite struct {
	DefaultLocale       string
	Locales             []string
	PrefixDefaultLocale bool
}

// Redirects says where URL changes are recorded.
type Redirects struct {
	File   string
	Format string
}

// Enabled reports whether the site declares a redirects file in a
// format this module can write.
func (r Redirects) Enabled() bool {
	return r.File != "" && (r.Format == RedirectsFormatNetlify || r.Format == RedirectsFormatVercel)
}

// Variables points at the site's `{{placeholder}}` document.
type Variables struct {
	File  string
	Field string
}

// Config is a site's whole configuration after merging and `$ref`
// resolution.
type Config struct {
	SchemaVersion  int
	SiteURL        string
	PagesDir       string
	PublicDir      string
	DefaultStorage string
	InputsFromGlob []string
	// Storages in declared order.
	Storages   []Storage
	Preview    PreviewSite
	Redirects  Redirects
	KnownPaths []string
	Variables  Variables
	// Collections keyed by name; CollectionKeys keeps the declared order.
	Collections    map[string]Collection
	CollectionKeys []string
	// Blocks and Inputs keyed by name.
	Blocks    map[string]Block
	BlockKeys []string
	Inputs    map[string]Field
	InputKeys []string
	// Raw is the merged document the config was parsed from.
	Raw map[string]any
}

// Storage returns the named storage.
func (c *Config) Storage(name string) (Storage, bool) {
	for _, s := range c.Storages {
		if s.Name == name {
			return s, true
		}
	}
	return Storage{}, false
}

// DefaultStorageConfig returns the storage a field without `storage`
// uses: the declared default, else the first declared.
func (c *Config) DefaultStorageConfig() (Storage, bool) {
	if len(c.Storages) == 0 {
		return Storage{}, false
	}
	if c.DefaultStorage != "" {
		return c.Storage(c.DefaultStorage)
	}
	return c.Storages[0], true
}

// ResolveStorage returns the storage a field uses: the named one, or the
// default when the name is empty.
func (c *Config) ResolveStorage(name string) (Storage, bool) {
	if name == "" {
		return c.DefaultStorageConfig()
	}
	return c.Storage(name)
}

// StorageNames lists the declared storages in order.
func (c *Config) StorageNames() []string {
	out := make([]string, len(c.Storages))
	for i, s := range c.Storages {
		out[i] = s.Name
	}
	return out
}

// ReferenceKeys lists the field names any reference field looks entries
// up by (`by: field:<k>`), sorted.
func (c *Config) ReferenceKeys() []string {
	seen := map[string]bool{}
	var walk func(fields []Field)
	walk = func(fields []Field) {
		for _, f := range fields {
			if Canonical(f.Type) == TypeReference && len(f.By) > len(ReferenceByFieldPrefix) && f.By[:len(ReferenceByFieldPrefix)] == ReferenceByFieldPrefix {
				seen[f.By[len(ReferenceByFieldPrefix):]] = true
			}
			walk(f.Fields)
			walk(f.Items)
		}
	}
	for _, coll := range c.Collections {
		walk(coll.Fields)
	}
	for _, b := range c.Blocks {
		walk(b.Fields)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}
