package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// invalidUTF8Byte is a continuation byte with no lead byte, which no UTF-8
// decoder accepts and every Unix filesystem stores without complaint.
const invalidUTF8Byte = "\xff"

// newRepoWithUnrepresentablePath builds a repository whose absolute path holds a
// byte JSON cannot carry.
func newRepoWithUnrepresentablePath(t *testing.T) string {
	t.Helper()
	requireGit(t)
	isolateEnvironment(t)

	repo := filepath.Join(t.TempDir(), "re"+invalidUTF8Byte+"po")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Skipf("this filesystem will not hold a path with invalid UTF-8: %v", err)
	}
	if out, err := exec.Command("git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return repo
}

// TestInitRefusesAnUnrepresentableRepositoryBeforeWriting is the finding that
// `mindrail init --json` did the entire job and then reported that it had
// failed.
//
// A repository path is a sequence of bytes and nothing requires them to be valid
// UTF-8. JSON strings are Unicode, so such a path cannot be published unchanged,
// and MR-001 refuses rather than emitting a mangled one (finding W9). The
// refusal used to happen at serialisation: by the time it fired, the scaffold,
// the runtime database, the schema and the workspace row had all been written,
// and the command exited 2 with ok:false over a repository it had just
// initialised completely. A caller that retries on failure re-runs a finished
// initialisation; a caller that trusts the exit code believes the repository is
// untouched.
func TestInitRefusesAnUnrepresentableRepositoryBeforeWriting(t *testing.T) {
	repo := newRepoWithUnrepresentablePath(t)

	got := run(t, repo, "init", "--json")
	got.requireExit(t, app.ExitUsage)

	payload := got.errorPayload(t)
	if payload.Code != app.CodePathNotRepresentable {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodePathNotRepresentable)
	}

	// The part that was wrong: not the refusal, but everything that happened
	// before it.
	for _, path := range []string{
		filepath.Join(repo, ".mindrail"),
		filepath.Join(repo, ".git", "mindrail"),
	} {
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("init created %s and then reported that it had failed", path)
		}
	}
}

// TestTheUnrepresentablePathRemediesBothWork carries out each of the two actions
// the refusal prints, because a refusal whose remedies do not work is a
// dead end with better wording.
func TestTheUnrepresentablePathRemediesBothWork(t *testing.T) {
	t.Run("run this command without --json", func(t *testing.T) {
		repo := newRepoWithUnrepresentablePath(t)

		if got := run(t, repo, "init"); got.code != app.ExitSuccess {
			t.Fatalf("init exited %d: %v\n%s", got.code, got.err, got.stdout)
		}
		if _, err := os.Lstat(filepath.Join(repo, ".mindrail")); err != nil {
			t.Errorf("the human run reported success and wrote no scaffold: %v", err)
		}
		if got := run(t, repo, "status"); got.code != app.ExitSuccess {
			t.Errorf("status exited %d after the remedy: %v\n%s", got.code, got.err, got.stdout)
		}
	})

	t.Run("rename the offending path", func(t *testing.T) {
		repo := newRepoWithUnrepresentablePath(t)

		// The remedy has to be read off the refusal it is carrying out, not
		// assumed: a path wrongly reported as an ordinary stored value (finding
		// F30's mistake, in the other direction) would print a different second
		// next_action, and a test that renamed anyway would not notice.
		got := run(t, repo, "init", "--json")
		got.requireExit(t, app.ExitUsage)

		payload := got.errorPayload(t)
		if payload.Code != app.CodePathNotRepresentable {
			t.Fatalf("code = %q, want %q", payload.Code, app.CodePathNotRepresentable)
		}
		if !strings.Contains(payload.Why, "in this report") {
			t.Fatalf("why = %q is not the location form's message", payload.Why)
		}
		if len(payload.NextAction) < 2 || !strings.Contains(payload.NextAction[1], "rename") {
			t.Fatalf("next_action = %v does not tell the reader to rename anything", payload.NextAction)
		}

		renamed := filepath.Join(filepath.Dir(repo), "repo")
		if err := os.Rename(repo, renamed); err != nil {
			t.Fatalf("carry out the remedy: %v", err)
		}

		got = run(t, renamed, "init", "--json")
		got.requireExit(t, app.ExitSuccess)
		assertNoErrorEnvelope(t, "init", got.stdout)
	})
}

// TestARepresentableRepositoryIsUnaffectedByThePreflight is the over-fire guard.
//
// The preflight runs a whole read-only startup ahead of every `init --json`, and
// on a repository with an ordinary path it must change nothing: not the exit
// code, not the envelope, and not the disk — the probe runs in read-only mode,
// which creates nothing (decision D-01), so the scaffold `init` reports as
// created must still be `init`'s own work.
func TestARepresentableRepositoryIsUnaffectedByThePreflight(t *testing.T) {
	repo := newRepo(t)

	got := run(t, repo, "init", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "init", got.stdout)

	var envelope struct {
		Data struct {
			ConfigCreated bool `json:"config_created"`
			TerminalState string
		} `json:"data"`
	}
	decodeData(t, got.stdout, &envelope.Data)
	if !envelope.Data.ConfigCreated {
		t.Errorf("config_created = false: the read-only preflight wrote the scaffold `init` should have")
	}
}

// TestTheUnrepresentablePreflightDoesNotHideRealFailures keeps the preflight from
// swallowing the conditions `init` exists to report. A start that fails during
// the probe has nothing to say about representability, and the real run's
// diagnosis is the one the reader needs.
func TestTheUnrepresentablePreflightDoesNotHideRealFailures(t *testing.T) {
	repo := newRepo(t)
	denyWrites(t, filepath.Join(repo, ".git"))

	got := run(t, repo, "init", "--json")
	got.requireExit(t, app.ExitUnavailable)

	payload := got.errorPayload(t)
	if payload.Code == app.CodePathNotRepresentable {
		t.Errorf("the preflight reported %q for a repository whose paths are perfectly representable",
			payload.Code)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
}
