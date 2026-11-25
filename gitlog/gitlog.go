// Package gitlog reads when each file of a git checkout last changed,
// for quality.Options.LastChanged. It is the one package of the module
// that runs a program (git); the checking packages never import it and
// stay free of git and the network, so a caller without a checkout
// passes its own dates.
package gitlog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Bounds of one read.
const (
	// DefaultMaxCommits is how many commits, newest first, are read when
	// Options.MaxCommits is zero. A file last changed before them is
	// undated and never counted as outdated.
	DefaultMaxCommits = 5000
	// MaxOutputBytes bounds what one git command may print.
	MaxOutputBytes = 64 << 20
	// DefaultTimeout is the deadline LastChanged sets when ctx has none.
	DefaultTimeout = 30 * time.Second
)

// Options tune LastChanged.
type Options struct {
	// MaxCommits bounds the history read; zero means DefaultMaxCommits.
	MaxCommits int
	// Now is the time of a file changed in the working tree and not
	// committed; zero means time.Now().
	Now time.Time
	// Git is the git binary; empty means "git" on the PATH.
	Git string
}

// ErrNotARepository is returned for a directory git does not track.
var ErrNotARepository = errors.New("not a git checkout")

// ErrShallowHistory is returned for a shallow clone (`git clone
// --depth`, the default of most CI checkouts). Its oldest commit lists
// every file as added, so the history would make every file look last
// changed there; it is refused rather than read. Fetch the full history
// (`git fetch --unshallow`, `fetch-depth: 0` in actions/checkout).
var ErrShallowHistory = errors.New("shallow clone: the git history is incomplete")

// checkRepository fails with ErrNotARepository when git does not track
// dir and with ErrShallowHistory when the clone is shallow.
func checkRepository(ctx context.Context, git, dir string) error {
	if _, err := run(ctx, git, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("%w: %s", ErrNotARepository, dir)
	}
	out, err := run(ctx, git, dir, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	switch strings.TrimSpace(string(out)) {
	case "false":
		return nil
	case "true":
		return fmt.Errorf("%w: %s", ErrShallowHistory, dir)
	default:
		// A git too old to answer (before 2.15) can't rule it out.
		return fmt.Errorf("%w: %s: git can't tell whether the clone is shallow", ErrShallowHistory, dir)
	}
}

// LastChanged maps each file under dir, as a path relative to dir with
// forward slashes, to when it last changed: the commit time of the
// newest of the last MaxCommits commits that touched it, or Now for a
// file changed in the working tree (modified, staged or untracked). It
// reads at most MaxOutputBytes per command and stops at ctx's deadline
// (DefaultTimeout when ctx has none); on any error it returns no dates.
// A shallow clone is ErrShallowHistory.
func LastChanged(ctx context.Context, dir string, opts Options) (map[string]time.Time, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	if opts.MaxCommits <= 0 {
		opts.MaxCommits = DefaultMaxCommits
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.Git == "" {
		opts.Git = "git"
	}
	if err := checkRepository(ctx, opts.Git, dir); err != nil {
		return nil, err
	}
	log, err := run(ctx, opts.Git, dir, "log", "--relative", "--no-renames", "--name-only",
		"--format=%x1e%ct", "-n", strconv.Itoa(opts.MaxCommits), "--", ".")
	if err != nil {
		return nil, err
	}
	out := ParseLog(log)
	dirty, err := run(ctx, opts.Git, dir, "diff", "--name-only", "--relative", "HEAD", "--", ".")
	if err != nil {
		// A repository without a commit has no HEAD to compare with.
		dirty = nil
	}
	untracked, err := run(ctx, opts.Git, dir, "ls-files", "--others", "--exclude-standard", "--", ".")
	if err != nil {
		return nil, err
	}
	for _, b := range [][]byte{dirty, untracked} {
		for _, line := range strings.Split(string(b), "\n") {
			if p := strings.TrimSpace(line); p != "" {
				out[p] = opts.Now
			}
		}
	}
	return out, nil
}

// ParseLog reads the output of `git log --name-only --format=%x1e%ct`:
// records separated by \x1e, each a Unix time and the files the commit
// touched, newest first. A file keeps the time of its first record.
func ParseLog(b []byte) map[string]time.Time {
	out := map[string]time.Time{}
	for _, rec := range bytes.Split(b, []byte{0x1e}) {
		sc := bufio.NewScanner(bytes.NewReader(rec))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		var at time.Time
		first := true
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if first {
				first = false
				secs, err := strconv.ParseInt(line, 10, 64)
				if err != nil {
					break
				}
				at = time.Unix(secs, 0).UTC()
				continue
			}
			if _, seen := out[line]; !seen {
				out[line] = at
			}
		}
	}
	return out
}

// run runs one git command in dir with quoting of names turned off, and
// returns what it printed, at most MaxOutputBytes.
func run(ctx context.Context, git, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, git, append([]string{"-c", "core.quotepath=off", "-C", dir}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	b, readErr := io.ReadAll(io.LimitReader(stdout, MaxOutputBytes+1))
	if len(b) > MaxOutputBytes {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("git %s printed more than %d bytes", args[0], MaxOutputBytes)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git %s: %w", args[0], ctx.Err())
		}
		return nil, fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	if readErr != nil {
		return nil, readErr
	}
	return b, nil
}
