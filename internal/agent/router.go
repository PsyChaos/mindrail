package agent

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

	"github.com/PsyChaos/mindrail/internal/credential"
)

const (
	maxRouteInput        = 64*1024 + 1
	maxRouteOutput       = 1024 * 1024
	credentialReadPeriod = time.Second
	// MaxRouteInputBytes is the maximum stdin prefix passed to the adapter.
	MaxRouteInputBytes = maxRouteInput
	// MaxRouteOutputBytes is the maximum adapter stdout accepted by the router.
	MaxRouteOutputBytes = maxRouteOutput
)

// CredentialStore is the narrow credential capability needed by Router.
type CredentialStore interface {
	Get(context.Context) (string, error)
}

// RouterDependencies exposes bounded seams for hosts and tests. Zero function
// values select the production embedded adapter implementation.
type RouterDependencies struct {
	FindPython func() (string, error)
	RunPython  func(context.Context, string, []byte, string) ([]byte, error)
	Store      CredentialStore
}

// Router resolves the optional JEV credential, invokes the embedded adapter,
// and validates that advice against the caller-supplied candidate set.
type Router struct {
	findPython func() (string, error)
	runPython  func(context.Context, string, []byte, string) ([]byte, error)
	store      CredentialStore
}

// Selection is a validated version-1 advisory selection.
type Selection struct {
	Candidate  *string
	Confidence float64
	Accepted   bool
	Reason     string
	Ordinal    int
}

// RouteResult contains safe typed metadata and the validated version-1 JSON
// record. JSON never contains the resolved credential.
type RouteResult struct {
	JSON             []byte
	Version          int
	Enabled          bool
	Advisory         bool
	Mode             string
	Status           string
	Reason           string
	CredentialSource string
	Selections       map[string]Selection
	CandidateCounts  map[string]int
}

// NewRouter builds the production router around a credential reader.
func NewRouter(store CredentialStore) *Router {
	return NewRouterWithDependencies(RouterDependencies{Store: store})
}

// NewRouterWithDependencies builds a router with explicit bounded seams.
func NewRouterWithDependencies(deps RouterDependencies) *Router {
	if deps.FindPython == nil {
		deps.FindPython = FindTrustedPython
	}
	if deps.RunPython == nil {
		deps.RunPython = runEmbeddedJEV
	}
	return &Router{findPython: deps.FindPython, runPython: deps.RunPython, store: deps.Store}
}

// Route returns advisory data or a safe disabled/fallback record. Operational
// failures are represented in the result so routing can never block normal work.
func (r *Router) Route(ctx context.Context, reader io.Reader) RouteResult {
	if ctx == nil {
		ctx = context.Background()
	}
	key, state, source := r.resolveKey(ctx)
	if state == "missing" {
		return staticResult(false, "disabled", "api_key_missing", source, nil)
	}
	if state == "unavailable" {
		return staticResult(true, "fallback", "credential_unavailable", source, nil)
	}

	python, err := r.findPython()
	if err != nil {
		return staticResult(true, "fallback", "python_unavailable", source, nil)
	}
	input, err := io.ReadAll(io.LimitReader(reader, maxRouteInput))
	if err != nil {
		return staticResult(true, "fallback", "adapter_failure", source, nil)
	}
	allowed, counts, ordinals := allowedCandidates(input)
	raw, runErr := r.runPython(ctx, python, input, key)
	parsed, status, valid := validateResult(raw, key, allowed, counts, ordinals)
	if valid && acceptableExit(status, runErr) {
		parsed.JSON = append([]byte(nil), raw...)
		parsed.CredentialSource = source
		return parsed
	}

	reason := "adapter_failure"
	if runErr != nil && (errors.Is(runErr, exec.ErrNotFound) || errors.Is(runErr, os.ErrNotExist)) {
		reason = "python_unavailable"
	}
	return staticResult(true, "fallback", reason, source, counts)
}

