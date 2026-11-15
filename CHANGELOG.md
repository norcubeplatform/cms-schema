# Changelog

Notable changes, newest first.

## Unreleased

### Changes

- A `{fields.<key>}` value that holds a full URL (`https://example.com/about/`) resolves to its path, so sites that store addresses with the host compare like sites that store paths.

## 0.1.0 (2026-11-15)

### New

- First version of the module: the typed schema model, the directory
  loader with `$ref` resolution, the collection, URL, storage and link
  resolvers, the schema, content and links validators, the
  `cms-schema-validate` command and the fixture sites.
- 53 frozen rule ids under `schema.`, `content.` and `links.` (see
  `report/rules.go`).

### Behaviour worth knowing

- A content file without a `slug` field resolves to its path under the
  locale folder, an `index` file standing for its folder, matching
  Astro's content loader ids.
- Collections that share a `path` are told apart by a literal `match`
  file name; only when that is impossible is
  `schema.duplicate_collection_path` reported.
- Regular expressions are evaluated with RE2; a pattern it cannot
  compile is `schema.invalid_regex` and is skipped in content checks.
