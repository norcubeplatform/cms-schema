package gitlog

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/norcubeplatform/cms-schema/content"
	"github.com/norcubeplatform/cms-schema/quality"
)

// MaxFileBytes bounds one source file read for a base, from the working
// tree or from git. A larger one is not judged.
const MaxFileBytes = 4 << 20

// SourceBases reads, from the git checkout at dir, what the source of
// each language version said when the version was last current with it
// (quality.Options.SourceBases). Paths are relative to dir with forward
// slashes, as the validate package names files.
//
// The base of a version is the source's blob a mark names, else the
// source at the newest of the last MaxCommits commits that changed the
// version. A version changed in the working tree is current: its base is
// the source as it is now. A version git does not know (untracked, or
// older than the commits read) or a source that did not exist at the base
// is answered as unknown, so it is not judged.
//
// The history is read once, when the reader is first called; it reads
// at most MaxOutputBytes per command and stops at ctx's deadline
// (DefaultTimeout when ctx has none). The returned reader must be called
// before cancel. On a failure it returns an error and no answers:
// ErrNotARepository outside a checkout, ErrShallowHistory in a shallow
// clone, whose history can't tell when a version last changed.
func SourceBases(ctx context.Context, dir string, opts Options) (reader quality.BaseReader, cancel func()) {
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	if opts.MaxCommits <= 0 {
		opts.MaxCommits = DefaultMaxCommits
	}
	if opts.Git == "" {
		opts.Git = "git"
	}
	return func(queries []quality.BaseQuery) ([]quality.SourceBase, error) {
		return readBases(ctx, dir, opts, queries)
	}, cancel
}

func readBases(ctx context.Context, dir string, opts Options, queries []quality.BaseQuery) ([]quality.SourceBase, error) {
	if err := checkRepository(ctx, opts.Git, dir); err != nil {
		return nil, err
	}
	prefixOut, err := run(ctx, opts.Git, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(string(prefixOut))
	log, err := run(ctx, opts.Git, dir, "log", "--relative", "--no-renames", "--name-only",
		"--format=%x1e%H", "-n", strconv.Itoa(opts.MaxCommits), "--", ".")
	if err != nil {
		return nil, err
	}
	commits := ParseCommits(log)
	changed, err := workingTreeChanges(ctx, opts.Git, dir)
	if err != nil {
		return nil, err
	}

	out := make([]quality.SourceBase, len(queries))
	current := make([][]byte, len(queries))
	specs := make([]string, len(queries))
	for i, q := range queries {
		now, ok := readFile(dir, q.Source)
		if !ok {
			continue
		}
		current[i] = now
		switch {
		case q.SourceBlob != "":
			if content.BlobID(now) == q.SourceBlob {
				out[i] = quality.SourceBase{Known: true, Same: true}
				continue
			}
			specs[i] = q.SourceBlob
		case changed[q.Translation]:
			out[i] = quality.SourceBase{Known: true, Same: true}
		default:
			commit, ok := commits[q.Translation]
			if !ok {
				continue
			}
			specs[i] = commit + ":" + prefix + q.Source
		}
	}
	blobs, err := catFiles(ctx, opts.Git, dir, specs)
	if err != nil {
		return nil, err
	}
	for i, spec := range specs {
		if spec == "" {
			continue
		}
		base, ok := blobs[spec]
		if !ok {
			continue
		}
		if bytes.Equal(base, current[i]) {
			out[i] = quality.SourceBase{Known: true, Same: true}
			continue
		}
		out[i] = quality.SourceBase{Known: true, Content: base}
	}
	return out, nil
}

// ParseCommits reads the output of `git log --name-only --format=%x1e%H`:
// records separated by \x1e, each a commit id and the files the commit
// touched, newest first. A file keeps the commit of its first record.
func ParseCommits(b []byte) map[string]string {
	out := map[string]string{}
	for _, rec := range bytes.Split(b, []byte{0x1e}) {
		sc := bufio.NewScanner(bytes.NewReader(rec))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		commit := ""
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if commit == "" {
				if !isCommitID(line) {
					break
				}
				commit = line
				continue
			}
			if _, seen := out[line]; !seen {
				out[line] = commit
			}
		}
	}
	return out
}

func isCommitID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}

// workingTreeChanges are the files under dir changed in the working tree
// and not committed: modified, staged or untracked.
func workingTreeChanges(ctx context.Context, git, dir string) (map[string]bool, error) {
	out := map[string]bool{}
	dirty, err := run(ctx, git, dir, "diff", "--name-only", "--relative", "HEAD", "--", ".")
	if err != nil {
		// A repository without a commit has no HEAD to compare with.
		dirty = nil
	}
	untracked, err := run(ctx, git, dir, "ls-files", "--others", "--exclude-standard", "--", ".")
	if err != nil {
		return nil, err
	}
	for _, b := range [][]byte{dirty, untracked} {
		for _, line := range strings.Split(string(b), "\n") {
			if p := strings.TrimSpace(line); p != "" {
				out[p] = true
			}
		}
	}
	return out, nil
}

// readFile reads a file of the working tree, at most MaxFileBytes; false
// when it can't be read or is larger.
func readFile(dir, rel string) ([]byte, bool) {
	if rel == "" || strings.Contains(rel, "..") {
		return nil, false
	}
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil || len(b) > MaxFileBytes {
		return nil, false
	}
	return b, true
}

// catFiles reads the objects the specs name (a blob id, or
// `<commit>:<path>`) with one `git cat-file --batch`. A spec git can't
// find, or whose object is not a blob or is larger than MaxFileBytes, is
// left out. Empty specs are skipped.
func catFiles(ctx context.Context, git, dir string, specs []string) (map[string][]byte, error) {
	var want []string
	seen := map[string]bool{}
	for _, s := range specs {
		if s != "" && !seen[s] && !strings.ContainsAny(s, "\n\r") {
			seen[s] = true
			want = append(want, s)
		}
	}
	out := map[string][]byte{}
	if len(want) == 0 {
		return out, nil
	}
	cmd := exec.CommandContext(ctx, git, "-c", "core.quotepath=off", "-C", dir, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(want, "\n") + "\n")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := bufio.NewReader(io.LimitReader(stdout, MaxOutputBytes+1))
	read := 0
	var readErr error
	for _, spec := range want {
		header, err := r.ReadString('\n')
		if err != nil {
			readErr = err
			break
		}
		read += len(header)
		fields := strings.Fields(header)
		// "<spec> missing" or "<spec> ambiguous".
		if len(fields) != 3 {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			readErr = fmt.Errorf("cat-file: bad header %q", strings.TrimSpace(header))
			break
		}
		if read+size > MaxOutputBytes {
			readErr = fmt.Errorf("git cat-file printed more than %d bytes", MaxOutputBytes)
			break
		}
		body := make([]byte, size+1) // the object and its newline
		if _, err := io.ReadFull(r, body); err != nil {
			readErr = err
			break
		}
		read += size + 1
		if fields[1] == "blob" && size <= MaxFileBytes {
			out[spec] = body[:size]
		}
	}
	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git cat-file: %w", ctx.Err())
		}
		return nil, fmt.Errorf("git cat-file: %w", readErr)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git cat-file: %w", ctx.Err())
		}
		return nil, fmt.Errorf("git cat-file: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
