package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/config"
)

const sentinelConfig = "# hand written\n[project]\nname = \"do-not-touch\"\n"

// TestWriteIfAbsentPreservesExistingConfig is spec §82's "Existing config
// sessizce overwrite edilmez." A second init must be safe to run on a
// repository someone has already configured.
func TestWriteIfAbsentPreservesExistingConfig(t *testing.T) {
	worktree := t.TempDir()
	configPath := filepath.Join(worktree, config.RepoDir, config.ConfigFileName)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte(sentinelConfig), 0o644); err != nil {
		t.Fatalf("writing sentinel: %v", err)
	}

	gotPath, written, err := config.WriteIfAbsent(worktree)
	if err != nil {
		t.Fatalf("WriteIfAbsent() error = %v, want nil", err)
	}
	if written {
		t.Error("written = true, want false: an existing config must be preserved")
	}
	if gotPath != configPath {
		t.Errorf("path = %q, want %q", gotPath, configPath)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(after) != sentinelConfig {
		t.Errorf("config changed:\n got %q\nwant %q", after, sentinelConfig)
	}
}

// TestWriteIfAbsentCreatesFromEmbeddedTemplate covers the other branch: a
// repository with no .mindrail/ at all gets the embedded scaffold verbatim.
func TestWriteIfAbsentCreatesFromEmbeddedTemplate(t *testing.T) {
	worktree := t.TempDir()

	path, written, err := config.WriteIfAbsent(worktree)
	if err != nil {
		t.Fatalf("WriteIfAbsent() error = %v, want nil", err)
	}
	if !written {
		t.Fatal("written = false, want true on a fresh repository")
	}
	want := filepath.Join(worktree, config.RepoDir, config.ConfigFileName)
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	if !bytes.Equal(got, config.DefaultTemplate()) {
		t.Errorf("created file is not the embedded template:\n got %q\nwant %q", got, config.DefaultTemplate())
	}

	// Running init twice is the realistic case, so the second call must be a
	// no-op rather than a rewrite of identical bytes.
	if _, secondWritten, secondErr := config.WriteIfAbsent(worktree); secondErr != nil || secondWritten {
		t.Errorf("second WriteIfAbsent() = (written %v, err %v), want (false, nil)", secondWritten, secondErr)
	}
}

// TestDefaultTemplateIsNotEmptyAndIsACopy protects the embedded bytes from a
// caller that edits what it was handed.
func TestDefaultTemplateIsNotEmptyAndIsACopy(t *testing.T) {
	first := config.DefaultTemplate()
	if len(first) == 0 {
		t.Fatal("DefaultTemplate() is empty")
	}

	original := first[0]
	first[0] = 'X'
	if second := config.DefaultTemplate(); second[0] != original {
		t.Fatalf("DefaultTemplate() returned shared storage: byte 0 = %q, want %q", second[0], original)
	}
}

// TestEnsureKnowledgeDirsCreatesGitkeep is decision D-05: Git tracks no empty
// directories, so MR-002 would find its write targets missing after a clone
// without the .gitkeep files.
func TestEnsureKnowledgeDirsCreatesGitkeep(t *testing.T) {
	worktree := t.TempDir()

	created, err := config.EnsureKnowledgeDirs(worktree)
	if err != nil {
		t.Fatalf("EnsureKnowledgeDirs() error = %v, want nil", err)
	}

	want := []string{
		".mindrail/knowledge/decisions",
		".mindrail/knowledge/invariants",
	}
	if !slices.Equal(created, want) {
		t.Fatalf("created = %v, want %v (repo-relative, forward slashes)", created, want)
	}

	for _, dir := range want {
		info, statErr := os.Stat(filepath.Join(worktree, filepath.FromSlash(dir)))
		if statErr != nil {
			t.Fatalf("stat %s: %v", dir, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}

		keep := filepath.Join(worktree, filepath.FromSlash(dir), ".gitkeep")
		if _, statErr := os.Stat(keep); statErr != nil {
			t.Fatalf("stat %s: %v", keep, statErr)
		}
	}
}

// TestEnsureKnowledgeDirsIsIdempotent keeps a second init quiet and
// non-destructive: nothing reported as created, nothing overwritten.
func TestEnsureKnowledgeDirsIsIdempotent(t *testing.T) {
	worktree := t.TempDir()

	if _, err := config.EnsureKnowledgeDirs(worktree); err != nil {
		t.Fatalf("first EnsureKnowledgeDirs() error = %v", err)
	}

	keep := filepath.Join(worktree, config.RepoDir, "knowledge", "decisions", ".gitkeep")
	if err := os.WriteFile(keep, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatalf("editing .gitkeep: %v", err)
	}

	created, err := config.EnsureKnowledgeDirs(worktree)
	if err != nil {
		t.Fatalf("second EnsureKnowledgeDirs() error = %v, want nil", err)
	}
	if len(created) != 0 {
		t.Errorf("created = %v, want empty on the second run", created)
	}

	got, err := os.ReadFile(keep)
	if err != nil {
		t.Fatalf("reading .gitkeep: %v", err)
	}
	if string(got) != "edited by hand\n" {
		t.Errorf(".gitkeep = %q, want it preserved", got)
	}
}

// TestScaffoldDoesNotCreateOutOfScopeFiles pins decision D-05's negative half:
// project.md and policies/ belong to later milestones, and shipping empty
// placeholders would advertise features that do not exist.
func TestScaffoldDoesNotCreateOutOfScopeFiles(t *testing.T) {
	worktree := t.TempDir()

	if _, _, err := config.WriteIfAbsent(worktree); err != nil {
		t.Fatalf("WriteIfAbsent() error = %v", err)
	}
	if _, err := config.EnsureKnowledgeDirs(worktree); err != nil {
		t.Fatalf("EnsureKnowledgeDirs() error = %v", err)
	}

	for _, unwanted := range []string{"project.md", "policies"} {
		if _, err := os.Stat(filepath.Join(worktree, config.RepoDir, unwanted)); err == nil {
			t.Errorf("%s was created; it belongs to a later milestone", unwanted)
		}
	}
}
