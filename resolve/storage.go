package resolve

import (
	"errors"
	"github.com/norcubeplatform/cms-schema/report"
	"strings"

	"github.com/norcubeplatform/cms-schema/schema"
)

// PublicValue composes the value that lands in frontmatter when the
// file `key` (storage-relative) is picked from a storage:
//   - publicUrl /uploads + key foo.jpg → /uploads/foo.jpg
//   - publicUrl https://cdn.x + key foo.jpg → https://cdn.x/foo.jpg
//   - no publicUrl, path src/assets + key a.svg → /src/assets/a.svg
func PublicValue(s schema.Storage, key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	if s.Type != schema.StorageTypeRepo || s.Path == "" {
		return key
	}
	if s.PublicURL != "" {
		return s.PublicURL + "/" + key
	}
	return "/" + s.Path + "/" + key
}

// KeyFromValue is the inverse of PublicValue: the storage-relative key a
// frontmatter value names, or false when the value does not belong to
// the storage.
func KeyFromValue(s schema.Storage, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
		if s.PublicURL != "" && strings.HasPrefix(value, s.PublicURL+"/") {
			return strings.TrimPrefix(value, s.PublicURL+"/"), true
		}
		return "", false
	}
	if s.Type != schema.StorageTypeRepo || s.Path == "" {
		return "", false
	}
	if s.PublicURL != "" && strings.HasPrefix(value, s.PublicURL+"/") {
		return strings.TrimPrefix(value, s.PublicURL+"/"), true
	}
	prefix := "/" + s.Path + "/"
	if strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix), true
	}
	return "", false
}

// ValuePath joins a storage-relative key with the storage's root to a
// repository path.
func ValuePath(s schema.Storage, key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	if s.Type != schema.StorageTypeRepo || s.Path == "" {
		return key
	}
	return s.Path + "/" + key
}

// AssetRepoPath maps an image, file or link value to the repository
// path it would be served from: a storage's publicUrl or repo path,
// else the public directory for root-relative values, else the value as
// a repository path. Query strings and fragments are dropped.
func AssetRepoPath(cfg *schema.Config, value string) string {
	value = strings.TrimSpace(value)
	if i := strings.IndexAny(value, "?#"); i >= 0 {
		value = value[:i]
	}
	if value == "" {
		return ""
	}
	for _, s := range cfg.Storages {
		if s.Type != schema.StorageTypeRepo || s.Path == "" {
			continue
		}
		if key, ok := KeyFromValue(s, value); ok {
			return CleanPath(ValuePath(s, key))
		}
	}
	if strings.HasPrefix(value, "/") {
		return CleanPath(cfg.PublicDir + value)
	}
	return CleanPath(value)
}

// ValidStorage reports what is wrong with a storage entry: one problem
// per cause, with its params (report.StorageCauses and, for a usedByCode
// entry, report.PatternCauses). Empty when it is fine. The finding is
// schema.invalid_storage with the storage's name as Params.Key.
func ValidStorage(s schema.Storage) []report.Problem {
	var problems []report.Problem
	add := func(c report.Cause, p *report.Params) {
		problems = append(problems, report.Problem{Cause: c, Params: p})
	}
	if s.Type == "" {
		add(report.CauseTypeMissing, nil)
	} else if s.Type != schema.StorageTypeRepo {
		add(report.CauseTypeUnimplemented, &report.Params{Value: s.Type})
	}
	if s.Type == schema.StorageTypeRepo && s.Path == "" {
		add(report.CausePathMissing, nil)
	}
	switch s.Mode {
	case schema.StorageModeReadOnly, schema.StorageModeReadWrite:
	default:
		add(report.CauseModeInvalid, &report.Params{Value: s.Mode, Allowed: []string{schema.StorageModeReadOnly, schema.StorageModeReadWrite}})
	}
	switch s.Naming {
	case schema.StorageNamingOriginal, schema.StorageNamingSlug, schema.StorageNamingSlugHash, schema.StorageNamingUUID:
	default:
		add(report.CauseNamingInvalid, &report.Params{Value: s.Naming, Allowed: []string{schema.StorageNamingSlug, schema.StorageNamingSlugHash, schema.StorageNamingUUID}})
	}
	switch s.Inline {
	case "", schema.StorageInlineRef, schema.StorageInlineSVG:
	default:
		add(report.CauseInlineInvalid, &report.Params{Value: s.Inline, Allowed: []string{schema.StorageInlineRef, schema.StorageInlineSVG}})
	}
	switch s.Dedup {
	case "":
	case schema.StorageDedupHash:
		if s.Naming != schema.StorageNamingSlugHash {
			add(report.CauseDedupNeedsSlugHash, nil)
		}
	default:
		add(report.CauseDedupInvalid, &report.Params{Value: s.Dedup, Allowed: []string{schema.StorageDedupHash}})
	}
	if v, has := s.Raw[schema.StorageKeyOptimizedAtBuild]; has && v != nil {
		if _, ok := v.(bool); !ok {
			add(report.CauseOptimizedAtBuildNotBoolean, &report.Params{Value: report.FlowValue(v)})
		}
	}
	return append(problems, usedByCodeProblems(s)...)
}

// usedByCodeProblems checks `usedByCode`: a list of at most
// schema.MaxUsedByCode path patterns, each relative to the storage's
// path and listed once.
func usedByCodeProblems(s schema.Storage) []report.Problem {
	v, has := s.Raw[schema.StorageKeyUsedByCode]
	if !has || v == nil {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return []report.Problem{{Cause: report.CauseUsedByCodeNotList}}
	}
	if len(list) > schema.MaxUsedByCode {
		return []report.Problem{{Cause: report.CauseUsedByCodeTooMany, Params: &report.Params{Count: report.Int(int64(len(list))), Max: report.Num(schema.MaxUsedByCode)}}}
	}
	var problems []report.Problem
	seen := map[string]bool{}
	for _, item := range list {
		p, ok := item.(string)
		if !ok {
			problems = append(problems, report.Problem{Cause: report.CauseUsedByCodeNotPath, Params: &report.Params{Value: report.FlowValue(item)}})
			continue
		}
		if err := schema.CheckCodePattern(p); err != nil {
			var pe *schema.CodePatternError
			if errors.As(err, &pe) {
				problems = append(problems, pe.Problem)
			}
			continue
		}
		if seen[p] {
			problems = append(problems, report.Problem{Cause: report.CauseUsedByCodeTwice, Params: &report.Params{Path: p}})
		}
		seen[p] = true
	}
	return problems
}
