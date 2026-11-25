package quality_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
)

// The source of a page is its version in the default language (cs in
// baseConfig); the English version is judged against it.
const (
	srcPath = "src/content/pages/cs/a.md"
	enPath  = "src/content/pages/en/a.md"
)

func sourcePage(title, label, link string) string {
	return page("title: " + title + "\nblocks:\n  - {_type: hero, label: " + label + ", link: " + link + ", photo: /images/" + label + ".jpg}")
}

func outdatedSite(source string, extra map[string]string) fstest.MapFS {
	files := map[string]string{
		srcPath: source,
		enPath:  sourcePage("A page title in English words", "Open", "/en/x/"),
	}
	for p, body := range extra {
		files[p] = body
	}
	return site(files)
}

// bases answers every query with one answer, and records the queries.
func bases(answer quality.SourceBase, asked *[]quality.BaseQuery) quality.BaseReader {
	return func(q []quality.BaseQuery) ([]quality.SourceBase, error) {
		*asked = append(*asked, q...)
		out := make([]quality.SourceBase, len(q))
		for i := range out {
			out[i] = answer
		}
		return out, nil
	}
}

func outdatedFindings(all []report.Finding) []report.Finding {
	var out []report.Finding
	for _, f := range all {
		if f.Rule == report.RuleQualityTranslationOutdated {
			out = append(out, f)
		}
	}
	return out
}

func TestTranslationOutdated(t *testing.T) {
	current := sourcePage("A page title that is long enough ok", "Read", "/cs/x/")

	t.Run("a text changed in the source", func(t *testing.T) {
		var asked []quality.BaseQuery
		before := sourcePage("An older page title that was here", "Read", "/cs/x/")
		res := run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Content: []byte(before)}, &asked)})
		got := outdatedFindings(res.Findings)
		if len(got) != 1 {
			t.Fatalf("got %v", got)
		}
		f := got[0]
		if f.File != enPath || f.Path != "title" || f.Locale != "en" || f.Measure == nil || f.Measure.Value != 1 || f.Measure.Unit != report.UnitChangedTexts {
			t.Errorf("finding %+v %+v", f, f.Measure)
		}
		source, blob, ok := quality.ParseOutdatedValue(f.Value)
		if !ok || source != srcPath || blob != content.BlobID([]byte(current)) {
			t.Errorf("value %q", f.Value)
		}
		if len(asked) != 1 || asked[0] != (quality.BaseQuery{Translation: enPath, Source: srcPath}) {
			t.Errorf("asked %+v", asked)
		}
		if n := res.Quality.Checked[report.RuleQualityTranslationOutdated]; n != 1 {
			t.Errorf("checked %d", n)
		}
	})

	// A link, an image, a number or a choice is not translated: a change
	// to it leaves every translation as current as it was.
	t.Run("only values that are not translated changed", func(t *testing.T) {
		var asked []quality.BaseQuery
		before := sourcePage("A page title that is long enough ok", "Read", "/cs/old-address/")
		res := run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Content: []byte(before)}, &asked)})
		if got := outdatedFindings(res.Findings); len(got) != 0 {
			t.Errorf("got %v", got)
		}
		image := page("title: A page title that is long enough ok\nblocks:\n  - {_type: hero, label: Read, link: /cs/x/, photo: /images/other.jpg}")
		res = run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Content: []byte(image)}, &asked)})
		if got := outdatedFindings(res.Findings); len(got) != 0 {
			t.Errorf("an image swap: %v", got)
		}
		reflowed := page("title: \"A page title  that is\\n long enough ok\"\nblocks:\n  - {_type: hero, label: Read, link: /cs/x/, photo: /images/Read.jpg}")
		res = run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Content: []byte(reflowed)}, &asked)})
		if got := outdatedFindings(res.Findings); len(got) != 0 {
			t.Errorf("white space: %v", got)
		}
		if n := res.Quality.Checked[report.RuleQualityTranslationOutdated]; n != 1 {
			t.Errorf("checked %d", n)
		}
	})

	t.Run("the source is as it was", func(t *testing.T) {
		var asked []quality.BaseQuery
		res := run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Same: true}, &asked)})
		if got := outdatedFindings(res.Findings); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	// Dates from another repository's history, a shallow clone or a file
	// older than the history read: no base, no verdict.
	t.Run("a base that can't be read is not judged", func(t *testing.T) {
		var asked []quality.BaseQuery
		res := run(t, outdatedSite(current, nil), quality.Options{SourceBases: bases(quality.SourceBase{}, &asked)})
		if got := outdatedFindings(res.Findings); len(got) != 0 {
			t.Errorf("got %v", got)
		}
		if n := res.Quality.Checked[report.RuleQualityTranslationOutdated]; n != 0 {
			t.Errorf("checked %d", n)
		}
	})

	t.Run("a mark that applies names the base", func(t *testing.T) {
		en := sourcePage("A page title in English words", "Open", "/en/x/")
		mark := func(blob string) string {
			return "schemaVersion: 1\ntranslation:\n  source: " + srcPath + "\n  sourceBlob: abc123\n  blob: " + blob + "\n"
		}
		var asked []quality.BaseQuery
		run(t, outdatedSite(current, map[string]string{content.SidecarPath(enPath): mark(content.BlobID([]byte(en)))}),
			quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Same: true}, &asked)})
		if len(asked) != 1 || asked[0].SourceBlob != "abc123" {
			t.Errorf("asked %+v", asked)
		}
		// The version changed since it was marked: its own change is the
		// newer base.
		asked = nil
		run(t, outdatedSite(current, map[string]string{content.SidecarPath(enPath): mark("0000")}),
			quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Same: true}, &asked)})
		if len(asked) != 1 || asked[0].SourceBlob != "" {
			t.Errorf("a stale mark was used: %+v", asked)
		}
	})

	t.Run("only versions with a source in the default language", func(t *testing.T) {
		var asked []quality.BaseQuery
		run(t, site(map[string]string{
			"src/content/pages/en/b.md": page("title: Only English"),
			"src/content/pages/de/b.md": page("title: Nur Deutsch"),
		}), quality.Options{SourceBases: bases(quality.SourceBase{Known: true, Same: true}, &asked)})
		if len(asked) != 0 {
			t.Errorf("asked %+v", asked)
		}
	})

	t.Run("skipped without a history", func(t *testing.T) {
		res := run(t, outdatedSite(current, nil), quality.Options{})
		if !slices.ContainsFunc(res.Quality.Skipped, func(s quality.Skipped) bool { return s.Rule == report.RuleQualityTranslationOutdated }) {
			t.Errorf("not skipped: %+v", res.Quality.Skipped)
		}
	})
}
