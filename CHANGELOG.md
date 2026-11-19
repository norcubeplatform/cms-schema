# Changelog

Notable changes, newest first.

## 0.2.0 (2026-11-19)

### New

- **Language versions, declared by the site.** A collection says how the pages that are one page in several languages are linked:

  ```yaml
  translations:
    by: field:translationKey   # pages with the same value are versions of each other
    # or:  by: path            # the same path under each locale folder
  ```

  Nothing is inferred: a collection without the block has no language versions. `schema.Collection.Translations` holds it, and `resolve.TranslationKey` gives a file's key (for `path`: its path under the collection without the locale folder and the extension).
- **`translatable` on a field** says whether its text is translated when a page gets a language version. `resolve.Translatable` applies the default when a field does not say: text, paragraph and rich text are translated, everything else is carried over, and neither the `slug` nor the linking field is translated.
- Two rules (55 in all):
  - `schema.invalid_translations`: `translations.by` is not `path` or `field:<key>`, is `path` in a collection whose locale does not come from the path, or names a field that is not a required top-level text field.
  - `links.translation_collision`: two files of one collection and one language share a translation key.

## 0.1.2 (2026-11-15)

### Changes

- `schema.duplicate_collection_path` is reported only when the `match` patterns of collections sharing a path cannot tell them apart (two pattern collections, or two claiming the same literal file name). One pattern collection beside single-document collections in the same directory is the documented layout and passes; the resolver prefers the literal name.

## 0.1.1 (2026-11-15)

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
