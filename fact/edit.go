package fact

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/norcubeplatform/cms-schema/schema"
)

// Apply writes one located fact into the bytes of its file and returns
// the new bytes; src itself is not changed. Only the lines the fact is
// about change. A fact the file already says returns src as it is.
func Apply(src []byte, e Edit) ([]byte, error) {
	f := e.Fact
	switch f.Kind {
	case KindRole:
		out, err := apply(src, e, nil, set{key: "role", value: scalarValue(f.Role), after: []string{"label", "type", "key", "name"}})
		if err != nil {
			return nil, err
		}
		if f.WhenEmpty != "" {
			return apply(out, e, nil, set{key: "whenEmpty", value: scalarValue(f.WhenEmpty), after: []string{"role"}})
		}
		if !schema.RoleAllowsWhenEmpty(f.Role) {
			// A whenEmpty left beside a role that takes none would be an
			// error.
			return apply(out, e, nil, set{key: "whenEmpty", remove: true})
		}
		return out, nil
	case KindTranslationLocales:
		return apply(src, e, []string{"translations"}, set{key: "locales", value: listValue(f.Locales), after: []string{"by", "localeFrom"}})
	case KindOptimizedAtBuild:
		return apply(src, e, nil, set{key: "optimizedAtBuild", value: boolValue(*f.OptimizedAtBuild), after: []string{"path", "type", "publicUrl"}})
	case KindUsedByCode:
		merged, err := mergedCodePaths(src, e)
		if err != nil {
			return nil, err
		}
		return apply(src, e, nil, set{key: schema.StorageKeyUsedByCode, value: listValue(merged), after: []string{"optimizedAtBuild", "publicUrl", "path", "type"}})
	}
	return nil, fmt.Errorf("%w: kind %q", ErrInvalid, f.Kind)
}

// mergedCodePaths is the storage's usedByCode as the file holds it now,
// with the fact's paths it lacks added at the end.
func mergedCodePaths(src []byte, e Edit) ([]string, error) {
	doc, err := topMapping(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.File, err)
	}
	m := doc
	for _, k := range e.In {
		if m = child(m, k); m == nil || m.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%w: %s has no mapping at %s", ErrNotFound, e.File, strings.Join(e.In, "."))
		}
	}
	var merged []string
	if list := child(m, schema.StorageKeyUsedByCode); list != nil && list.Kind == yaml.SequenceNode {
		for _, item := range list.Content {
			if item.Kind == yaml.ScalarNode && item.Value != "" && !slices.Contains(merged, item.Value) {
				merged = append(merged, item.Value)
			}
		}
	}
	for _, p := range e.Fact.Paths {
		if !slices.Contains(merged, p) {
			merged = append(merged, p)
		}
	}
	if len(merged) > schema.MaxUsedByCode {
		return nil, fmt.Errorf("%w: storage %q would list %d paths in usedByCode, at most %d", ErrInvalid, e.Fact.Storage, len(merged), schema.MaxUsedByCode)
	}
	return merged, nil
}

// value is what a key is set to: its YAML text and how to tell the
// file already holds it.
type value struct {
	text string
	same func(n *yaml.Node) bool
}

func scalarValue(s string) value {
	return value{text: yamlText(strNode(s)), same: func(n *yaml.Node) bool {
		return n.Kind == yaml.ScalarNode && n.Tag == "!!str" && n.Value == s
	}}
}

func boolValue(b bool) value {
	want := fmt.Sprint(b)
	return value{text: want, same: func(n *yaml.Node) bool {
		var got bool
		return n.Kind == yaml.ScalarNode && n.Tag == "!!bool" && n.Decode(&got) == nil && got == b
	}}
}

func listValue(items []string) value {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, s := range items {
		seq.Content = append(seq.Content, strNode(s))
	}
	return value{text: yamlText(seq), same: func(n *yaml.Node) bool {
		if n.Kind != yaml.SequenceNode || len(n.Content) != len(items) {
			return false
		}
		for i, c := range n.Content {
			if c.Kind != yaml.ScalarNode || c.Value != items[i] {
				return false
			}
		}
		return true
	}}
}

// oldScalars read as booleans or null to a YAML 1.1 parser, though
// yaml.v3 writes them plain: quoted, every parser reads a string.
var oldScalars = map[string]bool{"y": true, "yes": true, "n": true, "no": true, "on": true, "off": true, "true": true, "false": true, "null": true, "~": true}

