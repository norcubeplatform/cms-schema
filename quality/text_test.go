package quality

import (
	"strings"
	"testing"
)

func TestScanText(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		html   bool
		levels []int
		links  []string
		alts   []string
	}{
		{"atx and closing hashes", "# One #\n## Two\n####### not a heading", false, []int{1, 2}, nil, nil},
		{"setext", "Title\n=====\n\nSub\n---\n\n- item\n---\n", false, []int{1, 2}, nil, nil},
		{"tilde fence", "~~~\n# no\n~~~\n### yes", false, []int{3}, nil, nil},
		{"links and images", "[a **b**](/x) ![pic](/i.png) [![logo](/l.png)](/home)", false, nil, []string{"a b", "logo"}, []string{"pic", "logo"}},
		{"html in markdown", "## A\n<h4 class=x>B &amp; C</h4>", false, []int{2, 4}, nil, nil},
		{"html only, markdown signs are text", "# not<h2>x</h2><a href=\"/a\">Go &rarr;</a><img src=a.png alt=\"Chart\">", true, []int{2}, []string{"Go →"}, []string{"Chart"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			heads, links, images := scanText(c.text, c.html)
			var levels []int
			for _, h := range heads {
				levels = append(levels, h.level)
			}
			var texts, alts []string
			for _, l := range links {
				texts = append(texts, l.text)
			}
			for _, im := range images {
				alts = append(alts, im.alt)
			}
			if strings.Join(itoa(levels), ",") != strings.Join(itoa(c.levels), ",") || strings.Join(texts, "|") != strings.Join(c.links, "|") || strings.Join(alts, "|") != strings.Join(c.alts, "|") {
				t.Errorf("levels %v links %q alts %q", levels, texts, alts)
			}
		})
	}
	// A long text is read up to the bound and no further.
	heads, _, _ := scanText(strings.Repeat("x", maxTextScan)+"\n# late", false)
	if len(heads) != 0 {
		t.Errorf("read past the bound: %v", heads)
	}
}

func itoa(n []int) []string {
	out := make([]string, len(n))
	for i, v := range n {
		out[i] = string(rune('0' + v))
	}
	return out
}
