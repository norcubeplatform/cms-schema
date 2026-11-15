// Command cms-schema-validate checks a site checkout's `.norcube/cms/`
// configuration and content. It is the reference front end of the
// validate package; the Norcube CLI wraps the same package as
// `nrc cms validate`.
//
//	cms-schema-validate [flags] [path]
//
// Exit codes: 0 clean, 1 findings at error severity, 2 usage or I/O
// error, 3 not a CMS site.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/norcubeplatform/cms-schema/load"
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
	fs.Var(&only, "only", "scope to run: schema, content or links (repeatable; default all)")
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

	res, err := validate.Site(os.DirFS(path), validate.Options{Only: only, Collection: *collection, Strict: *strict})
	if err != nil {
		if errors.Is(err, load.ErrNotASite) {
			fmt.Fprintf(os.Stderr, "%s is not a Norcube CMS site: %v\n", path, err)
			return 3
		}
		fmt.Fprintln(os.Stderr, "error:", err)
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
		}{1, path, nonNil(res.Findings), map[string]int{"files": res.Files, "schemaFiles": res.SchemaFiles, "errors": res.Errors, "warnings": res.Warnings}}); err != nil {
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
	fmt.Fprintf(os.Stderr, "%d content files, %d schema files: %d errors, %d warnings\n", res.Files, res.SchemaFiles, res.Errors, res.Warnings)
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
