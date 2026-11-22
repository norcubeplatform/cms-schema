package schema

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Parse reads a merged configuration document (the shape the loader
// produces and the editor receives: a map with `collections`, `blocks`,
// `inputs`, `storages`, `preview` and the root keys) into a Config.
//
// Parse is lenient: a malformed knob is kept in Field.Extra or ignored,
// never fatal, because the validators report malformed knobs with a
// file and a path. The only error is a document that is not a mapping.
func Parse(doc map[string]any) (*Config, error) {
	if doc == nil {
		doc = map[string]any{}
	}
	c := &Config{
		SchemaVersion: CurrentVersion,
		PublicDir:     "public",
		Collections:   map[string]Collection{},
		Blocks:        map[string]Block{},
		Inputs:        map[string]Field{},
		Raw:           doc,
	}
	if v, ok := intOf(doc["schemaVersion"]); ok {
		c.SchemaVersion = v
	}
	c.SiteURL = strings.TrimRight(strings.TrimSpace(str(doc["siteUrl"])), "/")
	c.PagesDir = cleanPath(str(doc["pagesDir"]))
	if pd := strings.Trim(str(doc["publicDir"]), "/"); pd != "" {
		c.PublicDir = pd
	}
	c.DefaultStorage = str(doc["defaultStorage"])
	c.InputsFromGlob = stringList(doc["inputsFromGlob"])
	c.KnownPaths = trimmedStrings(doc["knownPaths"])

	if p, ok := doc["preview"].(map[string]any); ok {
		c.Preview = PreviewSite{
			DefaultLocale:       str(p["defaultLocale"]),
			Locales:             trimmedStrings(p["locales"]),
			PrefixDefaultLocale: boolOf(p["prefixDefaultLocale"]),
		}
	}
	if r, ok := doc["redirects"].(map[string]any); ok {
		c.Redirects = Redirects{File: cleanPath(str(r["file"])), Format: strings.ToLower(str(r["format"]))}
		if c.Redirects.File != "" && c.Redirects.Format == "" {
			c.Redirects.Format = RedirectsFormatNetlify
		}
	}
	if v, ok := doc["variables"].(map[string]any); ok {
		c.Variables = Variables{File: cleanPath(str(v["file"])), Field: str(v["field"])}
		if c.Variables.File != "" && c.Variables.Field == "" {
			c.Variables.Field = "variables"
		}
	}

	c.Storages = parseStorages(doc["storages"])

	for _, key := range sortedKeys(doc["blocks"]) {
		m, _ := mapOf(doc["blocks"])[key].(map[string]any)
		if m == nil {
			continue
		}
		c.Blocks[key] = Block{
			Key:         key,
			Label:       str(m["label"]),
			Description: str(m["description"]),
			Fields:      ParseFields(m["fields"]),
			Preview:     parsePreview(m["preview"]),
			Raw:         m,
		}
		c.BlockKeys = append(c.BlockKeys, key)
	}
	for _, key := range sortedKeys(doc["inputs"]) {
		m, _ := mapOf(doc["inputs"])[key].(map[string]any)
		if m == nil {
			continue
		}
		c.Inputs[key] = ParseField(m)
		c.InputKeys = append(c.InputKeys, key)
	}
	for _, key := range sortedKeys(doc["collections"]) {
		m, _ := mapOf(doc["collections"])[key].(map[string]any)
		if m == nil {
			continue
		}
		coll := Collection{
			Key:             key,
			Label:           str(m["label"]),
			Description:     str(m["description"]),
			Path:            cleanPath(str(m["path"])),
			Match:           str(m["match"]),
			HasBody:         boolOf(m["hasBody"]),
			ExtraKeysPolicy: str(m["extraKeysPolicy"]),
			Area:            str(m["area"]),
			Preview:         parsePreview(m["preview"]),
			PreviewPath:     str(m["previewPath"]),
			LocaleFrom:      str(m["localeFrom"]),
			Translations:    parseTranslations(m["translations"]),
			Fields:          ParseFields(m["fields"]),
			Raw:             m,
		}
		if v, ok := intOf(m["schemaVersion"]); ok {
			coll.SchemaVersion = &v
		}
		if coll.ExtraKeysPolicy == "" {
			coll.ExtraKeysPolicy = ExtraKeysShow
		}
		if coll.Area == "" {
			coll.Area = AreaContent
		}
		if coll.Match == "" {
			coll.Match = "**/*.md"
		}
		c.Collections[key] = coll
		c.CollectionKeys = append(c.CollectionKeys, key)
	}
	// The default `pages` collection may exist only through pagesDir.
	if _, ok := c.Collections[DefaultCollection]; !ok && c.PagesDir != "" && len(c.Collections) == 0 {
		c.Collections[DefaultCollection] = Collection{Key: DefaultCollection, Path: c.PagesDir, Match: "**/*.md", ExtraKeysPolicy: ExtraKeysShow, Area: AreaContent}
		c.CollectionKeys = append(c.CollectionKeys, DefaultCollection)
	}
	if coll, ok := c.Collections[DefaultCollection]; ok && coll.Path == "" && c.PagesDir != "" {
		coll.Path = c.PagesDir
		c.Collections[DefaultCollection] = coll
	}
	return c, nil
}

// ParseFields reads a `fields` or `items` list.
func ParseFields(v any) []Field {
	switch list := v.(type) {
	case []any:
		out := make([]Field, 0, len(list))
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, ParseField(m))
		}
		return out
	case map[string]any:
		// `items: {…}` is one shape.
		return []Field{ParseField(list)}
	}
	return nil
}

