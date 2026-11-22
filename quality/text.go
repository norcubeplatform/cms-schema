package quality

import (
	"html"
	"regexp"
	"sort"
	"strings"
)

// heading is one heading of a text.
type heading struct {
	level int
	text  string
	at    int
}

// link is one link of a text: what a screen reader announces and where
// it leads.
type link struct {
	text, href string
	at         int
}

// image is one image of a text, with its alt text; hasAlt is false for
// an HTML image without an alt attribute. html tells an <img> from a
// Markdown image.
type image struct {
	src, alt     string
	hasAlt, html bool
	at           int
}

var (
	atxHeading  = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	setextLine  = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
	fenceLine   = regexp.MustCompile("^ {0,3}(```|~~~)")
	htmlHeading = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]\s*>`)
	htmlLink    = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a\s*>`)
	htmlImage   = regexp.MustCompile(`(?is)<img\b([^>]*)>`)
	htmlTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	attrRe      = regexp.MustCompile(`(?is)\b([a-z-]+)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	// mdImage matches ![alt](src); mdLink [text](href) not preceded by !.
	mdImage     = regexp.MustCompile(`!\[([^\]]*)\]\(\s*<?([^)\s>]*)>?(?:\s+"[^"]*")?\s*\)`)
	mdLink      = regexp.MustCompile(`\[((?:[^\[\]]|\[[^\[\]]*\])*)\]\(\s*<?([^)\s>]*)>?(?:\s+"[^"]*")?\s*\)`)
	listOrQuote = regexp.MustCompile(`^ {0,3}([-*+]|\d+[.)]|>)[ \t]`)
	bareAlt     = regexp.MustCompile(`(?i)(^|\s)alt(\s|$|/)`)
)

// scanText reads the headings, links and images of a text, in order.
// Markdown is read line by line outside fenced code, with any HTML in
// it; HTML is read as HTML. A text is read up to maxTextScan
// characters.
func scanText(text string, isHTML bool) (heads []heading, links []link, images []image) {
	if len(text) > maxTextScan {
		text = text[:maxTextScan]
	}
	if !isHTML {
		heads, links, images = scanMarkdown(text)
	}
	h, l, i := scanHTML(text)
	heads, links, images = append(heads, h...), append(links, l...), append(images, i...)
	sort.SliceStable(heads, func(a, b int) bool { return heads[a].at < heads[b].at })
	sort.SliceStable(links, func(a, b int) bool { return links[a].at < links[b].at })
	sort.SliceStable(images, func(a, b int) bool { return images[a].at < images[b].at })
	return heads, links, images
}

func scanMarkdown(text string) (heads []heading, links []link, images []image) {
	offset := 0
	inFence, fence := false, ""
	prev := ""
	prevIsPara := false
	for _, line := range strings.Split(text, "\n") {
		at := offset
		offset += len(line) + 1
		line = strings.TrimRight(line, "\r")
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			switch {
			case !inFence:
				inFence, fence = true, m[1]
			case m[1] == fence:
				inFence = false
			}
			prevIsPara = false
			continue
		}
		if inFence {
			continue
		}
		if m := atxHeading.FindStringSubmatch(line); m != nil {
			heads = append(heads, heading{level: len(m[1]), text: strings.TrimSpace(m[2]), at: at})
			prevIsPara = false
			prev = line
			continue
		}
		if prevIsPara {
			if m := setextLine.FindStringSubmatch(line); m != nil {
				level := 2
				if m[1][0] == '=' {
					level = 1
				}
				heads = append(heads, heading{level: level, text: strings.TrimSpace(prev), at: at - len(prev) - 1})
				prevIsPara = false
				continue
			}
		}
		for _, m := range mdImage.FindAllStringSubmatchIndex(line, -1) {
			images = append(images, image{alt: strings.TrimSpace(line[m[2]:m[3]]), src: line[m[4]:m[5]], hasAlt: true, at: at + m[0]})
		}
		for _, m := range mdLink.FindAllStringSubmatchIndex(line, -1) {
			if m[0] > 0 && line[m[0]-1] == '!' {
				continue
			}
			inner := line[m[2]:m[3]]
			// An image used as a link is announced by its alt text.
			if im := mdImage.FindStringSubmatch(inner); im != nil {
				inner = im[1]
			}
			links = append(links, link{text: markdownText(inner), href: line[m[4]:m[5]], at: at + m[0]})
		}
		blank := strings.TrimSpace(line) == ""
		prevIsPara = !blank && !listOrQuote.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "<")
		prev = line
	}
	return heads, links, images
}

func scanHTML(text string) (heads []heading, links []link, images []image) {
	if !strings.Contains(text, "<") {
		return nil, nil, nil
	}
	for _, m := range htmlHeading.FindAllStringSubmatchIndex(text, -1) {
		heads = append(heads, heading{level: int(text[m[2]] - '0'), text: htmlText(text[m[4]:m[5]]), at: m[0]})
	}
	for _, m := range htmlLink.FindAllStringSubmatchIndex(text, -1) {
		attrs := attributes(text[m[2]:m[3]])
		inner := text[m[4]:m[5]]
		t := htmlText(inner)
		if t == "" {
			if label := strings.TrimSpace(attrs["aria-label"]); label != "" {
				t = label
			}
		}
		if t == "" {
			for _, im := range htmlImage.FindAllStringSubmatch(inner, -1) {
				if alt := strings.TrimSpace(attributes(im[1])["alt"]); alt != "" {
					t = alt
					break
				}
			}
		}
		links = append(links, link{text: t, href: attrs["href"], at: m[0]})
	}
	for _, m := range htmlImage.FindAllStringSubmatchIndex(text, -1) {
		attrs := attributes(text[m[2]:m[3]])
		alt, has := attrs["alt"]
		images = append(images, image{src: attrs["src"], alt: strings.TrimSpace(alt), hasAlt: has, html: true, at: m[0]})
	}
	return heads, links, images
}

// attributes reads the attributes of an HTML tag, names in lowercase,
// values unescaped.
func attributes(s string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(s, -1) {
		v := m[3]
		if m[4] != "" {
			v = m[4]
		} else if m[5] != "" {
			v = m[5]
		}
		out[strings.ToLower(m[1])] = html.UnescapeString(v)
	}
	// A bare `alt` (`<img alt>`) is present with an empty value.
	if _, ok := out["alt"]; !ok && bareAlt.MatchString(s) {
		out["alt"] = ""
	}
	return out
}

// htmlText is what a run of HTML says as text.
func htmlText(s string) string {
	return collapse(html.UnescapeString(htmlTag.ReplaceAllString(s, " ")))
}

// markdownText is what a link's Markdown text says: emphasis and code
// marks removed.
func markdownText(s string) string {
	s = htmlText(s)
	return collapse(strings.NewReplacer("**", "", "__", "", "*", "", "_", "", "`", "").Replace(s))
}
