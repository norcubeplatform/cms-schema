package fact

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Edit is one fact placed in one file: the file, the mapping it starts
// from and the steps from there to the field.
type Edit struct {
	Fact Fact
	// File is the schema file's repository path.
	File string
	// In are the mapping keys from the file's root to the collection,
	// block, input or storage the fact is about (`collections`, `pages`
	// for a collection written inline in config.yml); empty when the
	// file's root is it.
	In []string
	// Steps lead from there to the field of a role fact; none for the
	// mapping itself (an input file is one field).
	Steps []Step
}

// Step is one move toward a field: into the `fields` of the current
// mapping by Key, or into the item shape of a list (Item, with Shape
// naming the entry; empty for the one shape).
type Step struct {
	Key   string
	Item  bool
	Shape string
}

// locator finds where facts go, from the loaded schema.
type locator struct {
	fsys       fs.FS
	doc        *load.Document
	cfg        *schema.Config
	configFile string
	// root is the config file's top mapping; nil when it has none.
	root *yaml.Node
}

func newLocator(fsys fs.FS, doc *load.Document) (*locator, error) {
	l := &locator{fsys: fsys, doc: doc, cfg: doc.Config, configFile: load.ConfigFile}
	if doc.Legacy {
		l.configFile = load.LegacyConfigFile
	}
	raw, err := fs.ReadFile(fsys, l.configFile)
	if err != nil {
		return nil, err
	}
	root, err := topMapping(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", l.configFile, err)
	}
	l.root = root
	return l, nil
}

func (l *locator) locate(f Fact) (Edit, error) {
	switch f.Kind {
	case KindRole:
		return l.role(f)
	case KindTranslationLocales:
		coll, ok := l.cfg.Collections[f.Collection]
		if !ok {
			return Edit{}, fmt.Errorf("%w: collection %q", ErrNotFound, f.Collection)
		}
		if coll.Translations == nil {
			return Edit{}, fmt.Errorf("%w: collection %q has no language versions (no translations)", ErrInvalid, f.Collection)
		}
		for _, loc := range f.Locales {
			if !slices.Contains(l.cfg.Preview.Locales, loc) {
				return Edit{}, fmt.Errorf("%w: %q is not a language of the site (preview.locales)", ErrInvalid, loc)
			}
		}
		file, in, err := l.source(kindCollections, f.Collection)
		if err != nil {
			return Edit{}, err
		}
		return Edit{Fact: f, File: file, In: in}, nil
	case KindOptimizedAtBuild:
		if _, ok := l.cfg.Storage(f.Storage); !ok || child(child(l.root, "storages"), f.Storage) == nil {
			return Edit{}, fmt.Errorf("%w: storage %q", ErrNotFound, f.Storage)
		}
		return Edit{Fact: f, File: l.configFile, In: []string{"storages", f.Storage}}, nil
	case KindUsedByCode:
		st, ok := l.cfg.Storage(f.Storage)
		if !ok || child(child(l.root, "storages"), f.Storage) == nil {
			return Edit{}, fmt.Errorf("%w: storage %q", ErrNotFound, f.Storage)
		}
		if st.Type != schema.StorageTypeRepo || st.Path == "" {
			return Edit{}, fmt.Errorf("%w: storage %q keeps no files in the repository", ErrInvalid, f.Storage)
		}
		for _, p := range f.Paths {
			if schema.HasCodeWildcard(p) || strings.Contains(p, "**") {
				continue
			}
			if info, err := fs.Stat(l.fsys, path.Join(st.Path, p)); err != nil || info.IsDir() {
				return Edit{}, fmt.Errorf("%w: no file %s in storage %q", ErrNotFound, p, f.Storage)
			}
		}
		return Edit{Fact: f, File: l.configFile, In: []string{"storages", f.Storage}}, nil
	}
	return Edit{}, fmt.Errorf("%w: kind %q", ErrInvalid, f.Kind)
}

