package schema

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseTemplate(t *testing.T) {
	got, err := ParseTemplate("{field:kind|label} – {value} – {{ site_name }}")
	if err != nil {
		t.Fatal(err)
	}
	want := []TemplatePart{
		{Kind: TemplateField, Path: "kind", Label: true},
		{Kind: TemplateLiteral, Text: " – "},
		{Kind: TemplateValue},
		{Kind: TemplateLiteral, Text: " – "},
		{Kind: TemplateVariable, Variable: "site_name"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if fs := TemplateFields(append(got, TemplatePart{Kind: TemplateField, Path: "kind"})); len(fs) != 1 || fs[0] != "kind" {
		t.Errorf("fields %v", fs)
	}
	for _, bad := range []string{"", "  ", "Home", "{value} {title}", "{value} }", "{value", "{field:} {value}", "{field:a..b} {value}", "{field:a[0]} {value}", "{value}" + strings.Repeat("x", MaxTemplateBytes)} {
		if _, err := ParseTemplate(bad); !errors.Is(err, ErrTemplate) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestParseLocalizedKnobs(t *testing.T) {
	f := ParseField(map[string]any{"key": "img", "type": "image", "label": "I", "alt": map[string]any{"from": []any{"a", " b "}}})
	if !reflect.DeepEqual(f.AltFrom, []string{"a", "b"}) || f.AltSeparator != DefaultAltSeparator || f.Alt != "" || len(f.Extra) != 0 {
		t.Errorf("alt: %+v", f)
	}
	g := ParseField(map[string]any{"key": "t", "type": "group", "label": "T", "localized": true, "fallback": "cs", "role": "seoTitle",
		"fields": []any{map[string]any{"key": "cs", "type": "text"}, map[string]any{"key": "en", "type": "text"}}})
	if !g.IsLocalized() || g.Fallback != "cs" || g.ValueType() != TypeText || !g.HoldsRole(RoleSEOTitle) || g.HoldsRole(RoleShareImage) || len(g.Extra) != 0 {
		t.Errorf("group: %+v", g)
	}
	if plain := ParseField(map[string]any{"key": "t", "type": "group", "fields": []any{}}); plain.HoldsRole(RoleSEOTitle) || !plain.HoldsRole(RoleNone) {
		t.Error("a plain group holds no role")
	}
	sel := ParseField(map[string]any{"key": "r", "type": "select", "role": "noindex", "noindexWhen": []any{"noindex", 1}, "options": []any{
		map[string]any{"value": "noindex", "label": "Hidden", "labels": map[string]any{"en": "Hidden", "cs": " Skrytá ", "it": 3}}}})
	if !reflect.DeepEqual(sel.NoindexWhen, []string{"noindex", "1"}) || !sel.HoldsRole(RoleNoindex) || sel.HoldsRole(RoleDraft) {
		t.Errorf("select: %+v", sel)
	}
	if l, _ := OptionLabel(sel.Options, "noindex", "cs"); l != "Skrytá" {
		t.Errorf("cs label %q", l)
	}
	if l, _ := OptionLabel(sel.Options, "noindex", "de"); l != "Hidden" {
		t.Errorf("fallback label %q", l)
	}
	if _, ok := OptionLabel(sel.Options, "nope", "cs"); ok {
		t.Error("not an option")
	}
	rt := ParseField(map[string]any{"key": "d", "type": "richtext", "role": "seoDescription"})
	if rt.HoldsRole(RoleSEODescription) {
		t.Error("a rich text description needs its excerpt")
	}
	rt.Excerpt = ExcerptFirstParagraph
	if !rt.HoldsRole(RoleSEODescription) {
		t.Error("with its excerpt it holds the role")
	}
}

func TestOneLanguagePerFile(t *testing.T) {
	loc := []Field{{Key: "t", Type: TypeGroup, Localized: true, Fields: []Field{{Key: "cs", Type: TypeText}}}}
	inList := []Field{{Key: "l", Type: TypeArray, Items: []Field{{Key: "i", Type: TypeGroup, Fields: loc}}}}
	cases := []struct {
		c    Collection
		want bool
	}{
		{Collection{LocaleFrom: LocaleFromNone, Fields: loc}, true},
		{Collection{Fields: inList}, true},
		{Collection{LocaleFrom: LocaleFromPath, Fields: loc}, false},
		{Collection{LocaleFrom: "field:lang", Fields: loc}, false},
		{Collection{Translations: &Translations{By: "path"}, Fields: loc}, false},
		{Collection{Translations: &Translations{Locales: []string{"cs"}}, Fields: loc}, true},
		{Collection{}, false},
	}
	for i, c := range cases {
		if got := c.c.Localized(); got != c.want {
			t.Errorf("%d: %v", i, got)
		}
	}
}
