package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRevParseStripsOnlyTheOutputTerminator is the unit-level regression test
// for finding H5. strings.TrimSpace was applied to every rev-parse answer,
// including the three that are path names, and a trailing space or tab is a
// legal byte in a POSIX path.
func TestRevParseStripsOnlyTheOutputTerminator(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   string
	}{
		{name: "ordinary path", stdout: "/srv/repo\n", want: "/srv/repo"},
		{name: "trailing space", stdout: "/srv/repo \n", want: "/srv/repo "},
		{name: "two trailing spaces", stdout: "/srv/repo  \n", want: "/srv/repo  "},
		{name: "trailing tab", stdout: "/srv/repo\t\n", want: "/srv/repo\t"},
		{name: "trailing carriage return", stdout: "/srv/repo\r\n", want: "/srv/repo\r"},
		{name: "leading space", stdout: " /srv/repo\n", want: " /srv/repo"},
		{name: "embedded newline", stdout: "/srv/re\npo\n", want: "/srv/re\npo"},
		{name: "no terminator at all", stdout: "/srv/repo", want: "/srv/repo"},
		{name: "empty output", stdout: "\n", want: ""},
		// Only one terminator is git's. A second newline is content, however
		// unlikely, and inventing a rule that eats it is the same mistake in a
		// smaller size.
		{name: "path ending in a newline", stdout: "/srv/repo\n\n", want: "/srv/repo\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimOutputTerminator(tt.stdout); got != tt.want {
				t.Fatalf("trimOutputTerminator(%q) = %q, want %q", tt.stdout, got, tt.want)
			}
		})
	}
}

// TestAbsolutePathKeepsTrailingWhitespace drives the same defect through the
// adapter, where the truncated value is the one `init` writes against.
func TestAbsolutePathKeepsTrailingWhitespace(t *testing.T) {
	const worktree = "/srv/repo ws "

	fake := &FakeRunner{
		Responses: map[string]FakeResponse{
			"rev-parse --is-inside-work-tree":                   {Stdout: "true\n"},
			"rev-parse --is-bare-repository":                    {Stdout: "false\n"},
			"rev-parse --path-format=absolute --git-common-dir": {Stdout: worktree + "/.git\n"},
			"rev-parse --path-format=absolute --git-dir":        {Stdout: worktree + "/.git\n"},
			"rev-parse --path-format=absolute --show-toplevel":  {Stdout: worktree + "\n"},
		},
	}

	repo, err := NewAdapter(fake).Resolve(context.Background(), worktree)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if repo.WorktreeRoot != worktree {
		t.Fatalf("WorktreeRoot = %q, want %q — the path was silently shortened", repo.WorktreeRoot, worktree)
	}
	if repo.CommonDir != worktree+"/.git" {
		t.Fatalf("CommonDir = %q, want %q", repo.CommonDir, worktree+"/.git")
	}
}

// TestBooleanAnswersStillTolerateWhitespace is the over-fire guard for the
// change above: the answers that are *not* paths must keep being read
// leniently, so a git that pads "true" is still understood.
func TestBooleanAnswersStillTolerateWhitespace(t *testing.T) {
	for _, out := range []string{"true", "true\n", " true \n", "true\r\n", "TRUE\n", "\ttrue\t\n"} {
		if !parseGitBool(out) {
			t.Errorf("parseGitBool(%q) = false, want true", out)
		}
	}
	for _, out := range []string{"false\n", "", "\n", "yes\n"} {
		if parseGitBool(out) {
			t.Errorf("parseGitBool(%q) = true, want false", out)
		}
	}
}

