//go:build unix

package filesystem_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// TestUnwritablePathPrescribesTheRemedyThatClearsEachCondition is the sentence
// half of the classification. Naming the condition correctly is worth nothing
// while the words the user reads still send them to chmod a path whose mode is
// already right.
//
// The three conditions are asserted the same way in both directions: the remedy
// has to contain the one action that works, and must not contain either of the
// two that cannot. That symmetry is deliberate -- every finding this closes was a
// remedy that was merely *present*, not a remedy that was absent.
func TestUnwritablePathPrescribesTheRemedyThatClearsEachCondition(t *testing.T) {
	const (
		subject = "repository configuration file"
		target  = "/repo/.mindrail/config.toml"
	)

	cases := []struct {
		name string
		// The errno the kernel hands back for this condition.
		cause error
		// Substrings the remedy must contain, and must not.
		want    []string
		refused []string
	}{
		{
			name:    "a full filesystem",
			cause:   &os.PathError{Op: "write", Path: target, Err: syscall.ENOSPC},
			want:    []string{"free space"},
			refused: []string{"permission", "chmod", "remount"},
		},
		{
			name:    "a read-only mount",
			cause:   &os.PathError{Op: "write", Path: target, Err: syscall.EROFS},
			want:    []string{"remount", "read-write"},
			refused: []string{"permission", "chmod", "free space"},
		},
		{
			name:    "mode bits that refuse the write",
			cause:   &os.PathError{Op: "write", Path: target, Err: syscall.EACCES},
			want:    []string{"permission"},
			refused: []string{"free space", "remount"},
		},
		{
			// The path cannot be created because a directory above it is not
			// there. "Check the permissions" was the sentence for this too, over
			// a directory whose permissions were never consulted.
			name:    "a directory above the target is missing",
			cause:   &os.PathError{Op: "open", Path: target, Err: syscall.ENOENT},
			want:    []string{"create the missing directories"},
			refused: []string{"free space", "remount", "check the permissions"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := filesystem.UnwritablePath(filesystem.RootRepository, subject, target, tc.cause)

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("UnwritablePath = %v, which carries no domain payload", err)
			}
			if payload.Code != app.CodeRuntimePathUnwritable {
				t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
			}
			if app.ExitCode(err) != app.ExitUnavailable {
				t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
			}

			remedy := strings.Join(payload.NextAction, " ")
			for _, want := range tc.want {
				if !strings.Contains(remedy, want) {
					t.Errorf("NextAction = %q, want it to mention %q", payload.NextAction, want)
				}
			}
			for _, refused := range tc.refused {
				if strings.Contains(remedy, refused) {
					t.Errorf("NextAction = %q, which prescribes %q for %s; that cannot clear it",
						payload.NextAction, refused, tc.name)
				}
			}
		})
	}
}

// TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed is the regression
// guard on the two conditions that were already classified before this one
// existed. Routing every sentence through one classifier is only an improvement
// while the sentences that were already right stay right.
func TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed(t *testing.T) {
	dir := t.TempDir()
	occupied := filepath.Join(dir, ".mindrail")
	if err := os.WriteFile(occupied, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}

	t.Run("a file standing where a directory has to be", func(t *testing.T) {
		err := filesystem.UnwritablePath(filesystem.RootRepository, "", occupied, filesystem.ErrNotDirectory)

		payload, ok := app.PayloadOf(err)
		if !ok {
			t.Fatalf("UnwritablePath = %v, which carries no domain payload", err)
		}
		remedy := strings.Join(payload.NextAction, " ")
		if !strings.Contains(remedy, "remove or move aside") {
			t.Errorf("NextAction = %q, want it to say what to do with the entry in the way", payload.NextAction)
		}
		if strings.Contains(remedy, "free space") || strings.Contains(remedy, "remount") {
			t.Errorf("NextAction = %q; an obstruction is not a full disk and not a read-only mount",
				payload.NextAction)
		}
	})

	t.Run("a link that points at nothing", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		if err := os.Symlink(filepath.Join(dir, "absent"), link); err != nil {
			t.Fatalf("Symlink = %v, want no error", err)
		}

		err := filesystem.UnwritablePath(filesystem.RootRuntime, "", link, filesystem.ErrDanglingSymlink)

		payload, ok := app.PayloadOf(err)
		if !ok {
			t.Fatalf("UnwritablePath = %v, which carries no domain payload", err)
		}
		remedy := strings.Join(payload.NextAction, " ")
		if !strings.Contains(remedy, "remove or repoint the link") {
			t.Errorf("NextAction = %q, want the link remedy", payload.NextAction)
		}
	})
}

// TestProbeWritableLeavesAHealthyRepositoryAlone is the over-fire guard for the
// space reading probeDir now takes.
//
// Every test in this repository runs on a filesystem with room, so a reading
// that fired here would fail the suite loudly. Both shapes are covered: a
// directory that is already there, and one that is not yet -- which is the shape
// finding E1 is about, and the shape a first `mindrail init` always has.
func TestProbeWritableLeavesAHealthyRepositoryAlone(t *testing.T) {
	common := filepath.Join(t.TempDir(), "repo", ".git")
	worktree := filepath.Join(t.TempDir(), "repo")
	for _, dir := range []string{common, worktree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s = %v, want no error", dir, err)
		}
	}

	paths, err := filesystem.ResolveRuntimePaths(filesystem.PathOptions{
		CommonDir: common, WorktreeRoot: worktree,
	})
	if err != nil {
		t.Fatalf("ResolveRuntimePaths = %v, want no error", err)
	}

	t.Run("before anything has been created", func(t *testing.T) {
		for _, w := range paths.ProbeRoots() {
			if !w.Usable {
				t.Errorf("%s: ProbeRoots reported %+v, want usable; this filesystem has room", w.Kind, w)
			}
			if w.Exists {
				t.Errorf("%s: Exists = true before anything was created", w.Kind)
			}
		}
		if repo, known := paths.ProbeRepoConfig(); !known || !repo.Usable {
			t.Errorf("ProbeRepoConfig = %+v (known=%v), want usable", repo, known)
		}
	})

	t.Run("after the directories exist", func(t *testing.T) {
		if err := paths.EnsureDirs(); err != nil {
			t.Fatalf("EnsureDirs = %v, want no error", err)
		}
		for _, w := range paths.ProbeRoots() {
			if !w.Usable || !w.Exists {
				t.Errorf("%s: ProbeRoots reported %+v, want usable and existing", w.Kind, w)
			}
		}
	})
}