// knownKnobs lists, per canonical type, the keys the editor reads on a
// field of that type in addition to the common ones.
var commonKnobs = map[string]bool{
	"key": true, "type": true, "label": true, "description": true, "name": true,
	"required": true, "requiredMessage": true, "nullable": true, "hidden": true,
	"visibleIf": true, "whenHidden": true, "computed": true, "customValidators": true,
	"default": true, "_blockRef": true, "schemaVersion": true, "translatable": true,
	"role": true,
}

// parseTranslations reads a collection's `translations` block; nil when
// there is none. A block that is present but malformed is kept (with an
// empty By) so validation can report it.
func parseTranslations(v any) *Translations {
	if v == nil {
		return nil
	}
	m, _ := v.(map[string]any)
	return &Translations{By: strings.TrimSpace(str(m["by"])), Locales: trimmedStrings(m["locales"])}
}

var typeKnobs = map[string][]string{
	TypeText:        {"variables", "regex", "regexMessage", "minLength", "minLengthMessage", "maxLength", "maxLengthMessage", "recommendedLength"},
	TypeParagraph:   {"variables", "rows", "regex", "regexMessage", "minLength", "minLengthMessage", "maxLength", "maxLengthMessage", "recommendedLength"},
	TypeRichText:    {"format", "options", "minLength", "minLengthMessage", "maxLength", "maxLengthMessage", "recommendedLength"},
	TypeNumber:      {"min", "max", "step"},
	TypeBoolean:     {},
	TypeDate:        {},
	TypeColor:       {"format"},
	TypeRange:       {"min", "max", "step"},
	TypeMultiChoice: {"options", "min", "max", "widget"},
	TypeLink:        {"variables", "regex", "regexMessage", "minLength", "minLengthMessage", "maxLength", "maxLengthMessage"},
	TypeImage:       {"accept", "maxSize", "storage", "dir", "lockDir", "alt", "decorative"},
	TypeFile:        {"accept", "maxSize", "storage", "dir", "lockDir"},
	TypeReference:   {"collection", "by", "multiple"},
	TypeSelect:      {"options", "widget", "searchable", "searchPlaceholder"},
	TypeGroup:       {"fields", "preview", "inline"},
	TypeArray:       {"items", "widget", "idKey", "minItems", "minItemsMessage", "maxItems", "maxItemsMessage", "uniqueOn", "uniqueOnMessage", "components", "fields", "preview"},
}

// KnownKnobs returns the keys a field of type t may carry.
func KnownKnobs(t string) map[string]bool {
	out := map[string]bool{}
	for k := range commonKnobs {
		out[k] = true
	}
	for _, k := range typeKnobs[Canonical(t)] {
		out[k] = true
	}
	return out
}

