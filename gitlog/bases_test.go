package gitlog_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/gitlog"
	"github.com/norcubeplatform/cms-schema/quality"
	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/validate"
)

// A site in a subfolder of its repository, as on a real site: English is
// the default language and the source; Czech is the translation.
const (
	fixtureConfig = `preview:
  defaultLocale: en
  locales: [en, cs]
`
	fixturePages = `label: Pages
path: website/pages
match: '**/*.md'
previewPath: '/{locale}/{slug}/'
localeFrom: path
translations: {by: 'field:translationKey'}
fields:
  - {key: title, type: text, label: Title}
  - {key: translationKey, type: text, label: Key}
  - key: hero
    type: group
    label: Hero
    fields:
      - {key: heading, type: text, label: Heading}
      - {key: href, type: link, label: Link}
      - {key: image, type: image, label: Image, decorative: true}
      - {key: count, type: number, label: Count}
`
	enPage = "website/pages/en/community.md"
	csPage = "website/pages/cs/komunita.md"
)

func fixturePage(title, heading, href string) string {
	return "---\ntitle: " + title + "\ntranslationKey: community\nhero:\n  heading: " + heading + "\n  href: " + href + "\n  image: /a.svg\n  count: 3\n---\n"
}

type fixture struct {
	t    *testing.T
	repo string
	secs int64
}

func newFixture(t *testing.T) *fixture {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	f := &fixture{t: t, repo: t.TempDir(), secs: 1_000_000_000}
	git(t, f.repo, nil, "init", "-q")
	return f
}

func (f *fixture) write(files map[string]string) {
	for p, body := range files {
		full := filepath.Join(f.repo, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
}

// commit commits the files at a time far from the previous commit: the
// dates must never matter.
func (f *fixture) commit(files map[string]string) {
	f.write(files)
	git(f.t, f.repo, nil, "add", "-A")
	f.secs += 200 * 24 * 3600
	date := "@" + strconv.FormatInt(f.secs, 10) + " +0000"
	git(f.t, f.repo, []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}, "commit", "-q", "-m", "c")
}

func (f *fixture) outdated() []report.Finding {
	f.t.Helper()
	reader, cancel := gitlog.SourceBases(context.Background(), f.repo, gitlog.Options{})
	defer cancel()
	var readErr error
	res, err := validate.Site(os.DirFS(f.repo), validate.Options{Only: []string{validate.ScopeQuality}, Quality: quality.Options{
		SourceBases: func(q []quality.BaseQuery) ([]quality.SourceBase, error) {
			out, err := reader(q)
			readErr = err
			return out, err
		},
	}})
	if err != nil || readErr != nil {
		f.t.Fatal(err, readErr)
	}
	var out []report.Finding
	for _, x := range res.Findings {
		if x.Rule == report.RuleQualityTranslationOutdated {
			out = append(out, x)
		}
	}
	return out
}

func TestSourceBases(t *testing.T) {
	f := newFixture(t)
	f.commit(map[string]string{
		".norcube/cms/config.yml":            fixtureConfig,
		".norcube/cms/collections/pages.yml": fixturePages,
		enPage:                               fixturePage("Community", "Everyone can start", "/en/join/"),
		csPage:                               fixturePage("Komunita", "Každý může začít", "/cs/pridat/"),
	})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("one import: %v", got)
	}

	// A link, an image and a number change in the source, months later:
	// nothing to translate.
	f.commit(map[string]string{enPage: fixturePage("Community", "Everyone can start", "/en/join-us/")})
	f.commit(map[string]string{enPage: fixturePage("Community", "Everyone   can\n  start", "/en/join-us/")})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("a link and white space: %v", got)
	}

	// The source's heading changes: the translation is behind.
	changed := fixturePage("Community", "Anyone can start a team", "/en/join-us/")
	f.commit(map[string]string{enPage: changed})
	got := f.outdated()
	if len(got) != 1 || got[0].File != csPage || got[0].Path != "hero.heading" {
		t.Fatalf("a heading: %v", got)
	}
	if src, blob, ok := quality.ParseOutdatedValue(got[0].Value); !ok || src != enPage || blob != content.BlobID([]byte(changed)) {
		t.Errorf("value %q", got[0].Value)
	}

	// Marked up to date as of that source: current again.
	cs := fixturePage("Komunita", "Každý může začít", "/cs/pridat/")
	mark := "schemaVersion: 1\ntranslation:\n  source: " + enPage + "\n  sourceBlob: " + content.BlobID([]byte(changed)) + "\n  blob: " + content.BlobID([]byte(cs)) + "\n"
	f.commit(map[string]string{content.SidecarPath(csPage): mark})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("after the mark: %v", got)
	}

	// The source changes a text again: behind again, judged from the mark.
	f.commit(map[string]string{enPage: fixturePage("Our community", "Anyone can start a team", "/en/join-us/")})
	got = f.outdated()
	if len(got) != 1 || got[0].Path != "title" || got[0].Measure.Value != 1 {
		t.Fatalf("after the mark, a title: %v", got)
	}

	// The translation is updated: its own change is the base.
	f.commit(map[string]string{csPage: fixturePage("Naše komunita", "Kdokoli může založit tým", "/cs/pridat/")})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("after the update: %v", got)
	}

	// A translation edited and not committed is current.
	f.commit(map[string]string{enPage: fixturePage("Our community", "Anyone can start a team today", "/en/join-us/")})
	if got := f.outdated(); len(got) != 1 {
		t.Fatalf("committed: %v", got)
	}
	f.write(map[string]string{csPage: fixturePage("Naše komunita", "Kdokoli může založit tým ještě dnes", "/cs/pridat/")})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("in the working tree: %v", got)
	}
}

