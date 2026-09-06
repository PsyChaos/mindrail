package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestNotARepositoryCarriesGitStderr is the first half of finding H7. Every
// sibling classification in this package puts git's own sentence in
// `git_stderr` metadata; this one folded it into the joined cause string alone,
// so a machine consumer had to regex a field nobody promised the shape of.
func TestNotARepositoryCarriesGitStderr(t *testing.T) {
	const stderr = "fatal: not a git repository (or any parent up to mount point /)"

	fake := &FakeRunner{Default: FakeResponse{Stderr: stderr + "\n", Err: exit128()}}

	_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/somewhere")

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if got := payload.Metadata["git_stderr"]; !strings.Contains(got, stderr) {
		t.Fatalf("git_stderr metadata = %q, want git's own message %q", got, stderr)
	}
	if !strings.Contains(payload.Cause, stderr) {
		t.Errorf("cause = %q, want git's message to survive there too", payload.Cause)
	}
	// This is still the genuinely-absent case, so the remedy is still right.
	if !mentionsGitInit(payload.NextAction) {
		t.Errorf("next_action = %q, want the `git init` remedy on the case that genuinely has no repository", payload.NextAction)
	}
}

// TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository is finding H7 against
// the real git binary.
//
// A checkout whose `.git` file points at a repository that has been deleted
// reports the same "not a git repository" as an empty directory, and used to
// get the same remedy. Followed literally, `git init` here succeeds: it
// overwrites the `.git` file with a new empty repository and destroys the only
// surviving record of where the real one was.
func TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository(t *testing.T) {
	requireGit(t)
	isolateGitConfig(t)

	tmp := t.TempDir()
	worktree := filepath.Join(tmp, "checkout")
	target := filepath.Join(tmp, "gone", ".git")
	nested := filepath.Join(worktree, "src", "deep")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create fixture worktree: %v", err)
	}
	writeGitFile(t, worktree, target)

	for _, startDir := range []string{worktree, nested} {
		t.Run(shortLabel(worktree, startDir), func(t *testing.T) {
			_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), startDir)
			if err == nil {
				t.Fatalf("Resolve accepted %q, whose .git file points nowhere", startDir)
			}

			if !errors.Is(err, ErrDanglingGitFile) {
				t.Fatalf("Resolve error = %v, want ErrDanglingGitFile", err)
			}
			if errors.Is(err, ErrOrphanedWorktree) {
				t.Fatalf("an ordinary checkout was diagnosed as a linked worktree: %v", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != app.CodeNotAGitRepository {
				t.Fatalf("code = %q, want %q (D-18: the code is the stable contract)", payload.Code, app.CodeNotAGitRepository)
			}
			if got := app.ExitCode(err); got != app.ExitUsage {
				t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
			}
			assertPayloadIsActionable(t, payload)

			if mentionsGitInit(payload.NextAction) {
				t.Errorf("next_action = %q would overwrite the .git file with a new empty repository", payload.NextAction)
			}
			// The whole content of this failure is which pointer is broken and
			// where it points, so both have to be recoverable.
			if payload.Metadata["git_file"] != filepath.Join(worktree, ".git") {
				t.Errorf("git_file metadata = %q, want %q", payload.Metadata["git_file"], filepath.Join(worktree, ".git"))
			}
			if payload.Metadata["git_file_target"] != target {
				t.Errorf("git_file_target metadata = %q, want %q", payload.Metadata["git_file_target"], target)
			}
			if payload.Metadata["start_dir"] != startDir {
				t.Errorf("start_dir metadata = %q, want %q", payload.Metadata["start_dir"], startDir)
			}
			if !strings.Contains(payload.Metadata["git_stderr"], "not a git repository") {
				t.Errorf("git_stderr metadata = %q, want git's own message", payload.Metadata["git_stderr"])
			}
		})
	}
}