func (r *Router) resolveKey(ctx context.Context) (key, state, source string) {
	if key = os.Getenv("TYPESAFE_API_KEY"); strings.TrimSpace(key) != "" {
		return key, "available", "environment"
	}
	key, state = ReadCredential(ctx, r.store)
	switch state {
	case "available":
		return key, state, "keyring"
	case "unavailable":
		return "", state, "none"
	default:
		return "", state, "none"
	}
}

// ReadCredential reads only the configured credential store. It does not
// inspect process environment, making it safe for hosts with an explicit
// environment snapshot policy.
func ReadCredential(ctx context.Context, store CredentialStore) (key, state string) {
	if store == nil {
		return "", "missing"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(ctx, credentialReadPeriod)
	defer cancel()
	type readResult struct {
		key string
		err error
	}
	result := make(chan readResult, 1)
	go func() {
		value, err := store.Get(readCtx)
		result <- readResult{key: value, err: err}
	}()
	select {
	case value := <-result:
		if errors.Is(value.err, credential.ErrNotFound) || (value.err == nil && strings.TrimSpace(value.key) == "") {
			return "", "missing"
		}
		if value.err != nil {
			return "", "unavailable"
		}
		return value.key, "available"
	case <-readCtx.Done():
		return "", "unavailable"
	}
}

type routeDocument struct {
	Version    *int                       `json:"version"`
	Enabled    *bool                      `json:"enabled"`
	Advisory   *bool                      `json:"advisory"`
	Mode       string                     `json:"mode"`
	Status     string                     `json:"status"`
	Reason     string                     `json:"reason"`
	Selections map[string]json.RawMessage `json:"selections"`
}

type routeSelection struct {
	Candidate  *string  `json:"candidate"`
	Confidence *float64 `json:"confidence"`
	Accepted   *bool    `json:"accepted"`
	Reason     string   `json:"reason"`
}

func validateResult(raw []byte, key string, allowed map[string]map[string]bool, counts map[string]int, ordinals map[string]map[string]int) (RouteResult, string, bool) {
	if len(raw) == 0 || len(raw) > maxRouteOutput {
		return RouteResult{}, "", false
	}
	var document routeDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return RouteResult{}, "", false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return RouteResult{}, "", false
	}
	if document.Version == nil || *document.Version != 1 || document.Advisory == nil ||
		!*document.Advisory || document.Enabled == nil || document.Mode != "shadow" ||
		document.Reason == "" || document.Selections == nil {
		return RouteResult{}, "", false
	}
	if document.Status != "ok" && document.Status != "fallback" && document.Status != "disabled" {
		return RouteResult{}, "", false
	}
	if (document.Status == "disabled") == *document.Enabled ||
		(document.Status == "disabled" && len(document.Selections) != 0) ||
		(document.Status == "ok" && len(document.Selections) == 0) {
		return RouteResult{}, "", false
	}

	selections := make(map[string]Selection, len(document.Selections))
	for dimension, rawSelection := range document.Selections {
		candidates, known := allowed[dimension]
		if !known {
			return RouteResult{}, "", false
		}
		var selection routeSelection
		selectionDecoder := json.NewDecoder(bytes.NewReader(rawSelection))
		selectionDecoder.DisallowUnknownFields()
		if selectionDecoder.Decode(&selection) != nil || selection.Confidence == nil ||
			selection.Accepted == nil || selection.Reason == "" ||
			math.IsNaN(*selection.Confidence) || math.IsInf(*selection.Confidence, 0) ||
			*selection.Confidence < 0 || *selection.Confidence > 1 {
			return RouteResult{}, "", false
		}
		if selection.Candidate != nil && !candidates[*selection.Candidate] {
			return RouteResult{}, "", false
		}
		if document.Status == "ok" {
			if selection.Candidate == nil || !*selection.Accepted || selection.Reason != "accepted" {
				return RouteResult{}, "", false
			}
		} else if *selection.Accepted {
			return RouteResult{}, "", false
		}
		ordinal := -1
		if selection.Candidate != nil {
			ordinal = ordinals[dimension][*selection.Candidate]
		}
		selections[dimension] = Selection{
			Candidate: selection.Candidate, Confidence: *selection.Confidence,
			Accepted: *selection.Accepted, Reason: selection.Reason, Ordinal: ordinal,
		}
	}
	var value any
	if json.Unmarshal(raw, &value) != nil || jsonContainsSecret(value, key) {
		return RouteResult{}, "", false
	}
	return RouteResult{
		Version: *document.Version, Enabled: *document.Enabled, Advisory: *document.Advisory,
		Mode: document.Mode, Status: document.Status, Reason: document.Reason,
		Selections: selections, CandidateCounts: counts,
	}, document.Status, true
}

