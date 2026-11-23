# cms-schema

The schema of a [Norcube CMS](https://docs.norcube.com/cms) site as a Go
module: the typed model of `.norcube/cms/`, the loader that assembles the
directory and resolves `$ref`, the resolvers that map a content file to
its collection, URL and assets, and the validators that check a site the
way the editor does, offline.

The Norcube CLI wraps it as `nrc cms validate`; the CMS service uses the
same code to load schemas and build its content index. A site's CI can
run it to block a pull request that breaks the content model.

## Install

```sh
go install github.com/norcubeplatform/cms-schema/cmd/cms-schema-validate@latest
```

Requires Go 1.26 or later. The module depends on `gopkg.in/yaml.v3` and
nothing else.

## Use

```sh
cms-schema-validate                       # the site in the current directory
cms-schema-validate path/to/site --strict # warnings fail the run
cms-schema-validate --only schema         # schema files alone, no content
cms-schema-validate --json > report.json  # for CI
cms-schema-validate --quality             # also content quality (see below)
cms-schema-validate --quality --git       # with dates from git, so outdated translations are found
```

`--git` reads when each file last changed from the git history of the
path (`git log`, at most 5000 commits, 30 seconds; a file changed and
not committed counts as changed now) and passes the dates to the
quality checks. Package `gitlog` does it; the checking packages never
run a program.

Exit codes: `0` clean, `1` findings at error severity, `2` usage or I/O
error, `3` not a CMS site.

Each finding names the file, the field path inside it, a severity, a
rule id and a message:

```
error    src/content/pages/about.md#seats        content.required          Seats is required.
warning  src/content/pages/index.md#blocks[0].link  links.unresolved_link  links to /about/, which no content file produces
error    .norcube/cms/collections/pages.yml#fields.tier  schema.default_not_option  default "enterprise" of "Tier" is not one of its options
```

## What it checks

**schema** — the directory loads and makes sense: YAML parses, every
`$ref` names a block or an input, every field has `key`, `type` and
`label`, types and knobs are ones the editor reads, select defaults are
options, bounds are ordered, regexes compile, `visibleIf` and `computed`
paths name fields, storages and `defaultStorage` agree, the preview,
redirects and variables blocks are consistent, no two collections claim
one directory without literal `match` names to tell them apart, and a
collection's `translations.by` is `path` (with `localeFrom: path`) or
names a required top-level text field, `translations.locales` names
only site languages, `translatable` sits only on fields it can mean
something for, localized groups hold exactly the site's languages
([Languages in one file](#languages-in-one-file)), and field roles,
`whenEmpty:`, `titleTemplate:`, `excerpt:`, `noindexWhen:`,
`draftWhen:`, `alt:` and `decorative:` are used as described under
[Field roles](#field-roles).

**content** — every content file against its collection, with the
editor's rules: required, value shape (including select options), item
discriminators of multi-shape arrays, array bounds, regex, length, number
bounds, uniqueness, custom validators, out-of-schema keys per
`extraKeysPolicy`, a body where the collection has none. Fields hidden
by `hidden` or by a false `visibleIf` are skipped, as in the editor.

**links** — the site as a whole: two files resolving to one URL, site
links that no file produces and no `knownPaths` entry covers, two files
of one collection and one language sharing a translation key, images
and files that do not exist, reference fields whose target is gone,
orphaned identity sidecars.

**quality** (only when asked for: `--quality`, or `quality` in `Only`)
is a separate family: the site works, but a page could serve its
readers better. See [Content quality](#content-quality).

The rule ids are listed in [`report/rules.go`](report/rules.go) and are
frozen: a released id is never renamed or reused.

A bug in the module itself does not crash the caller: `validate.Site`,
`Schema`, `Page` and `Links` recover a panic into one error finding,
`schema.internal_error`, `content.internal_error` or
`links.internal_error` by the check it stopped, quoting the cause. The
other findings of that check are missing; in `Site` a panic in one
content file skips that file and the run goes on. Seeing one of these
is worth a bug report.

## As a library

```go
res, err := validate.Site(os.DirFS("."), validate.Options{Strict: true})
for _, f := range res.Findings {
    fmt.Println(f)
}
```

| Package    | What it holds                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------------- |
| `report`   | `Finding`, severities, impacts and measures, the frozen rule ids.                                 |
| `schema`   | The typed model (`Config`, `Collection`, `Field`, `Storage`…), `Parse`, array item shapes, conditions, `Walk`. |
| `load`     | `Load(fs.FS)`: directory or legacy single file, merge, `$ref`, schema version.                     |
| `content`  | Markdown-with-frontmatter and YAML data files to plain values; identity sidecars.                 |
| `resolve`  | Collection for a path, locale, URL and slug fallback, storage values, links in text, translatable texts, field roles and expected languages. |
| `quality`  | The content quality rules, their specs and `Check`.                                               |
| `validate` | `Schema`, `Page`, `Links`, `Quality` and the `Site` runner.                                       |
| `fact`     | Writing schema facts (a role, a localized group, an image's alt text, expected languages, `optimizedAtBuild`, `usedByCode`) into the schema files, keeping their formatting. See [Writing facts](#writing-facts). |
| `gitlog`   | When each file last changed, from a checkout's git history (`quality.Options.LastChanged`). The only package that runs a program. |

`schema.Walk` is the one reading of a document against its schema: it
visits every field with its definition and its path (`a.b[2].c`),
entering groups, list items of every shape and lists of primitives, at
most `schema.MaxWalkDepth` levels deep. The validators and
`resolve.TranslatableTexts` are built on it.

## Conditions

`visibleIf` (and the older `equals:` spelling, which means `op: eq`)
compares the value at `field` with `value`. `field` is a path: `key`,
`a.b`, `items.0.kind`, `$parent.key`, `$root.a.b`, `$item.key`; a path
that reaches nothing reads as `null`. `all`, `any` and `not` combine
conditions. `schema.Eval` evaluates one; it never panics, whatever the
two values are.

| `op`               | True when                                                                                       |
| ------------------ | ----------------------------------------------------------------------------------------------- |
| `eq`               | the field's value equals `value` (equality below).                                              |
| `neq`              | not `eq`.                                                                                       |
| `in`               | the field's value equals one entry of the list `value`. A string `value` is read as a JSON list (`'["a","b"]'`) or a comma-separated one (`a, b`). Any other `value` (a map, a number) is false. |
| `notIn`            | `value` is a list (as for `in`) and the field's value equals none of its entries.               |
| `truthy`, `falsy`  | JavaScript truthiness: `null`, `false`, `0`, `""` are falsy; any list or map is truthy.          |
| `empty`, `nonEmpty`| empty is `null`, absent, `""` or `[]`; whitespace and `{}` are not empty.                         |
| `gt`, `lt`         | both sides are numbers and compare so.                                                          |
| `matches`          | both sides are strings and the value matches the regular expression (an invalid one is false).  |

Equality, for `eq`, `neq`, `in` and `notIn`:

- `null` and absent are equal, and equal nothing else: not `""`, not
  `[]`, not `{}`. Test for "no value" with `empty`.
- Numbers compare by value (`1` equals `1.0`); a number never equals a
  string (`1` is not `"1"`).
- Strings and booleans compare exactly.
- A list compares as a whole: the same length and equal entries in the
  same order, by these same rules (`value: []` matches an empty list
  only). A map compares by its key set and each key's value.
- `in` on a list field asks whether the whole list is one of the
  entries (`value: [[a.jpg], [b.jpg]]`), not whether the lists share an
  item.
- Values nested deeper than `schema.MaxCompareDepth` (32) levels compare
  unequal.

## Field roles

A role says what a field means to the site, so a check, the editor or
Norcube AI can find the page's search title or an image's alt text
without guessing from its name. Roles are declared once, on the field,
in a collection, a block or an input:

```yaml
# blocks/seo.yml
fields:
  - key: page_title
    type: text
    label: Meta title
    role: seoTitle
  - key: page_description
    type: paragraph
    label: Meta description
    role: seoDescription
  - key: featured_image
    type: image
    label: Share image
    role: shareImage
    alt: featured_image_alt # the sibling that holds its alt text
  - key: featured_image_alt
    type: text
    label: Share image alt text
  - key: canonical_url
    type: link
    label: Canonical URL
    role: canonical
  - key: no_index
    type: boolean
    label: Hide from search engines
    role: noindex
  - key: draft
    type: boolean
    label: Draft
    role: draft
```

| Role             | Means                                              | Field types       | Where                                                  |
| ---------------- | -------------------------------------------------- | ----------------- | ------------------------------------------------------ |
| `seoTitle`       | The page's title in search results and the tab     | text, paragraph   | Once per collection, outside lists                     |
| `seoDescription` | The page's meta description                        | text, paragraph, richtext with `excerpt` | Once per collection, outside lists              |
| `shareImage`     | The image shown when the page is shared            | image             | Once per collection, outside lists                     |
| `noindex`        | True hides the page from search engines            | boolean, select with `noindexWhen` | Once per collection, outside lists    |
| `canonical`      | The address that should rank instead of this page  | link, text        | Once per collection, outside lists                     |
| `draft`          | True means the page is not published yet           | boolean, select with `draftWhen` | Once per collection, outside lists      |
| `alt`            | The alt text of the image next to it               | text, paragraph   | Once per fields list that holds exactly one image      |
| `none`           | No role, and never guessed to have one             | any               | Anywhere                                               |

A localized group (see [Languages in one file](#languages-in-one-file))
holds the role of its language fields' type, per language: `role:
seoTitle` on a localized group of text fields is the title of each
language version.

`role: none` records that a field holds no role: a field guessed from
its name (a `meta.title` that is not the page's search title) says it,
and the guess stops. It goes on any field, any number of times, and
takes no `whenEmpty`. `schema.RoleNone`; `schema.RoleValues()` lists the
seven roles and `none`.

**A select for noindex or draft.** A robots select or a workflow state
says which of its values switch the role on; every entry must be one
of its options, and there is no default (a select with the role and no
list is an error):

```yaml
- key: robots
  type: select
  label: Search engine indexing
  role: noindex
  noindexWhen: ['noindex,follow', 'noindex,nofollow'] # quoted: a comma splits a flow list
  options: ['index,follow', 'noindex,follow', 'noindex,nofollow']
- key: state
  type: select
  label: State
  role: draft
  draftWhen: [writing, review]
  options: [writing, review, published]
```

A draft page is skipped by the search and accessibility checks and is
not a language version (see [Content quality](#content-quality)). A
hidden page is listed by `quality.noindex` and skipped by the other
search checks.

**A title the site composes.** When the site's code builds the page
title from the field (`<title>{title} – Adéla Maršálová</title>`),
`titleTemplate:` next to `role: seoTitle` says how, and the checks
measure the composed title:

```yaml
- key: name
  type: group
  label: Name
  localized: true
  role: seoTitle
  titleTemplate: '{field:kind|label} – {value} – {{site_name}}'
  fields: [...]
```

| Placeholder | Is |
| ----------- | -- |
| `{value}` | The field's own text, in the page's language. Required. |
| `{field:<key>}` | Another field of the page outside lists (`a.b` into groups); a localized group in the page's language, a list of choices joined by commas. |
| `{field:<key>\|label}` | A select's option label in the page's language: the option's `labels.<locale>`, else its `label`, else its value. |
| `{{key}}` | A site variable from `variables.file`, in the page's language (as the editor resolves it). |

Any other brace is `schema.invalid_title_template`, as is a template
without `{value}`, longer than 300 bytes, on a field without `role:
seoTitle`, or naming a field the page does not have (judged in the
collection, so a block's template names the fields of each page that
uses it). A variable the site does not define is not an error (the
variables are content); a title using one is not measured. Titles
only: a site rarely composes its description, and the one common case,
the first paragraph of a text, is `excerpt`.

Option labels per language go in `labels`, next to the `label` the
editor shows:

```yaml
options:
  - value: solo
    label: Samostatná výstava
    labels: { cs: Samostatná výstava, en: Solo exhibition }
```

**A description from rich text.** `role: seoDescription` on a rich text
field says which part is the description with `excerpt:
firstParagraph`: the plain text of its first paragraph (Markdown split
at blank lines, headings and fenced code skipped, emphasis and link
marks removed, links and images keeping their text; HTML's first
`<p>`). `resolve.FirstParagraph` is that reading. Rich text without
`excerpt`, an unknown value, or `excerpt` elsewhere is
`schema.invalid_excerpt`.

"Once per collection" counts the collection's fields with every block
pulled in by `$ref`, through groups at any depth. A page role inside a
list (an array's items, a block used as a list item) would repeat per
item and is an error.

An image field says where its alt text is in one of three ways:

- `alt: <key>` on the image names a text or paragraph field in the same
  fields list (the same group or list item). Not a path: a field in
  another group cannot be named.
- `role: alt` on a text or paragraph field marks it as the alt text of
  the one image in its fields list, the "image object" shape:

  ```yaml
  - key: logo
    type: group
    label: Logo
    fields:
      - key: img
        type: image
        label: Logo
      - key: name
        type: text
        label: Client name
        role: alt
  ```

- `alt: { from: [title, technique], separator: " – " }` composes it
  from siblings, as the site's code does (`alt={`${title} – ${technique}`}`):
  the parts that are not empty, joined by `separator` (a space when it
  does not say), empty when every part is. 1 to 10 parts, a separator
  of at most 20 bytes.
- `decorative: true` on the image says it needs no alt text (a
  background, an ornament). It cannot be combined with the others.

A field named by `alt:` (or composed from) is text, paragraph, or a
localized group of them, whose alt text is then per language.

`alt` and `decorative` are image knobs; on another field type they are
reported as `schema.unknown_knob`. The rules:

| Rule                         | Reported when                                                                                   |
| ---------------------------- | ----------------------------------------------------------------------------------------------- |
| `schema.unknown_role`        | `role` is not one of the seven or `none` (or not a string).                                     |
| `schema.role_type_mismatch`  | The field's type cannot hold the role (`shareImage` on text, `noindex` on a multichoice), or a group that is not localized has a page role. |
| `schema.duplicate_role`      | A second field of the collection or block has the same page role, or a second `role: alt` sits in one fields list. |
| `schema.misplaced_role`      | A page role inside a list, or `role: alt` in a fields list without an image.                    |
| `schema.invalid_alt`         | `alt` names no text or paragraph sibling (or the image itself), `decorative` is not a boolean or comes with alt text, or `role: alt` sits beside several images, a decorative one, or one that names another field. |
| `schema.invalid_when_empty`  | `whenEmpty` is not `generated` or `siteDefault`, sits on a field without a role, or on a role other than `seoTitle`, `seoDescription` and `shareImage` (`none` included). |
| `schema.invalid_title_template` | A `titleTemplate` that does not parse, has no `{value}`, sits on a field without `role: seoTitle`, or names a field the page lacks. |
| `schema.invalid_excerpt`     | Rich text with role `seoDescription` and no `excerpt`, an unknown value, or `excerpt` on another field. |
| `schema.invalid_role_values` | A select with role `noindex` or `draft` lacks `noindexWhen` or `draftWhen`, the list is empty, longer than 100, repeats or names a value that is not an option, or the key sits where nothing reads it. |
| `schema.invalid_localized`   | See [Languages in one file](#languages-in-one-file).                                            |

All of them are errors. A block is checked in its own file and again where
it is used, so a block whose role is fine alone but lands inside a list
is reported at the collection.

### Values the site fills in code

Some sites fill an empty role field themselves: a share card rendered
per page at build time, a title composed from the page heading, one
site-wide description. `whenEmpty:` next to `role:` says so, and the
quality checks stop reporting those empty fields as missing:

```yaml
# The site renders a 1200 x 630 card per page (src/pages/og/[...path].png.ts)
# when the editor picks no image.
- key: og_image
  type: image
  label: Share image
  role: shareImage
  whenEmpty: generated

# The layout falls back to one site-wide description.
- key: description
  type: paragraph
  label: Meta description
  role: seoDescription
  whenEmpty: siteDefault
```

| Value         | Means                                                                 |
| ------------- | --------------------------------------------------------------------- |
| `generated`   | The site makes the value per page in code.                            |
| `siteDefault` | The site falls back to one value for the whole site.                  |

`whenEmpty` goes on a field with role `seoTitle`, `seoDescription` or
`shareImage`, and on no other. Not on `alt`: alt text describes one
particular image, and code cannot write it reliably (a copied title or
file name is worse for a screen reader than an honest gap); an image
that needs none says `decorative: true`. Not on `noindex` or
`canonical`: their empty value already means something (indexable,
canonical to itself), and no check reports it.

It is the owner's statement about the site's code, so it is never
guessed: a role guessed from a field name never has one, and
`resolve.RoleField.WhenEmpty` is set only for a declared role with a
valid value. What it changes in the checks is under [Content
quality](#content-quality).

### Languages in one file

Some sites keep every language of a page in one file: a work's title is
`{cs: …, en: …, it: …}` and the site builds `/portfolio/…`,
`/en/portfolio/…` and `/it/portfolio/…` from it. `localized: true` on
the group says its fields are the site's languages; `fallback:` names
the language an empty one shows on the site (the site's code does it,
`en || cs`):

```yaml
# collections/works.yml
localeFrom: none
fields:
  - key: title
    type: group
    label: Title
    localized: true
    fallback: cs
    role: seoTitle
    fields:
      - { key: cs, type: text, label: Czech }
      - { key: en, type: text, label: English }
      - { key: it, type: text, label: Italian }
```

A localized group has one field per language of `preview.locales`,
keyed by the language, and nothing else; all of one type, text,
paragraph or rich text; none with a role of its own (the role goes on
the group and means each language). It sits in a collection whose files
hold every language: not where the locale comes from the path or a
field, nor beside `translations.by`. Each file of such a collection is
a page per language, at `previewPath` with `{locale}` and a localized
`slug` read per language (`resolve.URLIn`). Any of that broken, a
`localized` that is not a boolean, a `fallback` without `localized` or
naming a language the site lacks, or an option's `labels` naming one,
is `schema.invalid_localized`.

`translations: { locales: [cs, en] }` without `by` says which languages
the pages of such a collection are expected in (by default every site
language); without localized groups, `by` stays required.

`schema.Field.IsLocalized`, `ValueType` (the language fields' type),
`LanguageField(locale)`, `schema.HasLocalized`,
`Collection.Localized()` and `Collection.OneLanguagePerFile()` read it;
`resolve.LocalizedValue(v, locale, fallback)` reads one language, saying
whether the text is the fallback's.

### Reading roles, and guessing them

`resolve.CollectionRoles(cfg, coll)` answers, for one collection, the
field of each page role (`Path` in dots, `seo.page_title`, as preview
slots name it) and every image field with the field holding its alt text
(`content_blocks[logos].list[].img` → `content_blocks[logos].list[].name`;
`[]` is an item of a list with one shape, `[<shape>]` of a list with
several). Each answer says whether it was `Declared` or guessed. At most
`resolve.MaxRoleImages` (500) images are listed.

A role answer also carries what a reader needs to show it right: the
value `Type`, `Localized` and `Fallback` (the role holds per language,
at `<Path>.<locale>`), `TitleTemplate` (the checks measure the composed
title), `Excerpt` (they measure the first paragraph) and `When` (the
select values that switch noindex or draft on). An image answer
carries `AltFrom` and `Separator` for a composed alt text and
`AltLocalized` when it holds per language.

A declared role wins. A role nobody declares is guessed, so sites that
predate roles still get answers. Keys and labels are compared in
lowercase with everything but letters and digits removed (`meta_title`,
`metaTitle` and `Meta title` are one name). A candidate must sit outside
lists, be of a type the role accepts and declare no role of its own; two
candidates for one role guess nothing.

| Role             | Key anywhere                                                                                   | Label                                         | Key directly in an seo group |
| ---------------- | ---------------------------------------------------------------------------------------------- | --------------------------------------------- | ---------------------------- |
| `seoTitle`       | `seo_title`, `meta_title`, `page_title`                                                         | SEO title, Meta title                         | `title`                      |
| `seoDescription` | `seo_description`, `meta_description`, `page_description`                                       | SEO description, Meta description             | `description`                |
| `shareImage`     | `share_image`, `social_image`, `og_image`, `open_graph_image`, `featured_image`, `seo_image`, `meta_image` | Share image, Social image, OG image, Open Graph image | `image`                      |
| `noindex`        | `noindex`, `no_index`, `hide_from_search`, `hide_from_search_engines`; a select `robots`, `meta_robots` | No index, Hide from search engines, Robots, Meta robots |                    |
| `canonical`      | `canonical`, `canonical_url`, `canonical_link`                                                  | Canonical, Canonical URL, Canonical link      |                              |
| `draft`          | `draft`, `is_draft` (a boolean)                                                                 | Draft                                         |                              |

A robots select is guessed only when some of its options carry the
directive `noindex` (or `none`); those options are the guessed `When`.

An seo group is a group whose key, or the block it was pulled in from by
`$ref`, is `seo`, `meta` or `metadata`. A top-level `title`,
`description` or `image` is the page's own, never guessed as a search
field.

`resolve.AltOf(siblings, image)` is the same decision for one image,
returning a `resolve.AltSource` (the keys, the separator, whether it is
decorative, declared, composed or localized): declared from
`decorative: true`, `alt: {from: [...]}`, `alt: <key>`, or a `role:
alt` sibling when the image is alone in its list; otherwise guessed from a text or
paragraph sibling without a role, `<key>_alt` or `<key>Alt` (also
`<key>_alt_text`), and, when the image is the only one in its list,
`alt`, `alt_text` or `alternative_text`, failing those `name`. Two
siblings fitting one step guess nothing.

## Expected languages

A collection with language versions may say which languages its pages
are expected in; by default every language in `preview.locales`:

```yaml
translations:
  by: field:translationKey
  locales: [cs] # case studies are written in Czech only
```

Each entry must be a site language; anything else is
`schema.invalid_translation_locales` (also for an empty list, a repeat
or a value that is not a list of codes). `resolve.ExpectedLocales(cfg,
coll)` gives the list, leaving out languages the site does not declare.
Counts of missing language versions should ask only for these. In a
collection whose files hold every language (localized groups), the
same list says which languages each file is a page in.

A collection with `area: site` holds site settings (menus, footers,
globals), not pages; `Collection.SiteArea()` says so, and page counts
leave its documents out.

## Translatable texts

When a page gets a version in another language, some of its values are
text a person reads and the rest (ids, addresses, numbers, images,
links, choices) is carried over as it is. `resolve.TranslatableIn`
decides for one field, `resolve.TranslatableTexts` lists the texts of
one page:

```go
texts, err := resolve.TranslatableTexts(cfg, coll, doc.Frontmatter, doc.Body)
// texts[i]: Path "blocks[0].heading", Label "Heading", Type "text",
//           Format "plain" | "markdown" | "html", Value "Welcome"
```

The rules, the first that applies deciding:

| #   | A field…                                                                  | Translated?                                  |
| --- | ------------------------------------------------------------------------- | -------------------------------------------- |
| 1   | is not `text`, `paragraph` or `richtext` (or a legacy name of one)        | Never, whatever it says.                     |
| 2   | says `translatable: true` or `false`                                      | As it says.                                  |
| 3   | is `computed` or `hidden`                                                 | No.                                          |
| 4   | sits in a group or a list that says `translatable: false`, or is `hidden` | No. The nearest one that says anything wins. |
| 5   | is the collection's linking field (`translations.by: field:<key>`)        | No. Top level only.                          |
| 6   | has the key `slug`                                                        | No, at any depth.                            |
| 7   | is `text` with a `regex`                                                  | No: an id, an icon name, a handle.           |
| 8   | anything else                                                             | Yes.                                         |

`translatable` on a group or a list covers everything inside it down to
the next field that sets its own. `translatable: true` on a group turns
translation back on inside a group that turned it off; it does not
override rules 3 and 5 to 7, which only the field itself can
(`translatable: true` on the field). On a field of any other type the
key changes nothing, and saying `true` there is reported as
`schema.translatable_ignored`.

A value with nothing to translate is left out whatever its field: no
letter in it (a phone number, a price, a date), or only an email or web
address (`resolve.HasWords`).

`TranslatableTexts` returns non-blank string values only, in schema
order with list items by index, and the Markdown body last (path
`body`, `Body: true`) when the collection has one. A page with more than
`resolve.MaxTranslatableTexts` (2000) texts returns
`resolve.ErrTooManyTexts`.

## Content quality

`quality.*` rules check what a site owner can improve: titles and
descriptions for search results, alt text and link text for people who
use a screen reader, language versions, heavy and unused media. They run
only when asked for (`cms-schema-validate --quality`,
`validate.Options{Only: []string{..., validate.ScopeQuality}}`), so an
existing CI run sees no change.

A quality finding is never an error. High and medium impact findings are
warnings, low impact and information are notices, and `--strict` leaves
them alone; `--strict-quality` (`Options.StrictQuality`) turns quality
warnings into errors for a site that wants to gate on them. Each finding
carries, besides file, path, rule, message and hint:

- `impact`: `high`, `medium`, `low` or `info` (listed, not a problem);
- `value`: what the finding is about as the site holds it (the title,
  the alt text, the link text, the missing language, the file). When it
  changes, the finding is a different one, so a dismissal tied to it
  lapses;
- `measure`: the number measured, with its unit and window, when there
  is one (`{"value": 72, "unit": "characters", "min": 30, "max": 60}`).

| Rule | Area | Impact | Threshold | Source |
| ---- | ---- | ------ | --------- | ------ |
| `quality.title_missing` | search | high | No SEO title and no page title to fall back to | Lighthouse `document-title` |
| `quality.title_length` | search | low | Outside 30 to 60 characters, or the field's `recommendedLength` | Google sets no limit and cuts titles to the device width [1]; 30 to 60 is the display convention tools use |
| `quality.title_duplicate` | search | medium | The same title on two or more indexable pages of one language, after trimming and ignoring case | Google, title links [1] |
| `quality.description_missing` | search | medium | An empty meta description | Lighthouse `meta-description` |
| `quality.description_length` | search | low | Outside 70 to 160 characters, or the field's `recommendedLength` | Google sets no limit and cuts snippets to fit [2]; a display convention |
| `quality.description_duplicate` | search | medium | The same meta description on two or more indexable pages of one language | Google, snippets [2] |
| `quality.share_image_missing` | search | low | No share image, on a collection with a share image field | Meta, sharing images [3] |
| `quality.noindex` | search | info | The page's noindex field is true, or holds a value its `noindexWhen` lists (listed, not scored) |  |
| `quality.image_alt_missing` | accessibility | high | An image that is not decorative with empty alt text | WCAG 2.2 1.1.1, level A [4]; Lighthouse `image-alt` |
| `quality.image_alt_poor` | accessibility | low | Alt text that is the file name, a camera's file name or a generic word (image, photo, obrázek, Bild…) | Google, image SEO [5] |
| `quality.heading_skip` | accessibility | low | A heading more than one level below the heading before it, in one text | axe `heading-order`, a best practice, not a WCAG requirement [6] |
| `quality.link_text_empty` | accessibility | medium | A link in a text with no text, no `aria-label` and no image alt text inside | WCAG 2.2 2.4.4, level A [7] |
| `quality.link_text_generic` | accessibility | low | Link text that is only a generic phrase (click here, read more, zde, více, hier, mehr…), in a text or a button label beside a link field | WCAG 2.2 2.4.4 [7]; Lighthouse `link-text` |
| `quality.translation_missing` | languages | medium | A page with no version in a language its collection expects (`translations.locales`, else every site language); a version that is a draft is listed as not published yet (info) |  |
| `quality.translation_outdated` | languages | medium | A version last changed more than 7 days before the newest version of the same page | A convention |
| `quality.translation_incomplete` | languages | low | A page that holds every language in one file leaves a language's texts empty, or the same as the language they fall back to |  |
| `quality.image_heavy` | media | medium | A raster image content uses over 200 KB (low impact) or over 1 MB (medium), or over the site's `quality.limits.imageKB`; images in a folder the build optimizes are skipped | Lighthouse flags any image that could be smaller [8]; crawlers flag 100 KB, a number Sitebulb calls relatively arbitrary [9], so 200 KB flags only clear cases |
| `quality.media_unused` | media | info | A file in a read-write repository storage that no content and no schema refers to, and that its storage's `usedByCode` does not name (listed, not scored) |  |

**Where the checks look.** Search rules read the fields that hold each
role (see [Field roles](#field-roles)), on pages of their own (a
collection with `previewPath`, not `area: site`) that are not hidden
from search engines; a page whose canonical address points elsewhere is
left out of the duplicate rules. The title is the SEO title, else the
first preview title slot with a value, else a top-level `title`. A
finding that stands on a role guessed from field names weighs `low`. A
value with a `{{placeholder}}` is not measured. Accessibility rules read
every image value (its alt text through `resolve.AltOf`), every rich
text field and the Markdown body (headings, links, images; fenced code
is skipped; HTML `alt=""` is a decorative image on purpose), and the
label beside a link field. Fields a page hides (`hidden`, `visibleIf`)
are skipped. Word lists for generic text exist for English, Czech and
German, and every list is checked on every page.

**Drafts.** A page whose draft field is on (`role: draft`, declared or
guessed) is skipped by the search and accessibility rules and counted
in `res.Quality.Drafts`. It is not a language version: a language whose
only version is a draft gets an info finding of
`quality.translation_missing` ("not published yet") on the draft, a
language with no version at all is reported only when the page has a
published version, and `quality.translation_outdated` compares
published versions only.

**Pages that hold every language.** A file of a collection with
localized groups is read once per expected language, as that
language's page: localized role fields, alt texts, link labels and rich
texts in that language, its URL from `previewPath`. A finding about a
language field has its path (`title.en`) and `locale`. A language left
empty that falls back shows the fallback's text: it is not missing and
is not measured again (the fallback's own page measured it). What is
the same in every language (a plain alt text, a rich text that is not
localized, the body) is checked once, on the default language's page.
`quality.translation_incomplete` then says, per page and language, which
localized texts are untranslated: empty while another language has
text, or, where the group falls back, the same as the fallback (after
trimming and joining white space). A group empty in every language, a
`slug`, a computed field and a group that says `translatable: false`
are left out. The finding names the texts and counts them (`measure`,
unit `fields`), impact low: the page is there, partly in another
language, which is less than no page at all (`translation_missing`). A
file is one version, so `translation_outdated` has nothing to compare
within it.

**Values the site composes.** A title with `titleTemplate` is measured
as the template composes it (length against 30 to 60, the field's
`recommendedLength` being the editor's guide to the field alone;
duplicates on the composed text), a rich text description with
`excerpt` as its first paragraph (against 70 to 160), an alt text with
`alt: {from: [...]}` as the parts joined. Such a finding's `value` is
what the site shows, and `basis` says how it was made
(`titleTemplate`, `excerpt`, `composedAlt`; `report.Bases()`), so a tool
does not write it into the field as is. `title_missing` and
`description_missing` still read the field itself.

**Fields the site fills in code.** When a role field declares
`whenEmpty` (see [Values the site fills in code](#values-the-site-fills-in-code))
and a page leaves it empty, the role's missing rule
(`title_missing`, `description_missing`, `share_image_missing`) does
not report the page. The page still counts as checked, so in a score it
passes: the site does show a value there, and leaving the page out would
make the share of passing pages look smaller than it is. A value the
page does hold is checked as before (length, duplicates). The module
cannot read what the code makes, so nothing is measured for those pages,
and for `seoTitle` the page title is not used in its place: the site
said it shows something else. `res.Quality.FilledInCode` counts the
pages per role and value, in role order, so a report can say "share
images: made by the site for 69 pages":

```json
"filledInCode": [{"role": "shareImage", "whenEmpty": "generated", "pages": 69}]
```

The count is a fact about the content and stays when the rule is turned
off. Since the module already reads an empty SEO title as the page
title, declare `whenEmpty` on `seoTitle` only when the site shows
something else (a title it composes, or one for the whole site). A
`siteDefault` description is the same text on every page that uses it;
search engines may then pick their own snippet, so writing one per page
is still worth it where it matters.

**What the caller supplies.** The checking packages never read git or
the network. `quality.Options` carries what only the caller knows;
package `gitlog` reads the dates from a local checkout (the newest of
the last 5000 commits touching each file, relative to the directory
asked about, a file changed and not committed as now; at most 64 MB of
output per command and a 30 second deadline; on any error, no dates):

```go
lastChanged, err := gitlog.LastChanged(ctx, dir, gitlog.Options{}) // or the caller's own dates
res, err := validate.Site(fsys, validate.Options{
    Only: []string{validate.ScopeSchema, validate.ScopeContent, validate.ScopeLinks, validate.ScopeQuality},
    Quality: quality.Options{
        LastChanged: lastChanged,            // path → when it last changed; nil skips translation_outdated
        ListMedia:   quality.FSMedia(fsys),  // lists files under quality.MediaRoots(cfg) with sizes; nil skips the media rules
        RootFiles:   quality.FSRootFiles(fsys), // file names at the repository root, to guess the build framework; nil guesses nothing
        Framework:   "",                     // or quality.FrameworkAstro when the caller knows; wins over RootFiles
        Limits:      quality.Limits{},       // zero keeps the schema's and the defaults
    },
})
```

A rule whose data is missing is skipped and named in
`res.Quality.Skipped` with the reason. `res.Quality.Checked` counts, per
rule, the instances it looked at (each indexable page for a title rule,
each image for alt text, each expected language version, each raster
image in use): what a score divides by. `quality.Specs()` gives each
rule's area, impact and threshold; `quality.Check` runs the rules on
pages a caller already holds.

**Images the build optimizes.** Some frameworks resize and re-encode
images at build time and never serve the original: Astro does it for
images under `src/assets` (imported in code or through `astro:assets`
and `<Image>`), while it copies `public/` as it is. A storage says so
with `optimizedAtBuild`:

```yaml
# config.yml
storages:
  images:
    type: repo
    path: public/images   # copied as is: weighed
    publicUrl: /images
  assets:
    type: repo
    path: src/assets/images
    optimizedAtBuild: true # resized at build: not weighed
```

`quality.image_heavy` skips the images in such a folder (a file in a
subfolder belongs to the deepest storage folder holding it);
`quality.media_unused` still lists the files nothing refers to. The
value must be `true` or `false`; anything else is
`schema.invalid_storage`.

When a storage does not say, the module guesses from the build
framework, which it reads from the file names at the repository root
(`Options.RootFiles`, or `Options.Framework` from a caller that knows):

| Framework | Marked by                                   | Optimized at build | Served as is |
| --------- | ------------------------------------------- | ------------------ | ------------ |
| Astro     | `astro.config.{mjs,js,ts,mts,cjs,cts}`      | `src/assets/**`    | `public/**`  |

Only Astro for now. Next.js is left out on purpose: `next/image`
optimizes at request time, only where the code uses it, and not at all
in a static export. A storage in neither folder, or on a site no row
matches, is served as is, as before. Guessing "served as is" changes
nothing against no guess at all, so an image found heavy there keeps its
full weight; the hint says how to confirm the folder. Only a guessed
"optimized at build" changes a result: its images are skipped until the
storage says otherwise.

`res.Quality.MediaFolders` lists every media root with its storage,
whether its images are optimized at build, whether that was guessed, and
how many raster images content uses there (skipped or checked; zero when
the files were not listed), so a report can say "src/assets/images:
optimized at build, 12 images". `res.Quality.Guessed` lists what the run
assumed because the schema does not say it, for a person to confirm by
declaring it. Only folders where content uses at least one raster image
are listed (elsewhere the guess changes nothing), so nothing is guessed
when the files were not listed:

```json
"guessed": [{"kind": "optimizedAtBuild", "storage": "assets", "path": "src/assets/images",
             "value": true, "framework": "astro", "basis": "astro.config.mjs"}]
```

`kind` is the storage key that confirms the guess, `value` the answer to
write there.

**Files the site's code uses.** A file no content refers to may still be
used by the site's code: a logo the layout imports, an icon a component
names. A storage lists such files in `usedByCode`, paths relative to its
`path`, where `*`, `?` and `[...]` match within a name and a `**` name
any number of folders:

```yaml
storages:
  images:
    type: repo
    path: public/images
    usedByCode: [logo.svg, 'og/**']
```

`quality.media_unused` skips a file a storage that holds it names this
way; it is in use, so a raster one is weighed by `quality.image_heavy`
like an image content uses. At most 500 entries
(`schema.MaxUsedByCode`) of at most 300 bytes; an entry that is empty,
absolute, holds `.` or `..`, uses `**` inside a name or is not a valid
pattern, a value that is not a list of strings, or an entry listed twice
is `schema.invalid_storage`. `schema.Storage.UsedByCode`,
`Storage.KeepsFile`, `schema.CheckCodePattern` and
`schema.MatchCodePattern` read it; `quality.KeptByCode(cfg, path)` asks
it of one file. `quality.UsedMedia(cfg, values...)` reads values the
rule never saw (a tool's unpublished changes) the way the rule reads a
page, so a tool removing unused files never removes one such a change
uses.

**The image weight limit.** `quality.limits.imageKB` in `config.yml`
sets the weight in kilobytes over which an image is heavy (low impact);
the medium band starts at 5.12 times it, the ratio of the defaults (200
KB and 1 MB), so `imageKB: 200` is the default and `imageKB: 500` flags
images over 500 KB as low and over 2.5 MB as medium:

```yaml
quality:
  limits:
    imageKB: 500
```

It is a whole number from 20 to 10000; anything else, an unknown key
under `limits`, or `limits` in a collection (an image is checked once
per file, not per page, so the limit is the site's) is the warning
`schema.invalid_quality`, and the default applies. A caller's
`Options.Limits` wins over the schema.

**Turning a rule off.** A site that does not want a check says so in
the schema, reviewable in a pull request:

```yaml
# config.yml: for the whole site
quality:
  ignore: [quality.share_image_missing]

# collections/legal.yml: for one collection's pages
quality:
  ignore: [quality.description_missing]
```

A rule turned off checks nothing and is listed in `res.Quality.Ignored`
(site) or `res.Quality.IgnoredIn` (per collection). An entry that is not
a quality rule is the warning `schema.invalid_quality`. Thresholds come
from the schema too: `recommendedLength` on the title or description
field, `translations.locales` on a collection, `quality.limits.imageKB`
for the site.

Sources: [1] https://developers.google.com/search/docs/appearance/title-link
[2] https://developers.google.com/search/docs/appearance/snippet
[3] https://developers.facebook.com/docs/sharing/webmasters/images/
[4] https://www.w3.org/WAI/WCAG22/Understanding/non-text-content.html
[5] https://developers.google.com/search/docs/appearance/google-images
[6] https://dequeuniversity.com/rules/axe/4.7/heading-order
[7] https://www.w3.org/WAI/WCAG22/Understanding/link-purpose-in-context.html
[8] https://developer.chrome.com/docs/lighthouse/performance/uses-optimized-images
[9] https://sitebulb.com/hints/performance/transferred-image-size-is-over-100kb/

## Writing facts

What a check guessed, a person confirms or corrects by writing it into
the schema. Package `fact` does that edit, so a tool can offer a button
instead of a snippet to copy:

```go
asIs := false
p, err := fact.Make(os.DirFS("."), []fact.Fact{
    {Kind: fact.KindRole, Collection: "pages", Field: "seo.title", Role: schema.RoleSEOTitle},
    {Kind: fact.KindRole, Collection: "pages", Field: "seo.image", Role: schema.RoleShareImage, WhenEmpty: schema.WhenEmptyGenerated},
    {Kind: fact.KindRole, Collection: "pages", Field: "meta.title", Role: schema.RoleNone},
    {Kind: fact.KindTranslationLocales, Collection: "cases", Locales: []string{"cs"}},
    {Kind: fact.KindOptimizedAtBuild, Storage: "images", OptimizedAtBuild: &asIs},
    {Kind: fact.KindLocalized, Collection: "works", Field: "title", Fallback: "cs"},
    {Kind: fact.KindRole, Collection: "works", Field: "title", Role: schema.RoleSEOTitle, TitleTemplate: "{value} – {{site_name}}"},
    {Kind: fact.KindRole, Collection: "pages", Field: "seo.robots", Role: schema.RoleNoindex, When: []string{"noindex,follow"}},
    {Kind: fact.KindAlt, Collection: "works", Field: "image", AltFrom: []string{"title", "technique"}, Separator: " – "},
})
// p.Changes: per file, Path, Before and After bytes. Nothing is written.
errs, err := p.NewErrors(os.DirFS(".")) // schema errors the plan would bring
```

| Kind                 | Names                                                 | Writes                                    |
| -------------------- | ----------------------------------------------------- | ----------------------------------------- |
| `role`               | `Collection` or `Block`, and `Field`                  | `role:` and, when given, `whenEmpty:`, `titleTemplate:`, `excerpt:` and `noindexWhen:` or `draftWhen:` (from `When`) on the field |
| `localized`          | `Collection` or `Block`, and `Field` (a group)        | `localized: true` and, when given, `fallback:` |
| `alt`                | `Collection` or `Block`, `Field` (an image), and `Alt` or `AltFrom` | `alt: <key>`, or `alt: {from: [...], separator: "..."}` |
| `translationLocales` | `Collection` (one with `translations`)                | `translations.locales`                    |
| `optimizedAtBuild`   | `Storage`                                             | `optimizedAtBuild:` on the storage        |
| `usedByCode`         | `Storage` and `Paths` (1 to 100)                      | the paths added to the storage's `usedByCode` |

`Field` is the field's place as `resolve.CollectionRoles` names it
(`seo.page_title`, `blocks[logos].list[].name`). `Make` follows it
through the loaded schema to the file that declares the field: the
collection's file, the block or input a `$ref` pulled in, a collection
written inline in `config.yml`, or the legacy single file. Storages are
in `config.yml`. A `usedByCode` fact adds its paths to what the file
lists (each once, the existing ones first); a path without a wildcard
must name a file of the storage.

The edit changes only its lines: a value is replaced where it stands
(its comment kept), a new key goes on its own line after `label`,
`type` or `key` (whichever sits on one line), inside `{…}` when the
field is written that way. Everything else, comments, blank lines,
quoting and key order, stays byte for byte. A role removes the knobs
left beside it that it does not take (`whenEmpty` beside `none`,
`noindexWhen` beside anything but `noindex`, and so on); a knob the fact
does not give is otherwise left as the file has it. Facts apply in
order and each is judged on the schema as the earlier ones leave it, so
one plan can make a group localized and give it a role. Setting what
the file already says changes nothing, so a plan made twice is empty
the second time.

Refused rather than guessed, each with its error: a fact that says
something the schema cannot hold (`fact.ErrInvalid`: an unknown role, a
role the field cannot hold as the fact says it (a select without its
values, rich text without its excerpt, a plain group), a value that is
not an option, a group whose fields are not the site's languages, an
alt text from a field that cannot hold one, a language the site lacks, at most
`fact.MaxFacts` (50) facts), something the schema does not declare
(`ErrNotFound`), a path or definition in more than one place (two
fields with one key, a collection both inline and in a file:
`ErrAmbiguous`), and a layout with no safe place for the line
(`ErrUnplaceable`). A `*fact.FactError` names the fact by index.
`fact.Apply(src, edit)` is the edit on one file's bytes.

## Fixtures

`testdata/sites/*` are small sites with an `expected.json` of the
findings a run must produce. They pin the rules for every implementation
of them: the dashboard's validators run the same fixtures. Regenerate
with `go test ./validate -run TestFixtures -update` and read the diff.

## Regular expressions

The editor evaluates `regex` knobs in JavaScript; this module uses Go's
RE2 engine. A pattern RE2 cannot compile (lookaround, backreferences) is
reported as `schema.invalid_regex` and skipped in content checks rather
than silently passed. Keep patterns to the common subset when a site is
checked in CI.

## License

Apache License 2.0, see [LICENSE](LICENSE).
