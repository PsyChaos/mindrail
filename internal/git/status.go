package git

import (
	"bytes"
	"context"
	"fmt"
)

// StatusEntry is one porcelain row: the index (X) and worktree (Y) status
// codes with the affected path, plus the source path for renames and copies.
// Paths arrive repository-relative; callers join them against the root.
type StatusEntry struct {
	X        byte
	Y        byte
	Path     string
	OrigPath string
}

// StatusEntries reads the worktree's change set through git status porcelain.
// Unlike DiffRenames corroboration, discovery is load-bearing: a git that
// cannot answer is an error, never an empty diff — an empty diff would
// certify a dirty tree clean. Unparseable rows fail the same way: porcelain
// has a fixed grammar, and guessing at it would invent or lose changes.
func StatusEntries(ctx context.Context, runner CommandRunner, dir string) ([]StatusEntry, error) {
	if runner == nil || dir == "" {
		return nil, fmt.Errorf("git: status discovery needs a runner and a directory")
	}
	stdout, stderr, err := runner.Run(ctx, dir,
		"status", "--porcelain=v1", "-z", "--untracked-files=normal", "--", ".")
	if err != nil {
		return nil, fmt.Errorf("git: status discovery failed: %w: %s", err, dish(stderr))
	}
	return parseStatusEntries(stdout)
}

// dish trims runner stderr for error wrapping: one line, no NULs, bounded.
func dish(stderr []byte) string {
	line, _, _ := bytes.Cut(stderr, []byte{'\n'})
	line = bytes.ReplaceAll(line, []byte{0}, []byte{' '})
	if len(line) > 160 {
		line = line[:160]
	}
	return string(line)
}

// parseStatusEntries reads NUL-separated porcelain v1 rows. Every row is two
// status bytes, a space, and a path; rename and copy rows carry the source
// path as a second NUL field, new path first. A row that does not fit the
// grammar fails the parse rather than joining the set half-read.
func parseStatusEntries(stdout []byte) ([]StatusEntry, error) {
	fields := bytes.Split(stdout, []byte{0})
	var entries []StatusEntry
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) == 0 {
			continue
		}
		if len(field) < 4 || field[2] != ' ' {
			return nil, fmt.Errorf("git: malformed status row %q", field)
		}
		entry := StatusEntry{X: field[0], Y: field[1], Path: string(field[3:])}
		if entry.Path == "" {
			return nil, fmt.Errorf("git: status row names no path")
		}
		if entry.X == 'R' || entry.Y == 'R' || entry.X == 'C' || entry.Y == 'C' {
			i++
			if i >= len(fields) || len(fields[i]) == 0 {
				return nil, fmt.Errorf("git: rename row %q names no source", entry.Path)
			}
			entry.OrigPath = string(fields[i])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
