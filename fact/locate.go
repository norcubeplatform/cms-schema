package fact

import (
	"github.com/norcubeplatform/cms-schema/report"
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
		return nil, inFile(l.configFile, err)
	}
	l.root = root
	return l, nil
}

func (l *locator) locate(f Fact) (Edit, error) {
	switch f.Kind {
	case KindRole:
		return l.role(f)
	case KindLocalized:
		return l.localized(f)
	case KindAlt:
		return l.alt(f)
	case KindTranslationLocales:
		coll, ok := l.cfg.Collections[f.Collection]
		if !ok {
			return Edit{}, refuse(CodeCollectionNotFound, &report.Params{Key: f.Collection})
		}
		if coll.Translations == nil {
			return Edit{}, refuse(CodeNoLanguageVersions, &report.Params{Key: f.Collection})
		}
		for _, loc := range f.Locales {
			if !slices.Contains(l.cfg.Preview.Locales, loc) {
				return Edit{}, refuse(CodeNotSiteLocale, &report.Params{Locale: loc})
			}
		}
		file, in, err := l.source(report.DeclarationKindCollection, f.Collection)
		if err != nil {
			return Edit{}, err
		}
		return Edit{Fact: f, File: file, In: in}, nil
	case KindOptimizedAtBuild:
		if _, ok := l.cfg.Storage(f.Storage); !ok || child(child(l.root, "storages"), f.Storage) == nil {
			return Edit{}, refuse(CodeStorageNotFound, &report.Params{Key: f.Storage})
		}
		return Edit{Fact: f, File: l.configFile, In: []string{"storages", f.Storage}}, nil
	case KindUsedByCode:
		st, ok := l.cfg.Storage(f.Storage)
		if !ok || child(child(l.root, "storages"), f.Storage) == nil {
			return Edit{}, refuse(CodeStorageNotFound, &report.Params{Key: f.Storage})
		}
		if st.Type != schema.StorageTypeRepo || st.Path == "" {
			return Edit{}, refuse(CodeStorageKeepsNoFiles, &report.Params{Key: f.Storage})
		}
		for _, p := range f.Paths {
			if schema.HasCodeWildcard(p) || strings.Contains(p, "**") {
				continue
			}
			if info, err := fs.Stat(l.fsys, path.Join(st.Path, p)); err != nil || info.IsDir() {
				return Edit{}, refuse(CodeStorageFileNotFound, &report.Params{Path: p, Key: f.Storage})
			}
		}
		return Edit{Fact: f, File: l.configFile, In: []string{"storages", f.Storage}}, nil
	}
	return Edit{}, refuse(CodeKindNotPlaced, &report.Params{Value: f.Kind})
}

// declarationPlaces is where each kind of declaration is written: its
// own folder, and the key of the config file's mapping that holds it
// inline.
var declarationPlaces = map[report.DeclarationKind]struct{ dir, key string }{
	report.DeclarationKindCollection: {dir: load.CollectionsDir, key: "collections"},
	report.DeclarationKindBlock:      {dir: load.BlocksDir, key: "blocks"},
	report.DeclarationKindInput:      {dir: load.InputsDir, key: "inputs"},
}

// role places a role fact: the field must hold the role as the fact
// declares it (a rich text description with its excerpt, a select with
// the values that switch the role on, each one of its options).
func (l *locator) role(f Fact) (Edit, error) {
	e, cur, _, err := l.field(f)
	if err != nil {
		return Edit{}, err
	}
	after := cur
	after.Role = f.Role
	if f.Excerpt != "" {
		after.Excerpt = f.Excerpt
	}
	switch f.Role {
	case schema.RoleNoindex:
		if len(f.When) > 0 {
			after.NoindexWhen = f.When
		}
	case schema.RoleDraft:
		if len(f.When) > 0 {
			after.DraftWhen = f.When
		}
	}
	if !after.HoldsRole(f.Role) {
		if cur.IsLocalized() {
			return Edit{}, refuse(CodeLocalizedCannotHold, &report.Params{Type: cur.ValueType(), Role: f.Role})
		}
		return Edit{}, refuse(CodeRoleCannotHold, &report.Params{Type: cur.Type, Role: f.Role})
	}
	values := schema.OptionValues(cur.Options)
	for _, v := range f.When {
		if !slices.Contains(values, v) {
			return Edit{}, refuse(CodeWhenNotOption, &report.Params{Value: v, Field: f.Field})
		}
	}
	return e, nil
}

