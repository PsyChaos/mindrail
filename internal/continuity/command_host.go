package continuity

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
	"time"
)

const (
	HostCommandEnv     = "MINDRAIL_CONTINUITY_HOST"
	HostAutoApproveEnv = "MINDRAIL_CONTINUITY_AUTO_SPAWN"
	hostCallTimeout    = 10 * time.Second
	hostOutputLimit    = 16 * 1024
)

type CommandHost struct {
	command string
	root    string
}

func HostFromEnvironment(root string) (HostBridge, error) {
	command := strings.TrimSpace(os.Getenv(HostCommandEnv))
	if command == "" {
		return UnavailableHost{}, nil
	}
	return NewCommandHost(root, command)
}

func NewCommandHost(root, command string) (*CommandHost, error) {
	if !filepath.IsAbs(command) {
		return nil, errors.New("continuity host command must be absolute")
	}
	command = filepath.Clean(command)
	resolved, err := filepath.EvalSymlinks(command)
	if err != nil {
		return nil, errors.New("continuity host command cannot be resolved")
	}
	command = resolved
	root, _ = filepath.Abs(root)
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, errors.New("continuity repository root cannot be resolved")
	}
	if rel, err := filepath.Rel(root, command); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("continuity host command cannot be repository-controlled")
	}
	info, err := os.Stat(command)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("continuity host command is not an executable regular file")
	}
	return &CommandHost{command: command, root: root}, nil
}

func (h *CommandHost) Capabilities(context.Context) HostCapabilities {
	approved := os.Getenv(HostAutoApproveEnv) == "1"
	_, err := os.Stat(h.command)
	return HostCapabilities{
		ContextTelemetry: true, IdempotentPrepare: err == nil,
		IdempotentActivate: true, SuccessorActivation: err == nil, LocalAutoSpawnApproved: approved && err == nil,
	}
}

func (h *CommandHost) PrepareSuccessor(ctx context.Context, envelope BootstrapEnvelope) (PreparedSuccessor, error) {
	if !h.Capabilities(ctx).LocalAutoSpawnApproved {
		return PreparedSuccessor{}, ErrHostCapabilityUnavailable
	}
	var out struct {
		Version       int    `json:"version"`
		Status        string `json:"status,omitempty"`
		OperationID   string `json:"operation_id"`
		TakeoverToken string `json:"takeover_token"`
	}
	err := h.call(ctx, map[string]any{
		"version": 1, "action": "prepare", "intent_id": envelope.IntentID,
		"task_id": envelope.TaskID, "checkpoint_id": envelope.CheckpointID, "takeover_token": envelope.TakeoverToken,
	}, &out)
	if err == nil && out.Version == 1 && out.Status == "not_prepared" && envelope.TakeoverToken == "" {
		return PreparedSuccessor{}, ErrHostPreparationMissing
	}
	if err != nil || out.Version != 1 || out.OperationID == "" || len(strings.TrimSpace(out.TakeoverToken)) < 32 {
		if err == nil {
			err = errors.New("continuity host returned an invalid prepare response")
		}
		return PreparedSuccessor{}, err
	}
	return PreparedSuccessor{OperationID: out.OperationID, TakeoverToken: out.TakeoverToken}, nil
}

func (h *CommandHost) ActivateSuccessor(ctx context.Context, operationID string) error {
	if !h.Capabilities(ctx).LocalAutoSpawnApproved || operationID == "" {
		return ErrHostCapabilityUnavailable
	}
	var out struct {
		Version int  `json:"version"`
		OK      bool `json:"ok"`
	}
	if err := h.call(ctx, map[string]any{"version": 1, "action": "activate", "operation_id": operationID}, &out); err != nil {
		return err
	}
	if out.Version != 1 || !out.OK {
		return errors.New("continuity host did not activate successor")
	}
	return nil
}

func (h *CommandHost) call(ctx context.Context, input any, output any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	command := exec.CommandContext(callCtx, h.command)
	command.Dir = filepath.Dir(h.command)
	command.Env = hostEnvironment()
	command.Stdin = bytes.NewReader(raw)
	var stdout limitedHostBuffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("continuity host returned trailing output")
	}
	return nil
}

func hostEnvironment() []string {
	out := []string{"PATH=/usr/bin:/bin", "MINDRAIL_CONTINUITY_ADAPTER=1"}
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR"} {
		if value := os.Getenv(name); value != "" {
			out = append(out, name+"="+value)
		}
	}
	return out
}

type limitedHostBuffer struct{ bytes.Buffer }

func (b *limitedHostBuffer) Write(p []byte) (int, error) {
	remaining := hostOutputLimit - b.Len()
	if remaining <= 0 {
		return 0, io.ErrShortWrite
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return remaining, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}