// A version git has never seen has no base: it is not judged.
func TestSourceBasesUntrackedSource(t *testing.T) {
	f := newFixture(t)
	f.commit(map[string]string{
		".norcube/cms/config.yml":            fixtureConfig,
		".norcube/cms/collections/pages.yml": fixturePages,
		csPage:                               fixturePage("Komunita", "Každý může začít", "/cs/pridat/"),
	})
	// The source is added after the translation's last change: it was
	// not there at the base.
	f.commit(map[string]string{enPage: fixturePage("Community", "Everyone can start", "/en/join/")})
	if got := f.outdated(); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestParseCommits(t *testing.T) {
	a, b := "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	got := gitlog.ParseCommits([]byte("\x1e" + a + "\n\nsrc/a.md\nsrc/b.md\n\x1e" + b + "\n\nsrc/a.md\nsrc/c.md\n\x1enot a commit\n\nsrc/d.md\n"))
	want := map[string]string{"src/a.md": a, "src/b.md": a, "src/c.md": b}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for p, c := range want {
		if got[p] != c {
			t.Errorf("%s: %s", p, got[p])
		}
	}
}

// shallowClone clones repo with --depth 1, as CI checkouts do.
func shallowClone(t *testing.T, repo string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shallow")
	git(t, repo, nil, "clone", "-q", "--depth", "1", "file://"+repo, dir)
	return dir
}

// A shallow clone's oldest commit lists every file as added, so every
// version would look current with its source: the history is refused,
// never judged, while a full clone of the same repository is read.
func TestShallowCloneIsRefused(t *testing.T) {
	f := newFixture(t)
	f.commit(map[string]string{
		".norcube/cms/config.yml":            fixtureConfig,
		".norcube/cms/collections/pages.yml": fixturePages,
		enPage:                               fixturePage("Community", "Everyone can start", "/en/join/"),
		csPage:                               fixturePage("Komunita", "Každý může začít", "/cs/pridat/"),
	})
	f.commit(map[string]string{enPage: fixturePage("Community", "Anyone can start a team", "/en/join/")})
	if got := f.outdated(); len(got) != 1 {
		t.Fatalf("the full repository: %v", got)
	}
	shallow := shallowClone(t, f.repo)
	queries := []quality.BaseQuery{{Translation: csPage, Source: enPage}}

	reader, cancel := gitlog.SourceBases(context.Background(), shallow, gitlog.Options{})
	defer cancel()
	if got, err := reader(queries); !errors.Is(err, gitlog.ErrShallowHistory) || got != nil {
		t.Fatalf("SourceBases on a shallow clone: %v, %v", got, err)
	}
	if got, err := gitlog.LastChanged(context.Background(), shallow, gitlog.Options{}); !errors.Is(err, gitlog.ErrShallowHistory) || got != nil {
		t.Fatalf("LastChanged on a shallow clone: %v, %v", got, err)
	}

	// The same repository cloned in full is read.
	full := filepath.Join(t.TempDir(), "full")
	git(t, f.repo, nil, "clone", "-q", "file://"+f.repo, full)
	reader, cancel = gitlog.SourceBases(context.Background(), full, gitlog.Options{})
	defer cancel()
	got, err := reader(queries)
	if err != nil || len(got) != 1 || !got[0].Known || got[0].Same {
		t.Fatalf("a full clone: %v, %v", got, err)
	}
}
