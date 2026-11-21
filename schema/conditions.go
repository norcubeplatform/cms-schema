package schema

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Scope is what a condition path can reach: the current group's values,
// the chain of parents up to the root, and the nearest array item.
type Scope struct {
	Self    map[string]any
	Parents []map[string]any
	Root    map[string]any
	Item    map[string]any
}

// RootScope is the scope at the top of a document.
func RootScope(frontmatter map[string]any) Scope {
	return Scope{Self: frontmatter, Root: frontmatter}
}

// Child is the scope after descending into a group.
func (s Scope) Child(child map[string]any) Scope {
	return Scope{Self: child, Parents: append([]map[string]any{s.Self}, s.Parents...), Root: s.Root, Item: s.Item}
}

// ItemScope is the scope after entering an array item.
func (s Scope) ItemScope(item map[string]any) Scope {
	return Scope{Self: item, Parents: append([]map[string]any{s.Self}, s.Parents...), Root: s.Root, Item: item}
}

// ResolvePath reads a condition path (`key`, `$parent.key`,
// `$root.a.b`, `$item.k`, dotted descents, numeric array indexes)
// against the scope. The second result is false when any segment is
// missing.
func ResolvePath(path string, s Scope) (any, bool) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, false
	}
	var cursor any = s.Self
	i := 0
	depth := 0
	for i < len(parts) && parts[i] == "$parent" {
		depth++
		i++
	}
	switch {
	case depth > 0:
		if depth-1 >= len(s.Parents) {
			return nil, false
		}
		cursor = s.Parents[depth-1]
	case i < len(parts) && parts[i] == "$root":
		cursor = s.Root
		i++
	case i < len(parts) && parts[i] == "$item":
		if s.Item == nil {
			return nil, false
		}
		cursor = s.Item
		i++
	}
	for ; i < len(parts); i++ {
		switch cur := cursor.(type) {
		case map[string]any:
			v, ok := cur[parts[i]]
			if !ok {
				return nil, false
			}
			cursor = v
		case []any:
			idx, err := strconv.Atoi(parts[i])
			if err != nil || idx < 0 || idx >= len(cur) {
				return nil, false
			}
			cursor = cur[idx]
		default:
			return nil, false
		}
	}
	return cursor, true
}

func splitPath(path string) []string {
	var out []string
	for _, p := range strings.Split(path, ".") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Eval evaluates a condition against the scope.
func Eval(c Condition, s Scope) bool {
	switch {
	case len(c.All) > 0:
		for _, sub := range c.All {
			if !Eval(sub, s) {
				return false
			}
		}
		return true
	case len(c.Any) > 0:
		for _, sub := range c.Any {
			if Eval(sub, s) {
				return true
			}
		}
		return false
	case c.Not != nil:
		return !Eval(*c.Not, s)
	}
	actual, _ := ResolvePath(c.Field, s)
	return runOp(c.Op, actual, c.Value)
}

func runOp(op string, actual, expected any) bool {
	switch op {
	case OpEq:
		return looseEqual(actual, expected)
	case OpNeq:
		return !looseEqual(actual, expected)
	case OpIn:
		arr := coerceArray(expected)
		if arr == nil {
			return false
		}
		for _, e := range arr {
			if looseEqual(actual, e) {
				return true
			}
		}
		return false
	case OpNotIn:
		arr := coerceArray(expected)
		if arr == nil {
			return false
		}
		for _, e := range arr {
			if looseEqual(actual, e) {
				return false
			}
		}
		return true
	case OpTruthy:
		return Truthy(actual)
	case OpFalsy:
		return !Truthy(actual)
	case OpEmpty:
		return IsEmptyValue(actual)
	case OpNonEmpty:
		return !IsEmptyValue(actual)
	case OpGt, OpLt:
		a, aok := numOf(actual)
		b, bok := numOf(expected)
		if !aok || !bok {
			return false
		}
		if op == OpGt {
			return a > b
		}
		return a < b
	case OpMatches:
		as, aok := actual.(string)
		es, eok := expected.(string)
		if !aok || !eok {
			return false
		}
		re, err := regexp.Compile(es)
		if err != nil {
			return false
		}
		return re.MatchString(as)
	}
	return false
}

// Truthy mirrors JavaScript truthiness for the values YAML produces.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int, int64, float64, float32:
		n, _ := numOf(x)
		return n != 0
	}
	return true
}

// IsEmptyValue is the condition evaluator's notion of empty: nil, an
// empty string, an empty list. Whitespace is not empty.
func IsEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// MaxCompareDepth bounds how deep `eq`, `neq`, `in` and `notIn` descend
// into lists and maps. Values nested deeper compare unequal, so a
// cyclic or absurdly deep value costs bounded work and never overflows
// the stack.
const MaxCompareDepth = 32

// looseEqual is the equality of `eq`, `neq`, `in` and `notIn`. It
// never panics, whatever the two values are.
//
//   - nil and absent are alike; nil equals nothing else (not an empty
//     string, not an empty list: use `empty` for "no value").
//   - Numbers compare by value across int and float kinds.
//   - Lists compare element by element, in order, with these same
//     rules; maps by key set, each value with these same rules.
//   - Anything else compares with Go's `==` when its type allows it
//     and structurally (reflect.DeepEqual) when it does not.
func looseEqual(a, b any) bool {
	return looseEqualAt(a, b, 0)
}

func looseEqualAt(a, b any, depth int) bool {
	if a == nil && b == nil {
		return true
	}
	if an, ok := numOf(a); ok {
		if bn, ok := numOf(b); ok {
			return an == bn
		}
		return false
	}
	if al, ok := listOf(a); ok {
		bl, ok := listOf(b)
		if !ok || len(al) != len(bl) || depth >= MaxCompareDepth {
			return false
		}
		for i := range al {
			if !looseEqualAt(al[i], bl[i], depth+1) {
				return false
			}
		}
		return true
	}
	if am, ok := a.(map[string]any); ok {
		bm, ok := b.(map[string]any)
		if !ok || len(am) != len(bm) || depth >= MaxCompareDepth {
			return false
		}
		for k, av := range am {
			bv, ok := bm[k]
			if !ok || !looseEqualAt(av, bv, depth+1) {
				return false
			}
		}
		return true
	}
	return safeEqual(a, b)
}

// listOf reads the list shapes a condition value can take.
func listOf(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

// safeEqual is `a == b` for the values Go can compare, and a structural
// comparison for the ones it cannot (a == b on two values of one
// uncomparable dynamic type panics; this never does).
func safeEqual(a, b any) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = reflect.DeepEqual(a, b)
		}
	}()
	return a == b
}

func coerceArray(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case string:
		t := strings.TrimSpace(x)
		if strings.HasPrefix(t, "[") {
			var parsed []any
			if err := json.Unmarshal([]byte(t), &parsed); err == nil {
				return parsed
			}
		}
		if t == "" {
			return nil
		}
		parts := strings.Split(t, ",")
		out := make([]any, len(parts))
		for i, p := range parts {
			out[i] = strings.TrimSpace(p)
		}
		return out
	}
	return nil
}
