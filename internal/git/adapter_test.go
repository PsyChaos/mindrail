package git

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// goldenArgv is the exact rev-parse sequence of design §2.3. It is written out
// rather than derived so that a change to the adapter has to change this table
// too, which is the point of a golden test.
var goldenArgv = [][]string{
	{"rev-parse", "--is-inside-work-tree"},
	{"rev-parse", "--path-format=absolute", "--git-common-dir"},
	{"rev-parse", "--path-format=absolute", "--git-dir"},
	{"rev-parse", "--path-format=absolute", "--show-toplevel"},
	{"rev-parse", "--is-bare-repository"},
}

func healthyResponses(commonDir, gitDir, worktree string) map[string]FakeResponse {
	return map[string]FakeResponse{
		"rev-parse --is-inside-work-tree":                   {Stdout: "true\n"},
		"rev-parse --path-format=absolute --git-common-dir": {Stdout: commonDir + "\n"},
		"rev-parse --path-format=absolute --git-dir":        {Stdout: gitDir + "\n"},
		"rev-parse --path-format=absolute --show-toplevel":  {Stdout: worktree + "\n"},
		"rev-parse --is-bare-repository":                    {Stdout: "false\n"},
	}
}

func TestAdapterResolveGoldenArgv(t *testing.T) {
	const (
		worktree  = "/srv/repo"
		commonDir = "/srv/repo/.git"
	)
	fake := &FakeRunner{Responses: healthyResponses(commonDir, commonDir, worktree)}

	repo, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo/a/b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got := make([][]string, 0, len(fake.Calls))
	for _, call := range fake.Calls {
		got = append(got, call.Args)
		if call.Dir != "/srv/repo/a/b" {
			t.Errorf("invocation %v ran in %q, want the start directory", call.Args, call.Dir)
		}
	}
	if !reflect.DeepEqual(got, goldenArgv) {
		t.Fatalf("invocations =\n%v\nwant\n%v", got, goldenArgv)
	}

	want := Repository{
		CommonDir:        commonDir,
		GitDir:           commonDir,
		WorktreeRoot:     worktree,
		IsLinkedWorktree: false,
		IsBare:           false,
	}
	if repo != want {
		t.Fatalf("Repository = %+v, want %+v", repo, want)
	}
}

func TestAdapterResolveLinkedWorktreeFlag(t *testing.T) {
	const (
		worktree  = "/srv/wt"
		commonDir = "/srv/repo/.git"
		gitDir    = "/srv/repo/.git/worktrees/wt"
	)
	fake := &FakeRunner{Responses: healthyResponses(commonDir, gitDir, worktree)}

	repo, err := NewAdapter(fake).Resolve(context.Background(), worktree)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !repo.IsLinkedWorktree {
		t.Fatalf("IsLinkedWorktree = false for GitDir %q against CommonDir %q", repo.GitDir, repo.CommonDir)
	}
}

func TestAdapterResolveNotARepository(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string]FakeResponse
		fallback  FakeResponse
		wantCalls int
	}{
		{
			name:      "rev-parse exits non-zero",
			fallback:  FakeResponse{Stderr: "fatal: not a git repository\n", Err: errors.New("exit status 128")},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &FakeRunner{Responses: tt.responses, Default: tt.fallback}

			repo, err := NewAdapter(fake).Resolve(context.Background(), "/tmp/not-a-repo")

			if !errors.Is(err, ErrNotARepository) {
				t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
			}
			if repo != (Repository{}) {
				t.Fatalf("Resolve returned %+v alongside an error", repo)
			}
			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != app.CodeNotAGitRepository {
				t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
			}
			assertPayloadIsActionable(t, payload)
			if got := app.ExitCode(err); got != app.ExitUsage {
				t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
			}
			if len(fake.Calls) != tt.wantCalls {
				t.Fatalf("made %d invocations, want %d: %v", len(fake.Calls), tt.wantCalls, fake.Calls)
			}
			if payload.Metadata["start_dir"] != "/tmp/not-a-repo" {
				t.Errorf("start_dir metadata = %q, want the directory the question was asked in", payload.Metadata["start_dir"])
			}
		})
	}
}

