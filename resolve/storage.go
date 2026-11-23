package resolve

import (
	"fmt"
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

// ValidStorage reports what is wrong with a storage entry, in the words
// the service uses when it refuses the config. Empty when it is fine.
func ValidStorage(s schema.Storage) []string {
	var problems []string
	if s.Type == "" {
		problems = append(problems, "missing `type:` (the only shipped driver is repo)")
	} else if s.Type != schema.StorageTypeRepo {
		problems = append(problems, "type "+quote(s.Type)+" is not implemented (only repo is)")
	}
	if s.Type == schema.StorageTypeRepo && s.Path == "" {
		problems = append(problems, "missing `path:`")
	}
	switch s.Mode {
	case schema.StorageModeReadOnly, schema.StorageModeReadWrite:
	default:
		problems = append(problems, "invalid mode "+quote(s.Mode)+" (allowed: read-only, read-write)")
	}
	switch s.Naming {
	case schema.StorageNamingOriginal, schema.StorageNamingSlug, schema.StorageNamingSlugHash, schema.StorageNamingUUID:
	default:
		problems = append(problems, "invalid naming "+quote(s.Naming)+" (allowed: slug, slug+hash, uuid, or empty for original)")
	}
	switch s.Inline {
	case "", schema.StorageInlineRef, schema.StorageInlineSVG:
	default:
		problems = append(problems, "invalid inline "+quote(s.Inline)+" (allowed: ref, svg)")
	}
	switch s.Dedup {
	case "":
	case schema.StorageDedupHash:
		if s.Naming != schema.StorageNamingSlugHash {
			problems = append(problems, "`dedup: hash` requires `naming: slug+hash`")
		}
	default:
		problems = append(problems, "invalid dedup "+quote(s.Dedup)+" (allowed: hash)")
	}
	if v, has := s.Raw[schema.StorageKeyOptimizedAtBuild]; has && v != nil {
		if _, ok := v.(bool); !ok {
			problems = append(problems, fmt.Sprintf("`%s` is %v, not true or false", schema.StorageKeyOptimizedAtBuild, v))
		}
	}
	return append(problems, usedByCodeProblems(s)...)
}

// usedByCodeProblems checks `usedByCode`: a list of at most
// schema.MaxUsedByCode path patterns, each relative to the storage's
// path and listed once.
func usedByCodeProblems(s schema.Storage) []string {
	v, has := s.Raw[schema.StorageKeyUsedByCode]
	if !has || v == nil {
		return nil
	}
	key := "`" + schema.StorageKeyUsedByCode + "`"
	list, ok := v.([]any)
	if !ok {
		return []string{key + " is not a list of paths"}
	}
	if len(list) > schema.MaxUsedByCode {
		return []string{fmt.Sprintf("%s lists %d paths, at most %d", key, len(list), schema.MaxUsedByCode)}
	}
	var problems []string
	seen := map[string]bool{}
	for _, item := range list {
		p, ok := item.(string)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s holds %v, not a path", key, item))
			continue
		}
		if err := schema.CheckCodePattern(p); err != nil {
			problems = append(problems, key+": "+strings.TrimPrefix(err.Error(), schema.ErrCodePattern.Error()+": "))
			continue
		}
		if seen[p] {
			problems = append(problems, fmt.Sprintf("%s lists %q twice", key, p))
		}
		seen[p] = true
	}
	return problems
}

func quote(s string) string { return "\"" + s + "\"" }
