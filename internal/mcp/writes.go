package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
)

// DecideIn records one decision. Title and decision are required; the rest
// rides the record's optional fields.
type DecideIn struct {
	Title        string `json:"title"`
	Decision     string `json:"decision"`
	Context      string `json:"context,omitempty"`
	Consequences string `json:"consequences,omitempty"`
}

// DecideOut names the persisted record.
type DecideOut struct {
	ID      string   `json:"id,omitempty"`
	Path    string   `json:"path,omitempty"`
	Refusal *Refusal `json:"refusal,omitempty"`
}

// InvariantIn records one active invariant. Mode is the lifecycle state
// and only "active" exists in 0.1; severity and scope level ride the
// invariant's required fields, so they are required here too — no default
// classification is invented.
type InvariantIn struct {
	Mode        string `json:"mode"`
	Statement   string `json:"statement"`
	Severity    string `json:"severity,omitempty"`
	ScopeLevel  string `json:"scope_level,omitempty"`
	ScopeTarget string `json:"scope_target,omitempty"`
	Rationale   string `json:"rationale,omitempty"`
}

// InvariantOut names the persisted record.
type InvariantOut struct {
	ID      string   `json:"id,omitempty"`
	Path    string   `json:"path,omitempty"`
	Refusal *Refusal `json:"refusal,omitempty"`
}

var decisionIDNumber = regexp.MustCompile(`^DEC-([0-9]+)$`)
var invariantIDNumber = regexp.MustCompile(`^INV-([0-9]+)$`)

func (s *Server) decide(_ context.Context, _ *sdk.CallToolRequest, in DecideIn) (*sdk.CallToolResult, DecideOut, error) {
	if in.Title == "" || in.Decision == "" {
		return nil, DecideOut{}, Invalid("decide needs a title and a decision")
	}
	id, err := allocateID(s.knowledgeDir("decisions"), "DEC", decisionIDNumber)
	if err != nil {
		return nil, DecideOut{}, err
	}
	made, err := record.NewDecision(id, time.Now().UTC(), in.Title, in.Decision,
		record.WithContext(in.Context), record.WithConsequences(in.Consequences))
	if err != nil {
		return nil, DecideOut{}, Invalid("decide input invalid: " + err.Error())
	}
	path, err := persistRecord(s.root, "decisions", id, made)
	if err != nil {
		return nil, DecideOut{}, err
	}
	return nil, DecideOut{ID: id, Path: path}, nil
}

func (s *Server) invariant(_ context.Context, _ *sdk.CallToolRequest, in InvariantIn) (*sdk.CallToolResult, InvariantOut, error) {
	if in.Mode == "" {
		return nil, InvariantOut{}, Invalid("invariant needs a mode")
	}
	if in.Mode != "active" {
		return nil, InvariantOut{Refusal: NotImplemented(
			"invariant mode "+in.Mode,
			"active",
			"Record the invariant with mode active; candidate proposals arrive after this version.")}, nil
	}
	if in.Statement == "" {
		return nil, InvariantOut{}, Invalid("active invariant needs a statement")
	}
	id, err := allocateID(s.knowledgeDir("invariants"), "INV", invariantIDNumber)
	if err != nil {
		return nil, InvariantOut{}, err
	}
	made, err := record.NewInvariant(id, time.Now().UTC(), in.Statement,
		record.WithSeverity(record.Severity(in.Severity)),
		record.WithScope(record.Scope{Level: record.ScopeLevel(in.ScopeLevel), Target: in.ScopeTarget}),
		record.WithRationale(in.Rationale))
	if err != nil {
		return nil, InvariantOut{}, Invalid("invariant input invalid: " + err.Error())
	}
	path, err := persistRecord(s.root, "invariants", id, made)
	if err != nil {
		return nil, InvariantOut{}, err
	}
	return nil, InvariantOut{ID: id, Path: path}, nil
}

func (s *Server) knowledgeDir(bucket string) string {
	return filepath.Join(s.root, ".mindrail", "knowledge", bucket)
}

// allocateID assigns max+1 inside one bucket, zero-padded to four. Agents
// never guess ids. Allocation and write are separate steps; the write is
// create-exclusive, so a lost race reports instead of duplicating
// (best effort in 0.1, decision D-187).
func allocateID(dir, prefix string, pattern *regexp.Regexp) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			entries, err = os.ReadDir(dir)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	max := 0
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 5 || name[len(name)-5:] != ".json" {
			continue
		}
		match := pattern.FindStringSubmatch(name[:len(name)-5])
		if match == nil {
			continue
		}
		if n, err := strconv.Atoi(match[1]); err == nil && n > max {
			max = n
		}
	}
	number := strconv.Itoa(max + 1)
	for len(number) < 4 {
		number = "0" + number
	}
	return prefix + "-" + number, nil
}

// persistRecord writes canonical record JSON create-exclusively: the file
// must not exist, so a concurrent allocator loses loudly instead of
// overwriting. The returned path is repo-relative, slash-spelled, matching
// the loader's vocabulary.
func persistRecord(root, bucket, id string, document any) (string, error) {
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", Invalid("record is not representable")
	}
	raw = append(raw, '\n')
	dir := filepath.Join(root, ".mindrail", "knowledge", bucket)
	abs := filepath.Join(dir, id+".json")
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("record id collision on %s: retry the call", id)
		}
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(raw); err != nil {
		return "", err
	}
	return "knowledge/" + bucket + "/" + id + ".json", nil
}
