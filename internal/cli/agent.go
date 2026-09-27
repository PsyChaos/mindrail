package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	agentasset "github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const (
	maxAgentRouteInput       = 64*1024 + 1
	maxAgentRouteOutput      = 1024 * 1024
	jevCredentialReadTimeout = time.Second
)

type agentCommandDeps struct {
	findPython func() (string, error)
	runPython  func(context.Context, string, []byte, string) ([]byte, error)
	store      credential.Store
}

func newAgentCommand() *cobra.Command {
	return newAgentCommandWith(agentCommandDeps{
		findPython: findTrustedPython,
		runPython:  runEmbeddedJEV,
		store:      credential.NewOSStore(),
	})
}

func newAgentCommandWith(deps agentCommandDeps) *cobra.Command {
	agent := &cobra.Command{
		Use:    "agent",
		Short:  "Internal coding-agent integrations",
		Args:   cobra.NoArgs,
		Hidden: true,
	}
	route := &cobra.Command{
		Use:    "route",
		Short:  "Return optional JEV routing advice",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentRoute(cmd, deps)
		},
	}
	agent.AddCommand(route)
	return agent
}

func runAgentRoute(cmd *cobra.Command, deps agentCommandDeps) error {
	key, keyState := resolveJEVKey(cmd.Context(), deps.store)
	if keyState == "missing" {
		return writeAgentResult(cmd.OutOrStdout(), disabledAgentResult())
	}
	if keyState == "unavailable" {
		return writeAgentResult(cmd.OutOrStdout(), fallbackAgentResult("credential_unavailable"))
	}

	python, err := deps.findPython()
	if err != nil {
		return writeAgentResult(cmd.OutOrStdout(), fallbackAgentResult("python_unavailable"))
	}

	input, readErr := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxAgentRouteInput))
	if readErr != nil {
		return writeAgentResult(cmd.OutOrStdout(), fallbackAgentResult("adapter_failure"))
	}
	raw, runErr := deps.runPython(cmd.Context(), python, input, key)
	status, valid := validateAgentResult(raw, key, allowedRouteCandidates(input))
	if valid && acceptableAdapterExit(status, runErr) {
		if len(raw) == 0 || raw[len(raw)-1] != '\n' {
			raw = append(raw, '\n')
		}
		_, err = cmd.OutOrStdout().Write(raw)
		return err
	}

	reason := "adapter_failure"
	if runErr != nil && (errors.Is(runErr, exec.ErrNotFound) || errors.Is(runErr, os.ErrNotExist)) {
		reason = "python_unavailable"
	}
	return writeAgentResult(cmd.OutOrStdout(), fallbackAgentResult(reason))
}

func resolveJEVKey(ctx context.Context, store credential.Store) (string, string) {
	if key := os.Getenv("TYPESAFE_API_KEY"); strings.TrimSpace(key) != "" {
		return key, "available"
	}
	return readJEVKey(ctx, store)
}

