package schema

import "strings"

// HiddenPaths collects the content paths of the fields a document hides,
// statically (`hidden`) or by a `visibleIf` that is false for it. What
// sits inside a hidden group or list is hidden too and not listed on its
// own; PathHidden answers for any path.
func HiddenPaths(cfg *Config, fields []Field, value map[string]any) map[string]bool {
	out := map[string]bool{}
	Walk(cfg, fields, value, func(v Visit) bool {
		if v.Item {
			return true
		}
		if v.Field.Hidden || (v.Field.VisibleIf != nil && !Eval(*v.Field.VisibleIf, v.Scope)) {
			out[v.Path] = true
			return false
		}
		return true
	})
	return out
}

// PathHidden reports whether a content path is one of hidden or sits
// inside one of them.
func PathHidden(p string, hidden map[string]bool) bool {
	if p == "" || len(hidden) == 0 {
		return false
	}
	for h := range hidden {
		if p == h || strings.HasPrefix(p, h+".") || strings.HasPrefix(p, h+"[") {
			return true
		}
	}
	return false
}