// TestOrphanedLinkedWorktreeIsNotDiagnosedAsAnAbsentRepository is finding H8.
//
// A linked worktree is administered from the main repository, so when that
// repository goes the worktree keeps every file and loses every way to reach
// one. The remedy is taken in the main repository, not in this directory, and
// `git init` here is wrong for H7's reason and one step further.
func TestOrphanedLinkedWorktreeIsNotDiagnosedAsAnAbsentRepository(t *testing.T) {
	repo := newRepoFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", linked)

	commonDir := filepath.Join(repo, ".git")
	if err := os.RemoveAll(repo); err != nil {
		t.Fatalf("remove the repository the worktree belongs to: %v", err)
	}

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), linked)
	if err == nil {
		t.Fatalf("Resolve accepted %q, whose repository is gone", linked)
	}

	if !errors.Is(err, ErrOrphanedWorktree) {
		t.Fatalf("Resolve error = %v, want ErrOrphanedWorktree", err)
	}
	if errors.Is(err, ErrDanglingGitFile) {
		t.Fatalf("a linked worktree was diagnosed as an ordinary checkout: %v", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeNotAGitRepository {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
	}
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
	}
	assertPayloadIsActionable(t, payload)

	if mentionsGitInit(payload.NextAction) {
		t.Errorf("next_action = %q advises creating a repository over a linked worktree", payload.NextAction)
	}
	if payload.Metadata["common_dir"] != commonDir {
		t.Errorf("common_dir metadata = %q, want %q", payload.Metadata["common_dir"], commonDir)
	}
	// The reader cannot act without knowing which repository to act in.
	if !strings.Contains(strings.Join(payload.NextAction, " "), commonDir) {
		t.Errorf("next_action = %q does not name the repository %q the remedy is taken in", payload.NextAction, commonDir)
	}
	if !strings.Contains(payload.Metadata["git_stderr"], "not a git repository") {
		t.Errorf("git_stderr metadata = %q, want git's own message", payload.Metadata["git_stderr"])
	}
}

// TestOrphanedLinkedWorktreeWithASurvivingRepository is the neighbouring shape:
// the administrative directory is gone but the repository is still there, so the
// remedy is taken in that repository rather than by restoring one that has not
// moved. It is here to prove the two are told apart, not merged.
//
// It used to assert that the remedy said `git worktree prune`, and that
// assertion was wrong: prune only removes administrative records for worktrees
// that have gone missing, so against a record that is already gone it prints
// nothing and changes nothing, and the `git worktree add` that followed it
// failed with "fatal: '<path>' already exists". A test that pins a remedy nobody
// can carry out is worse than no test, so it now pins the one that was measured
// to work — and pins the absence of the one that does not.
func TestOrphanedLinkedWorktreeWithASurvivingRepository(t *testing.T) {
	repo := newRepoFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", linked)

	if err := os.RemoveAll(filepath.Join(repo, ".git", "worktrees")); err != nil {
		t.Fatalf("remove the worktree administrative directory: %v", err)
	}

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), linked)
	if !errors.Is(err, ErrOrphanedWorktree) {
		t.Fatalf("Resolve error = %v, want ErrOrphanedWorktree", err)
	}

	payload, _ := app.PayloadOf(err)
	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "git worktree add "+linked) {
		t.Errorf("next_action = %q, want the worktree re-created in the repository that is still there",
			payload.NextAction)
	}
	// `git worktree add` refuses a path that exists, and this one does: the
	// reader is standing in it. A remedy that does not say to empty it first is
	// one that fails on its own first line.
	if !strings.Contains(remedy, "move "+linked+" aside") {
		t.Errorf("next_action = %q does not say to move %s aside; `git worktree add` refuses an existing directory",
			payload.NextAction, linked)
	}
	if strings.Contains(remedy, "run `git worktree prune`") {
		t.Errorf("next_action = %q prescribes a prune that cannot restore a record that is already gone",
			payload.NextAction)
	}
	if strings.Contains(remedy, "restore the repository") {
		t.Errorf("next_action = %q tells the reader to restore a repository that has not moved", payload.NextAction)
	}
	if mentionsGitInit(payload.NextAction) {
		t.Errorf("next_action = %q advises `git init` inside a live repository's worktree", payload.NextAction)
	}
}

