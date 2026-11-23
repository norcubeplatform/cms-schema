package gitlog_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/norcubeplatform/cms-schema/gitlog"
)

func TestParseLog(t *testing.T) {
	log := "\x1e200\n\nsrc/a.md\nsrc/b.md\n\x1e100\n\nsrc/a.md\nsrc/c.md\n\x1enot a time\n\nsrc/d.md\n"
	got := gitlog.ParseLog([]byte(log))
	want := map[string]int64{"src/a.md": 200, "src/b.md": 200, "src/c.md": 100}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for p, secs := range want {
		if got[p].Unix() != secs {
			t.Errorf("%s: %v", p, got[p])
		}
	}
}

func git(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// Dates come from the newest commit that touched each file, relative to
// the directory asked about; a file changed and not committed is now.
func TestLastChanged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	site := filepath.Join(repo, "site")
	if err := os.MkdirAll(filepath.Join(site, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, nil, "init", "-q")
	commit := func(secs int64, files ...string) {
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(repo, f), []byte(f+strconv.FormatInt(secs, 10)), 0o644); err != nil {
				t.Fatal(err)
			}
			git(t, repo, nil, "add", f)
		}
		date := "@" + strconv.FormatInt(secs, 10) + " +0000"
		git(t, repo, []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}, "commit", "-q", "-m", "c")
	}
	commit(1000000000, "site/content/á.md", "site/content/b.md", "outside.md")
	commit(1000002000, "site/content/b.md")
	if err := os.WriteFile(filepath.Join(site, "content", "new.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(9000, 0)
	got, err := gitlog.LastChanged(context.Background(), site, gitlog.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"content/á.md": 1000000000, "content/b.md": 1000002000, "content/new.md": 9000}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for p, secs := range want {
		if got[p].Unix() != secs {
			t.Errorf("%s: %v, want %d", p, got[p], secs)
		}
	}
	if _, err := gitlog.LastChanged(context.Background(), t.TempDir(), gitlog.Options{}); !errors.Is(err, gitlog.ErrNotARepository) {
		t.Errorf("not a repository: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gitlog.LastChanged(ctx, site, gitlog.Options{}); err == nil {
		t.Error("a cancelled run gave dates")
	}
}

// The checking packages stay free of git and programs: only gitlog and
// the command import os/exec or gitlog.
func TestCheckingPackagesRunNoProgram(t *testing.T) {
	for _, dir := range []string{"content", "fact", "load", "quality", "report", "resolve", "schema", "validate"} {
		files, _ := filepath.Glob(filepath.Join("..", dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			ast, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range ast.Imports {
				if p := strings.Trim(imp.Path.Value, `"`); p == "os/exec" || strings.HasSuffix(p, "/gitlog") || p == "net/http" {
					t.Errorf("%s imports %s", f, p)
				}
			}
		}
	}
}
