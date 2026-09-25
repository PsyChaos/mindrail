package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// defaultBases is the documented fallback chain for `verify --ci` when no
// --base is given (decision D-223): the first rev that resolves wins. None
// resolving is a structured refusal naming every candidate tried, never an
// empty diff certifying a broken range clean.
var defaultBases = []string{"origin/main", "main", "master"}

// ResolveRev resolves one user-supplied rev to its commit SHA. The SHA is
// what every later range command uses, so a rev shaped like `foo:bar` or
// carrying shell metacharacters cannot steer a subsequent `show rev:path`
// anywhere unexpected — resolution either yields 40 hex or fails here.
func ResolveRev(ctx context.Context, runner CommandRunner, dir, rev string) (string, error) {
	if runner == nil || dir == "" || rev == "" {
		return "", fmt.Errorf("git: revision resolution needs a runner, a directory and a revision")
	}
	stdout, stderr, err := runner.Run(ctx, dir, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("git: unknown revision %q: %w: %s", rev, err, dish(stderr))
	}
	sha := string(bytes.TrimSpace(stdout))
	if len(sha) != 40 || strings.ContainsAny(sha, " \t\n") {
		return "", fmt.Errorf("git: revision %q resolved to unusable output %q", rev, sha)
	}
	return sha, nil
}

// DefaultBase resolves the first candidate of the documented chain. The
// tried list rides the error so the refusal names what was attempted.
func DefaultBase(ctx context.Context, runner CommandRunner, dir string) (string, error) {
	if runner == nil || dir == "" {
		return "", fmt.Errorf("git: default-base resolution needs a runner and a directory")
	}
	for _, candidate := range defaultBases {
		if sha, err := ResolveRev(ctx, runner, dir, candidate); err == nil {
			return sha, nil
		}
	}
	return "", fmt.Errorf("git: no default base resolves (tried %s): pass --base explicitly",
		strings.Join(defaultBases, ", "))
}

// MergeBase computes the fork point of two resolved SHAs. Unrelated
// histories have none; that is an error, never an empty range.
func MergeBase(ctx context.Context, runner CommandRunner, dir, baseSHA, headSHA string) (string, error) {
	if runner == nil || dir == "" || baseSHA == "" || headSHA == "" {
		return "", fmt.Errorf("git: merge-base needs a runner, a directory, a base and a head")
	}
	stdout, stderr, err := runner.Run(ctx, dir, "merge-base", baseSHA, headSHA)
	if err != nil {
		return "", fmt.Errorf("git: merge-base of %q and %q failed: %w: %s", baseSHA, headSHA, err, dish(stderr))
	}
	sha := string(bytes.TrimSpace(stdout))
	if len(sha) != 40 || strings.ContainsAny(sha, " \t\n") {
		return "", fmt.Errorf("git: merge-base answered unusable output %q", sha)
	}
	return sha, nil
}

// RangeEntries reads the committed range mergeBase..head: what CI judges,
// never the worktree desk (decision D-222). Format and failure mode match
// StagedEntries row for row — a git that cannot answer is an error, never
// an empty diff.
func RangeEntries(ctx context.Context, runner CommandRunner, dir, mergeBaseSHA, headSHA string) ([]StatusEntry, error) {
	if runner == nil || dir == "" || mergeBaseSHA == "" || headSHA == "" {
		return nil, fmt.Errorf("git: range discovery needs a runner, a directory, a base and a head")
	}
	stdout, stderr, err := runner.Run(ctx, dir,
		"diff", "--name-status", "-z", mergeBaseSHA, headSHA, "--", ".")
	if err != nil {
		return nil, fmt.Errorf("git: range discovery failed: %w: %s", err, dish(stderr))
	}
	return parseStagedEntries(stdout)
}

// ShowRev reads one path's bytes at one resolved commit SHA. Missing at
// that rev reads as absent; any other git failure is an error for the same
// reason ShowStaged documents: a git that cannot answer must not read as
// an empty file.
func ShowRev(ctx context.Context, runner CommandRunner, dir, sha, rel string) ([]byte, bool, error) {
	if runner == nil || dir == "" || sha == "" || rel == "" {
		return nil, false, fmt.Errorf("git: revision read needs a runner, a directory, a revision and a path")
	}
	stdout, stderr, err := runner.Run(ctx, dir, "show", sha+":"+rel)
	if err != nil {
		if bytes.Contains(stderr, []byte("does not exist")) || bytes.Contains(stderr, []byte("exists on disk, but not in")) ||
			bytes.Contains(stderr, []byte("invalid object name")) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("git: revision read failed: %w: %s", err, dish(stderr))
	}
	return stdout, true, nil
}
