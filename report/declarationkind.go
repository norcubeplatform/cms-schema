package report

// DeclarationKind is the kind of schema declaration a refused fact looks
// up (Params.DeclarationKind): a collection, a block or an input. Its
// values are codes, frozen like causes: never renamed, never reused for
// another kind. English gives the words this module's sentences use; a
// client that words refusals itself words each kind (DeclarationKinds
// lists them, and catalog.json lists them as the values of the param
// declarationKind).
type DeclarationKind string

// Declaration kinds.
const (
	DeclarationKindCollection DeclarationKind = "collection"
	DeclarationKindBlock      DeclarationKind = "block"
	DeclarationKindInput      DeclarationKind = "input"
)

// declarationKinds is every kind with the words this module's sentences
// use for it, in the order DeclarationKinds lists them: the one place a
// kind is declared.
var declarationKinds = []struct {
	kind    DeclarationKind
	english string
}{
	{DeclarationKindCollection, "collection"},
	{DeclarationKindBlock, "block"},
	{DeclarationKindInput, "input"},
}

// DeclarationKinds lists every declaration kind.
func DeclarationKinds() []DeclarationKind {
	out := make([]DeclarationKind, len(declarationKinds))
	for i, k := range declarationKinds {
		out[i] = k.kind
	}
	return out
}

// English is the kind in this module's sentences: "collection". A value
// that is no kind of this version is returned as it is.
func (k DeclarationKind) English() string {
	for _, v := range declarationKinds {
		if v.kind == k {
			return v.english
		}
	}
	return string(k)
}
