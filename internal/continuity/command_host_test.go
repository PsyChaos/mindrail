package continuity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandHostPrepareActivateAndRevocation(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "host-adapter")
	script := `#!/bin/sh
input=$(cat)
case "$input" in
  *'"action":"prepare"'*) printf '%s\n' '{"version":1,"operation_id":"OP-1","takeover_token":"01234567890123456789012345678901"}' ;;
  *'"action":"activate"'*) printf '%s\n' '{"version":1,"ok":true}' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HostAutoApproveEnv, "1")
	repository := filepath.Join(dir, "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	host, err := NewCommandHost(repository, command)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := host.PrepareSuccessor(t.Context(), BootstrapEnvelope{IntentID: "CTI-1", TaskID: "TSK-1", CheckpointID: "CHK-1", TakeoverToken: "01234567890123456789012345678901"})
	if err != nil || prepared.OperationID != "OP-1" || len(prepared.TakeoverToken) != 32 {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	if err := host.ActivateSuccessor(t.Context(), prepared.OperationID); err != nil {
		t.Fatal(err)
	}
	t.Setenv(HostAutoApproveEnv, "0")
	if host.Capabilities(t.Context()).LocalAutoSpawnApproved {
		t.Fatal("revoked host still advertises auto-spawn approval")
	}
	if err := host.ActivateSuccessor(t.Context(), prepared.OperationID); err == nil {
		t.Fatal("revoked host activated a successor")
	}
}

func TestCommandHostEnvironmentDoesNotForwardProcessSecrets(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "secret")
	t.Setenv("GITHUB_TOKEN", "secret")
	joined := strings.Join(hostEnvironment(), "\n")
	if strings.Contains(joined, "TYPESAFE_API_KEY") || strings.Contains(joined, "GITHUB_TOKEN") || strings.Contains(joined, "secret") {
		t.Fatalf("secret-bearing process environment escaped: %q", joined)
	}
}

func TestCommandHostRejectsRepositoryControlledExecutable(t *testing.T) {
	root := t.TempDir()
	command := filepath.Join(root, "adapter")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCommandHost(root, command); err == nil {
		t.Fatal("repository-controlled adapter was accepted")
	}
	outside := t.TempDir()
	link := filepath.Join(outside, "linked-adapter")
	if err := os.Symlink(command, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCommandHost(root, link); err == nil {
		t.Fatal("symlink to repository-controlled adapter was accepted")
	}
}

func TestCommandHostResolvesSymlinkedRepositoryRoot(t *testing.T) {
	realRoot := t.TempDir()
	command := filepath.Join(realRoot, "adapter")
	if err := os.WriteFile(command, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "repository")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCommandHost(linkedRoot, command); err == nil {
		t.Fatal("adapter inside symlinked repository root was accepted")
	}
}
