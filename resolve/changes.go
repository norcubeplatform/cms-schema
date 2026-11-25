package resolve

import (
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

// Kinds of a TextChange.
const (
	TextChanged = "changed"
	TextAdded   = "added"
	TextRemoved = "removed"
)

// TextChange is one translatable text that differs between two versions
// of a page.
type TextChange struct {
	// Path is where the text sits (`a.b[2].c`, or BodyPath).
	Path  string
	Label string
	// Kind is TextChanged, TextAdded or TextRemoved.
	Kind string
	// Before and After are the text in each version; Before is empty for
	// an added text and After for a removed one.
	Before string
	After  string
	Body   bool
}

// ChangedTexts lists the translatable texts (TranslatableTexts) that
// differ between two versions of a page of the collection, by path, in
// the order of the newer version and then the texts only the older one
// had. Texts are compared after trimming and joining runs of white space,
// so a reflowed paragraph is the same text. Everything that is not a
// translatable text (a link, an image, a number, a select's value, a
// slug, a field with `translatable: false`) is never compared, so a change
// to it is no change here.
//
// When either version has more than MaxTranslatableTexts texts, the
// comparison is of the first ones and ErrTooManyTexts is returned with
// it.
func ChangedTexts(cfg *schema.Config, coll schema.Collection, beforeFM map[string]any, beforeBody string, afterFM map[string]any, afterBody string) ([]TextChange, error) {
	before, errBefore := TranslatableTexts(cfg, coll, beforeFM, beforeBody)
	after, errAfter := TranslatableTexts(cfg, coll, afterFM, afterBody)
	key := func(t Text) string {
		if t.Body {
			return "\x00" + BodyPath
		}
		return t.Path
	}
	old := make(map[string]Text, len(before))
	for _, t := range before {
		old[key(t)] = t
	}
	var out []TextChange
	seen := make(map[string]bool, len(after))
	for _, t := range after {
		k := key(t)
		seen[k] = true
		was, ok := old[k]
		switch {
		case !ok:
			out = append(out, TextChange{Path: t.Path, Label: t.Label, Kind: TextAdded, After: t.Value, Body: t.Body})
		case sameText(was.Value, t.Value):
		default:
			out = append(out, TextChange{Path: t.Path, Label: t.Label, Kind: TextChanged, Before: was.Value, After: t.Value, Body: t.Body})
		}
	}
	for _, t := range before {
		if !seen[key(t)] {
			out = append(out, TextChange{Path: t.Path, Label: t.Label, Kind: TextRemoved, Before: t.Value, Body: t.Body})
		}
	}
	if errBefore != nil {
		return out, errBefore
	}
	return out, errAfter
}

// sameText reports whether two texts read the same: equal after trimming
// and joining runs of white space.
func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}
