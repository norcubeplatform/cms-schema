// Package validate runs the checks: the schema on its own, every content
// file against its collection, and the links between files. Site runs
// all of them over a file system; Schema, Page and Links are the pieces
// for callers that already hold the parts.
package validate

import (
	"errors"
	"io/fs"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Scopes a run can be limited to.
const (
	ScopeSchema  = "schema"
	ScopeContent = "content"
	ScopeLinks   = "links"
)

// Options limit a run.
type Options struct {
	// Only lists the scopes to run; empty means all three.
	Only []string
	// Collection limits content and links checks to one collection's
	// files.
	Collection string
	// Strict promotes every warning to an error.
	Strict bool
}

// Result is what a run found.
type Result struct {
	Findings []report.Finding `json:"findings"`
	// Files is the number of content files checked.
	Files int `json:"files"`
	// SchemaFiles is the number of schema files loaded.
	SchemaFiles int `json:"schemaFiles"`
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
	// Config is the loaded configuration, for callers that want it.
	Config *schema.Config `json:"-"`
}

// Site validates the site rooted at fsys. load.ErrNotASite is returned
// when the directory holds no CMS configuration; other errors are I/O.
//
// Site never panics. A bug in this module surfaces as an
// `*.internal_error` finding: while loading the schema the run stops
// there, in one content file that file is skipped and the run goes on.
func Site(fsys fs.FS, opts Options) (res *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = internalResult(nil, internalError("", report.RuleSchemaInternalError, r)), nil
		}
	}()
	doc, findings, err := safeLoad(fsys)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		// Loading panicked; findings holds the one internal error.
		return internalResult(nil, findings[0]), nil
	}
	cfg := doc.Config
	res = &Result{Config: cfg, SchemaFiles: len(doc.Files)}
	want := scopes(opts.Only)
	exists := func(p string) bool {
		info, err := fs.Stat(fsys, p)
		return err == nil && !info.IsDir()
	}

	// Load problems are always reported; they make the rest unreliable.
	if want[ScopeSchema] {
		findings = append(findings, Schema(doc, exists)...)
	}

	if want[ScopeContent] || want[ScopeLinks] {
		files, err := contentFiles(fsys, cfg)
		if err != nil {
			return nil, err
		}
		var entries []Entry
		for _, p := range files {
			raw, err := fs.ReadFile(fsys, p)
			if err != nil {
				return nil, err
			}
			counted, entry, fileFindings := checkFile(cfg, p, raw, opts, want)
			if counted {
				res.Files++
			}
			findings = append(findings, fileFindings...)
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
		if want[ScopeLinks] {
			sidecars, err := sidecarFiles(fsys)
			if err != nil {
				return nil, err
			}
			findings = append(findings, Links(cfg, entries, exists, sidecars)...)
		}
	}

	finish(res, findings, opts.Strict)
	return res, nil
}

// safeLoad is load.Load with a panic turned into a nil document and one
// `schema.internal_error` finding.
func safeLoad(fsys fs.FS) (doc *load.Document, findings []report.Finding, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, nil
			findings = []report.Finding{internalError(load.ConfigDir, report.RuleSchemaInternalError, r)}
		}
	}()
	return load.Load(fsys)
}

// checkFile reads one content file into its findings and its entry for
// the links checks. counted is false for a file outside opts.Collection;
// entry is nil when the file is skipped. A panic in it is the file's
// one `content.internal_error` finding, and the file has no entry.
func checkFile(cfg *schema.Config, p string, raw []byte, opts Options, want map[string]bool) (counted bool, entry *Entry, out []report.Finding) {
	defer func() {
		if r := recover(); r != nil {
			counted, entry, out = true, nil, []report.Finding{internalError(p, report.RuleContentInternalError, r)}
		}
	}()
	doc, perr := content.Parse(p, raw)
	explicit, _ := doc.Frontmatter[schema.CollectionKey].(string)
	coll, hasColl := resolve.CollectionFor(cfg, p, explicit)
	if opts.Collection != "" && (!hasColl || coll.Key != opts.Collection) {
		return false, nil, nil
	}
	if perr != nil {
		return true, nil, []report.Finding{{File: p, Severity: report.SeverityError, Rule: report.RuleContentParseError, Message: "not valid YAML: " + strings.TrimPrefix(perr.Error(), "yaml: ")}}
	}
	if !hasColl {
		if want[ScopeContent] && cfg.PagesDir != "" && resolve.IsUnder(p, cfg.PagesDir) {
			out = append(out, report.Finding{File: p, Severity: report.SeverityWarning, Rule: report.RuleContentNoCollection, Message: "the file is under the pages directory but no collection claims it", Hint: "Add a collection whose `path` covers it, or stamp the file with `_collection`."})
		}
		return true, &Entry{Path: p, Title: resolve.FileID(p), doc: doc}, out
	}
	if want[ScopeContent] && doc.HasFrontmatter {
		out = append(out, Page(cfg, coll, doc)...)
	}
	return true, &Entry{
		Path:           p,
		Collection:     coll.Key,
		URL:            resolve.URL(p, coll, cfg.Preview, doc.Frontmatter),
		Locale:         resolve.Locale(p, coll, cfg.Preview, doc.Frontmatter),
		Title:          resolve.EntryTitle(coll, doc.Frontmatter, p),
		Keys:           resolve.EntryKeys(cfg, doc.Frontmatter),
		TranslationKey: resolve.TranslationKey(p, coll, cfg.Preview, doc.Frontmatter),
		Routes:         resolve.TemplateRoutes(coll),
		doc:            doc,
		coll:           coll,
		hasCol:         true,
	}, out
}

// internalResult is the result of a run that stopped on f.
func internalResult(cfg *schema.Config, f report.Finding) *Result {
	res := &Result{Config: cfg}
	finish(res, []report.Finding{f}, false)
	return res
}

func finish(res *Result, findings []report.Finding, strict bool) {
	if strict {
		for i := range findings {
			findings[i].Severity = report.SeverityError
		}
	}
	report.Sort(findings)
	res.Findings = findings
	res.Errors, res.Warnings = report.Count(findings)
}

func scopes(only []string) map[string]bool {
	if len(only) == 0 {
		return map[string]bool{ScopeSchema: true, ScopeContent: true, ScopeLinks: true}
	}
	out := map[string]bool{}
	for _, s := range only {
		out[strings.ToLower(strings.TrimSpace(s))] = true
	}
	return out
}

// contentFiles lists the content files under the site's content roots
// (or the whole tree when the config declares none), sorted, skipping
// `.norcube/` and `node_modules/`.
func contentFiles(fsys fs.FS, cfg *schema.Config) ([]string, error) {
	roots := resolve.ContentRoots(cfg)
	if len(roots) == 0 {
		roots = []string{"."}
	}
	seen := map[string]bool{}
	var out []string
	for _, root := range roots {
		err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fs.SkipDir
				}
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if p != root && (name == ".norcube" || name == "node_modules" || name == ".git" || strings.HasPrefix(p, load.ConfigDir)) {
					return fs.SkipDir
				}
				return nil
			}
			if !content.IsContentFile(p) || strings.HasPrefix(p, load.ConfigDir+"/") || seen[p] || p == load.LegacyConfigFile {
				return nil
			}
			seen[p] = true
			out = append(out, p)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func sidecarFiles(fsys fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, content.SidecarDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".yml") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
