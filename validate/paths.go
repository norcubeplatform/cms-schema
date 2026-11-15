package validate

import (
	"strconv"
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

// schemaScope is the static counterpart of schema.Scope: instead of
// values it holds the field lists a condition path can reach from a
// field's position in the schema, so a `visibleIf` or `computed.from`
// path can be checked without content.
type schemaScope struct {
	self    []schema.Field
	parents [][]schema.Field
	root    []schema.Field
	item    []schema.Field
	hasItem bool
}

func rootSchemaScope(fields []schema.Field) schemaScope {
	return schemaScope{self: fields, root: fields}
}

func (s schemaScope) child(fields []schema.Field) schemaScope {
	return schemaScope{self: fields, parents: append([][]schema.Field{s.self}, s.parents...), root: s.root, item: s.item, hasItem: s.hasItem}
}

func (s schemaScope) itemScope(fields []schema.Field) schemaScope {
	return schemaScope{self: fields, parents: append([][]schema.Field{s.self}, s.parents...), root: s.root, item: fields, hasItem: true}
}

// pathResolves reports whether a condition path names a field that
// exists in the schema at the path's scope. The second result is false
// when the answer cannot be known statically (the path indexes into an
// array or crosses a field without declared children), in which case
// the caller stays quiet.
func (s schemaScope) pathResolves(path string) (resolves, known bool) {
	var parts []string
	for _, p := range strings.Split(path, ".") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return false, true
	}
	fields := s.self
	i := 0
	depth := 0
	for i < len(parts) && parts[i] == "$parent" {
		depth++
		i++
	}
	switch {
	case depth > 0:
		if depth-1 >= len(s.parents) {
			return false, true
		}
		fields = s.parents[depth-1]
	case i < len(parts) && parts[i] == "$root":
		fields = s.root
		i++
	case i < len(parts) && parts[i] == "$item":
		if !s.hasItem {
			return false, true
		}
		fields = s.item
		i++
	}
	if i >= len(parts) {
		// `$root` or `$item` alone names the scope itself.
		return true, true
	}
	for ; i < len(parts); i++ {
		seg := parts[i]
		if _, err := strconv.Atoi(seg); err == nil {
			return true, false
		}
		var found *schema.Field
		for j := range fields {
			if fields[j].Key == seg {
				found = &fields[j]
				break
			}
		}
		if found == nil {
			return false, true
		}
		if i == len(parts)-1 {
			return true, true
		}
		switch {
		case schema.IsGroup(found.Type):
			fields = found.Fields
		case schema.IsArray(found.Type):
			return true, false
		default:
			// A scalar with a dotted continuation: nothing to descend into.
			return false, true
		}
	}
	return true, true
}
