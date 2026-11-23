# Changelog

Notable changes, newest first.

## Unreleased

### New

- **`optimizedAtBuild:`** on a storage says the site's build resizes and re-encodes the folder's images, so the original is never served (Astro's `src/assets`). `quality.image_heavy` skips them; `quality.media_unused` still applies. `schema.Storage.OptimizedAtBuild` (nil when the storage does not say), `schema.StorageKeyOptimizedAtBuild`. A value that is not a boolean is `schema.invalid_storage`.
- **A guess when the storage does not say**, from the build framework the repository root marks: on an Astro site (`astro.config.*`) `src/assets/**` is optimized at build and `public/**` served as is. A small table (`quality.DetectFramework`, `quality.Frameworks`, `quality.FrameworkAstro`); the caller passes the root file names (`quality.Options.RootFiles`, `quality.FSRootFiles`) or names the framework (`Options.Framework`). The CLI passes the checkout's root files.
- **`quality.Summary.MediaFolders`** (`mediaFolders`): per media root, its storage, whether its images are optimized at build, whether that was guessed, and how many raster images content uses there.
- **`quality.Summary.Guessed`** (`guessed`): what the run assumed because the schema does not say it, each with the storage key and value that confirm it (`quality.Guess`, kind `quality.GuessOptimizedAtBuild`, `quality.GuessKinds`).
- **`quality: { limits: { imageKB: <n> } }`** in `config.yml`: the image weight limit in kilobytes, 20 to 10000 (`schema.QualityLimits`, `schema.QualityImageKBMin`, `schema.QualityImageKBMax`). The medium band is 5.12 times it, the ratio of the defaults. Out of bounds, not a whole number, an unknown key, or `limits` in a collection is `schema.invalid_quality` and the default applies.

### Behaviour changes

- On an Astro site whose storages do not say `optimizedAtBuild`, images under `src/assets` are no longer weighed, and a heavy image under `public` weighs `low` (the folder's treatment is a guess) until the storage says `optimizedAtBuild: false`.
- `quality.Limits.ImageVeryHeavyBytes` left zero now scales with `ImageHeavyBytes` (5.12 times) instead of staying at 1 MB.
- The hint of `quality.image_heavy` mentions `optimizedAtBuild`.

## 0.6.0 (2026-11-22)

### New

- **`whenEmpty:`** next to `role:` says what the site does when the field is empty: `generated` (the site makes the value per page in code, such as a share card rendered at build time) or `siteDefault` (it falls back to one site-wide value). Allowed on `seoTitle`, `seoDescription` and `shareImage` only; not on `alt`, since code cannot write an image's alt text reliably, and not on `noindex` or `canonical`, whose empty value already means something. Never guessed. `schema.Field.WhenEmpty`, `schema.WhenEmptyGenerated`, `schema.WhenEmptySiteDefault`, `schema.WhenEmptyValues`, `schema.KnownWhenEmpty`, `schema.WhenEmptyRoles`, `schema.RoleAllowsWhenEmpty`; `resolve.RoleField.WhenEmpty` for a declared role.
- One rule (84 in all): `schema.invalid_when_empty`, an error for an unknown value, a `whenEmpty` without a role, or one on a role that does not take it. `validate.Roles` reports it too.
- **`quality.Summary.FilledInCode`** (`filledInCode` in JSON): per role and `whenEmpty` value, how many pages leave the field empty for the site to fill (`quality.FilledInCode`).

### Behaviour changes

- An empty role field that declares `whenEmpty` is no longer reported by `quality.title_missing`, `quality.description_missing` or `quality.share_image_missing`. The page still counts as checked, so it passes in a score. Filled values are checked as before. For an empty SEO title with `whenEmpty`, the page title is not measured in its place.
- `whenEmpty` is a key every field may carry, so it is not reported as `schema.unknown_knob`.
- The hints of `quality.share_image_missing` and `quality.description_missing` mention `whenEmpty`.

## 0.5.0 (2026-11-22)

### New

- **Content quality**, a family of rules apart from correctness: package `quality` and 17 `quality.*` rules (83 in all) in four areas. Search: `title_missing`, `title_length`, `title_duplicate`, `description_missing`, `description_length`, `description_duplicate`, `share_image_missing`, `noindex`. Accessibility: `image_alt_missing`, `image_alt_poor`, `heading_skip`, `link_text_empty`, `link_text_generic`. Languages: `translation_missing`, `translation_outdated`. Media: `image_heavy`, `media_unused`. Thresholds, impacts and sources are a table in the README; a field's `recommendedLength` wins over the defaults.
- Quality runs only when asked for: `validate.ScopeQuality` in `Options.Only`, or `cms-schema-validate --quality`. `validate.Scopes()` lists the four scopes.
- **Quality findings are never errors**: warnings (high and medium impact) or notices (low impact and information). `report.SeverityNotice`, `report.Impact` (`high`, `medium`, `low`, `info`), `report.SeverityOf`, `report.CountNotices`, `Rule.IsQuality`. `Options.StrictQuality` (`--strict-quality`) turns quality warnings into errors; `--strict` leaves them alone.
- `report.Finding` gains `impact`, `value` (what the finding is about; when it changes, the finding is a new one) and `measure` (`report.Measure`: the number measured, its unit and window). All three are empty on other findings.
- **`quality.Options`** carries what only the caller knows: `LastChanged` (when each file last changed, for `translation_outdated`), `ListMedia` (the files under `quality.MediaRoots(cfg)` with sizes, for the media rules; `quality.FSMedia` lists them from a checkout) and `Limits`. A rule without its data is skipped and named in the summary.
- `validate.Result.Quality` (`quality.Summary`): per rule how many instances were checked, the rules turned off, and the rules skipped with the reason. `Result.Notices` counts notices.
- **`quality: { ignore: [rule ids] }`** in `config.yml` (whole site) or in a collection turns quality rules off (`schema.Config.Quality`, `schema.Collection.Quality`). One rule: `schema.invalid_quality`, a warning for a block that is not a mapping, an `ignore` that is not a list, or an entry that is not a quality rule.
- `resolve.References(cfg, fields, frontmatter, body)` lists what a page points at (links, assets, references), the reading the links checks and the content index share. `schema.HiddenPaths` and `schema.PathHidden` say which fields a document hides.
- `quality.GenericLinkText` and `quality.PoorAlt` with word lists for English, Czech and German.

### Behaviour changes

- `report.Sort` orders notices after warnings.
- The links checks read references through `resolve.References`; their findings are unchanged.

## 0.4.0 (2026-11-22)

### New

- **Field roles.** `role:` on a field says what it means to the site: `seoTitle`, `seoDescription`, `shareImage`, `noindex` (a boolean, true hides the page from search engines), `canonical`, and `alt` (the alt text of the one image in its fields list). Page roles appear at most once per collection, outside lists. `schema.Field.Role`, `schema.PageRoles`, `schema.Roles`, `schema.KnownRole`, `schema.IsPageRole`, `schema.RoleTypes`, `schema.RoleAccepts` and the `schema.Role*` constants.
- **Image alt text.** On an image field, `alt: <key>` names the sibling that holds its alt text and `decorative: true` says it needs none (`schema.Field.Alt`, `schema.Field.Decorative`).
- **`resolve.CollectionRoles(cfg, coll)`**: the field of each page role and every image with the field holding its alt text, each marked declared or guessed. Undeclared roles are guessed from keys and labels by a small table (README, "Field roles"); two candidates guess nothing. **`resolve.AltOf(siblings, image)`** is the decision for one image. `resolve.MaxRoleImages` bounds the list.
- **`translations.locales`**: the languages a collection's pages are expected in, each a site language; every site language by default. `schema.Translations.Locales`, `resolve.ExpectedLocales(cfg, coll)`.
- **`schema.Collection.SiteArea()`**: whether a collection holds site settings rather than pages, for counts of pages.
- **`validate.Roles(cfg)`** checks roles and alt relations of a config that was never written to a site (a proposed schema).
- Six rules (65 in all), all errors: `schema.unknown_role`, `schema.duplicate_role`, `schema.role_type_mismatch`, `schema.misplaced_role`, `schema.invalid_alt`, `schema.invalid_translation_locales`.

### Behaviour changes

- `role` is a key every field may carry, and `alt` and `decorative` are image knobs, so none of them is reported as `schema.unknown_knob` there any more. `alt` or `decorative` on another field type still is, with a hint.

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