// TestBrokenGitFileDetectionDoesNotOverFire is the guard for the two new
// detections. Each subtest is a condition that is adjacent to them and must not
// be swept up.
func TestBrokenGitFileDetectionDoesNotOverFire(t *testing.T) {
	t.Run("a healthy repository resolves untouched", func(t *testing.T) {
		repo := newRepoFixture(t)
		nested := filepath.Join(repo, "a", "b")
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatalf("create nested directory: %v", err)
		}

		adapter := NewAdapter(NewExecRunner())
		for _, dir := range []string{repo, nested} {
			if _, err := adapter.Resolve(fixtureContext(t), dir); err != nil {
				t.Fatalf("Resolve %q: %v", dir, err)
			}
		}
	})

	t.Run("a healthy linked worktree resolves untouched", func(t *testing.T) {
		repo := newRepoFixture(t)
		linked := filepath.Join(filepath.Dir(repo), "linked")
		runGit(t, repo, "worktree", "add", "--detach", linked)

		got, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), linked)
		if err != nil {
			t.Fatalf("Resolve the linked worktree: %v", err)
		}
		if !got.IsLinkedWorktree {
			t.Fatalf("IsLinkedWorktree = false for %q", linked)
		}
		// Its `.git` is a file pointing into worktrees/, which is exactly the
		// shape the orphan check looks at. The difference is that it resolves.
		if fault := findDanglingGitFile(linked); fault != nil {
			t.Fatalf("findDanglingGitFile(%q) = %+v, want nil for a working worktree", linked, fault)
		}
	})

	t.Run("a genuinely absent repository keeps its own diagnosis", func(t *testing.T) {
		requireGit(t)
		isolateGitConfig(t)
		outside := t.TempDir()
		requireOutsideAnyRepository(t, outside)

		_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), outside)
		if err == nil {
			t.Fatalf("Resolve accepted a directory outside every repository")
		}
		if !errors.Is(err, ErrNotARepository) {
			t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
		}
		if errors.Is(err, ErrDanglingGitFile) || errors.Is(err, ErrOrphanedWorktree) {
			t.Fatalf("an empty directory was diagnosed as a broken checkout: %v", err)
		}

		payload, _ := app.PayloadOf(err)
		if !mentionsGitInit(payload.NextAction) {
			t.Errorf("next_action = %q, want the `git init` remedy back on the one case it is right for", payload.NextAction)
		}
	})

	t.Run("a .git file whose target exists is not called dangling", func(t *testing.T) {
		requireGit(t)
		isolateGitConfig(t)

		tmp := t.TempDir()
		worktree := filepath.Join(tmp, "checkout")
		target := filepath.Join(tmp, "target")
		if err := os.MkdirAll(worktree, 0o700); err != nil {
			t.Fatalf("create fixture worktree: %v", err)
		}
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatalf("create fixture target: %v", err)
		}
		writeGitFile(t, worktree, target)

		if fault := findDanglingGitFile(worktree); fault != nil {
			t.Fatalf("findDanglingGitFile(%q) = %+v, want nil: the target is on disk", worktree, fault)
		}

		_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), worktree)
		if errors.Is(err, ErrDanglingGitFile) || errors.Is(err, ErrOrphanedWorktree) {
			t.Fatalf("a .git file pointing at a directory that exists was reported as broken: %v", err)
		}
	})

	t.Run("a damaged repository is still damaged, not broken-pointer", func(t *testing.T) {
		repo := newRepoFixture(t)
		config := filepath.Join(repo, ".git", "config")
		if err := os.WriteFile(config, []byte("[core\n"), 0o600); err != nil {
			t.Fatalf("damage the repository config: %v", err)
		}

		_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), repo)
		if !errors.Is(err, ErrRepositoryUnreadable) {
			t.Fatalf("Resolve error = %v, want ErrRepositoryUnreadable", err)
		}
		if errors.Is(err, ErrDanglingGitFile) || errors.Is(err, ErrOrphanedWorktree) {
			t.Fatalf("an unparseable config was reported as a broken .git pointer: %v", err)
		}
	})

	t.Run("git's bracket phrasings never reach the pointer classification", func(t *testing.T) {
		// The message discriminator, isolated. git prints a bracket for "I
		// walked up and found nothing" and a colon for "a pointer failed to
		// resolve"; only the second may reach a filesystem walk.
		absent := []string{
			"fatal: not a git repository (or any parent up to mount point /)",
			"fatal: not a git repository (or any of the parent directories): .git",
		}
		for _, stderr := range absent {
			if gitReportedBrokenIndirection(stderr) {
				t.Errorf("gitReportedBrokenIndirection(%q) = true, want false", stderr)
			}
			if !gitReportedNoRepository(stderr) {
				t.Errorf("gitReportedNoRepository(%q) = false, want true", stderr)
			}
		}
		for _, stderr := range []string{
			"fatal: not a git repository: (null)",
			"fatal: not a git repository: '/srv/gone/.git'",
		} {
			if !gitReportedBrokenIndirection(stderr) {
				t.Errorf("gitReportedBrokenIndirection(%q) = false, want true", stderr)
			}
		}
	})

	t.Run("the pointer classification needs both signals", func(t *testing.T) {
		// The filesystem walk alone must not decide. With git reporting the
		// bracket phrasing — which is what it prints on the far side of a mount
		// point it refused to cross — the walk's answer is not consulted at
		// all, so a `.git` file git never looked at cannot be blamed.
		tmp := t.TempDir()
		writeGitFile(t, tmp, filepath.Join(tmp, "gone"))

		if fault := findDanglingGitFile(tmp); fault == nil {
			t.Fatalf("the fixture is not dangling, so this proves nothing")
		}
		if fault := brokenGitFileFault(tmp, "fatal: not a git repository (or any parent up to mount point /)"); fault != nil {
			t.Fatalf("brokenGitFileFault fired on git's absent-repository phrasing: %+v", fault)
		}
		if fault := brokenGitFileFault(tmp, "fatal: not a git repository: (null)"); fault == nil {
			t.Fatalf("brokenGitFileFault did not fire when both signals agree")
		}
	})
}