func strNode(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if oldScalars[strings.ToLower(s)] {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

// yamlText is a node as YAML on one line, quoted where a plain value
// would read as something else.
func yamlText(n *yaml.Node) string {
	b, err := yaml.Marshal(n)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

// set puts key: value in a mapping, or removes the key. A new key goes
// on its own line after the first key of after that sits on one line
// (else after any such key).
type set struct {
	key    string
	value  value
	after  []string
	remove bool
}

// apply finds the mapping the edit is about, then the mapping at the
// keys of within it, and sets one key there.
func apply(src []byte, e Edit, within []string, op set) ([]byte, error) {
	doc, err := topMapping(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.File, err)
	}
	m := doc
	for _, k := range e.In {
		if m = child(m, k); m == nil || m.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%w: %s has no mapping at %s", ErrNotFound, e.File, strings.Join(e.In, "."))
		}
	}
	for _, st := range e.Steps {
		if m, err = step(m, st); err != nil {
			return nil, fmt.Errorf("%s: %w", e.File, err)
		}
	}
	for _, k := range within {
		if m = child(m, k); m == nil || m.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%w: %s has no %s mapping there", ErrUnplaceable, e.File, k)
		}
	}
	t := newText(src)
	if op.remove {
		return t.remove(m, op.key)
	}
	return t.set(m, op)
}

// step moves from a field's mapping into one of its fields, or into its
// list's item shape.
func step(m *yaml.Node, st Step) (*yaml.Node, error) {
	if !st.Item {
		fields := child(m, "fields")
		if fields == nil || fields.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%w: no fields list holds %q", ErrNotFound, st.Key)
		}
		var found []*yaml.Node
		for _, item := range fields.Content {
			if k := child(item, "key"); k != nil && k.Kind == yaml.ScalarNode && k.Value == st.Key {
				found = append(found, item)
			}
		}
		switch len(found) {
		case 0:
			return nil, fmt.Errorf("%w: no field has the key %q", ErrNotFound, st.Key)
		case 1:
		default:
			return nil, fmt.Errorf("%w: %d fields have the key %q", ErrAmbiguous, len(found), st.Key)
		}
		if child(found[0], schema.RefKey) != nil {
			return nil, fmt.Errorf("%w: field %q is a $ref", ErrAmbiguous, st.Key)
		}
		return found[0], nil
	}
	items := child(m, "items")
	var entries []*yaml.Node
	switch {
	case items == nil:
		return nil, fmt.Errorf("%w: the list declares no items", ErrNotFound)
	case items.Kind == yaml.MappingNode:
		entries = []*yaml.Node{items}
	case items.Kind == yaml.SequenceNode:
		entries = items.Content
	default:
		return nil, fmt.Errorf("%w: items is not a list", ErrNotFound)
	}
	var found []*yaml.Node
	for i, it := range entries {
		name := scalarAt(it, "name")
		if name == "" {
			name = scalarAt(it, "key")
		}
		if name == "" {
			name = fmt.Sprintf("__entry_%d", i)
		}
		if st.Shape == "" || st.Shape == name {
			found = append(found, it)
		}
	}
	switch {
	case len(found) == 0:
		return nil, fmt.Errorf("%w: item shape %q", ErrNotFound, st.Shape)
	case len(found) > 1:
		return nil, fmt.Errorf("%w: %d item shapes fit %q", ErrAmbiguous, len(found), st.Shape)
	case found[0].Kind != yaml.MappingNode || child(found[0], schema.RefKey) != nil:
		return nil, fmt.Errorf("%w: item shape %q is a $ref", ErrAmbiguous, st.Shape)
	}
	return found[0], nil
}

// text is a file's bytes with line offsets, to turn node positions into
// byte offsets.
type text struct {
	src   []byte
	lines []int
}

func newText(src []byte) text {
	t := text{src: src, lines: []int{0}}
	for i, c := range src {
		if c == '\n' {
			t.lines = append(t.lines, i+1)
		}
	}
	return t
}

