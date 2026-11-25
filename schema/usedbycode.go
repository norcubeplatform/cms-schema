package schema

import (
	"errors"
	"path"
	"strings"
	"unicode"

	"github.com/norcubeplatform/cms-schema/report"
)

// ErrCodePattern marks a usedByCode entry that is not a path pattern.
var ErrCodePattern = errors.New("not a usedByCode path")

// CodePatternError is a usedByCode entry that is not a path pattern:
// why, as a cause of report.PatternCauses with the entry as Params.Path.
// It matches ErrCodePattern.
type CodePatternError struct{ report.Problem }

func (e *CodePatternError) Error() string { return ErrCodePattern.Error() + ": " + e.String() }
func (e *CodePatternError) Unwrap() error { return ErrCodePattern }

// CheckCodePattern reports what is wrong with one usedByCode entry: a
// path relative to its storage folder, where `*`, `?` and `[...]` match
// within a name (as path.Match) and a `**` name any number of folders.
// Nil when it is one.
func CheckCodePattern(p string) error {
	bad := func(c report.Cause) error {
		return &CodePatternError{report.Problem{Cause: c, Params: &report.Params{Path: p}}}
	}
	switch {
	case p == "":
		return bad(report.CausePatternEmpty)
	case len(p) > MaxUsedByCodePattern:
		return &CodePatternError{report.Problem{Cause: report.CausePatternTooLong, Params: &report.Params{Path: p, Max: report.Num(MaxUsedByCodePattern)}}}
	case strings.HasPrefix(p, "/"):
		return bad(report.CausePatternAbsolute)
	case strings.ContainsRune(p, '\\'):
		return bad(report.CausePatternBackslash)
	case strings.ContainsFunc(p, unicode.IsControl):
		return bad(report.CausePatternControl)
	}
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "":
			return bad(report.CausePatternEmptyName)
		case seg == "." || seg == "..":
			return bad(report.CausePatternDotName)
		case seg == "**":
		case strings.Contains(seg, "**"):
			return bad(report.CausePatternDoubleStarName)
		default:
			if _, err := path.Match(seg, ""); err != nil {
				return bad(report.CausePatternInvalid)
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
