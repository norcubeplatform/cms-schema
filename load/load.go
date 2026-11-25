// Package load assembles a site's `.norcube/cms/` directory (or the
// legacy single `norcube-cms.config.yml`) into one merged document, the
// same way the CMS service does: `config.yml` supplies the root keys,
// each file under `collections/`, `blocks/` and `inputs/` becomes one
// entry keyed by its basename, `inputsFromGlob` adds more inputs, every
// `$ref` is replaced by the block or input it names, and the schema
// version is checked.
//
// Unlike the service, which skips a file it cannot parse with a log
// line, Load reports every such problem as a finding and keeps going,
// so a site author sees all of them at once.
package load

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Paths inside a site.
const (
	ConfigDir        = ".norcube/cms"
	ConfigFile       = ConfigDir + "/config.yml"
	CollectionsDir   = ConfigDir + "/collections"
	BlocksDir        = ConfigDir + "/blocks"
	InputsDir        = ConfigDir + "/inputs"
	LegacyConfigFile = "norcube-cms.config.yml"
	maxRefDepth      = 10
	kindCollections  = "collections"
	kindBlocks       = "blocks"
	kindInputs       = "inputs"
)

// ErrNotASite is returned when neither the directory layout nor the
// legacy file exists.
var ErrNotASite = errors.New("no .norcube/cms/config.yml and no norcube-cms.config.yml")

// Document is the loaded configuration.
type Document struct {
	// Root is the merged mapping after `$ref` resolution, with every
	// key in source order.
	Root *yaml.Node
	// Merged is Root as plain values.
	Merged map[string]any
	// Config is the typed view of Merged.
	Config *schema.Config
	// Files lists the schema files that were read, in load order.
	Files []string
	// Legacy is true when the single-file layout was used.
	Legacy bool
}

// Load reads the site rooted at fsys. Problems in individual files are
// returned as findings; the error is reserved for I/O failures and for
// ErrNotASite.
func Load(fsys fs.FS) (*Document, []report.Finding, error) {
	var findings []report.Finding
	doc := &Document{}

	rootRaw, err := fs.ReadFile(fsys, ConfigFile)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		legacy, lerr := fs.ReadFile(fsys, LegacyConfigFile)
		if lerr != nil {
			if errors.Is(lerr, fs.ErrNotExist) {
				return nil, nil, ErrNotASite
			}
			return nil, nil, lerr
		}
		doc.Legacy = true
		doc.Files = append(doc.Files, LegacyConfigFile)
		root, f := parseMapping(LegacyConfigFile, legacy)
		findings = append(findings, f...)
		if root == nil {
			root = emptyMapping()
		}
		return finish(doc, root, findings)
	}
	doc.Files = append(doc.Files, ConfigFile)
	root, f := parseMapping(ConfigFile, rootRaw)
	findings = append(findings, f...)
	if root == nil {
		root = emptyMapping()
	}

	collections, files, f := loadDir(fsys, CollectionsDir)
	doc.Files = append(doc.Files, files...)
	findings = append(findings, f...)
	blocks, files, f := loadDir(fsys, BlocksDir)
	doc.Files = append(doc.Files, files...)
	findings = append(findings, f...)
	inputs, files, f := loadDir(fsys, InputsDir)
	doc.Files = append(doc.Files, files...)
	findings = append(findings, f...)
	if globs := inputGlobs(root); len(globs) > 0 {
		extra, files, f := loadGlobs(fsys, globs)
		doc.Files = append(doc.Files, files...)
		findings = append(findings, f...)
		inputs = mergeMaps(inputs, extra)
	}

	root = mergeInto(root, kindCollections, collections)
	root = mergeInto(root, kindBlocks, blocks)
	root = mergeInto(root, kindInputs, inputs)
	return finish(doc, root, findings)
}