// localized places a localized fact: a group whose fields are exactly
// the site's languages, falling back to one of them.
func (l *locator) localized(f Fact) (Edit, error) {
	e, cur, _, err := l.field(f)
	if err != nil {
		return Edit{}, err
	}
	locales := l.cfg.Preview.Locales
	if !schema.IsGroup(cur.Type) {
		return Edit{}, refuse(CodeNotGroup, &report.Params{Field: f.Field, Type: cur.Type})
	}
	if len(cur.Fields) != len(locales) {
		return Edit{}, refuse(CodeGroupFieldCount, &report.Params{Field: f.Field, Count: report.Int(int64(len(cur.Fields))), Total: report.Int(int64(len(locales)))})
	}
	for _, c := range cur.Fields {
		if !slices.Contains(locales, c.Key) {
			return Edit{}, refuse(CodeGroupFieldNotLocale, &report.Params{Key: c.Key, Field: f.Field})
		}
	}
	if f.Fallback != "" && !slices.Contains(locales, f.Fallback) {
		return Edit{}, refuse(CodeNotSiteLocale, &report.Params{Locale: f.Fallback})
	}
	return e, nil
}

// alt places an alt fact: an image whose named siblings can hold alt
// text.
func (l *locator) alt(f Fact) (Edit, error) {
	e, cur, siblings, err := l.field(f)
	if err != nil {
		return Edit{}, err
	}
	if schema.Canonical(cur.Type) != schema.TypeImage {
		return Edit{}, refuse(CodeNotImage, &report.Params{Field: f.Field, Type: cur.Type})
	}
	keys := f.AltFrom
	if f.Alt != "" {
		keys = []string{f.Alt}
	}
	for _, k := range keys {
		var found *schema.Field
		for i := range siblings {
			if siblings[i].Key == k {
				found = &siblings[i]
			}
		}
		if found == nil {
			return Edit{}, refuse(CodeAltFieldNotFound, &report.Params{Key: k, Field: f.Field})
		}
		t := found.ValueType()
		if k == cur.Key || (t != schema.TypeText && t != schema.TypeParagraph) || (schema.IsGroup(found.Type) && !found.IsLocalized()) {
			return Edit{}, refuse(CodeCannotHoldAlt, &report.Params{Key: k})
		}
	}
	return e, nil
}

// field follows a field path through the loaded schema, switching files
// where a `$ref` pulled a block or input in. It returns the edit, the
// field and the fields list it sits in.
func (l *locator) field(f Fact) (Edit, schema.Field, []schema.Field, error) {
	fail := func(err error) (Edit, schema.Field, []schema.Field, error) { return Edit{}, schema.Field{}, nil, err }
	steps, err := parsePath(f.Field)
	if err != nil {
		return fail(err)
	}
	var fields []schema.Field
	var file string
	var in []string
	if f.Collection != "" {
		coll, ok := l.cfg.Collections[f.Collection]
		if !ok {
			return fail(refuse(CodeCollectionNotFound, &report.Params{Key: f.Collection}))
		}
		fields = coll.Fields
		file, in, err = l.source(report.DeclarationKindCollection, f.Collection)
	} else {
		b, ok := l.cfg.Blocks[f.Block]
		if !ok {
			return fail(refuse(CodeBlockNotFound, &report.Params{Key: f.Block}))
		}
		fields = b.Fields
		file, in, err = l.source(report.DeclarationKindBlock, f.Block)
	}
	if err != nil {
		return fail(err)
	}
	var rel []Step
	var cur *schema.Field
	var siblings []schema.Field
	for depth, st := range steps {
		if depth > schema.MaxWalkDepth {
			return fail(refuse(CodeTooDeep, &report.Params{Field: f.Field, Max: report.Num(schema.MaxWalkDepth)}))
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
				return fail(refuse(CodeFieldNotFound, &report.Params{Key: st.Key, Field: f.Field}))
			case 1:
			default:
				return fail(refuse(CodeFieldsShareKey, &report.Params{Count: report.Int(int64(len(found))), Key: st.Key}))
			}
			fd := found[0]
			siblings = fields
			cur, fields = &fd, fd.Fields
			if fd.BlockRef != "" {
				if file, in, err = l.refSource(fd.BlockRef); err != nil {
					return fail(err)
				}
				rel = nil
				continue
			}
			rel = append(rel, st)
			continue
		}
		if cur == nil || !schema.IsArray(cur.Type) {
			return fail(refuse(CodeItemOfNonList, &report.Params{Field: f.Field}))
		}
		if cur.Type == schema.TypeBlocks || len(cur.Components) > 0 {
			b, ok := l.cfg.Blocks[st.Shape]
			if !ok || !slices.Contains(cur.Components, st.Shape) {
				return fail(refuse(CodeListBlockNotFound, &report.Params{Key: st.Shape, Field: f.Field}))
			}
			if file, in, err = l.source(report.DeclarationKindBlock, st.Shape); err != nil {
				return fail(err)
			}
			rel, fields = nil, b.Fields
			cur = &schema.Field{Type: schema.TypeGroup, Fields: b.Fields}
			continue
		}
		e, err := pickEntry(schema.ItemEntries(*cur), st.Shape, f.Field)
		if err != nil {
			return fail(err)
		}
		item := e.Field
		// A primitive item stands alone in its list.
		siblings = []schema.Field{item}
		cur, fields = &item, item.Fields
		if item.BlockRef != "" {
			if file, in, err = l.refSource(item.BlockRef); err != nil {
				return fail(err)
			}
			rel = nil
			continue
		}
		rel = append(rel, Step{Item: true, Shape: e.Name})
	}
	return Edit{Fact: f, File: file, In: in, Steps: rel}, *cur, siblings, nil
}