func readJEVKey(ctx context.Context, store credential.Store) (string, string) {
	if store == nil {
		return "", "missing"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(ctx, jevCredentialReadTimeout)
	defer cancel()
	type readResult struct {
		key string
		err error
	}
	result := make(chan readResult, 1)
	go func() {
		key, err := store.Get(readCtx)
		result <- readResult{key: key, err: err}
	}()

	var key string
	var err error
	select {
	case value := <-result:
		key, err = value.key, value.err
	case <-readCtx.Done():
		return "", "unavailable"
	}
	if errors.Is(err, credential.ErrNotFound) || (err == nil && strings.TrimSpace(key) == "") {
		return "", "missing"
	}
	if err != nil {
		return "", "unavailable"
	}
	return key, "available"
}

func disabledAgentResult() map[string]any {
	return map[string]any{
		"version": 1, "enabled": false, "advisory": true, "mode": "shadow",
		"status": "disabled", "reason": "api_key_missing", "selections": map[string]any{},
	}
}

func fallbackAgentResult(reason string) map[string]any {
	return map[string]any{
		"version": 1, "enabled": true, "advisory": true, "mode": "shadow",
		"status": "fallback", "reason": reason, "selections": map[string]any{},
	}
}

func writeAgentResult(w io.Writer, result map[string]any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

type agentRouteResult struct {
	Version    *int                       `json:"version"`
	Enabled    *bool                      `json:"enabled"`
	Advisory   *bool                      `json:"advisory"`
	Mode       string                     `json:"mode"`
	Status     string                     `json:"status"`
	Reason     string                     `json:"reason"`
	Selections map[string]json.RawMessage `json:"selections"`
}

type agentRouteSelection struct {
	Candidate  *string  `json:"candidate"`
	Confidence *float64 `json:"confidence"`
	Accepted   *bool    `json:"accepted"`
	Reason     string   `json:"reason"`
}

func validateAgentResult(raw []byte, key string, allowed map[string]map[string]bool) (string, bool) {
	if len(raw) == 0 || len(raw) > maxAgentRouteOutput {
		return "", false
	}
	var result agentRouteResult
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", false
	}
	if result.Version == nil || *result.Version != 1 || result.Advisory == nil ||
		!*result.Advisory || result.Enabled == nil || result.Mode != "shadow" ||
		result.Reason == "" || result.Selections == nil {
		return "", false
	}
	if result.Status != "ok" && result.Status != "fallback" && result.Status != "disabled" {
		return "", false
	}
	if (result.Status == "disabled") == *result.Enabled {
		return "", false
	}
	if result.Status == "disabled" && len(result.Selections) != 0 {
		return "", false
	}
	if result.Status == "ok" && len(result.Selections) == 0 {
		return "", false
	}
	for dimension, rawSelection := range result.Selections {
		candidates, knownDimension := allowed[dimension]
		if !knownDimension {
			return "", false
		}
		var selection agentRouteSelection
		selectionDecoder := json.NewDecoder(bytes.NewReader(rawSelection))
		selectionDecoder.DisallowUnknownFields()
		if selectionDecoder.Decode(&selection) != nil || selection.Confidence == nil ||
			selection.Accepted == nil || selection.Reason == "" ||
			math.IsNaN(*selection.Confidence) || math.IsInf(*selection.Confidence, 0) ||
			*selection.Confidence < 0 || *selection.Confidence > 1 {
			return "", false
		}
		if selection.Candidate != nil && !candidates[*selection.Candidate] {
			return "", false
		}
		if result.Status == "ok" {
			if selection.Candidate == nil || !*selection.Accepted || selection.Reason != "accepted" {
				return "", false
			}
		} else if *selection.Accepted {
			return "", false
		}
	}
	var document any
	valid := json.Unmarshal(raw, &document) == nil && !jsonValueContainsSecret(document, key)
	return result.Status, valid
}

func allowedRouteCandidates(input []byte) map[string]map[string]bool {
	var caller struct {
		Tools []struct {
			ID string `json:"id"`
		} `json:"tools"`
		Agents []struct {
			ID string `json:"id"`
		} `json:"agents"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
		Efforts []struct {
			ID string `json:"id"`
		} `json:"efforts"`
	}
	if json.Unmarshal(input, &caller) != nil {
		return map[string]map[string]bool{}
	}
	allowed := map[string]map[string]bool{}
	for dimension, candidates := range map[string][]struct {
		ID string `json:"id"`
	}{
		"tool": caller.Tools, "agent": caller.Agents, "model": caller.Models, "effort": caller.Efforts,
	} {
		if len(candidates) == 0 {
			continue
		}
		allowed[dimension] = map[string]bool{}
		for _, candidate := range candidates {
			allowed[dimension][candidate.ID] = true
		}
	}
	return allowed
}

func acceptableAdapterExit(status string, err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return status == "fallback" && errors.As(err, &exitErr) && exitErr.ExitCode() == 2
}

func jsonValueContainsSecret(value any, key string) bool {
	switch item := value.(type) {
	case string:
		return strings.Contains(item, key)
	case []any:
		for _, child := range item {
			if jsonValueContainsSecret(child, key) {
				return true
			}
		}
	case map[string]any:
		for name, child := range item {
			if strings.Contains(name, key) || jsonValueContainsSecret(child, key) {
				return true
			}
		}
	}
	return false
}

func runEmbeddedJEV(ctx context.Context, python string, input []byte, key string) ([]byte, error) {
	command := exec.CommandContext(ctx, python, "-I", "-c", agentasset.JevRouteSource())
	var output boundedBuffer
	command.Stdin = bytes.NewReader(input)
	command.Env = jevChildEnvironment(key)
	command.Stdout = &output
	command.Stderr = io.Discard
	err := command.Run()
	return output.Bytes(), err
}

func jevChildEnvironment(key string) []string {
	// The interpreter path is absolute and the adapter needs no inherited
	// process settings. In particular, proxy, TLS trust, and dynamic-loader
	// variables must not reach the secret-bearing child.
	return []string{"TYPESAFE_API_KEY=" + key}
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := maxAgentRouteOutput - b.Len()
	if remaining <= 0 {
		return 0, io.ErrShortWrite
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return remaining, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

func findTrustedPython() (string, error) {
	// Do not use PATH here. A repository can put its own `python3` first in
	// PATH; launching it would disclose the inherited API key before the
	// embedded adapter starts. These are administrator-controlled installation
	// locations on the POSIX platforms supported by the adapter's deadline.
	for _, candidate := range []string{
		"/usr/bin/python3",
		"/usr/local/bin/python3",
		"/opt/homebrew/bin/python3",
	} {
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || repositoryAncestorFrom(filepath.Dir(resolved)) != "" {
			continue
		}
		return resolved, nil
	}
	return "", exec.ErrNotFound
}

func repositoryAncestorFrom(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