// ParseField reads one field definition.
func ParseField(m map[string]any) Field {
	f := Field{
		Key:               str(m["key"]),
		Type:              str(m["type"]),
		Label:             str(m["label"]),
		Description:       str(m["description"]),
		Name:              str(m["name"]),
		BlockRef:          str(m[BlockRefKey]),
		Required:          boolOf(m["required"]),
		RequiredMessage:   str(m["requiredMessage"]),
		Nullable:          boolOf(m["nullable"]),
		Hidden:            boolOf(m["hidden"]),
		Translatable:      boolPtr(m["translatable"]),
		WhenHidden:        str(m["whenHidden"]),
		Regex:             str(m["regex"]),
		RegexMessage:      str(m["regexMessage"]),
		MinLengthMessage:  str(m["minLengthMessage"]),
		MaxLengthMessage:  str(m["maxLengthMessage"]),
		Format:            str(m["format"]),
		Widget:            str(m["widget"]),
		Searchable:        boolOf(m["searchable"]),
		SearchPlaceholder: str(m["searchPlaceholder"]),
		Accept:            str(m["accept"]),
		Storage:           str(m["storage"]),
		Dir:               str(m["dir"]),
		LockDir:           boolOf(m["lockDir"]),
		Role:              strings.TrimSpace(str(m["role"])),
		Collection:        str(m["collection"]),
		By:                str(m["by"]),
		Multiple:          boolOf(m["multiple"]),
		Inline:            boolOf(m["inline"]),
		IDKey:             str(m["idKey"]),
		MinItemsMessage:   str(m["minItemsMessage"]),
		MaxItemsMessage:   str(m["maxItemsMessage"]),
		UniqueOn:          str(m["uniqueOn"]),
		UniqueOnMessage:   str(m["uniqueOnMessage"]),
		Raw:               m,
	}
	if d, ok := m["default"]; ok {
		f.Default, f.HasDefault = d, true
	}
	if v, ok := m["variables"].(bool); ok {
		f.Variables = &v
	}
	f.MinLength = intPtr(m["minLength"])
	f.MaxLength = intPtr(m["maxLength"])
	f.Rows = intPtr(m["rows"])
	f.MinItems = intPtr(m["minItems"])
	f.MaxItems = intPtr(m["maxItems"])
	f.Min = floatPtr(m["min"])
	f.Max = floatPtr(m["max"])
	f.Step = floatPtr(m["step"])
	if ms, ok := numOf(m["maxSize"]); ok {
		f.MaxSize = int64(ms)
	}
	if rl, ok := m["recommendedLength"].(map[string]any); ok {
		f.RecommendedLength = &RecommendedLength{From: intPtr(rl["from"]), To: intPtr(rl["to"])}
	}
	if opts, ok := m["options"].(map[string]any); ok && Canonical(f.Type) == TypeRichText {
		f.RichTextOptions = opts
	} else {
		f.Options = ParseOptions(m["options"])
	}
	if c := parseCondition(m["visibleIf"]); c != nil {
		f.VisibleIf = c
	}
	if cm, ok := m["computed"].(map[string]any); ok {
		f.Computed = &Computed{From: str(cm["from"]), Transform: str(cm["transform"]), Mode: str(cm["mode"])}
	}
	if list, ok := m["customValidators"].([]any); ok {
		for _, item := range list {
			cm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			cv := CustomValidator{Type: str(cm["type"]), Message: str(cm["message"]), Params: map[string]any{}}
			for k, v := range cm {
				if k != "type" && k != "message" {
					cv.Params[k] = v
				}
			}
			f.Custom = append(f.Custom, cv)
		}
	}
	if Canonical(f.Type) == TypeImage {
		f.Alt = strings.TrimSpace(str(m["alt"]))
		f.Decorative = boolOf(m["decorative"])
	}
	f.Fields = ParseFields(m["fields"])
	f.Items = ParseFields(m["items"])
	f.Preview = parsePreview(m["preview"])
	f.Components = trimmedStrings(m["components"])
	if f.By == "" && Canonical(f.Type) == TypeReference {
		f.By = ReferenceByPath
	}

	known := KnownKnobs(f.Type)
	for k, v := range m {
		if !known[k] {
			if f.Extra == nil {
				f.Extra = map[string]any{}
			}
			f.Extra[k] = v
		}
	}
	return f
}

// ParseOptions normalises a select's options (strings or {value, label,
// description} maps) to SelectOption.
func ParseOptions(v any) []SelectOption {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]SelectOption, 0, len(list))
	for _, item := range list {
		switch o := item.(type) {
		case string:
			out = append(out, SelectOption{Value: o})
		case map[string]any:
			out = append(out, SelectOption{Value: scalarString(o["value"]), Label: str(o["label"]), Description: str(o["description"])})
		default:
			if s := scalarString(o); s != "" {
				out = append(out, SelectOption{Value: s})
			}
		}
	}
	return out
}

// OptionValues returns the option values in order.
func OptionValues(opts []SelectOption) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Value
	}
	return out
}

