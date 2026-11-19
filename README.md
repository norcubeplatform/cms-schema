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
```

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
names a required top-level text field, and `translatable` sits only on
fields it can mean something for.

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

The rule ids are listed in [`report/rules.go`](report/rules.go) and are
frozen: a released id is never renamed or reused.

## As a library

```go
res, err := validate.Site(os.DirFS("."), validate.Options{Strict: true})
for _, f := range res.Findings {
    fmt.Println(f)
}
```

| Package    | What it holds                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------------- |
| `report`   | `Finding`, severities, the frozen rule ids.                                                       |
| `schema`   | The typed model (`Config`, `Collection`, `Field`, `Storage`…), `Parse`, array item shapes, conditions, `Walk`. |
| `load`     | `Load(fs.FS)`: directory or legacy single file, merge, `$ref`, schema version.                     |
| `content`  | Markdown-with-frontmatter and YAML data files to plain values; identity sidecars.                 |
| `resolve`  | Collection for a path, locale, URL and slug fallback, storage values, links in text, translatable texts. |
| `validate` | `Schema`, `Page`, `Links` and the `Site` runner.                                                  |

`schema.Walk` is the one reading of a document against its schema: it
visits every field with its definition and its path (`a.b[2].c`),
entering groups, list items of every shape and lists of primitives, at
most `schema.MaxWalkDepth` levels deep. The validators and
`resolve.TranslatableTexts` are built on it.

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
