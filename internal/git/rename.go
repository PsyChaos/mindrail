package git

import (
	"bytes"
	"context"
	"fmt"
)

// RenameEntry is one file move the index may corroborate a structural match
// with: the repository-relative old and new paths of a similarity-detected
// rename. Entries are signals, not verdicts — matching decides, Git merely
// corroborates (decision D-101).
type RenameEntry struct {
	OldPath string
	NewPath string
}

// DiffRenames reads the worktree's rename signal through git diff with
// similarity detection. It is best-effort corroboration by contract: any
// failure (no HEAD, no git, not a repository) yields an empty set and a nil
// error, and matching proceeds structurally. Callers join the relative paths
// against the worktree root themselves; this function never touches the
// filesystem beyond asking git.
func DiffRenames(ctx context.Context, runner CommandRunner, dir string) ([]RenameEntry, error) {
	if runner == nil || dir == "" {
		return nil, fmt.Errorf("git: rename signal needs a runner and a directory")
	}
	stdout, _, err := runner.Run(ctx, dir,
		"diff", "--no-color", "--name-status", "-z", "-M", "HEAD", "--", ".")
	if err != nil {
		return nil, nil
	}
	return parseRenameStatus(stdout), nil
}

// parseRenameStatus reads NUL-separated --name-status rows. An R entry is
// status, score, old, new — note this is the reverse of status-porcelain -z,
// which emits new-then-old. Every other entry is status plus one path and is
// skipped. Malformed tails are dropped rather than reported: a truncated diff
// is a missing signal, not an error.
func parseRenameStatus(stdout []byte) []RenameEntry {
	fields := bytes.Split(stdout, []byte{0})
	var entries []RenameEntry
	for i := 0; i < len(fields); i++ {
		status := string(fields[i])
		if len(status) < 2 || status[0] != 'R' {
			continue
		}
		if i+2 >= len(fields) {
			break
		}
		old, new := string(fields[i+1]), string(fields[i+2])
		i += 2
		if old == "" || new == "" || old == new {
			continue
		}
		entries = append(entries, RenameEntry{OldPath: old, NewPath: new})
	}
	return entries
}
