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
func Site(fsys fs.FS, opts Options) (*Result, error) {
	doc, findings, err := load.Load(fsys)
	if err != nil {
		return nil, err
	}
	cfg := doc.Config
	res := &Result{Config: cfg, SchemaFiles: len(doc.Files)}
	want := scopes(opts.Only)
	exists := func(p string) bool {
		info, err := fs.Stat(fsys, p)
		return err == nil && !info.IsDir()
	}

	if want[ScopeSchema] {
		findings = append(findings, Schema(doc, exists)...)
	} else {
		// Load problems are always reported; they make the rest unreliable.
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
			doc, perr := content.Parse(p, raw)
			explicit, _ := doc.Frontmatter[schema.CollectionKey].(string)
			coll, hasColl := resolve.CollectionFor(cfg, p, explicit)
			if opts.Collection != "" && (!hasColl || coll.Key != opts.Collection) {
				continue
			}
			res.Files++
			if perr != nil {
				findings = append(findings, report.Finding{File: p, Severity: report.SeverityError, Rule: report.RuleContentParseError, Message: "not valid YAML: " + strings.TrimPrefix(perr.Error(), "yaml: ")})
				continue
			}
			if !hasColl {
				if want[ScopeContent] && cfg.PagesDir != "" && resolve.IsUnder(p, cfg.PagesDir) {
					findings = append(findings, report.Finding{File: p, Severity: report.SeverityWarning, Rule: report.RuleContentNoCollection, Message: "the file is under the pages directory but no collection claims it", Hint: "Add a collection whose `path` covers it, or stamp the file with `_collection`."})
				}
				entries = append(entries, Entry{Path: p, Title: resolve.FileID(p), doc: doc})
				continue
			}
			if want[ScopeContent] && doc.HasFrontmatter {
				findings = append(findings, Page(cfg, coll, doc)...)
			}
			entries = append(entries, Entry{
				Path:       p,
				Collection: coll.Key,
				URL:        resolve.URL(p, coll, cfg.Preview, doc.Frontmatter),
				Locale:     resolve.Locale(p, coll, cfg.Preview, doc.Frontmatter),
				Title:      resolve.EntryTitle(coll, doc.Frontmatter, p),
				Keys:       resolve.EntryKeys(cfg, doc.Frontmatter),
				Routes:     resolve.TemplateRoutes(coll),
				doc:        doc,
				coll:       coll,
				hasCol:     true,
			})
		}
		if want[ScopeLinks] {
			sidecars, err := sidecarFiles(fsys)
			if err != nil {
				return nil, err
			}
			findings = append(findings, Links(cfg, entries, exists, sidecars)...)
		}
	}

	if opts.Strict {
		for i := range findings {
			findings[i].Severity = report.SeverityError
		}
	}
	report.Sort(findings)
	res.Findings = findings
	res.Errors, res.Warnings = report.Count(findings)
	return res, nil
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