// TestAdapterResolveInsideGitDirectory pins the case the classifier already
// separates in code: git found a repository, said this is not its worktree and
// said it is not bare, which leaves only "inside .git". Collapsing it into the
// outside-every-repository error hands the reader a remedy — `git init` — that
// creates a nested repository when it is followed.
func TestAdapterResolveInsideGitDirectory(t *testing.T) {
	const startDir = "/srv/repo/.git/refs"
	fake := &FakeRunner{Responses: map[string]FakeResponse{
		"rev-parse --is-inside-work-tree": {Stdout: "false\n"},
		"rev-parse --is-bare-repository":  {Stdout: "false\n"},
	}}

	repo, err := NewAdapter(fake).Resolve(context.Background(), startDir)

	if repo != (Repository{}) {
		t.Fatalf("Resolve returned %+v alongside an error", repo)
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
	}
	// The broader sentinel still holds: callers that only care that there is no
	// worktree here must not have to learn a second name for it.
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve error = %v, want it to still unwrap to ErrNotARepository", err)
	}
	if len(fake.Calls) != 2 {
		t.Fatalf("made %d invocations, want 2: %v", len(fake.Calls), fake.Calls)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	// The code is the machine contract and is not reworded (decision D-18); the
	// prose around it is what has to differ.
	if payload.Code != app.CodeNotAGitRepository {
		t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
	}
	assertPayloadIsActionable(t, payload)
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
	}
	if payload.Metadata["start_dir"] != startDir {
		t.Errorf("start_dir metadata = %q, want %q", payload.Metadata["start_dir"], startDir)
	}

	outside, _ := app.PayloadOf(notARepositoryError(startDir, "", nil))
	if payload.Why == outside.Why {
		t.Errorf("the .git case reuses the outside-every-repository why: %q", payload.Why)
	}
	if slices.Equal(payload.NextAction, outside.NextAction) {
		t.Errorf("the .git case reuses the outside-every-repository remedy: %q", payload.NextAction)
	}
	for _, action := range payload.NextAction {
		if strings.Contains(action, "git init") {
			t.Errorf("next_action %q would create a repository inside .git", action)
		}
	}
}

// TestBareRepositoryErrorCarriesStartDir keeps the producer half of the
// start_dir contract: both worktree-less classifications say where the question
// was asked, so a renderer has something to show instead of a bare refusal.
func TestBareRepositoryErrorCarriesStartDir(t *testing.T) {
	payload, ok := app.PayloadOf(bareRepositoryError("/srv/bare.git"))
	if !ok {
		t.Fatalf("bareRepositoryError carries no domain payload")
	}
	if payload.Metadata["start_dir"] != "/srv/bare.git" {
		t.Fatalf("start_dir metadata = %q, want %q", payload.Metadata["start_dir"], "/srv/bare.git")
	}
}

func TestAdapterResolveBareRepository(t *testing.T) {
	fake := &FakeRunner{Responses: map[string]FakeResponse{
		"rev-parse --is-inside-work-tree": {Stdout: "false\n"},
		"rev-parse --is-bare-repository":  {Stdout: "true\n"},
	}}

	repo, err := NewAdapter(fake).Resolve(context.Background(), "/srv/bare.git")

	if !errors.Is(err, ErrBareRepository) {
		t.Fatalf("Resolve error = %v, want ErrBareRepository", err)
	}
	if repo != (Repository{}) {
		t.Fatalf("Resolve returned %+v alongside an error", repo)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeBareRepository {
		t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeBareRepository)
	}
	assertPayloadIsActionable(t, payload)
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (usage, decision D-31)", got, app.ExitUsage)
	}

	// A bare repository must be diagnosed without asking for a worktree that
	// cannot exist; --show-toplevel would fail with a less useful message.
	for _, call := range fake.Calls {
		for _, arg := range call.Args {
			if arg == "--show-toplevel" {
				t.Fatalf("adapter asked a bare repository for its worktree: %v", call.Args)
			}
		}
	}
}