const (
	kindCollections = "collections"
	kindBlocks      = "blocks"
	kindInputs      = "inputs"
)

// role follows a field path through the loaded schema, switching files
// where a `$ref` pulled a block or input in.
func (l *locator) role(f Fact) (Edit, error) {
	steps, err := parsePath(f.Field)
	if err != nil {
		return Edit{}, err
	}
	var fields []schema.Field
	var file string
	var in []string
	if f.Collection != "" {
		coll, ok := l.cfg.Collections[f.Collection]
		if !ok {
			return Edit{}, fmt.Errorf("%w: collection %q", ErrNotFound, f.Collection)
		}
		fields = coll.Fields
		file, in, err = l.source(kindCollections, f.Collection)
	} else {
		b, ok := l.cfg.Blocks[f.Block]
		if !ok {
			return Edit{}, fmt.Errorf("%w: block %q", ErrNotFound, f.Block)
		}
		fields = b.Fields
		file, in, err = l.source(kindBlocks, f.Block)
	}
	if err != nil {
		return Edit{}, err
	}
	var rel []Step
	var cur *schema.Field
	for depth, st := range steps {
		if depth > schema.MaxWalkDepth {
			return Edit{}, fmt.Errorf("%w: %q nests deeper than %d", ErrInvalid, f.Field, schema.MaxWalkDepth)
		}
		if !st.Item {
			var found []schema.Field
			for _, fd := range fields {
				if fd.Key == st.Key {
					found = append(found, fd)
				}
			}
			switch len(found) {
			case 0:
				return Edit{}, fmt.Errorf("%w: field %q of %q", ErrNotFound, st.Key, f.Field)
			case 1:
			default:
				return Edit{}, fmt.Errorf("%w: %d fields have the key %q", ErrAmbiguous, len(found), st.Key)
			}
			fd := found[0]
			cur, fields = &fd, fd.Fields
			if fd.BlockRef != "" {
				if file, in, err = l.refSource(fd.BlockRef); err != nil {
					return Edit{}, err
				}
				rel = nil
				continue
			}
			rel = append(rel, st)
			continue
		}
		if cur == nil || !schema.IsArray(cur.Type) {
			return Edit{}, fmt.Errorf("%w: %q steps into a list item of a field that is not a list", ErrNotFound, f.Field)
		}
		if cur.Type == schema.TypeBlocks || len(cur.Components) > 0 {
			b, ok := l.cfg.Blocks[st.Shape]
			if !ok || !slices.Contains(cur.Components, st.Shape) {
				return Edit{}, fmt.Errorf("%w: block %q in %q", ErrNotFound, st.Shape, f.Field)
			}
			if file, in, err = l.source(kindBlocks, st.Shape); err != nil {
				return Edit{}, err
			}
			rel, fields = nil, b.Fields
			cur = &schema.Field{Type: schema.TypeGroup, Fields: b.Fields}
			continue
		}
		e, err := pickEntry(schema.ItemEntries(*cur), st.Shape)
		if err != nil {
			return Edit{}, fmt.Errorf("%q: %w", f.Field, err)
		}
		item := e.Field
		cur, fields = &item, item.Fields
		if item.BlockRef != "" {
			if file, in, err = l.refSource(item.BlockRef); err != nil {
				return Edit{}, err
			}
			rel = nil
			continue
		}
		rel = append(rel, Step{Item: true, Shape: e.Name})
	}
	if !schema.RoleAccepts(f.Role, cur.Type) {
		return Edit{}, fmt.Errorf("%w: a %s field cannot hold role %s", ErrInvalid, cur.Type, f.Role)
	}
	return Edit{Fact: f, File: file, In: in, Steps: rel}, nil
}

