package schema

import "fmt"

// Entry is one allowed shape of an array field, with the name items
// carry in their discriminator.
type Entry struct {
	Name  string
	Field Field
}

// ItemEntries lists an array field's shapes. The name is the entry's
// `name`, else its `key`, else a synthetic `__entry_<i>` that no item
// can match (the validator reports those).
func ItemEntries(f Field) []Entry {
	out := make([]Entry, len(f.Items))
	for i, it := range f.Items {
		name := it.Name
		if name == "" {
			name = it.Key
		}
		if name == "" {
			name = fmt.Sprintf("__entry_%d", i)
		}
		out[i] = Entry{Name: name, Field: it}
	}
	return out
}

// IsMultiShape reports whether the array accepts more than one shape.
func IsMultiShape(f Field) bool { return len(f.Items) > 1 }

// ArrayIDKey is the discriminator key the array's items carry.
func ArrayIDKey(f Field) string {
	if f.IDKey != "" {
		return f.IDKey
	}
	if Canonical(f.Type) == TypeArray && f.Type == TypeBlocks {
		return BlockComponentKey
	}
	return ArrayItemTypeKey
}

// ResolveItemEntry picks the shape of one item: by discriminator for
// mappings, by value type for primitives, the lone shape for
// single-shape arrays. A mapping without a discriminator falls back to
// the first group shape, as the editor does. Returns nil when nothing
// fits.
func ResolveItemEntry(f Field, item any) *Entry {
	entries := ItemEntries(f)
	if len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 {
		return &entries[0]
	}
	if m, ok := item.(map[string]any); ok {
		if tag, ok := m[ArrayIDKey(f)].(string); ok {
			for i := range entries {
				if entries[i].Name == tag {
					return &entries[i]
				}
			}
		}
		for i := range entries {
			if IsGroup(entries[i].Field.Type) {
				return &entries[i]
			}
		}
		return nil
	}
	accepts := primitiveAcceptor(item)
	if accepts == nil {
		return nil
	}
	var match *Entry
	for i := range entries {
		if accepts(entries[i].Field.Type) {
			if match != nil {
				return nil // ambiguous
			}
			match = &entries[i]
		}
	}
	return match
}

// PrimitiveBucket is the value type a primitive field type produces:
// "string", "number", "boolean", or "" for groups and arrays.
func PrimitiveBucket(t string) string {
	switch Canonical(t) {
	case TypeText, TypeParagraph, TypeRichText, TypeLink, TypeDate, TypeImage, TypeFile, TypeSelect, TypeColor, TypeReference:
		return "string"
	case TypeNumber, TypeRange:
		return "number"
	case TypeBoolean:
		return "boolean"
	}
	return ""
}

func primitiveAcceptor(v any) func(string) bool {
	switch v.(type) {
	case string:
		return func(t string) bool { return PrimitiveBucket(t) == "string" }
	case int, int64, float64:
		return func(t string) bool { return PrimitiveBucket(t) == "number" }
	case bool:
		return func(t string) bool { return PrimitiveBucket(t) == "boolean" }
	}
	return nil
}

// ItemFields returns the fields of the shape one array item has, or nil
// for a primitive shape or an item no shape fits. Legacy `blocks`
// fields look the component up in the config's blocks.
func ItemFields(c *Config, f Field, item map[string]any) []Field {
	if f.Type == TypeBlocks || len(f.Components) > 0 {
		name, _ := item[BlockComponentKey].(string)
		if c != nil {
			if b, ok := c.Blocks[name]; ok {
				return b.Fields
			}
		}
		return nil
	}
	e := ResolveItemEntry(f, item)
	if e == nil || !IsGroup(e.Field.Type) {
		return nil
	}
	return e.Field.Fields
}