func TestAdapterPropagatesRunnerFailures(t *testing.T) {
	tests := []struct {
		name     string
		runErr   error
		wantCode app.Code
		wantExit int
	}{
		{name: "timeout", runErr: timeoutError([]string{"rev-parse"}, DefaultTimeout), wantCode: app.CodeGitTimeout, wantExit: app.ExitUnavailable},
		{name: "missing binary", runErr: unavailableError("git", "", errors.New("executable file not found in $PATH")), wantCode: app.CodeGitUnavailable, wantExit: app.ExitUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &FakeRunner{Default: FakeResponse{Err: tt.runErr}}

			_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo")

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != tt.wantCode {
				t.Fatalf("payload code = %q, want %q", payload.Code, tt.wantCode)
			}
			if got := app.ExitCode(err); got != tt.wantExit {
				t.Fatalf("ExitCode = %d, want %d", got, tt.wantExit)
			}
		})
	}

	t.Run("cancellation is not reported as a missing repository", func(t *testing.T) {
		fake := &FakeRunner{Default: FakeResponse{Err: context.Canceled}}

		_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo")

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want it to unwrap to context.Canceled", err)
		}
		if errors.Is(err, ErrNotARepository) {
			t.Fatalf("a cancelled probe was misread as an absent repository")
		}
	})
}

func TestAdapterVersion(t *testing.T) {
	fake := &FakeRunner{Responses: map[string]FakeResponse{
		"--version": {Stdout: "git version 2.55.0\n"},
	}}

	version, err := NewAdapter(fake).Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != "git version 2.55.0" {
		t.Fatalf("Version = %q, want the trimmed output of `git --version`", version)
	}
	if len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, []string{"--version"}) {
		t.Fatalf("invocations = %v, want a single `--version`", fake.Calls)
	}

	t.Run("a missing binary is reported as unavailable", func(t *testing.T) {
		broken := &FakeRunner{Default: FakeResponse{Err: errors.New("executable file not found in $PATH")}}

		if _, err := NewAdapter(broken).Version(context.Background()); !errors.Is(err, ErrGitUnavailable) {
			t.Fatalf("Version error = %v, want ErrGitUnavailable", err)
		}
	})
}

func TestFakeRunnerRecordsInvocations(t *testing.T) {
	fake := &FakeRunner{
		Responses: map[string]FakeResponse{"rev-parse --git-dir": {Stdout: "/repo/.git\n", Stderr: "warn\n"}},
		Default:   FakeResponse{Stdout: "fallback\n"},
	}

	stdout, stderr, err := fake.Run(context.Background(), "/repo", "rev-parse", "--git-dir")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(stdout) != "/repo/.git\n" || string(stderr) != "warn\n" {
		t.Fatalf("Run = (%q, %q), want the scripted response", stdout, stderr)
	}

	stdout, _, _ = fake.Run(context.Background(), "/repo", "status")
	if string(stdout) != "fallback\n" {
		t.Fatalf("unscripted argv returned %q, want the default response", stdout)
	}

	want := []Invocation{
		{Dir: "/repo", Args: []string{"rev-parse", "--git-dir"}},
		{Dir: "/repo", Args: []string{"status"}},
	}
	if !reflect.DeepEqual(fake.Calls, want) {
		t.Fatalf("Calls = %+v, want %+v", fake.Calls, want)
	}

	t.Run("recorded arguments are a private copy", func(t *testing.T) {
		args := []string{"rev-parse", "--git-dir"}
		recorder := &FakeRunner{}
		if _, _, err := recorder.Run(context.Background(), "/repo", args...); err != nil {
			t.Fatalf("Run: %v", err)
		}
		args[1] = "mutated"
		if recorder.Calls[0].Args[1] != "--git-dir" {
			t.Fatalf("mutating the caller's slice changed the recorded invocation")
		}
	})

	t.Run("a nil response map still serves the default", func(t *testing.T) {
		bare := &FakeRunner{Default: FakeResponse{Stdout: "ok"}}
		stdout, _, err := bare.Run(context.Background(), "/repo", "rev-parse")
		if err != nil || string(stdout) != "ok" {
			t.Fatalf("Run on a nil map = (%q, %v), want the default", stdout, err)
		}
	})
}

// assertPayloadIsActionable enforces spec §84: an error that says only what
// broke leaves the reader with nothing to do.
func assertPayloadIsActionable(t *testing.T, payload app.ErrorPayload) {
	t.Helper()

	if payload.Why == "" {
		t.Errorf("payload %q has no why", payload.Code)
	}
	if payload.Impact == "" {
		t.Errorf("payload %q has no impact", payload.Code)
	}
	if len(payload.NextAction) == 0 {
		t.Errorf("payload %q has no next action", payload.Code)
	}
}