// TestTheWalkStopsAtAGitEntryItCannotFollow pins the contract the Lstat in
// inspectGitEntry exists for, which nothing exercised (finding F17).
//
// The walk's rule is "stop at the first `.git` entry of any kind", and a `.git`
// symlink whose target has been deleted is an entry. Reading it with Stat
// instead makes it look absent, and the walk carries on into the parent — where
// it finds a different repository's `.git` file and blames a directory the
// reader was never in.
//
// The shape is unreachable from brokenGitFileFault today, because git answers a
// dangling `.git` symlink with the bracket phrasing and the walk is never
// consulted. That is a fact about one caller, not about this function: its
// contract is stated in its own comment, and a contract no test can fail is not
// one.
func TestTheWalkStopsAtAGitEntryItCannotFollow(t *testing.T) {
	parent := t.TempDir()
	// The parent holds a genuinely dangling `.git` file, so a walk that does not
	// stop below has something wrong to find and report.
	writeGitFile(t, parent, filepath.Join(parent, "gone"))

	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatalf("create %q: %v", child, err)
	}
	if err := os.Symlink(filepath.Join(parent, "deleted"), filepath.Join(child, ".git")); err != nil {
		t.Skipf("this filesystem does not support symlinks: %v", err)
	}

	if fault := findDanglingGitFile(parent); fault == nil {
		t.Fatal("the parent fixture is not dangling, so this proves nothing")
	}
	if fault := findDanglingGitFile(child); fault != nil {
		t.Fatalf("findDanglingGitFile(%q) walked past a `.git` symlink it could not follow "+
			"and blamed %q, which is a directory the reader is not in", child, fault.GitFile)
	}
}