// at is the byte offset of a node's 1-based line and column (columns
// count characters, as yaml.v3 does).
func (t text) at(n *yaml.Node) (int, error) {
	if n.Line < 1 || n.Line > len(t.lines) {
		return 0, fmt.Errorf("%w: no position", ErrUnplaceable)
	}
	off := t.lines[n.Line-1]
	for c := 1; c < n.Column; c++ {
		if off >= len(t.src) || t.src[off] == '\n' {
			return 0, fmt.Errorf("%w: no position", ErrUnplaceable)
		}
		_, w := utf8.DecodeRune(t.src[off:])
		off += w
	}
	return off, nil
}

// end is the byte offset just after a node's text.
func (t text) end(n *yaml.Node) (int, error) {
	start, err := t.at(n)
	if err != nil {
		return 0, err
	}
	switch {
	case n.Kind == yaml.ScalarNode:
		return t.scalarEnd(n, start)
	case n.Style&yaml.FlowStyle != 0:
		return t.flowEnd(start)
	case len(n.Content) > 0:
		return t.end(n.Content[len(n.Content)-1])
	}
	return 0, fmt.Errorf("%w: an empty value", ErrUnplaceable)
}

func (t text) scalarEnd(n *yaml.Node, start int) (int, error) {
	s := t.src
	switch {
	case n.Style&(yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) != 0:
		return 0, fmt.Errorf("%w: a value over several lines", ErrUnplaceable)
	case n.Style&yaml.DoubleQuotedStyle != 0:
		for i := start + 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
			case '"':
				return i + 1, nil
			case '\n':
				return 0, fmt.Errorf("%w: a value over several lines", ErrUnplaceable)
			}
		}
	case n.Style&yaml.SingleQuotedStyle != 0:
		for i := start + 1; i < len(s); i++ {
			switch {
			case s[i] == '\'' && i+1 < len(s) && s[i+1] == '\'':
				i++
			case s[i] == '\'':
				return i + 1, nil
			case s[i] == '\n':
				return 0, fmt.Errorf("%w: a value over several lines", ErrUnplaceable)
			}
		}
	default:
		if n.Value != "" && bytes.HasPrefix(s[start:], []byte(n.Value)) {
			return start + len(n.Value), nil
		}
		return 0, fmt.Errorf("%w: a value over several lines", ErrUnplaceable)
	}
	return 0, fmt.Errorf("%w: an unterminated value", ErrUnplaceable)
}

// flowEnd is the offset after the bracket that closes the flow
// collection opening at start.
func (t text) flowEnd(start int) (int, error) {
	s := t.src
	if start >= len(s) || (s[start] != '[' && s[start] != '{') {
		return 0, fmt.Errorf("%w: no flow collection", ErrUnplaceable)
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '\'':
			for i++; i < len(s); i++ {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '#':
			if i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				for i < len(s) && s[i] != '\n' {
					i++
				}
			}
		}
	}
	return 0, fmt.Errorf("%w: an unterminated flow collection", ErrUnplaceable)
}

// restIsBlank reports whether nothing but spaces or a comment follows
// off on its line, and where the next line starts.
func (t text) restIsBlank(off int) (bool, int) {
	i := off
	for i < len(t.src) && (t.src[i] == ' ' || t.src[i] == '\t') {
		i++
	}
	ok := i >= len(t.src) || t.src[i] == '\n' || t.src[i] == '\r' || (t.src[i] == '#' && i > off)
	for i < len(t.src) && t.src[i] != '\n' {
		i++
	}
	if i < len(t.src) {
		i++
	}
	return ok, i
}

func splice(src []byte, from, to int, with string) []byte {
	out := make([]byte, 0, len(src)-(to-from)+len(with))
	out = append(out, src[:from]...)
	out = append(out, with...)
	return append(out, src[to:]...)
}

func (t text) set(m *yaml.Node, op set) ([]byte, error) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != op.key {
			continue
		}
		if op.value.same(v) {
			return t.src, nil
		}
		if v.Kind == yaml.ScalarNode && v.Tag == "!!null" && v.Value == "" {
			// `key:` with nothing after it.
			kEnd, err := t.end(k)
			if err != nil {
				return nil, err
			}
			colon := bytes.IndexByte(t.src[kEnd:], ':')
			if colon < 0 {
				return nil, fmt.Errorf("%w: no colon after %s", ErrUnplaceable, op.key)
			}
			at := kEnd + colon + 1
			return splice(t.src, at, at, " "+op.value.text), nil
		}
		vEnd, err := t.end(v)
		if err != nil {
			return nil, err
		}
		if v.Kind == yaml.ScalarNode || v.Style&yaml.FlowStyle != 0 {
			vStart, err := t.at(v)
			if err != nil {
				return nil, err
			}
			return splice(t.src, vStart, vEnd, op.value.text), nil
		}
		// A block list or mapping: the pair becomes one line.
		kStart, err := t.at(k)
		if err != nil {
			return nil, err
		}
		return splice(t.src, kStart, vEnd, op.key+": "+op.value.text), nil
	}
	return t.insert(m, op)
}

