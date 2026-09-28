package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/hostadapter"
)

const maxHostEventBytes = 64 << 10

// HostEventRequest is the narrow, bounded bridge between lifecycle commands
// and the host-runtime domain. Payload is an official hook/status-line JSON
// object. The CLI does not select a task or session; the injected adapter must
// bind the host identity explicitly or spool it until an MCP bootstrap does.
type HostEventRequest struct {
	Host     string
	Kind     string
	StartDir string
	Payload  []byte
}

type hostCommandDeps struct {
	ingest   func(context.Context, HostEventRequest) error
	runner   git.CommandRunner
	delegate func(context.Context, string, []byte, io.Writer, io.Writer) error
}

func newHostCommand(o Options) *cobra.Command {
	ingest := o.IngestHostEvent
	if ingest == nil {
		ingest = func(ctx context.Context, request HostEventRequest) error {
			return hostadapter.IngestHostEvent(ctx, request.Host, request.Kind, request.StartDir, request.Payload)
		}
	}
	return newHostCommandWith(hostCommandDeps{ingest: ingest, runner: o.Runner, delegate: runStatusDelegate})
}

func newHostCommandWith(deps hostCommandDeps) *cobra.Command {
	host := &cobra.Command{Use: "host", Short: "Internal host runtime adapters", Args: cobra.NoArgs, Hidden: true}
	for _, provider := range []string{"claude", "codex"} {
		provider := provider
		group := &cobra.Command{Use: provider, Args: cobra.NoArgs, Hidden: true}
		hook := &cobra.Command{Use: "hook", Args: cobra.NoArgs, Hidden: true, RunE: func(cmd *cobra.Command, _ []string) error {
			return runHostInput(cmd, deps, provider, "hook", false)
		}}
		group.AddCommand(hook)
		if provider == "claude" {
			group.AddCommand(&cobra.Command{Use: "statusline", Args: cobra.NoArgs, Hidden: true, RunE: func(cmd *cobra.Command, _ []string) error {
				return runHostInput(cmd, deps, provider, "statusline", true)
			}})
		}
		host.AddCommand(group)
	}
	return host
}

func runHostInput(cmd *cobra.Command, deps hostCommandDeps, provider, kind string, delegate bool) error {
	payload, err := readBoundedHostInput(cmd.InOrStdin())
	if err != nil {
		// Telemetry is optional. A malformed or oversized observation must not
		// block the coding host lifecycle that emitted it.
		return nil
	}
	startDir, err := resolveStartDir(globalFlagsOf(cmd).chdir)
	if err == nil && deps.ingest != nil {
		_ = deps.ingest(cmd.Context(), HostEventRequest{Host: provider, Kind: kind, StartDir: startDir, Payload: payload})
	}
	if delegate && err == nil && deps.delegate != nil {
		command, lookupErr := statusDelegateCommand(cmd.Context(), deps.runner, startDir)
		if lookupErr == nil && command != "" {
			return deps.delegate(cmd.Context(), command, payload, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}
	}
	return nil
}

func readBoundedHostInput(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxHostEventBytes+1))
	if err != nil || len(body) > maxHostEventBytes {
		return nil, errors.New("host event exceeds the bounded input")
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, errors.New("host event must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("host event has trailing JSON")
	}
	return body, nil
}

func statusDelegateCommand(ctx context.Context, runner git.CommandRunner, startDir string) (string, error) {
	if runner == nil {
		runner = git.NewExecRunner()
	}
	out, _, err := runner.Run(ctx, startDir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	common := strings.TrimSpace(string(out))
	if common == "" || strings.ContainsAny(common, "\x00\r\n") {
		return "", errors.New("invalid Git common directory")
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(startDir, common)
	}
	rootOut, _, err := runner.Run(ctx, startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	worktree := strings.TrimSpace(string(rootOut))
	if worktree == "" || !filepath.IsAbs(worktree) || strings.ContainsAny(worktree, "\x00\r\n") {
		return "", errors.New("invalid Git worktree root")
	}
	worktreeKey := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(worktree))))
	commonRoot, err := os.OpenRoot(filepath.Clean(common))
	if err != nil {
		return "", err
	}
	defer commonRoot.Close()
	body, err := readStatusDelegate(commonRoot, filepath.Join("mindrail", "host-adapters", worktreeKey, "claude-statusline.json"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var saved struct{ Type, Command string }
	if err := json.Unmarshal(body, &saved); err != nil {
		return "", err
	}
	if saved.Type != "command" || strings.TrimSpace(saved.Command) == "" {
		return "", nil
	}
	return saved.Command, nil
}

func readStatusDelegate(root *os.Root, rel string) ([]byte, error) {
	for path := rel; path != "."; path = filepath.Dir(path) {
		info, err := root.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("status-line delegate path traverses a symbolic link")
		}
		if path == rel {
			if !info.Mode().IsRegular() || info.Size() > maxHostEventBytes {
				return nil, errors.New("status-line delegate is not a bounded regular file")
			}
			if info.Mode().Perm()&0o077 != 0 {
				return nil, errors.New("status-line delegate permissions are too broad")
			}
		}
	}
	file, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxHostEventBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxHostEventBytes {
		return nil, errors.New("status-line delegate is too large")
	}
	return body, nil
}

func runStatusDelegate(ctx context.Context, command string, input []byte, stdout, stderr io.Writer) error {
	var process *exec.Cmd
	if runtime.GOOS == "windows" {
		process = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	} else {
		process = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	process.Stdin = bytes.NewReader(input)
	process.Stdout = stdout
	process.Stderr = stderr
	return process.Run()
}
