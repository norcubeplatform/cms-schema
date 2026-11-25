// Command cms-schema-validate checks a site checkout's `.norcube/cms/`
// configuration and content. It is the reference front end of the
// validate package; the Norcube CLI wraps the same package as
// `nrc cms validate`.
//
//	cms-schema-validate [flags] [path]
//
// Exit codes: 0 clean, 1 findings at error severity, 2 usage or I/O
// error, 3 not a CMS site. Content quality findings (--quality) are
// warnings and notices and never fail the run unless --strict-quality.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/norcubeplatform/cms-schema/gitlog"
	"github.com/norcubeplatform/cms-schema/load"
	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("cms-schema-validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var only multiFlag
	fs.Var(&only, "only", "scope to run: schema, content, links or quality (repeatable; default the first three)")
	withQuality := fs.Bool("quality", false, "also run the content quality checks (titles, descriptions, alt text, languages, media)")
	withGit := fs.Bool("git", false, "with --quality, read the git history of path, so quality.translation_outdated runs")
	strictQuality := fs.Bool("strict-quality", false, "treat content quality warnings as errors")
	collection := fs.String("collection", "", "limit content and links checks to one collection")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	quiet := fs.Bool("quiet", false, "print nothing but the summary")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cms-schema-validate [flags] [path]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}

	if *withQuality {
		if len(only) == 0 {
			only = append(only, validate.ScopeSchema, validate.ScopeContent, validate.ScopeLinks)
		}
		only = append(only, validate.ScopeQuality)
	}
	if *withGit && !*withQuality {
		fmt.Fprintln(os.Stderr, "--git goes with --quality: the dates are for the content quality checks")
		return 2
	}
	site := os.DirFS(path)
	qopts := quality.Options{ListMedia: quality.FSMedia(site), RootFiles: quality.FSRootFiles(site)}
	var historyErr error
	if *withGit {
		reader, cancel := gitlog.SourceBases(context.Background(), path, gitlog.Options{})
		defer cancel()
		qopts.SourceBases = func(q []quality.BaseQuery) ([]quality.SourceBase, error) {
			out, err := reader(q)
			if err != nil {
				historyErr = err
			}
			return out, err
		}
	}
	res, err := validate.Site(site, validate.Options{
		Only: only, Collection: *collection, Strict: *strict, StrictQuality: *strictQuality,
		Quality: qopts,
	})
	if err != nil {
		if errors.Is(err, load.ErrNotASite) {
			fmt.Fprintf(os.Stderr, "%s is not a Norcube CMS site: %v\n", path, err)
			return 3
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	if historyErr != nil {
		if errors.Is(historyErr, gitlog.ErrShallowHistory) {
			fmt.Fprintf(os.Stderr, "error: --git: %s is a shallow clone, so its history can't tell which translations are out of date; fetch the full history (git fetch --unshallow, or fetch-depth: 0 in actions/checkout), or drop --git\n", path)
			return 2
		}
		fmt.Fprintln(os.Stderr, "error: reading the git history:", historyErr)
		return 2
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(struct {
			SchemaVersion int              `json:"schemaVersion"`
			Path          string           `json:"path"`
			Findings      []report.Finding `json:"findings"`
			Summary       map[string]int   `json:"summary"`
			Quality       *quality.Summary `json:"quality,omitempty"`
		}{1, path, nonNil(res.Findings), map[string]int{"files": res.Files, "schemaFiles": res.SchemaFiles, "errors": res.Errors, "warnings": res.Warnings, "notices": res.Notices}, res.Quality}); err != nil {
			return 2
		}
	} else if !*quiet {
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, f := range res.Findings {
			where := f.File
			if f.Path != "" {
				where += "#" + f.Path
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Severity, where, f.Rule, f.Message)
		}
		w.Flush()
	}
	if res.Notices > 0 {
		fmt.Fprintf(os.Stderr, "%d content files, %d schema files: %d errors, %d warnings, %d notices\n", res.Files, res.SchemaFiles, res.Errors, res.Warnings, res.Notices)
	} else {
		fmt.Fprintf(os.Stderr, "%d content files, %d schema files: %d errors, %d warnings\n", res.Files, res.SchemaFiles, res.Errors, res.Warnings)
	}
	if res.Errors > 0 {
		return 1
	}
	return 0
}

func nonNil(f []report.Finding) []report.Finding {
	if f == nil {
		return []report.Finding{}
	}
	return f
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			*m = append(*m, p)
		}
	}
	return nil
}
