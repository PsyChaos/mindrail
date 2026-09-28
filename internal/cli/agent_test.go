package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/credential"
)

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("stdin must not be read") }

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type fakeCredentialStore struct {
	value    string
	getErr   error
	getCalls int
}

func (s *fakeCredentialStore) Get(context.Context) (string, error) {
	s.getCalls++
	return s.value, s.getErr
}

func (*fakeCredentialStore) Set(context.Context, string) error { return nil }
func (*fakeCredentialStore) Delete(context.Context) error      { return nil }

type blockingAgentCredentialStore struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingAgentCredentialStore) Get(context.Context) (string, error) {
	close(s.started)
	<-s.release
	return "late secret", nil
}

func (*blockingAgentCredentialStore) Set(context.Context, string) error { return nil }
func (*blockingAgentCredentialStore) Delete(context.Context) error      { return nil }

func executeAgentRoute(t *testing.T, deps agentCommandDeps, input io.Reader) (string, error) {
	t.Helper()
	command := newAgentCommandWith(deps)
	command.SetArgs([]string{"route"})
	command.SetIn(input)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	return output.String(), err
}

func decodeAgentResult(t *testing.T, output string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode result: %v; output=%q", err, output)
	}
	return result
}

func TestAgentRouteMissingKeyDoesNotReadOrLaunch(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", " \t\n")
	deps := agentCommandDeps{
		findPython: func() (string, error) { t.Fatal("python lookup called"); return "", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("python called")
			return nil, nil
		},
	}
	output, err := executeAgentRoute(t, deps, panicReader{})
	if err != nil {
		t.Fatal(err)
	}
	result := decodeAgentResult(t, output)
	if result["status"] != "disabled" || result["reason"] != "api_key_missing" {
		t.Fatalf("unexpected disabled result: %#v", result)
	}
	want := "{\"advisory\":true,\"enabled\":false,\"mode\":\"shadow\",\"reason\":\"api_key_missing\",\"selections\":{},\"status\":\"disabled\",\"version\":1}\n"
	if output != want {
		t.Fatalf("disabled JSON changed:\n got %q\nwant %q", output, want)
	}
}

