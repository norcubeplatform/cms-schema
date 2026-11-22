package resolve_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

func TestReferences(t *testing.T) {
	fields := schema.ParseFields([]any{
		map[string]any{"key": "cta", "type": "link"},
		map[string]any{"key": "hero", "type": "image"},
		map[string]any{"key": "author", "type": "reference", "multiple": true},
		map[string]any{"key": "text", "type": "richtext"},
		map[string]any{"key": "pics", "type": "array", "items": []any{map[string]any{"type": "image"}}},
	})
	fm := map[string]any{
		"cta": "/about/", "hero": "https://cdn.example/x.jpg", "author": []any{"a", " ", "b"},
		"text": "See [x](/x/)", "pics": []any{"/images/p.jpg"},
	}
	var got []string
	for _, r := range resolve.References(nil, fields, fm, "![b](/images/b.png)") {
		got = append(got, fmt.Sprintf("%s %s %s", r.Kind, r.Path, r.Value))
	}
	want := []string{
		"link cta /about/",
		"reference author[0] a",
		"reference author[2] b",
		"link text /x/",
		"link body /images/b.png",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
}