func finish(doc *Document, root *yaml.Node, findings []report.Finding) (*Document, []report.Finding, error) {
	findings = append(findings, derefRefs(root)...)
	findings = append(findings, checkVersions(root)...)
	doc.Root = root
	v, err := content.NodeToValue(root)
	if err != nil {
		return nil, nil, err
	}
	merged, _ := v.(map[string]any)
	if merged == nil {
		merged = map[string]any{}
	}
	doc.Merged = merged
	cfg, err := schema.Parse(merged)
	if err != nil {
		return nil, nil, err
	}
	doc.Config = cfg
	return doc, findings, nil
}

// FileFor returns the schema file that defines a collection, block or
// input, for findings that point at one.
func FileFor(kind, key string) string {
	switch kind {
	case kindCollections:
		return CollectionsDir + "/" + key + ".yml"
	case kindBlocks:
		return BlocksDir + "/" + key + ".yml"
	case kindInputs:
		return InputsDir + "/" + key + ".yml"
	}
	return ConfigFile
}

// CollectionFile, BlockFile and InputFile name the file a schema entry
// lives in under the directory layout.
func CollectionFile(key string) string { return FileFor(kindCollections, key) }
func BlockFile(key string) string      { return FileFor(kindBlocks, key) }
func InputFile(key string) string      { return FileFor(kindInputs, key) }

func parseMapping(file string, raw []byte) (*yaml.Node, []report.Finding) {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, []report.Finding{report.New(file, "", report.RuleSchemaParseError, report.CauseNotYAML, &report.Params{Detail: yamlErr(err)})}
	}
	if node.Kind == 0 || len(node.Content) == 0 {
		return nil, nil
	}
	m := topLevelMapping(&node)
	if m == nil {
		return nil, []report.Finding{report.New(file, "", report.RuleSchemaNotMapping, "", nil)}
	}
	return m, nil
}

func yamlErr(err error) string {
	return strings.TrimPrefix(err.Error(), "yaml: ")
}

func emptyMapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func topLevelMapping(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		return topLevelMapping(n.Content[0])
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// loadDir reads every `.yml`/`.yaml` directly under dir into a mapping
// keyed by basename, in alphabetical order. A missing directory is not
// an error.
func loadDir(fsys fs.FS, dir string) (*yaml.Node, []string, []report.Finding) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil, nil
	}
	type item struct{ key, file string }
	var items []item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(e.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		key := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		if key == "" {
			continue
		}
		items = append(items, item{key: key, file: dir + "/" + e.Name()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	out := emptyMapping()
	var files []string
	var findings []report.Finding
	for _, it := range items {
		files = append(files, it.file)
		raw, err := fs.ReadFile(fsys, it.file)
		if err != nil {
			findings = append(findings, report.New(it.file, "", report.RuleSchemaParseError, report.CauseUnreadable, &report.Params{Detail: err.Error()}))
			continue
		}
		m, f := parseMapping(it.file, raw)
		findings = append(findings, f...)
		if m == nil {
			continue
		}
		out.Content = append(out.Content, scalar(it.key), m)
	}
	if len(out.Content) == 0 {
		return nil, files, findings
	}
	return out, files, findings
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func inputGlobs(root *yaml.Node) []string {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "inputsFromGlob" {
			continue
		}
		v := root.Content[i+1]
		if v.Kind == yaml.ScalarNode && v.Value != "" {
			return []string{v.Value}
		}
		if v.Kind == yaml.SequenceNode {
			var out []string
			for _, e := range v.Content {
				if e.Kind == yaml.ScalarNode && e.Value != "" {
					out = append(out, e.Value)
				}
			}
			return out
		}
	}
	return nil
}

// loadGlobs reads `<dir>/*.yml` patterns; anything broader is reported.
func loadGlobs(fsys fs.FS, globs []string) (*yaml.Node, []string, []report.Finding) {
	out := emptyMapping()
	seen := map[string]int{}
	var files []string
	var findings []report.Finding
	for _, g := range globs {
		dir, ext, ok := splitGlob(g)
		if !ok {
			findings = append(findings, report.New(ConfigFile, "inputsFromGlob", report.RuleSchemaInputsGlobUnsupported, "", &report.Params{Path: g}))
			continue
		}
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.ToLower(path.Ext(e.Name())) != "."+ext {
				continue
			}
			key := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
			if key == "" {
				continue
			}
			file := dir + "/" + e.Name()
			files = append(files, file)
			raw, err := fs.ReadFile(fsys, file)
			if err != nil {
				continue
			}
			m, f := parseMapping(file, raw)
			findings = append(findings, f...)
			if m == nil {
				continue
			}
			if idx, exists := seen[key]; exists {
				out.Content[idx] = m
				continue
			}
			out.Content = append(out.Content, scalar(key), m)
			seen[key] = len(out.Content) - 1
		}
	}
	if len(out.Content) == 0 {
		return nil, files, findings
	}
	return out, files, findings
}

func splitGlob(pattern string) (dir, ext string, ok bool) {
	slash := strings.LastIndex(pattern, "/")
	if slash < 0 {
		return "", "", false
	}
	dir = pattern[:slash]
	base := pattern[slash+1:]
	if !strings.HasPrefix(base, "*.") {
		return "", "", false
	}
	ext = strings.ToLower(strings.TrimPrefix(base, "*."))
	if ext == "" || strings.ContainsAny(ext, "*?[/") || strings.ContainsAny(dir, "*?[") {
		return "", "", false
	}
	return dir, ext, true
}

// mergeMaps returns a's entries followed by b's; b wins on collision.
func mergeMaps(a, b *yaml.Node) *yaml.Node {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := emptyMapping()
	seen := map[string]int{}
	add := func(m *yaml.Node) {
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i].Value
			if idx, ok := seen[k]; ok {
				out.Content[idx] = m.Content[i+1]
				continue
			}
			out.Content = append(out.Content, m.Content[i], m.Content[i+1])
			seen[k] = len(out.Content) - 1
		}
	}
	add(a)
	add(b)
	return out
}

