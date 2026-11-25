package quality

import (
	"io/fs"
	"strings"

	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// Build frameworks the module knows how image folders are served by.
const (
	FrameworkAstro = "astro"
)

// GuessOptimizedAtBuild is the kind of a Guess about a storage's
// `optimizedAtBuild`.
const GuessOptimizedAtBuild = schema.StorageKeyOptimizedAtBuild

// GuessKinds lists every kind of Guess.
func GuessKinds() []string { return []string{GuessOptimizedAtBuild} }

// Frameworks lists the build frameworks of the guess table, in its
// order.
func Frameworks() []string {
	out := make([]string, 0, len(buildFrameworks))
	for _, fw := range buildFrameworks {
		out = append(out, fw.Name)
	}
	return out
}

// maxRootFiles bounds the root file names read for the framework.
const maxRootFiles = 1000

// buildFramework is how one framework treats image folders when the
// schema says nothing.
type buildFramework struct {
	Name string
	// Configs are the root file names that mark a site built with it.
	Configs []string
	// Optimized are the folders whose images the build resizes and
	// re-encodes; AsIs the folders it copies unchanged.
	Optimized []string
	AsIs      []string
}

// buildFrameworks is the guess table, first match wins. Only Astro for
// now:
//
//	Framework  Optimized at build                      Served as is
//	Astro      src/assets (astro:assets, <Image>)      public
//
// Not added on purpose: Next.js (next/image optimizes at request time
// on the server or a loader, not at build, and only where the code uses
// the component; a static export does not optimize at all).
var buildFrameworks = []buildFramework{
	{
		Name:      FrameworkAstro,
		Configs:   []string{"astro.config.mjs", "astro.config.js", "astro.config.ts", "astro.config.mts", "astro.config.cjs", "astro.config.cts"},
		Optimized: []string{"src/assets"},
		AsIs:      []string{"public"},
	},
}

// DetectFramework names the build framework the repository's root
// files mark (FrameworkAstro), and the file that told; empty when none.
func DetectFramework(rootFiles []string) (framework, basis string) {
	names := map[string]bool{}
	for i, f := range rootFiles {
		if i >= maxRootFiles {
			break
		}
		names[strings.TrimPrefix(strings.TrimSpace(f), "./")] = true
	}
	for _, fw := range buildFrameworks {
		for _, c := range fw.Configs {
			if names[c] {
				return fw.Name, c
			}
		}
	}
	return "", ""
}

// FSRootFiles lists the names of the files at the root of a file system
// rooted at the site, for DetectFramework. Nil when it cannot be read.
func FSRootFiles(fsys fs.FS) []string {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && len(out) < maxRootFiles {
			out = append(out, e.Name())
		}
	}
	return out
}

// MediaFolder is one media root and how the site serves its images.
type MediaFolder struct {
	// Path is the repository folder, Storage the storage that declares it.
	Path    string `json:"path"`
	Storage string `json:"storage"`
	// OptimizedAtBuild is true when the build resizes and re-encodes the
	// folder's images; quality.image_heavy skips them.
	OptimizedAtBuild bool `json:"optimizedAtBuild"`
	// Guessed is true when the schema does not say and the answer comes
	// from the framework table (Summary.Guessed names it when content
	// uses images there).
	Guessed bool `json:"guessed,omitempty"`
	// Framework is the build framework a guess comes from; empty when
	// the schema says.
	Framework string `json:"framework,omitempty"`
	// Images counts the raster images content uses in the folder: skipped
	// by quality.image_heavy when OptimizedAtBuild, checked otherwise.
	// Zero when the media files were not listed.
	Images int `json:"images"`
}

// Guess is a fact the run assumed because the schema does not say it,
// for a person to confirm by declaring it.
type Guess struct {
	// Kind is what was guessed: GuessOptimizedAtBuild, the storage key
	// that confirms it.
	Kind string `json:"kind"`
	// Storage is the storage the guess is about, Path its folder.
	Storage string `json:"storage"`
	Path    string `json:"path"`
	// Value is the guessed answer; `optimizedAtBuild: <Value>` on the
	// storage confirms it.
	Value bool `json:"value"`
	// Framework is the build framework the guess comes from, Basis the
	// root file that marked it (empty when the caller named the
	// framework).
	Framework string `json:"framework"`
	Basis     string `json:"basis,omitempty"`
}

// mediaFolders answers, per media root, whether its images are
// optimized at build: declared on a storage of that path, else guessed
// from the framework, else served as is. The guesses come back too.
func mediaFolders(cfg *schema.Config, roots []string, opts Options) ([]MediaFolder, []Guess) {
	framework, basis := opts.Framework, ""
	if framework == "" {
		framework, basis = DetectFramework(opts.RootFiles)
	}
	var fw *buildFramework
	for i := range buildFrameworks {
		if buildFrameworks[i].Name == framework {
			fw = &buildFrameworks[i]
		}
	}
	var folders []MediaFolder
	var guesses []Guess
	for _, root := range roots {
		var storage string
		var declared *bool
		for _, s := range cfg.Storages {
			if s.Type != schema.StorageTypeRepo || s.Path != root || s.Mode == schema.StorageModeReadOnly {
				continue
			}
			if storage == "" {
				storage = s.Name
			}
			if s.OptimizedAtBuild != nil {
				storage, declared = s.Name, s.OptimizedAtBuild
				break
			}
		}
		f := MediaFolder{Path: root, Storage: storage}
		switch {
		case declared != nil:
			f.OptimizedAtBuild = *declared
		case fw != nil && underAnyDir(root, fw.Optimized):
			f.OptimizedAtBuild, f.Guessed, f.Framework = true, true, fw.Name
		case fw != nil && underAnyDir(root, fw.AsIs):
			f.Guessed, f.Framework = true, fw.Name
		}
		if f.Guessed {
			guesses = append(guesses, Guess{Kind: GuessOptimizedAtBuild, Storage: storage, Path: root, Value: f.OptimizedAtBuild, Framework: fw.Name, Basis: basis})
		}
		folders = append(folders, f)
	}
	return folders, guesses
}

// folderOf is the index of the deepest folder holding a path, or -1.
func folderOf(p string, folders []MediaFolder) int {
	best := -1
	for i, f := range folders {
		if resolve.IsUnder(p, f.Path) && p != f.Path && (best < 0 || len(f.Path) > len(folders[best].Path)) {
			best = i
		}
	}
	return best
}

func underAnyDir(p string, dirs []string) bool {
	for _, d := range dirs {
		if resolve.IsUnder(p, d) {
			return true
		}
	}
	return false
}