func parseCondition(v any) *Condition {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if list, ok := m["all"].([]any); ok {
		c := &Condition{}
		for _, item := range list {
			if sub := parseCondition(item); sub != nil {
				c.All = append(c.All, *sub)
			}
		}
		return c
	}
	if list, ok := m["any"].([]any); ok {
		c := &Condition{}
		for _, item := range list {
			if sub := parseCondition(item); sub != nil {
				c.Any = append(c.Any, *sub)
			}
		}
		return c
	}
	if sub, ok := m["not"]; ok {
		c := &Condition{Not: parseCondition(sub)}
		if c.Not == nil {
			c.Not = &Condition{}
		}
		return c
	}
	c := &Condition{Field: str(m["field"]), Op: str(m["op"]), Value: m["value"]}
	// The older `equals:` spelling means `op: eq`.
	if c.Op == "" {
		if eq, ok := m["equals"]; ok {
			c.Op, c.Value = OpEq, eq
		}
	}
	return c
}

func parsePreview(v any) *Preview {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	p := &Preview{
		Title:       parseResolvers(m["title"]),
		Description: parseResolvers(m["description"]),
		Image:       parseResolvers(m["image"]),
	}
	if badges, ok := m["badges"].([]any); ok {
		for _, chip := range badges {
			if rs := parseResolvers(chip); len(rs) > 0 {
				p.Badges = append(p.Badges, rs)
			}
		}
	}
	return p
}

func parseResolvers(v any) []Resolver {
	switch r := v.(type) {
	case string:
		// A bare string is a single key (the group `preview: {title: heading}` shorthand).
		return []Resolver{{Key: r}}
	case []any:
		var out []Resolver
		for _, item := range r {
			switch x := item.(type) {
			case string:
				out = append(out, Resolver{Literal: x})
			case map[string]any:
				if k := str(x["key"]); k != "" {
					out = append(out, Resolver{Key: k})
				}
			}
		}
		return out
	}
	return nil
}

func parseStorages(v any) []Storage {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	var out []Storage
	for _, name := range sortedKeys(m) {
		entry, ok := m[name].(map[string]any)
		if !ok {
			out = append(out, Storage{Name: name, Raw: map[string]any{}})
			continue
		}
		s := Storage{
			Name:   name,
			Type:   str(entry["type"]),
			Mode:   str(entry["mode"]),
			Accept: str(entry["accept"]),
			Naming: str(entry["naming"]),
			Inline: str(entry["inline"]),
			Dedup:  str(entry["dedup"]),
			Path:   strings.Trim(strings.TrimSpace(str(entry["path"])), "/"),
			Raw:    entry,
		}
		if s.Mode == "" {
			s.Mode = StorageModeReadWrite
		}
		if ms, ok := numOf(entry["max_size"]); ok {
			s.MaxSize = int64(ms)
		}
		pu := str(entry["publicUrl"])
		if pu == "" {
			pu = str(entry["public_url"])
		}
		s.PublicURL = strings.TrimRight(strings.TrimSpace(pu), "/")
		out = append(out, s)
	}
	return out
}

// --- small helpers -----------------------------------------------------

func str(v any) string {
	s, _ := v.(string)
	return s
}

// scalarString renders a scalar option value as the string the editor
// compares against (numbers and booleans included).
func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int, int64, float64:
		n, _ := numOf(x)
		if n == math.Trunc(n) {
			return fmt.Sprintf("%d", int64(n))
		}
		return fmt.Sprintf("%v", n)
	}
	return ""
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

func numOf(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}

func intOf(v any) (int, bool) {
	n, ok := numOf(v)
	if !ok || n != math.Trunc(n) {
		return 0, false
	}
	return int(n), true
}

func intPtr(v any) *int {
	if n, ok := intOf(v); ok {
		return &n
	}
	return nil
}

func floatPtr(v any) *float64 {
	if n, ok := numOf(v); ok {
		return &n
	}
	return nil
}

func mapOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func sortedKeys(v any) []string {
	m := mapOf(v)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortStrings(s []string) { sort.Strings(s) }

func stringList(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []any:
		return trimmedStrings(x)
	}
	return nil
}

func trimmedStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func cleanPath(p string) string {
	return strings.Trim(strings.TrimSpace(p), "/")
}

// boolPtr is a boolean knob that may be absent: nil unless the value is a
// boolean.
func boolPtr(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}