// TestResolveRealRepositoryWithTrailingSpaceInItsPath is finding H5 end to end
// against the real git binary.
//
// The consequence being guarded is not a cosmetic one. WorktreeRoot is where
// `init` creates `.mindrail/`, so a shortened value made it create the
// directory in a *different, newly created* directory beside the repository,
// print READY FOR TARGETED WORK and exit 0.
func TestResolveRealRepositoryWithTrailingSpaceInItsPath(t *testing.T) {
	repo := newRepoFixtureNamed(t, "repo ws ")

	got, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), repo)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if got.WorktreeRoot != repo {
		t.Fatalf("WorktreeRoot = %q, want the repository's own path %q", got.WorktreeRoot, repo)
	}
	// The shortened value was still a plausible path, which is why nothing
	// downstream noticed. Asserting the string alone would miss a future fix
	// that produced a *different* real directory, so identity is checked too.
	if !sameFile(t, got.WorktreeRoot, repo) {
		t.Fatalf("WorktreeRoot %q is not the same directory as %q", got.WorktreeRoot, repo)
	}
	if _, err := os.Stat(got.WorktreeRoot); err != nil {
		t.Fatalf("WorktreeRoot %q does not exist: %v", got.WorktreeRoot, err)
	}
	if !strings.HasSuffix(got.WorktreeRoot, " ") {
		t.Fatalf("WorktreeRoot = %q, want the trailing space git reported", got.WorktreeRoot)
	}
	if want := filepath.Join(repo, ".git"); !sameFile(t, got.CommonDir, want) {
		t.Fatalf("CommonDir = %q, want %q", got.CommonDir, want)
	}
}

// TestResolveRealRepositoryOverFireOnUnusualPathNames is the other half of the
// guard: the same code has to be exactly as correct on the ordinary name, and
// on the other characters a path is allowed to contain.
//
// A newline is included on purpose. rev-parse's answer for such a repository is
// two lines, so a fix that split the output on newlines instead of stripping
// one terminator would pass every other case here and fail this one.
func TestResolveRealRepositoryOverFireOnUnusualPathNames(t *testing.T) {
	names := []struct {
		name string
		dir  string
	}{
		{name: "ordinary name", dir: "repo"},
		{name: "leading space", dir: " repo"},
		{name: "interior space", dir: "my repo"},
		{name: "trailing tab", dir: "repo\t"},
		{name: "embedded newline", dir: "re\npo"},
		{name: "quote and backslash", dir: `re"p\o`},
		{name: "non-utf8 byte", dir: "re\xffpo"},
		{name: "unicode", dir: "dépôt-仓库"},
	}

	for _, tt := range names {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoFixtureNamed(t, tt.dir)

			got, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), repo)
			if err != nil {
				t.Fatalf("Resolve %q: %v", repo, err)
			}
			if got.WorktreeRoot != repo {
				t.Fatalf("WorktreeRoot = %q, want %q", got.WorktreeRoot, repo)
			}
			if !sameFile(t, got.WorktreeRoot, repo) {
				t.Fatalf("WorktreeRoot %q is not the same directory as %q", got.WorktreeRoot, repo)
			}
			if want := filepath.Join(repo, ".git"); !sameFile(t, got.CommonDir, want) {
				t.Fatalf("CommonDir = %q, want %q", got.CommonDir, want)
			}
			if got.IsBare || got.IsLinkedWorktree {
				t.Fatalf("Resolve %q = %+v, want an ordinary main worktree", repo, got)
			}
		})
	}
}

// newRepoFixtureNamed builds a real repository in a directory with the exact
// name given, rather than the sanitised one newRepoFixture uses.
func newRepoFixtureNamed(t *testing.T, name string) string {
	t.Helper()
	requireGit(t)
	isolateGitConfig(t)

	repo := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Skipf("this filesystem cannot hold a directory named %q: %v", name, err)
	}

	runGit(t, "", "init", "--quiet", "--initial-branch=main", repo)
	runGit(t, repo, "config", "user.email", "fixture@example.invalid")
	runGit(t, repo, "config", "user.name", "Mindrail Fixture")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "commit", "--quiet", "--allow-empty", "-m", "root commit")

	return repo
}