// mergeInto sets key on parent to value; when both are mappings the
// per-file entries overlay the inline ones key by key, keeping the
// inline order.
func mergeInto(parent *yaml.Node, key string, value *yaml.Node) *yaml.Node {
	if value == nil {
		return parent
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value != key {
			continue
		}
		existing := parent.Content[i+1]
		if existing.Kind == yaml.MappingNode {
			parent.Content[i+1] = overlay(existing, value)
		} else {
			parent.Content[i+1] = value
		}
		return parent
	}
	parent.Content = append(parent.Content, scalar(key), value)
	return parent
}

func overlay(base, override *yaml.Node) *yaml.Node {
	out := emptyMapping()
	idx := map[string]int{}
	for i := 0; i+1 < len(override.Content); i += 2 {
		idx[override.Content[i].Value] = i + 1
	}
	used := map[string]bool{}
	for i := 0; i+1 < len(base.Content); i += 2 {
		k := base.Content[i].Value
		if vi, ok := idx[k]; ok {
			out.Content = append(out.Content, base.Content[i], override.Content[vi])
			used[k] = true
			continue
		}
		out.Content = append(out.Content, base.Content[i], base.Content[i+1])
	}
	for i := 0; i+1 < len(override.Content); i += 2 {
		if used[override.Content[i].Value] {
			continue
		}
		out.Content = append(out.Content, override.Content[i], override.Content[i+1])
	}
	return out
}

// checkVersions reports a collection, block or input whose
// `schemaVersion` this module does not support.
func checkVersions(root *yaml.Node) []report.Finding {
	var out []report.Finding
	for i := 0; i+1 < len(root.Content); i += 2 {
		kind, val := root.Content[i].Value, root.Content[i+1]
		if (kind != kindCollections && kind != kindBlocks && kind != kindInputs) || val.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(val.Content); j += 2 {
			name, entry := val.Content[j].Value, val.Content[j+1]
			if entry.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k+1 < len(entry.Content); k += 2 {
				if entry.Content[k].Value != "schemaVersion" {
					continue
				}
				var v int
				if err := entry.Content[k+1].Decode(&v); err != nil || !schema.SupportedVersions[v] {
					out = append(out, report.New(FileFor(kind, name), "schemaVersion", report.RuleSchemaUnsupportedVersion, "", &report.Params{Value: entry.Content[k+1].Value}))
				}
			}
		}
	}
	return out
}