func allowedCandidates(input []byte) (map[string]map[string]bool, map[string]int, map[string]map[string]int) {
	type candidate struct {
		ID string `json:"id"`
	}
	var caller struct {
		Tools   []candidate `json:"tools"`
		Agents  []candidate `json:"agents"`
		Models  []candidate `json:"models"`
		Efforts []candidate `json:"efforts"`
	}
	allowed := map[string]map[string]bool{}
	counts := map[string]int{}
	ordinals := map[string]map[string]int{}
	if json.Unmarshal(input, &caller) != nil {
		return allowed, counts, ordinals
	}
	for dimension, candidates := range map[string][]candidate{
		"tool": caller.Tools, "agent": caller.Agents, "model": caller.Models, "effort": caller.Efforts,
	} {
		if len(candidates) == 0 {
			continue
		}
		allowed[dimension] = map[string]bool{}
		ordinals[dimension] = map[string]int{}
		counts[dimension] = len(candidates)
		for index, item := range candidates {
			allowed[dimension][item.ID] = true
			if _, exists := ordinals[dimension][item.ID]; !exists {
				ordinals[dimension][item.ID] = index
			}
		}
	}
	return allowed, counts, ordinals
}

func staticResult(enabled bool, status, reason, source string, counts map[string]int) RouteResult {
	if counts == nil {
		counts = map[string]int{}
	}
	document := map[string]any{
		"version": 1, "enabled": enabled, "advisory": true, "mode": "shadow",
		"status": status, "reason": reason, "selections": map[string]any{},
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(document)
	return RouteResult{
		JSON: output.Bytes(), Version: 1, Enabled: enabled, Advisory: true, Mode: "shadow",
		Status: status, Reason: reason, CredentialSource: source,
		Selections: map[string]Selection{}, CandidateCounts: counts,
	}
}

func acceptableExit(status string, err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return status == "fallback" && errors.As(err, &exitErr) && exitErr.ExitCode() == 2
}

func jsonContainsSecret(value any, key string) bool {
	switch item := value.(type) {
	case string:
		return strings.Contains(item, key)
	case []any:
		for _, child := range item {
			if jsonContainsSecret(child, key) {
				return true
			}
		}
	case map[string]any:
		for name, child := range item {
			if strings.Contains(name, key) || jsonContainsSecret(child, key) {
				return true
			}
		}
	}
	return false
}

func runEmbeddedJEV(ctx context.Context, python string, input []byte, key string) ([]byte, error) {
	command := exec.CommandContext(ctx, python, "-I", "-c", JevRouteSource())
	var output boundedBuffer
	command.Stdin = bytes.NewReader(input)
	command.Env = childEnvironment(key)
	command.Stdout = &output
	command.Stderr = io.Discard
	err := command.Run()
	return output.Bytes(), err
}

func childEnvironment(key string) []string {
	return []string{"TYPESAFE_API_KEY=" + key}
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := maxRouteOutput - b.Len()
	if remaining <= 0 {
		return 0, io.ErrShortWrite
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return remaining, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}

// FindTrustedPython returns an administrator-controlled Python executable and
// deliberately ignores PATH-controlled repository shims.
func FindTrustedPython() (string, error) {
	for _, candidate := range []string{
		"/usr/bin/python3", "/usr/local/bin/python3", "/opt/homebrew/bin/python3",
	} {
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || repositoryAncestor(filepath.Dir(resolved)) != "" {
			continue
		}
		return resolved, nil
	}
	return "", exec.ErrNotFound
}

func repositoryAncestor(dir string) string {
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
