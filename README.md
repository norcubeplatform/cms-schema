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
one directory without literal `match` names to tell them apart.

**content** — every content file against its collection, with the
editor's rules: required, value shape (including select options), item
discriminators of multi-shape arrays, array bounds, regex, length, number
bounds, uniqueness, custom validators, out-of-schema keys per
`extraKeysPolicy`, a body where the collection has none. Fields hidden
by `hidden` or by a false `visibleIf` are skipped, as in the editor.

**links** — the site as a whole: two files resolving to one URL, site
links that no file produces and no `knownPaths` entry covers, images
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
| `schema`   | The typed model (`Config`, `Collection`, `Field`, `Storage`…), `Parse`, array item shapes, conditions. |
| `load`     | `Load(fs.FS)`: directory or legacy single file, merge, `$ref`, schema version.                     |
| `content`  | Markdown-with-frontmatter and YAML data files to plain values; identity sidecars.                 |
| `resolve`  | Collection for a path, locale, URL and slug fallback, storage values, links in text.              |
| `validate` | `Schema`, `Page`, `Links` and the `Site` runner.                                                  |

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
