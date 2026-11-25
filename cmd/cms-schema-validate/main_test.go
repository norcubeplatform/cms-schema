package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// --git on a shallow clone is an error (exit 2), never a run that finds
// every translation current; the full clone of the same site passes.
func TestGitOnAShallowClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	files := map[string]string{
		".norcube/cms/config.yml":            "preview:\n  defaultLocale: en\n  locales: [en, cs]\n",
		".norcube/cms/collections/pages.yml": "label: Pages\npath: pages\nmatch: '**/*.md'\npreviewPath: '/{locale}/{slug}/'\nlocaleFrom: path\ntranslations: {by: path}\nfields:\n  - {key: title, type: text, label: Title}\n",
		"pages/en/about.md":                  "---\ntitle: About\n---\n",
		"pages/cs/about.md":                  "---\ntitle: O nás\n---\n",
	}
	for p, body := range files {
		full := filepath.Join(repo, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "init", "-q")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "c")
	shallow := filepath.Join(t.TempDir(), "shallow")
	git(t, repo, "clone", "-q", "--depth", "1", "file://"+repo, shallow)

	if code := run([]string{"--quality", "--git", "--quiet", shallow}); code != 2 {
		t.Fatalf("shallow clone: exit %d, want 2", code)
	}
	if code := run([]string{"--quality", "--git", "--quiet", repo}); code != 0 {
		t.Fatalf("full repository: exit %d, want 0", code)
	}
}
