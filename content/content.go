// Package content reads content files the way the CMS does: Markdown
// with a YAML frontmatter fence, or a whole-file YAML data document, into
// plain Go values (maps, slices, strings, numbers, booleans) that match
// what the editor sees after its YAML to JSON round trip.
package content

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Extensions of files the CMS treats as content.
var Extensions = []string{".md", ".mdx", ".markdown", ".yml", ".yaml", ".json"}

// IsContentFile reports whether the path has a content extension.
func IsContentFile(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	for _, e := range Extensions {
		if ext == e {
			return true
		}
	}
	return false
}

// IsDataFile reports whether the path is a whole-file YAML data document
// rather than Markdown with a frontmatter fence.
func IsDataFile(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".yml", ".yaml":
		return true
	}
	return false
}

// Document is one parsed content file.
type Document struct {
	Path string
	// Frontmatter is the YAML head (or the whole data file) as plain values.
	Frontmatter map[string]any
	// Body is the Markdown after the fence; empty for data files.
	Body string
	// HasFrontmatter is false for a Markdown file without a fence.
	HasFrontmatter bool
	// IsData is true for a whole-file YAML document.
	IsData bool
}

// Parse reads one content file. A parse error in the YAML is returned;
// the caller decides whether that is a finding or fatal.
func Parse(p string, raw []byte) (Document, error) {
	doc := Document{Path: p, Frontmatter: map[string]any{}}
	text := string(raw)
	if IsDataFile(p) {
		doc.IsData = true
		if strings.TrimSpace(text) == "" {
			return doc, nil
		}
		fm, err := ParseYAMLMap([]byte(text))
		if err != nil {
			return doc, err
		}
		doc.Frontmatter, doc.HasFrontmatter = fm, true
		return doc, nil
	}
	head, body, has := SplitFrontmatter(text)
	doc.Body = body
	if !has {
		return doc, nil
	}
	doc.HasFrontmatter = true
	fm, err := ParseYAMLMap([]byte(head))
	if err != nil {
		return doc, err
	}
	doc.Frontmatter = fm
	return doc, nil
}

// SplitFrontmatter pulls the YAML head out of a Markdown file. Leading
// whitespace before the opening fence is accepted. The third result is
// false, and the whole text is the body, when there is no fence.
func SplitFrontmatter(raw string) (frontmatter, body string, has bool) {
	trimmed := strings.TrimLeft(raw, " \r\n\t")
	if !strings.HasPrefix(trimmed, "---") {
		return "", raw, false
	}
	rest := strings.TrimPrefix(trimmed, "---")
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")
	closer := strings.Index(rest, "\n---")
	if closer < 0 {
		return "", raw, false
	}
	frontmatter = rest[:closer]
	body = rest[closer+len("\n---"):]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body, true
}

// ParseYAMLMap parses a YAML document into a string-keyed map of plain
// values. Scalars keep the type their tag gives them: integers and
// floats become float64 (as JSON would), booleans stay booleans, null is
// nil, and everything else, timestamps included, stays the text as
// written. An empty document is an empty map; a document whose root is
// not a mapping is an error.
func ParseYAMLMap(raw []byte) (map[string]any, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	if node.Kind == 0 || len(node.Content) == 0 {
		return map[string]any{}, nil
	}
	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("document root is a %s, not a mapping", kindName(root.Kind))
	}
	v, err := NodeToValue(root)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// NodeToValue converts a YAML node tree to plain values with the scalar
// rules of ParseYAMLMap. Mapping keys are rendered as strings.
func NodeToValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return NodeToValue(n.Content[0])
	case yaml.AliasNode:
		if n.Alias == nil {
			return nil, nil
		}
		return NodeToValue(n.Alias)
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			key := k.Value
			if k.Kind != yaml.ScalarNode {
				key = fmt.Sprint(k.Value)
			}
			v, err := NodeToValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := NodeToValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.ScalarNode:
		return scalarValue(n), nil
	}
	return nil, nil
}

func scalarValue(n *yaml.Node) any {
	// A quoted scalar is a string whatever it looks like.
	if n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return n.Value
	}
	switch n.Tag {
	case "!!null":
		return nil
	case "!!bool":
		switch strings.ToLower(n.Value) {
		case "true", "yes", "on":
			return true
		case "false", "no", "off":
			return false
		}
		return n.Value
	case "!!int":
		if i, err := strconv.ParseInt(n.Value, 0, 64); err == nil {
			return float64(i)
		}
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			return f
		}
		return n.Value
	case "!!float":
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f
		}
		return n.Value
	}
	return n.Value
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "list"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.MappingNode:
		return "mapping"
	case yaml.DocumentNode:
		return "document"
	case yaml.AliasNode:
		return "alias"
	}
	return "node"
}

// FormatNumber renders a float the way the editor prints it: integers
// without a fraction.
func FormatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
