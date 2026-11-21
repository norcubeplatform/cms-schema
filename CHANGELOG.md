# Changelog

Notable changes, newest first.

## 0.3.1 (2026-11-21)

### Fixed

- **A condition comparing two lists or two maps no longer panics.** `visibleIf: {field: gallery, op: eq, value: []}` (or `equals: []`, or `{}`) on a page whose `gallery` is a list made `schema.Eval`, and with it `validate.Page` and `validate.Site`, panic with "comparing uncomparable type []interface {}", ending a caller that ran the validator in a goroutine of its own. `eq`, `neq`, `in` and `notIn` now compare lists element by element in order and maps by key set and values, with the same loose rules as scalars (numbers by value across int and float kinds), bounded by `schema.MaxCompareDepth` (32). `null` and absent still equal only each other, so `value: []` matches an empty list, not a missing field (use `empty` for that). `in` on a list field asks whether the whole list is one of the entries. Scalars compare exactly as before. The semantics are a table in the README.
- No other value can make the evaluator panic: any type Go cannot compare with `==` is compared structurally.

### New

- **`validate.Site`, `Schema`, `Page` and `Links` never panic.** A panic in a check (a bug in this module) is recovered into one error finding quoting the cause; in `Site` a panic in one content file skips that file and the run goes on. Three rules (59 in all): `schema.internal_error`, `content.internal_error`, `links.internal_error`.
- `schema.MaxCompareDepth`.

## 0.3.0 (2026-11-19)

### New

- **`resolve.TranslatableTexts(cfg, coll, frontmatter, body)`** lists the texts of a page that are translated when it gets a language version: path (`blocks[0].heading`), label, field type, format (`plain`, `markdown`, `html`) and value, in schema order, the body last. More than `resolve.MaxTranslatableTexts` (2000) texts is `resolve.ErrTooManyTexts`.
- **`resolve.TranslatableIn(coll, ancestors, field)`** decides for a field anywhere in the schema, given the groups and lists around it; `resolve.Translatable` is the same question for a top-level field. `resolve.TranslatableType` says whether a field type is text at all. The rules are a table in the README.
- **`schema.Walk`** visits every field of a document with its definition, path, depth, enclosing fields and condition scope: groups, list items of every shape, and each item of a list of primitives. Bounded by `schema.MaxWalkDepth` (32). `schema.JoinPath` and `schema.IndexPath` build the paths. The content and links validators now run on it instead of walkers of their own; their findings are unchanged.
- One rule (56 in all): `schema.translatable_ignored`, a warning for `translatable: true` on a field whose type is never translated (a number, an image, a select…).
- **`resolve.HasWords(value)`**: whether a value holds anything to translate. A value with no letter (a phone number, a price, a date) or one that is only an email or web address is left out of `TranslatableTexts`, whatever its field.
- `resolve.BodyPath`, `resolve.FormatPlain`, `resolve.FormatMarkdown`, `resolve.FormatHTML`, `schema.RichTextFormatMarkdown`, `schema.RichTextFormatHTML`.

### Behaviour changes

`resolve.Translatable` answers differently for these fields:

- **A `text` field with a `regex` is no longer translated by default.** A pattern marks an id, an icon name or a handle. Say `translatable: true` on the field where the text is for people.
- **`translatable` on a group or a list covers everything inside it**, down to the next field that sets its own. Before, the key was read on the field itself only.
- **`computed` and `hidden` fields are not translated** unless they say `translatable: true`; fields inside a hidden group or list neither.
- **`translatable: true` no longer makes a field of another type translatable** (a select, a link, a number). Only text, paragraph and rich text are; the schema validator reports the key there.
- The linking field is recognised at the top level only; a nested field that shares its key is ordinary text.

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
