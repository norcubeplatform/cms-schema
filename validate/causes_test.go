package validate_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

// The sites under testdata/causes are built to make every rule fire for
// each of its causes. Each site's findings.json pins what a run reports,
// sentences included: the English a person reads in a terminal changes
// only when that file changes in the same commit. The dashboard's tests
// may read the same files.

// causeRun is one run of a causes site.
type causeRun struct {
	Name     string            `json:"name"`
	Findings []report.Finding  `json:"findings"`
	Skipped  []quality.Skipped `json:"skipped,omitempty"`
}

type causeGolden struct {
	Runs []causeRun `json:"runs"`
}

// causeSiteRuns are the runs made on a causes site: every scope with the
// quality data a test can give, and, on the quality site, runs without
// that data and with readers that fail, for the reasons a rule is
// skipped.
func causeSiteRuns(name string, fsys fs.FS) map[string]validate.Options {
	all := []string{validate.ScopeSchema, validate.ScopeContent, validate.ScopeLinks, validate.ScopeQuality}
	full := validate.Options{Only: all, Quality: quality.Options{
		ListMedia:   sizedMedia(fsys),
		RootFiles:   quality.FSRootFiles(fsys),
		SourceBases: basesFrom(fsys),
	}}
	runs := map[string]validate.Options{"full": full}
	if name == "quality" {
		runs["bare"] = validate.Options{Only: all}
		failing := errors.New("the reader failed")
		runs["failing"] = validate.Options{Only: all, Quality: quality.Options{
			ListMedia:   func([]string) ([]quality.MediaFile, error) { return nil, failing },
			SourceBases: func([]quality.BaseQuery) ([]quality.SourceBase, error) { return nil, failing },
		}}
	}
	return runs
}

// sizedMedia lists the media files of fsys; a file whose text is
// "bytes <n>" weighs n bytes, so a fixture can hold a heavy image
// without being one.
func sizedMedia(fsys fs.FS) quality.MediaLister {
	list := quality.FSMedia(fsys)
	return func(roots []string) ([]quality.MediaFile, error) {
		files, err := list(roots)
		for i, f := range files {
			raw, rerr := fs.ReadFile(fsys, f.Path)
			if rerr != nil {
				continue
			}
			if n, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "bytes "); ok {
				if size, perr := strconv.ParseInt(n, 10, 64); perr == nil {
					files[i].Size = size
				}
			}
		}
		return files, err
	}
}

// basesFrom answers a language version's base from bases/<source path>:
// the source as it was when the version was last current with it. A
// source without such a file has no known base.
func basesFrom(fsys fs.FS) quality.BaseReader {
	return func(queries []quality.BaseQuery) ([]quality.SourceBase, error) {
		out := make([]quality.SourceBase, len(queries))
		for i, q := range queries {
			raw, err := fs.ReadFile(fsys, path.Join("bases", q.Source))
			if err == nil {
				out[i] = quality.SourceBase{Known: true, Content: raw}
			}
		}
		return out, nil
	}
}

// runCauseSite runs every run of one causes site, in a stable order.
func runCauseSite(t *testing.T, name string, fsys fs.FS) causeGolden {
	t.Helper()
	var g causeGolden
	runs := causeSiteRuns(name, fsys)
	for _, run := range []string{"full", "bare", "failing"} {
		opts, ok := runs[run]
		if !ok {
			continue
		}
		res, err := validate.Site(fsys, opts)
		if err != nil {
			t.Fatalf("%s: %v", run, err)
		}
		r := causeRun{Name: run, Findings: res.Findings}
		if res.Quality != nil {
			r.Skipped = res.Quality.Skipped
		}
		if run != "full" {
			// The other runs are there for what they skip.
			r.Findings = nil
		}
		g.Runs = append(g.Runs, r)
	}
	return g
}

func causeSites(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "testdata", "causes", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no causes sites: %v", err)
	}
	return dirs
}

// TestCauseSitesEnglish pins every finding of the causes sites, with
// its sentence and hint.
func TestCauseSitesEnglish(t *testing.T) {
	for _, dir := range causeSites(t) {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			got := runCauseSite(t, name, os.DirFS(dir))
			b, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			b = append(b, '\n')
			golden := filepath.Join(dir, "findings.json")
			if *update {
				if err := os.WriteFile(golden, b, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read %s: %v (run with -update to create it)", golden, err)
			}
			if !bytes.Equal(b, want) {
				t.Errorf("findings differ from %s; run with -update and review the diff", golden)
			}
		})
	}
}

// notInCauseSites are the causes no site can produce: a bug in this
// module (a recovered panic) and a schema file the file system fails to
// read. TestInternalErrorsKeepTheCatalog and the load tests make them.
var notInCauseSites = map[report.Rule]report.Cause{
	report.RuleSchemaInternalError:  "",
	report.RuleContentInternalError: "",
	report.RuleLinksInternalError:   "",
	report.RuleSchemaParseError:     report.CauseUnreadable,
}

// TestCauseSitesKeepTheCatalog holds every finding of the causes sites
// to its catalog entry (a listed rule and cause, exactly the params it
// declares, Message and Hint made by report.English) and checks that
// the sites reach every rule and cause of the catalog, and every reason
// a quality rule is skipped.
func TestCauseSitesKeepTheCatalog(t *testing.T) {
	type code struct {
		rule  report.Rule
		cause report.Cause
	}
	reached := map[code]bool{}
	for r, c := range notInCauseSites {
		reached[code{r, c}] = true
	}
	skips := map[quality.SkipReason]bool{quality.SkipLanguagesCapped: true}
	for _, dir := range causeSites(t) {
		name := filepath.Base(dir)
		fsys := os.DirFS(dir)
		for run, opts := range causeSiteRuns(name, fsys) {
			res, err := validate.Site(fsys, opts)
			if err != nil {
				t.Fatalf("%s %s: %v", name, run, err)
			}
			for _, f := range res.Findings {
				if err := report.CheckFinding(f); err != nil {
					t.Errorf("%s %s: %s#%s: %v", name, run, f.File, f.Path, err)
				}
				reached[code{f.Rule, f.Cause}] = true
			}
			if res.Quality == nil {
				continue
			}
			for _, s := range res.Quality.Skipped {
				skips[s.Reason] = true
				if s.Message != s.English() {
					t.Errorf("skipped %s: message %q, English %q", s.Reason, s.Message, s.English())
				}
			}
		}
	}
	for _, s := range report.Catalog() {
		if !reached[code{s.Rule, s.Cause}] {
			t.Errorf("no causes site reaches %s %q", s.Rule, s.Cause)
		}
	}
	for _, s := range quality.SkipReasons() {
		if !skips[s.Reason] {
			t.Errorf("no causes site skips a rule for %s", s.Reason)
		}
	}
}
