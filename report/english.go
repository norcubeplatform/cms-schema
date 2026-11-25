package report

import (
	"strconv"
	"strings"
)

// Helpers the English sentences of the catalog are made with. They keep
// today's wording: a client that wants other words, or another
// language, makes its own sentences from the same Params.

// q quotes a text as Go's %q does.
func q(s string) string { return strconv.Quote(s) }

// list renders a list of codes or names the way Go's %v prints a list:
// `[cs en]`.
func list(xs []string) string { return "[" + strings.Join(xs, " ") + "]" }

// comma joins names with ", ".
func comma(xs []string) string { return strings.Join(xs, ", ") }

// num renders a bound or a measured value without an exponent.
func num(p *float64) string {
	if p == nil {
		return "0"
	}
	return FormatNumber(*p)
}

// gnum renders a number as Go's %v does.
func gnum(p *float64) string {
	if p == nil {
		return "0"
	}
	return FormatFloat(*p)
}

// integer renders a count.
func integer(p *int64) string { return strconv.FormatInt(deref(p), 10) }

// more is " and <n> more" when the names list leaves some out.
func more(p *Params) string {
	if deref(p.More) <= 0 {
		return ""
	}
	return " and " + integer(p.More) + " more"
}

// orNowhere is a link's address, or "nowhere" for none.
func orNowhere(s string) string {
	if strings.TrimSpace(s) == "" {
		return "nowhere"
	}
	return s
}

// truncate cuts a value shown inside a hint to 60 characters.
func truncate(s string) string {
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// plural is word, with an s unless n is 1.
func plural(n int64, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// or is s, else def when s is empty.
func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// custom is the schema's own message when it gives one, else def.
func custom(x in, def string) string { return or(x.CustomMessage, def) }