func pickEntry(entries []schema.Entry, shape string) (schema.Entry, error) {
	if shape == "" {
		if len(entries) != 1 {
			return schema.Entry{}, fmt.Errorf("%w: the list has %d item shapes; name one", ErrAmbiguous, len(entries))
		}
		return entries[0], nil
	}
	var found []schema.Entry
	for _, e := range entries {
		if e.Name == shape {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return schema.Entry{}, fmt.Errorf("%w: item shape %q", ErrNotFound, shape)
	case 1:
		return found[0], nil
	}
	return schema.Entry{}, fmt.Errorf("%w: %d item shapes are named %q", ErrAmbiguous, len(found), shape)
}

// refSource is the file of what a `$ref` names: an input wins over a
// block of the same name, as in the loader.
func (l *locator) refSource(name string) (string, []string, error) {
	if _, ok := l.cfg.Inputs[name]; ok {
		return l.source(kindInputs, name)
	}
	if _, ok := l.cfg.Blocks[name]; ok {
		return l.source(kindBlocks, name)
	}
	return "", nil, fmt.Errorf("%w: block or input %q", ErrNotFound, name)
}

// source is the one place a collection, block or input is written: its
// own file, or an entry in the config file. Written in two places (a
// file and inline, or two files) it is ambiguous.
func (l *locator) source(kind, name string) (string, []string, error) {
	type cand struct {
		file string
		in   []string
	}
	var cands []cand
	if !l.doc.Legacy {
		dir := map[string]string{kindCollections: load.CollectionsDir, kindBlocks: load.BlocksDir, kindInputs: load.InputsDir}[kind]
		seen := map[string]bool{}
		for _, f := range l.doc.Files {
			ext := strings.ToLower(path.Ext(f))
			if seen[f] || (ext != ".yml" && ext != ".yaml") || strings.TrimSuffix(path.Base(f), path.Ext(f)) != name {
				continue
			}
			d := path.Dir(f)
			// Inputs also come from inputsFromGlob folders.
			glob := kind == kindInputs && f != load.ConfigFile && d != load.CollectionsDir && d != load.BlocksDir
			if d == dir || glob {
				seen[f] = true
				cands = append(cands, cand{file: f})
			}
		}
	}
	if child(child(l.root, kind), name) != nil {
		cands = append(cands, cand{file: l.configFile, in: []string{kind, name}})
	}
	switch len(cands) {
	case 0:
		return "", nil, fmt.Errorf("%w: no file declares %s %q", ErrNotFound, strings.TrimSuffix(kind, "s"), name)
	case 1:
		return cands[0].file, cands[0].in, nil
	}
	files := make([]string, len(cands))
	for i, c := range cands {
		files[i] = c.file
	}
	return "", nil, fmt.Errorf("%w: %s %q is declared in %s", ErrAmbiguous, strings.TrimSuffix(kind, "s"), name, strings.Join(files, " and "))
}

var pathToken = regexp.MustCompile(`([^.\[\]]+)|\[([^\[\]]*)\]`)

// parsePath reads a field path: keys joined by dots, `[]` or `[shape]`
// after a list's key.
func parsePath(p string) ([]Step, error) {
	var out []Step
	var rebuilt strings.Builder
	for _, m := range pathToken.FindAllStringSubmatchIndex(p, -1) {
		if m[2] >= 0 {
			key := p[m[2]:m[3]]
			if len(out) > 0 {
				rebuilt.WriteByte('.')
			}
			rebuilt.WriteString(key)
			out = append(out, Step{Key: key})
			continue
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%w: field path %q starts with a list item", ErrInvalid, p)
		}
		shape := p[m[4]:m[5]]
		rebuilt.WriteString("[" + shape + "]")
		out = append(out, Step{Item: true, Shape: shape})
	}
	if len(out) == 0 || rebuilt.String() != p || out[len(out)-1].Item {
		return nil, fmt.Errorf("%w: %q is not a field path (keys joined by dots, [] or [shape] for a list item, ending at a field)", ErrInvalid, p)
	}
	return out, nil
}