func TestAgentRouteEnvironmentKeyWinsOverKeyring(t *testing.T) {
	const envSecret = "ENV_SENTINEL_SECRET"
	t.Setenv("TYPESAFE_API_KEY", envSecret)
	store := &fakeCredentialStore{value: "KEYRING_SENTINEL_SECRET"}
	input := `{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`
	deps := agentCommandDeps{
		store:      store,
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(_ context.Context, _ string, gotInput []byte, key string) ([]byte, error) {
			if key != envSecret {
				t.Fatalf("resolved key = %q", key)
			}
			if string(gotInput) != input || bytes.Contains(gotInput, []byte(envSecret)) {
				t.Fatalf("secret entered adapter request: %q", gotInput)
			}
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if store.getCalls != 0 {
		t.Fatal("keyring was read despite environment override")
	}
	if strings.Contains(output, envSecret) {
		t.Fatal("environment secret leaked to output")
	}
}

func TestAgentRouteUsesKeyringWithoutMutatingParentEnvironment(t *testing.T) {
	const keyringSecret = "KEYRING_SENTINEL_SECRET"
	t.Setenv("TYPESAFE_API_KEY", " \t")
	store := &fakeCredentialStore{value: keyringSecret}
	deps := agentCommandDeps{
		store:      store,
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(_ context.Context, _ string, input []byte, key string) ([]byte, error) {
			if key != keyringSecret {
				t.Fatalf("resolved key = %q", key)
			}
			if bytes.Contains(input, []byte(keyringSecret)) {
				t.Fatal("keyring secret entered adapter request")
			}
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"provider_unavailable","selections":{}}`), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"choose"}`))
	if err != nil {
		t.Fatal(err)
	}
	if store.getCalls != 1 {
		t.Fatalf("keyring Get calls = %d", store.getCalls)
	}
	if got := os.Getenv("TYPESAFE_API_KEY"); got != " \t" {
		t.Fatalf("parent environment mutated: %q", got)
	}
	if strings.Contains(output, keyringSecret) {
		t.Fatal("keyring secret leaked to output")
	}
}

func TestAgentRouteMissingKeyringCredentialIsDisabled(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	store := &fakeCredentialStore{getErr: credential.ErrNotFound}
	deps := agentCommandDeps{
		store: store,
		findPython: func() (string, error) {
			t.Fatal("python lookup called")
			return "", nil
		},
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("python called")
			return nil, nil
		},
	}
	output, err := executeAgentRoute(t, deps, panicReader{})
	if err != nil {
		t.Fatal(err)
	}
	result := decodeAgentResult(t, output)
	if result["status"] != "disabled" || result["reason"] != "api_key_missing" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouteUnavailableKeyringFailsOpenWithoutDisclosure(t *testing.T) {
	const sentinel = "KEYRING_ERROR_SENTINEL"
	t.Setenv("TYPESAFE_API_KEY", "")
	store := &fakeCredentialStore{getErr: errors.New("backend failure " + sentinel)}
	deps := agentCommandDeps{
		store: store,
		findPython: func() (string, error) {
			t.Fatal("python lookup called")
			return "", nil
		},
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("python called")
			return nil, nil
		},
	}
	output, err := executeAgentRoute(t, deps, panicReader{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, sentinel) {
		t.Fatal("credential backend detail leaked")
	}
	result := decodeAgentResult(t, output)
	if result["status"] != "fallback" || result["reason"] != "credential_unavailable" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouteBlockingCredentialStoreFailsOpenOnContextDeadline(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	store := &blockingAgentCredentialStore{started: make(chan struct{}), release: make(chan struct{})}
	deps := agentCommandDeps{
		store: store,
		findPython: func() (string, error) {
			t.Fatal("python lookup called")
			return "", nil
		},
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("python called")
			return nil, nil
		},
	}
	command := newAgentCommandWith(deps)
	command.SetArgs([]string{"route"})
	command.SetIn(panicReader{})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- command.ExecuteContext(ctx) }()
	<-store.started
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("agent route hung on a blocking credential store")
	}
	close(store.release)

	result := decodeAgentResult(t, output.String())
	if result["status"] != "fallback" || result["reason"] != "credential_unavailable" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRoutePassesInputAndValidResultThrough(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	wantInput := `{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`
	wantOutput := `{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(_ context.Context, python string, input []byte, _ string) ([]byte, error) {
			if python != "/trusted/python3" {
				t.Fatalf("python=%q", python)
			}
			if string(input) != wantInput {
				t.Fatalf("input=%q", input)
			}
			return []byte(wantOutput), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(wantInput))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output) != wantOutput {
		t.Fatalf("output=%q", output)
	}
}

func TestAgentRouteMissingInterpreterFailsOpen(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "", exec.ErrNotFound },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("python called")
			return nil, nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader("secret input"))
	if err != nil {
		t.Fatal(err)
	}
	result := decodeAgentResult(t, output)
	if result["status"] != "fallback" || result["reason"] != "python_unavailable" {
		t.Fatalf("unexpected fallback: %#v", result)
	}
}

func TestAgentRouteInvalidOutputIsSanitized(t *testing.T) {
	secret := "env-secret"
	t.Setenv("TYPESAFE_API_KEY", secret)
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte("traceback contains " + secret), errors.New("launch detail " + secret)
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, secret) {
		t.Fatal("secret leaked to output")
	}
	result := decodeAgentResult(t, output)
	if result["reason"] != "adapter_failure" {
		t.Fatalf("unexpected fallback: %#v", result)
	}
}

func TestAgentRouteRejectsSecretInOtherwiseValidOutput(t *testing.T) {
	secret := "BREAKER_SENTINEL_SECRET"
	t.Setenv("TYPESAFE_API_KEY", secret)
	adapterOutput := `{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"provider_` + secret + `","selections":{}}`
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(adapterOutput), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, secret) {
		t.Fatal("otherwise-valid adapter output leaked the environment key")
	}
	if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
		t.Fatalf("secret-bearing output passed through: %#v", result)
	}
}

func TestAgentRouteRejectsInconsistentFallbackStatus(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	adapterOutput := `{"version":1,"enabled":false,"advisory":true,"mode":"shadow","status":"fallback","reason":"provider_unavailable","selections":{}}`
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(adapterOutput), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
		t.Fatalf("inconsistent status passed through: %#v", result)
	}
}

func TestAgentRouteBoundsStdinBeforeLaunchingAdapter(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	read := -1
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(_ context.Context, _ string, input []byte, _ string) ([]byte, error) {
			read = len(input)
			return []byte(`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"input_too_large","selections":{}}`), nil
		},
	}
	input := strings.NewReader(strings.Repeat("x", maxAgentRouteInput*2))
	output, err := executeAgentRoute(t, deps, input)
	if err != nil {
		t.Fatal(err)
	}
	if read != maxAgentRouteInput {
		t.Fatalf("adapter received %d bytes, want bounded %d", read, maxAgentRouteInput)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "input_too_large" {
		t.Fatalf("valid bounded fallback was not passed through: %#v", result)
	}
}

func TestAgentRouteRejectsOversizedAdapterOutput(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	prefix := `{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"`
	suffix := `","selections":{}}`
	adapterOutput := prefix + strings.Repeat("x", maxAgentRouteOutput) + suffix
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(adapterOutput), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
		t.Fatalf("oversized output passed through: %#v", result)
	}
}

