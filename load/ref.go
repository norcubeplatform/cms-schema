package load

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

// derefRefs replaces every `$ref: <name>` mapping in the document with
// the block or input it names. Blocks and inputs share one registry;
// inputs win on a name collision. The resolved copy gets `type: group`
// and `key: <name>` when it lacks them and a `_blockRef: <name>`
// annotation, as the editor expects. An unresolvable reference or a
// chain deeper than ten levels is a finding; the `$ref` mapping is left
// in place so later checks can still point at it.
func derefRefs(root *yaml.Node) []report.Finding {
	registry := buildRegistry(root)
	var findings []report.Finding
	// Walk the top-level kinds so findings can name the file.
	for i := 0; i+1 < len(root.Content); i += 2 {
		kind, val := root.Content[i].Value, root.Content[i+1]
		if val.Kind != yaml.MappingNode || (kind != kindCollections && kind != kindBlocks && kind != kindInputs) {
			continue
		}
		for j := 0; j+1 < len(val.Content); j += 2 {
			file := FileFor(kind, val.Content[j].Value)
			derefNode(val.Content[j+1], registry, 0, file, &findings)
		}
	}
	return findings
}

func buildRegistry(root *yaml.Node) *yaml.Node {
	blocks := mappingAt(root, kindBlocks)
	inputs := mappingAt(root, kindInputs)
	if blocks == nil && inputs == nil {
		return nil
	}
	return mergeMaps(blocks, inputs)
}

func mappingAt(root *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key && root.Content[i+1].Kind == yaml.MappingNode {
			return root.Content[i+1]
		}
	}
	return nil
}

func derefNode(n *yaml.Node, registry *yaml.Node, depth int, file string, findings *[]report.Finding) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			resolveOrRecurse(c, registry, depth, file, findings)
		}
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			resolveOrRecurse(n.Content[i], registry, depth, file, findings)
		}
	}
}

func resolveOrRecurse(n *yaml.Node, registry *yaml.Node, depth int, file string, findings *[]report.Finding) {
	if n == nil || n.Kind != yaml.MappingNode {
		derefNode(n, registry, depth, file, findings)
		return
	}
	name, ok := extractRef(n)
	if !ok {
		derefNode(n, registry, depth, file, findings)
		return
	}
	if depth >= maxRefDepth {
		*findings = append(*findings, report.Finding{File: file, Severity: report.SeverityError, Rule: report.RuleSchemaRefCycle, Message: fmt.Sprintf("$ref %q nests deeper than %d levels; the blocks reference each other in a cycle", name, maxRefDepth)})
		return
	}
	target := lookup(registry, name)
	if target == nil {
		*findings = append(*findings, report.Finding{File: file, Severity: report.SeverityError, Rule: report.RuleSchemaUnresolvedRef, Message: fmt.Sprintf("$ref %q names no block and no input", name), Hint: fmt.Sprintf("Add %s or %s, or fix the name.", BlockFile(name), InputFile(name))})
		return
	}
	replacement := clone(target)
	ensureType(replacement)
	ensureKey(replacement, name)
	ensureBlockRef(replacement, name)
	derefNode(replacement, registry, depth+1, file, findings)
	*n = *replacement
}

func extractRef(n *yaml.Node) (string, bool) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == schema.RefKey && n.Content[i+1].Kind == yaml.ScalarNode {
			return n.Content[i+1].Value, true
		}
	}
	return "", false
}

func lookup(registry *yaml.Node, name string) *yaml.Node {
	if registry == nil {
		return nil
	}
	for i := 0; i+1 < len(registry.Content); i += 2 {
		if registry.Content[i].Value == name {
			return registry.Content[i+1]
		}
	}
	return nil
}

func hasKey(n *yaml.Node, key string) bool {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return true
		}
	}
	return false
}

func ensureType(n *yaml.Node) {
	if n.Kind != yaml.MappingNode || hasKey(n, "type") {
		return
	}
	n.Content = append([]*yaml.Node{scalar("type"), scalar(schema.TypeGroup)}, n.Content...)
}

func ensureKey(n *yaml.Node, key string) {
	if n.Kind != yaml.MappingNode || hasKey(n, "key") {
		return
	}
	n.Content = append([]*yaml.Node{scalar("key"), scalar(key)}, n.Content...)
}

func ensureBlockRef(n *yaml.Node, name string) {
	if n.Kind != yaml.MappingNode || hasKey(n, schema.BlockRefKey) {
		return
	}
	n.Content = append(n.Content, scalar(schema.BlockRefKey), scalar(name))
}

func clone(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	out := &yaml.Node{Kind: n.Kind, Style: n.Style, Tag: n.Tag, Value: n.Value, Anchor: n.Anchor, Line: n.Line, Column: n.Column}
	if len(n.Content) > 0 {
		out.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			out.Content[i] = clone(c)
		}
	}
	if n.Alias != nil {
		out.Alias = clone(n.Alias)
	}
	return out
}
