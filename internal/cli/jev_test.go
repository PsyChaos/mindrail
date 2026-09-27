package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const testJEVSecret = "ts-test-secret-never-print"

type browserCredentialStore struct {
	mu        sync.Mutex
	key       string
	getErr    error
	setErr    error
	deleteErr error
	sets      int
	deletes   int
}

func (s *browserCredentialStore) Get(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key, s.getErr
}

func (s *browserCredentialStore) Set(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets++
	if s.setErr == nil {
		s.key = key
	}
	return s.setErr
}

func (s *browserCredentialStore) Delete(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	if s.deleteErr == nil {
		s.key = ""
	}
	return s.deleteErr
}

func baseJEVDeps(store credential.Store) jevCommandDeps {
	return jevCommandDeps{
		store:  store,
		getenv: func(string) string { return "" },
		connect: func(ctx context.Context, target credential.Store) error {
			return target.Set(ctx, testJEVSecret)
		},
	}
}

func TestJEVStatusReportsOnlyEffectiveConnectionSource(t *testing.T) {
	tests := []struct {
		name      string
		store     *browserCredentialStore
		env       string
		connected bool
		source    string
		wantCode  app.Code
	}{
		{name: "environment wins", store: &browserCredentialStore{key: testJEVSecret}, env: "override-secret", connected: true, source: "environment"},
		{name: "keyring", store: &browserCredentialStore{key: testJEVSecret}, connected: true, source: "keyring"},
		{name: "missing", store: &browserCredentialStore{getErr: credential.ErrNotFound}, source: "none"},
		{name: "empty", store: &browserCredentialStore{}, source: "none"},
		{name: "unavailable", store: &browserCredentialStore{getErr: errors.New(testJEVSecret)}, source: "unavailable", wantCode: app.CodeJEVCredentialUnavailable},
		{name: "unsupported", store: &browserCredentialStore{getErr: credential.ErrUnsupportedPlatform}, source: "unavailable", wantCode: app.CodeJEVPersistenceUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := baseJEVDeps(tc.store)
			deps.getenv = func(string) string { return tc.env }
			result, err := resolveJEVStatus(t.Context(), deps)
			if result.Connected != tc.connected || result.Source != tc.source {
				t.Fatalf("result = %#v, err = %v", result, err)
			}
			assertJEVErrorCode(t, err, tc.wantCode)
		})
	}
}

func TestJEVDisconnectReportsUnsupportedPersistencePrecisely(t *testing.T) {
	deps := baseJEVDeps(&browserCredentialStore{deleteErr: credential.ErrUnsupportedPlatform})
	_, err := executeJEVCommand(t, deps, "jev", "disconnect", "--json")
	assertJEVErrorCode(t, err, app.CodeJEVPersistenceUnsupported)
}

func TestJEVConnectDistinguishesStoredDestinationFromEffectiveSource(t *testing.T) {
	store := &browserCredentialStore{}
	deps := baseJEVDeps(store)
	deps.getenv = func(string) string { return "automation-secret" }
	out, err := executeJEVCommand(t, deps, "jev", "connect", "--json")
	if err != nil {
		t.Fatalf("execute: %v, output: %s", err, out)
	}
	for _, want := range []string{`"connected":true`, `"source":"environment"`, `"stored_in":"keyring"`, `"environment_override":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %s, want %s", out, want)
		}
	}
	if strings.Contains(out, testJEVSecret) || strings.Contains(out, "automation-secret") {
		t.Fatalf("output leaked key: %s", out)
	}
}

func TestJEVDisconnectTextRespectsWhetherAKeyWasRemoved(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "removed", want: "keyring credential was removed"},
		{name: "not stored", err: credential.ErrNotFound, want: "No keyring credential was stored"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &browserCredentialStore{key: testJEVSecret, deleteErr: tc.err}
			deps := baseJEVDeps(store)
			deps.getenv = func(string) string { return "automation-secret" }
			out, err := executeJEVCommand(t, deps, "jev", "disconnect")
			if err != nil || !strings.Contains(out, tc.want) {
				t.Fatalf("output = %q, err = %v", out, err)
			}
		})
	}
}

func TestJEVDisconnectReportsIndeterminateWithoutClaimingNoMutation(t *testing.T) {
	store := &lateDeleteStore{started: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
	deps := baseJEVDeps(store)
	deps.mutationTimeout = 20 * time.Millisecond
	_, err := executeJEVCommand(t, deps, "jev", "disconnect", "--json")
	assertJEVErrorCode(t, err, app.CodeJEVDisconnectIndeterminate)
	payload, _ := app.PayloadOf(err)
	if strings.Contains(payload.Impact, "No key") || !strings.Contains(payload.Impact, "may still be present") || !strings.Contains(payload.Impact, "may complete") {
		t.Fatalf("impact makes an inaccurate deletion claim: %q", payload.Impact)
	}
	if !strings.Contains(strings.Join(payload.NextAction, " "), "jev status") {
		t.Fatalf("next action = %#v, want status check", payload.NextAction)
	}
	select {
	case <-store.started:
	default:
		t.Fatal("disconnect returned indeterminate before dispatch started")
	}
	close(store.release)
	<-store.done
	if !store.deleted {
		t.Fatal("late deletion did not complete; test does not prove post-return mutation")
	}
}

type lateDeleteStore struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
	deleted bool
}

func (*lateDeleteStore) Get(context.Context) (string, error) { return "", nil }
func (*lateDeleteStore) Set(context.Context, string) error   { return nil }
func (s *lateDeleteStore) Delete(context.Context) error {
	close(s.started)
	<-s.release
	s.deleted = true
	close(s.done)
	return nil
}

func TestJEVCommandsPassBoundedContextsToCredentialStore(t *testing.T) {
	store := blockingJEVStore{}
	deps := baseJEVDeps(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := resolveJEVStatus(ctx, deps); err == nil {
		t.Fatal("status accepted a cancelled credential operation")
	}

	root := commandForJEVTest(t, deps)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"jev", "disconnect"})
	if err := root.ExecuteContext(ctx); err == nil {
		t.Fatal("disconnect accepted a cancelled credential operation")
	}
}

type blockingJEVStore struct{}

func (blockingJEVStore) Get(ctx context.Context) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
func (blockingJEVStore) Set(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}
func (blockingJEVStore) Delete(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func executeJEVCommand(t *testing.T, deps jevCommandDeps, args ...string) (string, error) {
	t.Helper()
	root := commandForJEVTest(t, deps)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	if strings.Contains(stderr.String(), testJEVSecret) {
		t.Fatalf("stderr leaked the key: %q", stderr.String())
	}
	return stdout.String(), err
}

func commandForJEVTest(t *testing.T, deps jevCommandDeps) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "mindrail", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool(flagJSON, false, "")
	root.PersistentFlags().Bool(flagVerbose, false, "")
	root.PersistentFlags().Bool(flagNoColor, false, "")
	root.PersistentFlags().StringP(flagChdir, "C", "", "")
	root.AddCommand(newJEVCommandWith(deps))
	return root
}

func assertJEVErrorCode(t *testing.T, err error, want app.Code) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != want {
		t.Fatalf("error = %v, code = %q, want %q", err, payload.Code, want)
	}
}