// TestReadGitFileMirrorsGit pins the parse against git's own
// read_gitfile_gently: the prefix is fixed, trailing newlines and spaces are
// removed, and a relative target is resolved against the file's directory.
// Looking at a different path than git looked at is the one way this check gets
// the answer wrong.
func TestReadGitFileMirrorsGit(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{name: "absolute target", content: "gitdir: /srv/repo/.git\n", want: "/srv/repo/.git", wantOK: true},
		{name: "no trailing newline", content: "gitdir: /srv/repo/.git", want: "/srv/repo/.git", wantOK: true},
		{name: "trailing spaces are git's to strip", content: "gitdir: /srv/repo/.git  \n", want: "/srv/repo/.git", wantOK: true},
		// The three rows that tell this parse apart from strings.TrimSpace and
		// from the narrower "\n " it used to trim (finding F17). git strips
		// every trailing isspace byte and keeps the leading ones, so a tab or a
		// carriage return on the end has to go and a space after the prefix has
		// to stay. A parse that agreed with git on none of these still passed
		// the whole suite.
		{name: "a trailing tab is whitespace to git", content: "gitdir: /srv/repo/.git\t\n", want: "/srv/repo/.git", wantOK: true},
		{name: "CRLF, which an editor on Windows writes", content: "gitdir: /srv/repo/.git\r\n", want: "/srv/repo/.git", wantOK: true},
		// git keeps the extra space, so the target is no longer absolute and is
		// resolved against the file's directory — which is what git does with it
		// too. TrimSpace would eat it and look at /srv/repo/.git instead.
		{name: "leading space is part of the path git looks for", content: "gitdir:  /srv/repo/.git\n", want: filepath.Join(dir, " /srv/repo/.git"), wantOK: true},
		{name: "relative target", content: "gitdir: ../elsewhere/.git\n", want: filepath.Join(filepath.Dir(dir), "elsewhere", ".git"), wantOK: true},
		{name: "wrong prefix", content: "gitdirx: /srv/repo/.git\n", wantOK: false},
		{name: "not the format at all", content: "ref: refs/heads/main\n", wantOK: false},
		{name: "empty target", content: "gitdir: \n", wantOK: false},
		{name: "empty file", content: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".git")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			got, ok := readGitFile(path, dir)
			if ok != tt.wantOK {
				t.Fatalf("readGitFile(%q) ok = %v, want %v", tt.content, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("readGitFile(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestLinkedWorktreeCommonDir(t *testing.T) {
	tests := []struct {
		target string
		want   string
		wantOK bool
	}{
		{target: "/srv/repo/.git/worktrees/feature", want: "/srv/repo/.git", wantOK: true},
		{target: "/srv/bare.git/worktrees/feature", want: "/srv/bare.git", wantOK: true},
		// An ordinary repository's .git is not a linked worktree's directory,
		// and neither is a path that merely contains the word somewhere else.
		{target: "/srv/repo/.git", wantOK: false},
		{target: "/srv/worktrees", wantOK: false},
		{target: "/srv/worktrees-backup/thing", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, ok := linkedWorktreeCommonDir(tt.target)
			if ok != tt.wantOK {
				t.Fatalf("linkedWorktreeCommonDir(%q) ok = %v, want %v", tt.target, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("linkedWorktreeCommonDir(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func writeGitFile(t *testing.T, dir, target string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+target+"\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
}

func shortLabel(root, dir string) string {
	if rel, err := filepath.Rel(root, dir); err == nil && rel != "." {
		return "from " + rel
	}
	return "from the worktree root"
}
