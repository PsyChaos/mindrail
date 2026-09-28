package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeHost(t *testing.T, deps hostCommandDeps, input string, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootWith(Options{})
	for _, command := range root.Commands() {
		if command.Name() == "host" {
			root.RemoveCommand(command)
			break
		}
	}
	root.AddCommand(newHostCommandWith(deps))
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(input))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func TestHostHookForwardsBoundedOfficialPayloadWithoutChoosingTask(t *testing.T) {
	var got HostEventRequest
	deps := hostCommandDeps{ingest: func(_ context.Context, request HostEventRequest) error { got = request; return nil }}
	payload := `{"session_id":"thr_1","hook_event_name":"SessionStart","model":"gpt-5"}`
	stdout, stderr, err := executeHost(t, deps, payload, "host", "codex", "hook")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("hook output stdout=%q stderr=%q", stdout, stderr)
	}
	if got.Host != "codex" || got.Kind != "hook" || string(got.Payload) != payload || got.StartDir == "" {
		t.Fatalf("request = %#v", got)
	}
}

func TestHostHookTelemetryFailureAndOversizeNeverBlockHost(t *testing.T) {
	calls := 0
	deps := hostCommandDeps{ingest: func(context.Context, HostEventRequest) error { calls++; return errors.New("down") }}
	if _, _, err := executeHost(t, deps, `{}`, "host", "claude", "hook"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	if _, _, err := executeHost(t, deps, `{"x":"`+strings.Repeat("x", maxHostEventBytes)+`"}`, "host", "claude", "hook"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("oversize reached sink: calls=%d", calls)
	}
}

func TestClaudeStatuslinePreservesPayloadAndDelegateOutput(t *testing.T) {
	payload := []byte(`{"session_id":"s","model":{"id":"m"}}`)
	var stdout bytes.Buffer
	if err := runStatusDelegate(context.Background(), "cat", payload, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), payload) {
		t.Fatalf("delegate output = %q", stdout.Bytes())
	}
}

func TestStatusDelegateReadRejectsLinksAndBroadPermissions(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(rootPath, "delegate.json"), []byte(`{"type":"command","command":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readStatusDelegate(root, "delegate.json"); err == nil {
		t.Fatal("broad delegate permissions accepted")
	}
	if err := os.Chmod(filepath.Join(rootPath, "delegate.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readStatusDelegate(root, "delegate.json"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("delegate.json", filepath.Join(rootPath, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readStatusDelegate(root, "linked.json"); err == nil {
		t.Fatal("delegate symlink accepted")
	}
}
