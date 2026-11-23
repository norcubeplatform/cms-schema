package quality

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/norcubeplatform/cms-schema/report"
	"github.com/norcubeplatform/cms-schema/resolve"
	"github.com/norcubeplatform/cms-schema/schema"
)

// rasterExtensions are the image formats whose file size the media rules
// judge. Vector images and video are left out.
var rasterExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true, ".tif": true, ".tiff": true}

// MediaRoots are the repository directories the media rules look in: the
// path of every read-write repository storage, sorted. A read-only
// storage holds files the site manages elsewhere.
func MediaRoots(cfg *schema.Config) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range cfg.Storages {
		if s.Type != schema.StorageTypeRepo || s.Path == "" || s.Mode == schema.StorageModeReadOnly || seen[s.Path] {
			continue
		}
		seen[s.Path] = true
		out = append(out, s.Path)
	}
	sort.Strings(out)
	return out
}

// FSMedia lists media files from a file system rooted at the site, as a
// local checkout is: every file under each root but hidden ones, at most
// MaxMediaFiles.
func FSMedia(fsys fs.FS) MediaLister {
	return func(roots []string) ([]MediaFile, error) {
		var out []MediaFile
		for _, root := range roots {
			err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						return fs.SkipDir
					}
					return err
				}
				if strings.HasPrefix(d.Name(), ".") && p != root {
					if d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if len(out) >= MaxMediaFiles {
					return fs.SkipAll
				}
				info, err := d.Info()
				if err != nil {
					return nil
				}
				out = append(out, MediaFile{Path: p, Size: info.Size()})
				return nil
			})
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
		return out, nil
	}
}

// media runs the media rules: the size of each raster image content uses,
// and the files nothing refers to. Images in a folder the site's build
// optimizes are not weighed.
func (r *run) media(pages []Page) {
	roots := MediaRoots(r.cfg)
	if len(roots) == 0 {
		return
	}
	r.folders, r.guesses = mediaFolders(r.cfg, roots, r.opts)
	heavyOn, unusedOn := !r.off[report.RuleQualityImageHeavy], !r.off[report.RuleQualityMediaUnused]
	if !heavyOn && !unusedOn {
		return
	}
	if r.opts.ListMedia == nil {
		r.skip(report.RuleQualityImageHeavy, "the media files were not listed")
		r.skip(report.RuleQualityMediaUnused, "the media files were not listed")
		return
	}
	files, err := r.opts.ListMedia(roots)
	if err != nil {
		reason := "the media files could not be listed: " + err.Error()
		r.skip(report.RuleQualityImageHeavy, reason)
		r.skip(report.RuleQualityMediaUnused, reason)
		return
	}
	if len(files) > MaxMediaFiles {
		files = files[:MaxMediaFiles]
	}
	used := r.usedFiles(pages, roots)

	heavy, veryHeavy := r.imageLimits()
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		p := resolve.CleanPath(f.Path)
		if !underAny(p, roots) || strings.HasPrefix(path.Base(p), ".") {
			continue
		}
		if !used[p] {
			if unusedOn {
				r.emit(report.Finding{File: p, Rule: report.RuleQualityMediaUnused,
					Message: "No content and no schema refers to this file.",
					Hint:    "If the site's code does not use it either, delete it to keep the media library tidy.",
					Value:   p, Measure: &report.Measure{Value: f.Size, Unit: report.UnitBytes}}, report.ImpactInfo)
			}
			continue
		}
		if !rasterExtensions[strings.ToLower(path.Ext(p))] {
			continue
		}
		folder := MediaFolder{}
		if i := folderOf(p, r.folders); i >= 0 {
			r.folders[i].Images++
			folder = r.folders[i]
		}
		if !heavyOn || folder.OptimizedAtBuild {
			continue
		}
		r.count(report.RuleQualityImageHeavy)
		limit, impact := veryHeavy, report.ImpactMedium
		switch {
		case f.Size > veryHeavy:
		case f.Size > heavy:
			limit, impact = heavy, report.ImpactLow
		default:
			continue
		}
		hint := "A large image makes the page slow to load, above all on a phone. Save it smaller (about 2000 pixels wide is plenty) or as WebP, and upload it again."
		if folder.Guessed {
			hint += " The folder is taken to be served as is, a guess from the site's framework: say `optimizedAtBuild: false` on its storage to confirm it, or `true` if the build resizes its images."
		} else {
			hint += " If the site's build resizes the images of this folder, say `optimizedAtBuild: true` on its storage."
		}
		// A guess that the folder is served as is weighs low, as a
		// guessed role does.
		r.emit(report.Finding{File: p, Rule: report.RuleQualityImageHeavy,
			Message: fmt.Sprintf("The image is %s, over %s.", humanBytes(f.Size), humanBytes(limit)),
			Hint:    hint,
			Value:   p, Measure: &report.Measure{Value: f.Size, Unit: report.UnitBytes, Max: &limit}}, lowered(impact, folder.Guessed))
	}
}

// imageLimits are the two image weights: the caller's Limits, else the
// schema's `quality.limits.imageKB`, else the defaults. The upper one
// scales with the lower as the defaults do (1 MB to 200 KB).
func (r *run) imageLimits() (heavy, veryHeavy int64) {
	heavy, veryHeavy = r.opts.Limits.ImageHeavyBytes, r.opts.Limits.ImageVeryHeavyBytes
	if heavy <= 0 {
		heavy = DefaultImageHeavyBytes
		if kb := r.cfg.Quality.Limits.ImageKB; kb > 0 {
			heavy = int64(kb) * 1024
		}
	}
	if veryHeavy <= 0 {
		veryHeavy = heavy * DefaultImageVeryHeavyBytes / DefaultImageHeavyBytes
	}
	return heavy, max(heavy, veryHeavy)
}

// usedFiles are the media files something refers to: any value of a page
// (declared or not, list items too), a link or image in any text, and
// any value in the schema (a default image). Only paths under the roots
// are kept.
func (r *run) usedFiles(pages []Page, roots []string) map[string]bool {
	used := map[string]bool{}
	add := func(v string) {
		for _, cand := range candidates(v) {
			p := resolve.AssetRepoPath(r.cfg, cand)
			if p != "" && underAny(p, roots) {
				used[p] = true
			}
		}
	}
	var visit func(v any, depth int)
	visit = func(v any, depth int) {
		if depth > schema.MaxWalkDepth {
			return
		}
		switch x := v.(type) {
		case string:
			add(x)
		case []any:
			for _, item := range x {
				visit(item, depth+1)
			}
		case map[string]any:
			for _, item := range x {
				visit(item, depth+1)
			}
		}
	}
	for _, p := range pages {
		visit(p.Frontmatter, 0)
		add(p.Body)
	}
	visit(r.cfg.Raw, 0)
	return used
}

// candidates are the values in a string that may name a file: the string
// itself when it is one address, and every link and image in it when it
// is text. Percent-encoded names are tried decoded too.
func candidates(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	if !strings.ContainsAny(s, " \t\n") && resolve.IsAssetValue(s) {
		out = append(out, s)
	}
	if len(s) > 1 {
		out = append(out, resolve.InternalLinks(s)...)
	}
	for _, c := range out {
		if dec, err := url.PathUnescape(c); err == nil && dec != c {
			out = append(out, dec)
		}
	}
	return out
}

func underAny(p string, roots []string) bool {
	for _, root := range roots {
		if resolve.IsUnder(p, root) && p != root {
			return true
		}
	}
	return false
}

func humanBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%d KB", n/1024)
	}
	return fmt.Sprintf("%d bytes", n)
}
