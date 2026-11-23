package schema

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
)

// ErrCodePattern marks a usedByCode entry that is not a path pattern.
var ErrCodePattern = errors.New("not a usedByCode path")

// CheckCodePattern reports what is wrong with one usedByCode entry: a
// path relative to its storage folder, where `*`, `?` and `[...]` match
// within a name (as path.Match) and a `**` name any number of folders.
// Nil when it is one.
func CheckCodePattern(p string) error {
	bad := func(why string) error { return fmt.Errorf("%w: %q %s", ErrCodePattern, p, why) }
	switch {
	case p == "":
		return bad("is empty")
	case len(p) > MaxUsedByCodePattern:
		return bad(fmt.Sprintf("is longer than %d bytes", MaxUsedByCodePattern))
	case strings.HasPrefix(p, "/"):
		return bad("starts with /: write it relative to the storage's path")
	case strings.ContainsRune(p, '\\'):
		return bad("holds a backslash")
	case strings.ContainsFunc(p, unicode.IsControl):
		return bad("holds a control character")
	}
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "":
			return bad("has an empty name between slashes")
		case seg == "." || seg == "..":
			return bad("names . or ..")
		case seg == "**":
		case strings.Contains(seg, "**"):
			return bad("uses ** inside a name: ** stands alone between slashes")
		default:
			if _, err := path.Match(seg, ""); err != nil {
				return bad("is not a valid pattern")
			}
		}
	}
	return nil
}

// HasCodeWildcard reports whether a usedByCode entry is a pattern
// rather than one file's path.
func HasCodeWildcard(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

// MatchCodePattern reports whether rel, a path relative to a storage
// folder, matches a usedByCode entry. An invalid entry matches nothing.
func MatchCodePattern(pattern, rel string) bool {
	if CheckCodePattern(pattern) != nil || rel == "" {
		return false
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// Any number of folders, the last name always matched by the
			// rest of the pattern.
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// KeepsFile reports whether the storage's usedByCode names a file, by
// its repository path. A path outside the storage folder is never kept.
func (s Storage) KeepsFile(repoPath string) bool {
	if s.Path == "" || len(s.UsedByCode) == 0 {
		return false
	}
	rel, ok := strings.CutPrefix(repoPath, s.Path+"/")
	if !ok || rel == "" {
		return false
	}
	for _, p := range s.UsedByCode {
		if MatchCodePattern(p, rel) {
			return true
		}
	}
	return false
}