// pickEntry picks the item shape a step names in the fact's field path.
func pickEntry(entries []schema.Entry, shape, field string) (schema.Entry, error) {
	if shape == "" {
		if len(entries) != 1 {
			return schema.Entry{}, refuse(CodeShapeUnnamed, &report.Params{Count: report.Int(int64(len(entries))), Field: field})
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
		return schema.Entry{}, refuse(CodeShapeNotFound, &report.Params{Key: shape, Field: field})
	case 1:
		return found[0], nil
	}
	return schema.Entry{}, refuse(CodeShapesNamedTwice, &report.Params{Count: report.Int(int64(len(found))), Key: shape, Field: field})
}

// refSource is the file of what a `$ref` names: an input wins over a
// block of the same name, as in the loader.
func (l *locator) refSource(name string) (string, []string, error) {
	if _, ok := l.cfg.Inputs[name]; ok {
		return l.source(report.DeclarationKindInput, name)
	}
	if _, ok := l.cfg.Blocks[name]; ok {
		return l.source(report.DeclarationKindBlock, name)
	}
	return "", nil, refuse(CodeRefNotFound, &report.Params{Key: name})
}

// source is the one place a collection, block or input is written: its
// own file, or an entry in the config file. Written in two places (a
// file and inline, or two files) it is ambiguous.
func (l *locator) source(kind report.DeclarationKind, name string) (string, []string, error) {
	place, ok := declarationPlaces[kind]
	if !ok {
		panic("fact: no place for declaration kind " + string(kind))
	}
	type cand struct {
		file string
		in   []string
	}
	var cands []cand
	if !l.doc.Legacy {
		seen := map[string]bool{}
		for _, f := range l.doc.Files {
			ext := strings.ToLower(path.Ext(f))
			if seen[f] || (ext != ".yml" && ext != ".yaml") || strings.TrimSuffix(path.Base(f), path.Ext(f)) != name {
				continue
			}
			d := path.Dir(f)
			// Inputs also come from inputsFromGlob folders.
			glob := kind == report.DeclarationKindInput && f != load.ConfigFile && d != load.CollectionsDir && d != load.BlocksDir
			if d == place.dir || glob {
				seen[f] = true
				cands = append(cands, cand{file: f})
			}
		}
	}
	if child(child(l.root, place.key), name) != nil {
		cands = append(cands, cand{file: l.configFile, in: []string{place.key, name}})
	}
	switch len(cands) {
	case 0:
		return "", nil, refuse(CodeNoFileDeclares, &report.Params{DeclarationKind: kind, Key: name})
	case 1:
		return cands[0].file, cands[0].in, nil
	}
	files := make([]string, len(cands))
	for i, c := range cands {
		files[i] = c.file
	}
	return "", nil, refuse(CodeDeclaredTwice, &report.Params{DeclarationKind: kind, Key: name, Names: files})
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
			return nil, refuse(CodePathStartsWithItem, &report.Params{Field: p})
		}
		shape := p[m[4]:m[5]]
		rebuilt.WriteString("[" + shape + "]")
		out = append(out, Step{Item: true, Shape: shape})
	}
	if len(out) == 0 || rebuilt.String() != p || out[len(out)-1].Item {
		return nil, refuse(CodeNotFieldPath, &report.Params{Field: p})
	}
	return out, nil
}