func TestAgentRouteReadErrorFailsOpenWithoutLaunching(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	sentinel := errors.New("sentinel read failure")
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			t.Fatal("adapter launched after stdin read error")
			return nil, nil
		},
	}
	output, err := executeAgentRoute(t, deps, failingReader{err: sentinel})
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
		t.Fatalf("unexpected read-error fallback: %#v", result)
	}
}

func TestAgentRouteAddsRecordTerminatingNewline(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	adapterOutput := `{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"provider_unavailable","selections":{}}`
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(adapterOutput), nil
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if output != adapterOutput+"\n" {
		t.Fatalf("output=%q want one newline-terminated JSON record", output)
	}
}

func TestAgentRouteClassifiesInterpreterRaceAsUnavailable(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return nil, &os.PathError{Op: "fork/exec", Path: "/trusted/python3", Err: os.ErrNotExist}
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(`{"goal":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "python_unavailable" {
		t.Fatalf("interpreter disappearance misclassified: %#v", result)
	}
}

func TestAgentRouteRejectsEscapedSecretAndInconsistentStatus(t *testing.T) {
	secret := `quote"key`
	t.Setenv("TYPESAFE_API_KEY", secret)
	outputs := []string{
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"provider_unavailable","selections":{},"detail":"quote\"key"}`,
		`{"version":1,"enabled":false,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{}}`,
	}
	for _, adapterOutput := range outputs {
		deps := agentCommandDeps{
			findPython: func() (string, error) { return "/trusted/python3", nil },
			runPython: func(context.Context, string, []byte, string) ([]byte, error) {
				return []byte(adapterOutput), nil
			},
		}
		output, err := executeAgentRoute(t, deps, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		result := decodeAgentResult(t, output)
		if result["reason"] != "adapter_failure" {
			t.Fatalf("unexpected result for %q: %#v", adapterOutput, result)
		}
		if strings.Contains(output, secret) {
			t.Fatal("secret leaked to output")
		}
	}
}

func TestAgentRouteRejectsInvalidSelectionContracts(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	input := `{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`
	outputs := []string{
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{}}`,
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"surprise":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`,
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"shell","confidence":0.9,"accepted":true,"reason":"accepted"}}}`,
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":false,"reason":"accepted"}}}`,
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"low_confidence","selections":{"tool":{"candidate":"rg","confidence":"high","accepted":false,"reason":"low_confidence"}}}`,
		`{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"fallback","reason":"low_confidence","selections":{"tool":{"candidate":"rg","confidence":0.5,"accepted":true,"reason":"accepted"}}}`,
	}
	for _, adapterOutput := range outputs {
		deps := agentCommandDeps{
			findPython: func() (string, error) { return "/trusted/python3", nil },
			runPython: func(context.Context, string, []byte, string) ([]byte, error) {
				return []byte(adapterOutput), nil
			},
		}
		output, err := executeAgentRoute(t, deps, strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
			t.Fatalf("invalid adapter output passed through: %#v", result)
		}
	}
}

func TestAgentRouteRejectsSuccessfulAdviceFromFailedProcess(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "env-secret")
	input := `{"goal":"choose","tools":[{"id":"rg","description":"search"}]}`
	valid := `{"version":1,"enabled":true,"advisory":true,"mode":"shadow","status":"ok","reason":"advice_available","selections":{"tool":{"candidate":"rg","confidence":0.9,"accepted":true,"reason":"accepted"}}}`
	deps := agentCommandDeps{
		findPython: func() (string, error) { return "/trusted/python3", nil },
		runPython: func(context.Context, string, []byte, string) ([]byte, error) {
			return []byte(valid), errors.New("process crashed after output")
		},
	}
	output, err := executeAgentRoute(t, deps, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if result := decodeAgentResult(t, output); result["reason"] != "adapter_failure" {
		t.Fatalf("failed process advice passed through: %#v", result)
	}
}

func TestAgentRouteRunsEmbeddedAdapterWithoutNetwork(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	t.Setenv("TYPESAFE_API_KEY", "é")
	output, err := executeAgentRoute(t, agentCommandDeps{
		findPython: findTrustedPython,
	}, strings.NewReader(`{"goal":"not read by an invalid-key adapter"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := decodeAgentResult(t, output)
	if result["status"] != "fallback" || result["reason"] != "invalid_api_key" {
		t.Fatalf("unexpected embedded result: %#v", result)
	}
}

func TestAgentCommandIsHidden(t *testing.T) {
	command := newAgentCommand()
	if !command.Hidden {
		t.Fatal("agent command must remain hidden")
	}
	route, _, err := command.Find([]string{"route"})
	if err != nil {
		t.Fatal(err)
	}
	if !route.Hidden {
		t.Fatal("agent route command must remain hidden")
	}
}

func TestFindTrustedPythonIgnoresPATH(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "python3")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := findTrustedPython()
	if err != nil {
		t.Skip("no trusted system Python is installed")
	}
	if got == fake {
		t.Fatal("PATH-controlled interpreter was selected")
	}
}
