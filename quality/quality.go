package quality

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
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
	ImageHeavyBytes     int64
	ImageVeryHeavyBytes int64
}

// Options is what the caller knows and the module cannot.
type Options struct {
	// SourceBases reads what the source of each language version said
	// when the version was last current with it (BaseReader). Nil skips
	// quality.translation_outdated; a version whose base it can't read is
	// not judged.
	SourceBases BaseReader
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
	// Blob is the git blob id of the file as read (content.BlobID); empty
	// when unknown, and a mark then never applies.
	Blob string
	// Mark is the translation mark of the file's sidecar, if any.
	Mark *content.TranslationMark

	// view is set on one language of a page that holds every language
	// in localized groups.
	view *view
}

// view is one language of a page whose file holds every language.
type view struct {
	locale string
	// primary is the view that also checks what does not change with the
	// language (an image's plain alt text, a rich text that is not
	// localized), so it is reported once.
	primary bool
}

// localeOf is the language a page is read in.
func (p Page) localeOf() string {
	if p.view != nil {
		return p.view.locale
	}
	return p.Locale
}

// once reports whether a value that is the same in every language is
// checked on this page: always, but on one view only of a localized
// page.
func (p Page) once() bool { return p.view == nil || p.view.primary }

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
	// Drafts counts the pages the search and accessibility rules skipped
	// because their draft field says they are not published yet.
	Drafts int `json:"drafts,omitempty"`
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
	r.variables = siteVariables(r.cfg, pages)
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
	// variables are the site's `{{key}}` values, for title templates.
	variables []resolve.SiteVariable
	// drafts holds the pages whose draft field is on, by path.
	drafts map[string]bool
}

func newRun(cfg *schema.Config, opts Options) *run {
	if cfg == nil {
		cfg = &schema.Config{}
	}
	r := &run{cfg: cfg, opts: opts, checked: map[report.Rule]int{}, off: map[report.Rule]bool{}, roles: map[string]resolve.Roles{}, filled: map[[2]string]int{}, drafts: map[string]bool{}}
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
	s := Summary{Checked: map[report.Rule]int{}, Skipped: r.skipped, MediaFolders: r.folders, Guessed: r.guesses, Drafts: len(r.drafts)}
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

// page runs the rules that read one page alone. A draft is skipped. A
// page whose file holds every language (localized groups) is read once
// per language it is expected in.
func (r *run) page(p Page) {
	if p.Frontmatter == nil {
		p.Frontmatter = map[string]any{}
	}
	if r.isDraft(p) {
		r.drafts[p.Path] = true
		return
	}
	hidden := schema.HiddenPaths(r.cfg, p.Collection.Fields, p.Frontmatter)
	for _, v := range r.views(p) {
		from := len(r.out)
		r.search(v, hidden)
		r.accessibility(v)
		for i := from; i < len(r.out); i++ {
			r.out[i].Locale = localeOfPath(v, r.out[i].Path)
		}
	}
}

// views are the pages one file makes: itself, or one per expected
// language of a localized collection.
func (r *run) views(p Page) []Page {
	if !p.Collection.Localized() {
		return []Page{p}
	}
	locales := resolve.ExpectedLocales(r.cfg, p.Collection)
	if len(locales) == 0 {
		locales = []string{p.Locale}
	}
	primary := locales[0]
	if slices.Contains(locales, r.cfg.Preview.DefaultLocale) {
		primary = r.cfg.Preview.DefaultLocale
	}
	out := make([]Page, 0, len(locales))
	for _, l := range locales {
		v := p
		v.Locale = l
		v.URL = resolve.URLIn(p.Path, p.Collection, r.cfg.Preview, p.Frontmatter, l)
		v.view = &view{locale: l, primary: l == primary}
		out = append(out, v)
	}
	return out
}

// isDraft reports whether a page's draft field (role draft) says it is
// not published yet: true, or a value its draftWhen lists.
func (r *run) isDraft(p Page) bool {
	f, ok := r.rolesOf(p.Collection).Field(schema.RoleDraft)
	return ok && roleOn(f, resolve.ReadDotted(p.Frontmatter, f.Path))
}

// roleOn reports whether a noindex or draft field is on: a boolean that
// is true, or a select whose value its list names.
func roleOn(f resolve.RoleField, v any) bool {
	if len(f.When) > 0 {
		s := strings.TrimSpace(fmt.Sprint(v))
		return v != nil && slices.Contains(f.When, s)
	}
	b, _ := v.(bool)
	return b
}

// siteVariables are the `{{key}}` values of the site's variables
// document, when it is among the pages.
func siteVariables(cfg *schema.Config, pages []Page) []resolve.SiteVariable {
	if cfg.Variables.File == "" {
		return nil
	}
	for _, p := range pages {
		if p.Path == cfg.Variables.File {
			return resolve.SiteVariables(p.Frontmatter, cfg.Variables.Field)
		}
	}
	return nil
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
