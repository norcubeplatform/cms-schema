package resolve_test

import (
	"reflect"
	"testing"

	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Only translatable texts are compared: a link, an image, a number, a
// select's value or a slug that changes is no change.
func TestChangedTexts(t *testing.T) {
	coll := schema.Collection{HasBody: true, Fields: []schema.Field{
		{Key: "title", Type: schema.TypeText, Label: "Title"},
		{Key: "slug", Type: schema.TypeText},
		{Key: "href", Type: schema.TypeLink},
		{Key: "image", Type: schema.TypeImage},
		{Key: "count", Type: schema.TypeNumber},
		{Key: "variant", Type: schema.TypeSelect},
		{Key: "icon", Type: schema.TypeText, Regex: "^[a-z-]+$"},
		{Key: "items", Type: schema.TypeArray, Label: "Items", Items: []schema.Field{{Type: schema.TypeGroup, Fields: []schema.Field{{Key: "name", Type: schema.TypeText, Label: "Name"}}}}},
	}}
	before := map[string]any{"title": "Hello there", "slug": "a", "href": "/a/", "image": "/a.jpg", "count": 1, "variant": "primary", "icon": "star",
		"items": []any{map[string]any{"name": "One"}, map[string]any{"name": "Two"}}}
	same := map[string]any{"title": "  Hello\n  there ", "slug": "b", "href": "/b/", "image": "/b.jpg", "count": 2, "variant": "secondary", "icon": "moon",
		"items": []any{map[string]any{"name": "One"}, map[string]any{"name": "Two"}}}
	got, err := resolve.ChangedTexts(nil, coll, before, "Body text.", same, "Body\ntext.")
	if err != nil || len(got) != 0 {
		t.Fatalf("no text changed: %v %v", got, err)
	}
	after := map[string]any{"title": "Hello", "items": []any{map[string]any{"name": "One"}, map[string]any{"name": "Three"}, map[string]any{"name": "Four"}}}
	got, err = resolve.ChangedTexts(nil, coll, before, "Body text.", after, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []resolve.TextChange{
		{Path: "title", Label: "Title", Kind: resolve.TextChanged, Before: "Hello there", After: "Hello"},
		{Path: "items[1].name", Label: "Name", Kind: resolve.TextChanged, Before: "Two", After: "Three"},
		{Path: "items[2].name", Label: "Name", Kind: resolve.TextAdded, After: "Four"},
		{Path: resolve.BodyPath, Label: "Body", Kind: resolve.TextRemoved, Before: "Body text.", Body: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
