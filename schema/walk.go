package schema

import "strconv"

// MaxWalkDepth bounds how many groups and list items Walk enters below
// the top level. Real content is a few levels deep; the bound keeps a
// crafted document from deciding how much work a walk is.
const MaxWalkDepth = 32

// Visit is one schema field at one position in a document.
type Visit struct {
	// Field is the definition. For an item of a list of primitives it is
	// the list's item shape, which usually has no key or label of its own.
	Field Field
	// Value is what the document holds there; Has is false when the key
	// is absent (fields of a present container are visited either way).
	Value any
	Has   bool
	// Path is where the value sits: `a.b[2].c`.
	Path string
	// Depth counts the groups and list items entered to get here; 0 at
	// the top level.
	Depth int
	// Ancestors are the enclosing fields, outermost first: groups, lists,
	// and the item shape of a list item when it is declared as a field.
	// The slice is shared between visits and must not be changed or kept.
	Ancestors []Field
	// Item is true when Value is one item of a list of primitives.
	Item bool
	// Scope is what a condition on the field can reach.
	Scope Scope
}

// Parent returns the nearest enclosing field, or nil at the top level.
func (v Visit) Parent() *Field {
	if len(v.Ancestors) == 0 {
		return nil
	}
	return &v.Ancestors[len(v.Ancestors)-1]
}

// Walk visits every field of a document with its definition, in schema
// order and list items by index, parents before their children. It
// enters groups and list items (blocks by `_component`, multi-shape
// lists by their discriminator, single-shape lists by their one shape)
// and visits each item of a list of primitives with the list's item
// shape. Returning false from visit skips what is inside that field.
// Fields without a key are skipped; nothing below MaxWalkDepth is
// visited.
func Walk(cfg *Config, fields []Field, value map[string]any, visit func(Visit) (descend bool)) {
	walk(cfg, fields, value, "", RootScope(value), nil, 0, visit)
}

func walk(cfg *Config, fields []Field, value map[string]any, base string, scope Scope, ancestors []Field, depth int, visit func(Visit) bool) {
	for _, fd := range fields {
		if fd.Key == "" {
			continue
		}
		p := JoinPath(base, fd.Key)
		v, has := value[fd.Key]
		if !visit(Visit{Field: fd, Value: v, Has: has, Path: p, Depth: depth, Ancestors: ancestors, Scope: scope}) || depth >= MaxWalkDepth {
			continue
		}
		switch {
		case IsGroup(fd.Type):
			if m, ok := v.(map[string]any); ok {
				walk(cfg, fd.Fields, m, p, scope.Child(m), under(ancestors, fd), depth+1, visit)
			}
		case IsArray(fd.Type):
			items, ok := v.([]any)
			if !ok {
				continue
			}
			for i, item := range items {
				ip := IndexPath(p, i)
				if m, ok := item.(map[string]any); ok {
					sub, shape := itemShape(cfg, fd, m)
					if sub == nil {
						continue
					}
					inside := under(ancestors, fd)
					if shape != nil {
						inside = append(inside, *shape)
					}
					walk(cfg, sub, m, ip, scope.ItemScope(m), inside, depth+1, visit)
					continue
				}
				if shape := primitiveShape(fd, item); shape != nil {
					visit(Visit{Field: *shape, Value: item, Has: true, Path: ip, Depth: depth + 1, Ancestors: under(ancestors, fd), Item: true, Scope: scope})
				}
			}
		}
	}
}

// under returns ancestors with fd added, never writing into the
// caller's slice: siblings share it.
func under(ancestors []Field, fd Field) []Field {
	return append(ancestors[:len(ancestors):len(ancestors)], fd)
}

// primitiveShape is the shape of a list item that is not a mapping, or
// nil when the list has none for it.
func primitiveShape(f Field, item any) *Field {
	if item == nil || f.Type == TypeBlocks || len(f.Components) > 0 {
		return nil
	}
	e := ResolveItemEntry(f, item)
	if e == nil || PrimitiveBucket(e.Field.Type) == "" {
		return nil
	}
	return &e.Field
}

// JoinPath appends a field key to a content path.
func JoinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// IndexPath appends a list index to a content path.
func IndexPath(base string, i int) string {
	return base + "[" + strconv.Itoa(i) + "]"
}
