package validate_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

var update = flag.Bool("update", false, "rewrite testdata/sites/*/expected.json from the current run")

// expectedFinding is the frozen part of a finding: where it is and
// which rule fired. Messages and hints may improve without breaking
// the fixtures.
type expectedFinding struct {
	File     string          `json:"file,omitempty"`
	Path     string          `json:"path,omitempty"`
	Severity report.Severity `json:"severity"`
	Rule     report.Rule     `json:"rule"`
}

// TestFixtures runs every site under testdata/sites and compares the
// findings with the site's expected.json. The same fixtures are meant
// to be consumed by the dashboard's test suite, so the two
// implementations of a rule cannot drift apart unnoticed.
func TestFixtures(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "testdata", "sites", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no fixture sites found")
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			res, err := validate.Site(os.DirFS(dir), validate.Options{})
			if err != nil {
				t.Fatalf("Site: %v", err)
			}
			got := make([]expectedFinding, 0, len(res.Findings))
			for _, f := range res.Findings {
				got = append(got, expectedFinding{File: f.File, Path: f.Path, Severity: f.Severity, Rule: f.Rule})
			}
			expectedPath := filepath.Join(dir, "expected.json")
			if *update {
				b, err := json.MarshalIndent(got, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(expectedPath, append(b, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			raw, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read %s: %v (run with -update to create it)", expectedPath, err)
			}
			var want []expectedFinding
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("findings differ from %s", expectedPath)
				for _, g := range diff(got, want) {
					t.Logf("  %s", g)
				}
				for _, f := range res.Findings {
					t.Logf("got: %s", f)
				}
			}
			// Every finding names a rule from the frozen list.
			known := map[report.Rule]bool{}
			for _, r := range report.Rules() {
				known[r] = true
			}
			for _, f := range res.Findings {
				if !known[f.Rule] {
					t.Errorf("finding uses unlisted rule %q", f.Rule)
				}
			}
		})
	}
}

func diff(got, want []expectedFinding) []string {
	key := func(f expectedFinding) string {
		return f.File + "#" + f.Path + " " + string(f.Severity) + " " + string(f.Rule)
	}
	g := map[string]int{}
	for _, f := range got {
		g[key(f)]++
	}
	w := map[string]int{}
	for _, f := range want {
		w[key(f)]++
	}
	var out []string
	for k, n := range g {
		if w[k] != n {
			out = append(out, "+ "+k)
		}
	}
	for k, n := range w {
		if g[k] != n {
			out = append(out, "- "+k)
		}
	}
	return out
}
