package quality

import (
	"github.com/norcubeplatform/cms-schema/report"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// genericLinkText are link texts that say nothing about where a link
// leads, per language, compared after lowercasing and trimming
// punctuation and arrows. Every list is checked whatever the page's
// language, since pages mix languages in links. The English list follows
// Lighthouse's link-text audit.
var genericLinkText = map[string][]string{
	"en": {"click here", "click this", "click", "here", "right here", "go", "this", "this link", "link",
		"more", "read more", "learn more", "see more", "more info", "more information", "information", "continue", "details"},
	"cs": {"zde", "tady", "sem", "klikněte zde", "klikni zde", "klikněte sem", "klikni sem", "klikněte", "odkaz", "tento odkaz",
		"více", "víc", "číst více", "čtěte více", "zjistit více", "zjistěte více", "více informací", "další", "pokračovat", "podrobnosti"},
	"de": {"hier", "hier klicken", "klicken sie hier", "klick hier", "klicken", "link", "dieser link",
		"mehr", "weiterlesen", "mehr erfahren", "mehr lesen", "mehr informationen", "weiter", "details"},
}

// genericAltText are alt texts that describe no image, per language.
var genericAltText = map[string][]string{
	"en": {"image", "img", "photo", "picture", "pic", "graphic", "illustration", "icon", "logo", "banner", "placeholder", "untitled", "alt", "alt text"},
	"cs": {"obrázek", "obrazek", "foto", "fotka", "fotografie", "snímek", "ilustrace", "ikona", "logo", "grafika", "náhled", "bez názvu"},
	"de": {"bild", "foto", "grafik", "abbildung", "illustration", "symbol", "logo", "platzhalter", "ohne titel"},
}

var (
	genericLinks = flatten(genericLinkText)
	genericAlts  = flatten(genericAltText)
	// cameraName is a file name a camera or phone gives a picture.
	cameraName = regexp.MustCompile(`(?i)^(img|dsc|dscn|dcim|pxl|photo|image|screenshot|screen shot|bildschirmfoto|snímek obrazovky)[ _-]?\d`)
)

func flatten(m map[string][]string) map[string]bool {
	out := map[string]bool{}
	for _, list := range m {
		for _, w := range list {
			out[w] = true
		}
	}
	return out
}

// normalWords is a text as the lists compare it: lowercase, white space
// collapsed, punctuation and arrows trimmed from both ends.
func normalWords(s string) string {
	s = strings.ToLower(collapse(s))
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r)
	})
}

// GenericLinkText reports whether a link text is one of the generic
// phrases.
func GenericLinkText(text string) bool { return genericLinks[normalWords(text)] }

// PoorAlt reports why an alt text describes nothing, or "" when it may:
// it is a generic word, the image's file name, or a name a camera gives.
// image is the image's value (its address); its file name is compared
// with and without the extension, and a bare name without separators or
// digits ("acme" for acme.svg, a logo named after its company) passes.
func PoorAlt(alt, image string) report.Cause {
	a := strings.TrimSpace(alt)
	if a == "" {
		return ""
	}
	if genericAlts[normalWords(a)] {
		return report.CauseGenericWord
	}
	if cameraName.MatchString(a) {
		return report.CauseCameraFileName
	}
	base := path.Base(strings.TrimSpace(image))
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	if base == "" || base == "." || base == "/" {
		return ""
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	switch {
	case strings.EqualFold(a, base):
		return report.CauseFileName
	case strings.EqualFold(a, stem) && strings.ContainsFunc(a, func(r rune) bool { return r == '-' || r == '_' || unicode.IsDigit(r) }):
		return report.CauseFileName
	}
	return ""
}