// insert adds key: value to a mapping that lacks the key.
func (t text) insert(m *yaml.Node, op set) ([]byte, error) {
	if len(m.Content) == 0 {
		return nil, fmt.Errorf("%w: an empty mapping", ErrUnplaceable)
	}
	order := []int{}
	for _, a := range op.after {
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == a {
				order = append(order, i)
			}
		}
	}
	for i := len(m.Content) - 2; i >= 0; i -= 2 {
		order = append(order, i)
	}
	flow := m.Style&yaml.FlowStyle != 0
	for _, i := range order {
		k, v := m.Content[i], m.Content[i+1]
		if v.Kind != yaml.ScalarNode && v.Style&yaml.FlowStyle == 0 {
			continue
		}
		if v.Kind == yaml.ScalarNode && v.Value == "" && v.Tag == "!!null" {
			continue
		}
		vEnd, err := t.end(v)
		if err != nil {
			continue
		}
		if flow {
			return splice(t.src, vEnd, vEnd, ", "+op.key+": "+op.value.text), nil
		}
		blank, next := t.restIsBlank(vEnd)
		if !blank {
			continue
		}
		line := strings.Repeat(" ", k.Column-1) + op.key + ": " + op.value.text + "\n"
		src := t.src
		if next == len(src) && (len(src) == 0 || src[len(src)-1] != '\n') {
			src = append(append([]byte(nil), src...), '\n')
			next = len(src)
		}
		return splice(src, next, next, line), nil
	}
	return nil, fmt.Errorf("%w: no key on a line of its own to put %s after", ErrUnplaceable, op.key)
}

func (t text) remove(m *yaml.Node, key string) ([]byte, error) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != key {
			continue
		}
		kStart, err := t.at(k)
		if err != nil {
			return nil, err
		}
		vEnd := 0
		if v.Kind == yaml.ScalarNode && v.Value == "" && v.Tag == "!!null" {
			kEnd, err := t.end(k)
			if err != nil {
				return nil, err
			}
			vEnd = kEnd + 1
		} else if vEnd, err = t.end(v); err != nil {
			return nil, err
		}
		if m.Style&yaml.FlowStyle != 0 {
			// Take the comma before the pair, else the one after it.
			j := kStart - 1
			for j >= 0 && (t.src[j] == ' ' || t.src[j] == '\t' || t.src[j] == '\n') {
				j--
			}
			if j >= 0 && t.src[j] == ',' {
				return splice(t.src, j, vEnd, ""), nil
			}
			j = vEnd
			for j < len(t.src) && (t.src[j] == ' ' || t.src[j] == '\t') {
				j++
			}
			if j < len(t.src) && t.src[j] == ',' {
				j++
				for j < len(t.src) && t.src[j] == ' ' {
					j++
				}
			}
			return splice(t.src, kStart, j, ""), nil
		}
		lineStart := t.lines[k.Line-1]
		if strings.TrimLeft(string(t.src[lineStart:kStart]), " ") != "" {
			return nil, fmt.Errorf("%w: %s shares its line with another key", ErrUnplaceable, key)
		}
		blank, next := t.restIsBlank(vEnd)
		if !blank {
			return nil, fmt.Errorf("%w: something follows %s on its line", ErrUnplaceable, key)
		}
		return splice(t.src, lineStart, next, ""), nil
	}
	return t.src, nil
}

// topMapping parses a schema file and returns its top mapping.
func topMapping(src []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnplaceable, err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		return doc.Content[0], nil
	}
	return nil, fmt.Errorf("%w: the file is not a mapping", ErrUnplaceable)
}

// child is the value at key in a mapping, or nil.
func child(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalarAt(m *yaml.Node, key string) string {
	if v := child(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}
