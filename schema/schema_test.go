package schema

import (
	"testing"
)

func TestParseFieldKnobs(t *testing.T) {
	f := ParseField(map[string]any{
		"key": "tier", "type": "select", "label": "Tier",
		"options":          []any{"free", map[string]any{"value": "pro", "label": "Pro"}, float64(3)},
		"default":          "free",
		"visibleIf":        map[string]any{"field": "plan", "equals": "x"},
		"computed":         map[string]any{"from": "title", "transform": "slug"},
		"customValidators": []any{map[string]any{"type": "regex", "pattern": "^a", "message": "m"}},
		"minLength":        float64(2), "toolbar": []any{"bold"},
		"recommendedLength": map[string]any{"from": float64(1), "to": float64(5)},
	})
	if len(f.Options) != 3 || f.Options[1].Label != "Pro" || f.Options[2].Value != "3" {
		t.Fatalf("options: %+v", f.Options)
	}
	if !f.HasDefault || f.Default != "free" {
		t.Fatal("default")
	}
	if f.VisibleIf == nil || f.VisibleIf.Op != OpEq || f.VisibleIf.Value != "x" {
		t.Fatalf("equals shorthand: %+v", f.VisibleIf)
	}
	if f.Computed == nil || f.Computed.From != "title" {
		t.Fatal("computed")
	}
	if len(f.Custom) != 1 || f.Custom[0].Params["pattern"] != "^a" || f.Custom[0].Message != "m" {
		t.Fatalf("custom: %+v", f.Custom)
	}
	if f.MinLength == nil || *f.MinLength != 2 {
		t.Fatal("minLength")
	}
	if f.RecommendedLength == nil || *f.RecommendedLength.To != 5 {
		t.Fatal("recommendedLength")
	}
	// A select does not read minLength, toolbar or recommendedLength.
	for _, k := range []string{"minLength", "toolbar", "recommendedLength"} {
		if _, ok := f.Extra[k]; !ok {
			t.Errorf("expected %s in Extra, got %v", k, f.Extra)
		}
	}
	rt := ParseField(map[string]any{"key": "b", "type": "richtext", "label": "B", "options": map[string]any{"bold": true}})
	if rt.RichTextOptions["bold"] != true || len(rt.Extra) != 0 {
		t.Fatalf("richtext options: %+v extra %v", rt.RichTextOptions, rt.Extra)
	}
	ref := ParseField(map[string]any{"key": "p", "type": "reference", "label": "P"})
	if ref.By != ReferenceByPath {
		t.Fatalf("reference by default: %q", ref.By)
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := Parse(map[string]any{
		"pagesDir": "src/content/pages/",
		"storages": map[string]any{
			"b": map[string]any{"type": "repo", "path": "/public/b/", "publicUrl": "/b/", "max_size": float64(10)},
			"a": map[string]any{"type": "repo", "path": "src/a"},
		},
		"defaultStorage": "b",
		"preview":        map[string]any{"defaultLocale": "cs", "locales": []any{"cs", "en"}, "prefixDefaultLocale": true},
		"redirects":      map[string]any{"file": "public/_redirects"},
		"variables":      map[string]any{"file": "x.md"},
		"knownPaths":     []any{"/a/", " ", "/b/*"},
		"collections": map[string]any{
			"pages": map[string]any{"label": "Pages", "fields": []any{map[string]any{"key": "t", "type": "text", "label": "T"}}},
			"works": map[string]any{"path": "src/content/works", "match": "*.md", "fields": []any{}},
		},
		"blocks": map[string]any{"hero": map[string]any{"label": "Hero", "fields": []any{}}},
		"inputs": map[string]any{"kicker": map[string]any{"key": "kicker", "type": "text", "label": "K"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PagesDir != "src/content/pages" || cfg.PublicDir != "public" {
		t.Fatalf("dirs: %q %q", cfg.PagesDir, cfg.PublicDir)
	}
	if len(cfg.Storages) != 2 || cfg.Storages[0].Name != "a" || cfg.Storages[1].PublicURL != "/b" || cfg.Storages[1].Path != "public/b" || cfg.Storages[1].MaxSize != 10 {
		t.Fatalf("storages: %+v", cfg.Storages)
	}
	if s, ok := cfg.DefaultStorageConfig(); !ok || s.Name != "b" {
		t.Fatal("default storage")
	}
	if cfg.Redirects.Format != RedirectsFormatNetlify || !cfg.Redirects.Enabled() {
		t.Fatal("redirects default format")
	}
	if cfg.Variables.Field != "variables" {
		t.Fatal("variables field default")
	}
	if len(cfg.KnownPaths) != 2 {
		t.Fatalf("knownPaths: %v", cfg.KnownPaths)
	}
	if cfg.Collections["pages"].Path != "src/content/pages" || cfg.Collections["pages"].Match != "**/*.md" || cfg.Collections["pages"].ExtraKeysPolicy != ExtraKeysShow {
		t.Fatalf("pages defaults: %+v", cfg.Collections["pages"])
	}
	if cfg.Collections["works"].Match != "*.md" {
		t.Fatal("works match")
	}
	if len(cfg.CollectionKeys) != 2 || len(cfg.BlockKeys) != 1 || len(cfg.InputKeys) != 1 {
		t.Fatal("keys")
	}
	// pagesDir alone yields a pages collection.
	cfg2, _ := Parse(map[string]any{"pagesDir": "content"})
	if cfg2.Collections["pages"].Path != "content" {
		t.Fatal("implicit pages collection")
	}
}

func TestConditions(t *testing.T) {
	root := map[string]any{
		"tier": "pro", "seats": float64(5), "tags": []any{"a"}, "flag": false, "name": "",
		"meta":  map[string]any{"on": true},
		"items": []any{map[string]any{"kind": "x", "inner": map[string]any{"deep": "d"}}},
	}
	rs := RootScope(root)
	eval := func(c Condition, s Scope) bool { return Eval(c, s) }
	cases := []struct {
		name string
		c    Condition
		s    Scope
		want bool
	}{
		{"eq", Condition{Field: "tier", Op: OpEq, Value: "pro"}, rs, true},
		{"neq", Condition{Field: "tier", Op: OpNeq, Value: "pro"}, rs, false},
		{"in list", Condition{Field: "tier", Op: OpIn, Value: []any{"pro", "enterprise"}}, rs, true},
		{"in csv string", Condition{Field: "tier", Op: OpIn, Value: "free, pro"}, rs, true},
		{"in json string", Condition{Field: "tier", Op: OpIn, Value: `["pro"]`}, rs, true},
		{"notIn", Condition{Field: "tier", Op: OpNotIn, Value: []any{"free"}}, rs, true},
		{"truthy false", Condition{Field: "flag", Op: OpTruthy}, rs, false},
		{"falsy missing", Condition{Field: "nope", Op: OpFalsy}, rs, true},
		{"empty string", Condition{Field: "name", Op: OpEmpty}, rs, true},
		{"nonEmpty list", Condition{Field: "tags", Op: OpNonEmpty}, rs, true},
		{"gt", Condition{Field: "seats", Op: OpGt, Value: float64(3)}, rs, true},
		{"lt with int expected", Condition{Field: "seats", Op: OpLt, Value: 3}, rs, false},
		{"matches", Condition{Field: "tier", Op: OpMatches, Value: "^p"}, rs, true},
		{"matches bad regex", Condition{Field: "tier", Op: OpMatches, Value: "("}, rs, false},
		{"all", Condition{All: []Condition{{Field: "tier", Op: OpEq, Value: "pro"}, {Field: "meta.on", Op: OpTruthy}}}, rs, true},
		{"any", Condition{Any: []Condition{{Field: "tier", Op: OpEq, Value: "free"}, {Field: "flag", Op: OpFalsy}}}, rs, true},
		{"not", Condition{Not: &Condition{Field: "tier", Op: OpEq, Value: "pro"}}, rs, false},
		{"$root from child", Condition{Field: "$root.tier", Op: OpEq, Value: "pro"}, rs.Child(root["meta"].(map[string]any)), true},
		{"$parent from child", Condition{Field: "$parent.seats", Op: OpGt, Value: float64(1)}, rs.Child(root["meta"].(map[string]any)), true},
		{"$item from nested group", Condition{Field: "$item.kind", Op: OpEq, Value: "x"}, rs.ItemScope(root["items"].([]any)[0].(map[string]any)).Child(map[string]any{"deep": "d"}), true},
		{"$item outside an item", Condition{Field: "$item.kind", Op: OpEq, Value: "x"}, rs, false},
		{"array index path", Condition{Field: "items.0.kind", Op: OpEq, Value: "x"}, rs, true},
	}
	for _, tc := range cases {
		if got := eval(tc.c, tc.s); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestItemEntries(t *testing.T) {
	arr := Field{Type: TypeArray, Items: []Field{
		{Key: "hero", Type: TypeGroup, Fields: []Field{{Key: "t", Type: TypeText}}},
		{Key: "cta", Name: "call", Type: TypeGroup},
		{Key: "tag", Type: TypeText},
		{Key: "n", Type: TypeNumber},
	}}
	entries := ItemEntries(arr)
	if entries[1].Name != "call" || entries[2].Name != "tag" {
		t.Fatalf("names: %+v", entries)
	}
	if e := ResolveItemEntry(arr, map[string]any{"_type": "call"}); e == nil || e.Name != "call" {
		t.Fatal("by discriminator")
	}
	if e := ResolveItemEntry(arr, map[string]any{}); e == nil || e.Name != "hero" {
		t.Fatal("first group fallback")
	}
	if e := ResolveItemEntry(arr, "x"); e == nil || e.Name != "tag" {
		t.Fatal("string primitive")
	}
	if e := ResolveItemEntry(arr, float64(3)); e == nil || e.Name != "n" {
		t.Fatal("number primitive")
	}
	if e := ResolveItemEntry(arr, true); e != nil {
		t.Fatal("no boolean shape")
	}
	custom := Field{Type: TypeArray, IDKey: "_component", Items: arr.Items}
	if e := ResolveItemEntry(custom, map[string]any{"_component": "hero"}); e == nil || e.Name != "hero" {
		t.Fatal("custom idKey")
	}
	single := Field{Type: TypeArray, Items: []Field{{Key: "x", Type: TypeGroup}}}
	if e := ResolveItemEntry(single, map[string]any{"_type": "whatever"}); e == nil || e.Name != "x" {
		t.Fatal("single shape")
	}
	cfg := &Config{Blocks: map[string]Block{"hero": {Fields: []Field{{Key: "t"}}}}}
	legacy := Field{Type: TypeBlocks}
	if fs := ItemFields(cfg, legacy, map[string]any{"_component": "hero"}); len(fs) != 1 {
		t.Fatal("legacy blocks lookup")
	}
}

func TestKnownKnobs(t *testing.T) {
	if !KnownKnobs(TypeText)["regex"] || KnownKnobs(TypeText)["options"] || !KnownKnobs(TypeURL)["regex"] {
		t.Fatal("knobs")
	}
	if Canonical(TypeMarkdown) != TypeRichText || Canonical(TypeObject) != TypeGroup || Canonical(TypeBlocks) != TypeArray {
		t.Fatal("canonical")
	}
}
