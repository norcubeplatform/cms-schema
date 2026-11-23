package quality

import (
	"sort"
	"strings"
	"time"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// MediaFile is one file under a media storage, with its size.
type MediaFile struct {
	// Path is the repository path (`public/images/hero.jpg`).
	Path string
	Size int64
}

// MediaLister lists the files under the given repository directories
// (MediaRoots), with their sizes. It is called at most once per run.
type MediaLister func(roots []string) ([]MediaFile, error)

// Limits override the schema's and the default thresholds; a zero value
// keeps them. ImageVeryHeavyBytes left zero scales with ImageHeavyBytes
// as the defaults do.
type Limits struct {
	OutdatedAfter       time.Duration
	ImageHeavyBytes     int64
	ImageVeryHeavyBytes int64
}

// Options is what the caller knows and the module cannot.
type Options struct {
	// LastChanged maps a content file's repository path to when it last
	// changed. Nil skips quality.translation_outdated; a file missing from
	// it is undated and never counted as outdated.
	LastChanged map[string]time.Time
	// ListMedia lists the media files. Nil skips the media rules.
	ListMedia MediaLister
	Limits    Limits
	// RootFiles are the names of the files at the repository root, from
	// which the build framework is guessed (DetectFramework) for media
	// folders whose storage does not say `optimizedAtBuild`. Nil guesses
	// nothing.
	RootFiles []string
	// Framework names the build framework when the caller knows it
	// (FrameworkAstro); it wins over RootFiles.
	Framework string
}

// Page is one content file as the checks read it.
type Page struct {
	Path       string
	Collection schema.Collection
	// URL is where the page renders; empty for a document that is not a
	// page of its own (no previewPath), which the search rules skip.
	URL            string
	Locale         string
	TranslationKey string
	Frontmatter    map[string]any
	Body           string
	IsData         bool
}

// Skipped is a rule that did not run, and why.
type Skipped struct {
	Rule   report.Rule `json:"rule"`
	Reason string      `json:"reason"`
}

// Summary says what a run checked, for a score: per rule, how many
// instances it looked at (each indexable page for a title rule, each
// image for alt text, each expected language version). A rule turned
// off or skipped checked nothing.
type Summary struct {
	Checked map[report.Rule]int `json:"checked"`
	// Ignored lists the rules the site's `quality.ignore` turns off;
	// IgnoredIn the ones a collection turns off for its pages.
	Ignored   []report.Rule            `json:"ignored,omitempty"`
	IgnoredIn map[string][]report.Rule `json:"ignoredIn,omitempty"`
	Skipped   []Skipped                `json:"skipped,omitempty"`
	// FilledInCode counts the pages whose role field is empty because the
	// site fills it in code, as the schema says (`whenEmpty`), per role
	// and value in schema.PageRoles order. Those pages pass the role's
	// missing rule.
	FilledInCode []FilledInCode `json:"filledInCode,omitempty"`
	// MediaFolders says, per media root, whether the site's build
	// optimizes its images and how many images content uses there.
	MediaFolders []MediaFolder `json:"mediaFolders,omitempty"`
	// Guessed lists what the run assumed because the schema does not say
	// it, for a person to confirm: only guesses that change a result, so
	// a media folder where content uses no image (or whose files were
	// not listed) is left out.
	Guessed []Guess `json:"guessed,omitempty"`
}

// FilledInCode is how many pages, checked by the search rules, leave one
// role field empty for the site to fill.
type FilledInCode struct {
	// Role is schema.RoleSEOTitle, RoleSEODescription or RoleShareImage.
	Role string `json:"role"`
	// WhenEmpty is schema.WhenEmptyGenerated or WhenEmptySiteDefault.
	WhenEmpty string `json:"whenEmpty"`
	Pages     int    `json:"pages"`
}

// Check runs every quality rule over the pages. It reads nothing but its
// arguments and what opts.ListMedia returns.
func Check(cfg *schema.Config, pages []Page, opts Options) ([]report.Finding, Summary) {
	r := newRun(cfg, opts)
	for _, p := range pages {
		r.page(p)
	}
	r.duplicates()
	r.languages(pages)
	r.media(pages)
	report.Sort(r.out)
	return r.out, r.summary()
}

// run is the state of one Check.
type run struct {
	cfg  *schema.Config
	opts Options
	out  []report.Finding
	// checked counts instances per rule.
	checked map[report.Rule]int
	// off holds the rules the site turns off.
	off     map[report.Rule]bool
	skipped []Skipped
	roles   map[string]resolve.Roles
	// titles and descriptions of indexable pages, for duplicates.
	titles, descriptions []dupCandidate
	// filled counts pages per role and whenEmpty value.
	filled map[[2]string]int
	// folders and guesses are the media folders and what was guessed
	// about them.
	folders []MediaFolder
	guesses []Guess
}

func newRun(cfg *schema.Config, opts Options) *run {
	if cfg == nil {
		cfg = &schema.Config{}
	}
	r := &run{cfg: cfg, opts: opts, checked: map[report.Rule]int{}, off: map[report.Rule]bool{}, roles: map[string]resolve.Roles{}, filled: map[[2]string]int{}}
	for _, id := range cfg.Quality.Ignore {
		if _, ok := Spec(report.Rule(id)); ok {
			r.off[report.Rule(id)] = true
		}
	}
	return r
}

// on reports whether a rule runs for a page of the collection.
func (r *run) on(rule report.Rule, coll schema.Collection) bool {
	if r.off[rule] {
		return false
	}
	for _, id := range coll.Quality.Ignore {
		if id == string(rule) {
			return false
		}
	}
	return true
}

// count adds one instance a rule checked.
func (r *run) count(rule report.Rule) { r.checked[rule]++ }

// emit adds a finding at the impact, capped at the rule's own.
func (r *run) emit(f report.Finding, impact report.Impact) {
	if spec, ok := Spec(f.Rule); ok && weight(impact) > weight(spec.Impact) {
		impact = spec.Impact
	}
	f.Impact = impact
	f.Severity = report.SeverityOf(impact)
	f.Value = cutRunes(f.Value, maxValueRunes)
	r.out = append(r.out, f)
}

func (r *run) skip(rule report.Rule, reason string) {
	if !r.off[rule] {
		r.skipped = append(r.skipped, Skipped{Rule: rule, Reason: reason})
	}
}

func (r *run) rolesOf(coll schema.Collection) resolve.Roles {
	if roles, ok := r.roles[coll.Key]; ok {
		return roles
	}
	roles := resolve.CollectionRoles(r.cfg, coll)
	r.roles[coll.Key] = roles
	return roles
}

func (r *run) summary() Summary {
	s := Summary{Checked: map[report.Rule]int{}, Skipped: r.skipped, MediaFolders: r.folders, Guessed: r.guesses}
	for _, spec := range specs {
		if r.off[spec.Rule] {
			s.Ignored = append(s.Ignored, spec.Rule)
			continue
		}
		s.Checked[spec.Rule] = r.checked[spec.Rule]
	}
	for _, role := range schema.PageRoles() {
		for _, v := range schema.WhenEmptyValues() {
			if n := r.filled[[2]string{role, v}]; n > 0 {
				s.FilledInCode = append(s.FilledInCode, FilledInCode{Role: role, WhenEmpty: v, Pages: n})
			}
		}
	}
	for _, key := range r.cfg.CollectionKeys {
		var rules []report.Rule
		for _, id := range r.cfg.Collections[key].Quality.Ignore {
			if _, ok := Spec(report.Rule(id)); ok && !r.off[report.Rule(id)] {
				rules = append(rules, report.Rule(id))
			}
		}
		if len(rules) > 0 {
			if s.IgnoredIn == nil {
				s.IgnoredIn = map[string][]report.Rule{}
			}
			s.IgnoredIn[key] = rules
		}
	}
	return s
}

// page runs the rules that read one page alone.
func (r *run) page(p Page) {
	if p.Frontmatter == nil {
		p.Frontmatter = map[string]any{}
	}
	hidden := schema.HiddenPaths(r.cfg, p.Collection.Fields, p.Frontmatter)
	r.search(p, hidden)
	r.accessibility(p)
}

// weight orders impacts; info weighs nothing.
func weight(i report.Impact) int {
	switch i {
	case report.ImpactHigh:
		return 3
	case report.ImpactMedium:
		return 2
	case report.ImpactLow:
		return 1
	}
	return 0
}

// lowered caps an impact at low: the finding stands on a guess.
func lowered(i report.Impact, guessed bool) report.Impact {
	if guessed && weight(i) > weight(report.ImpactLow) {
		return report.ImpactLow
	}
	return i
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// collapse trims a text and joins its runs of white space into one
// space.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
