package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// StagedEntries reads the index change set through git diff: what the next
// commit would record, never the worktree desk around it (decision D-216).
// Like worktree discovery, a git that cannot answer is an error, never an
// empty diff — an empty diff would certify a dirty index clean.
func StagedEntries(ctx context.Context, runner CommandRunner, dir string) ([]StatusEntry, error) {
	if runner == nil || dir == "" {
		return nil, fmt.Errorf("git: staged discovery needs a runner and a directory")
	}
	stdout, stderr, err := runner.Run(ctx, dir,
		"diff", "--cached", "--name-status", "-z", "--", ".")
	if err != nil {
		return nil, fmt.Errorf("git: staged discovery failed: %w: %s", err, dish(stderr))
	}
	return parseStagedEntries(stdout)
}

// parseStagedEntries reads NUL-separated name-status rows with -z: a lone
// status word, then the path — renames and copies carry the source path as
// a second field (old path first, new path second). Unparseable rows fail
// the parse rather than joining the set half-read.
func parseStagedEntries(stdout []byte) ([]StatusEntry, error) {
	fields := bytes.Split(stdout, []byte{0})
	var entries []StatusEntry
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) == 0 {
			continue
		}
		status := string(field)
		rename := strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C")
		want := 1
		if rename {
			want = 2
		}
		if i+want >= len(fields) {
			return nil, fmt.Errorf("git: staged row without path %q", status)
		}
		entry := StatusEntry{}
		if rename {
			old := fields[i+1]
			new := fields[i+2]
			if len(old) == 0 || len(new) == 0 {
				return nil, fmt.Errorf("git: rename row without paths %q", status)
			}
			entry.Path = string(new)
			entry.OrigPath = string(old)
			i += 2
		} else {
			path := fields[i+1]
			if len(path) == 0 {
				return nil, fmt.Errorf("git: staged row without path %q", status)
			}
			entry.Path = string(path)
			i++
		}
		switch {
		case status == "A":
			entry.X = 'A'
		case status == "M" || status == "T":
			entry.X = 'M'
		case status == "D":
			entry.X = 'D'
		case rename:
			entry.X = status[0]
		case status == "U":
			entry.X = 'M'
		default:
			return nil, fmt.Errorf("git: unknown staged status %q", status)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ShowStaged reads one path's index bytes: what the commit would record.
// Missing in the index reads as absent; any other git failure is an error
// (a git that cannot answer must not read as an empty file).
func ShowStaged(ctx context.Context, runner CommandRunner, dir, rel string) ([]byte, bool, error) {
	if runner == nil || dir == "" || rel == "" {
		return nil, false, fmt.Errorf("git: staged read needs a runner, a directory and a path")
	}
	stdout, stderr, err := runner.Run(ctx, dir, "show", ":"+rel)
	if err != nil {
		if bytes.Contains(stderr, []byte("does not exist")) || bytes.Contains(stderr, []byte("exists on disk, but not in")) ||
			bytes.Contains(stderr, []byte("bad revision")) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("git: staged read failed: %w: %s", err, dish(stderr))
	}
	return stdout, true, nil
}

// ShowHEAD reads one path's HEAD bytes: the committed truth the staged
// change is judged against. Missing in HEAD reads as absent; any other
// git failure is an error for the same reason.
func ShowHEAD(ctx context.Context, runner CommandRunner, dir, rel string) ([]byte, bool, error) {
	if runner == nil || dir == "" || rel == "" {
		return nil, false, fmt.Errorf("git: HEAD read needs a runner, a directory and a path")
	}
	stdout, stderr, err := runner.Run(ctx, dir, "show", "HEAD:"+rel)
	if err != nil {
		if bytes.Contains(stderr, []byte("does not exist")) || bytes.Contains(stderr, []byte("exists on disk, but not in")) ||
			bytes.Contains(stderr, []byte("bad revision")) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("git: HEAD read failed: %w: %s", err, dish(stderr))
	}
	return stdout, true, nil
}
