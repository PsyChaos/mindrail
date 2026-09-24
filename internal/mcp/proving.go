package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolValidate is the twelfth wire name; ToolComplete the thirteenth.
const (
	ToolValidate = "mindrail_validate"
	ToolComplete = "mindrail_complete"
)

func (s *Server) registerProving() {
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolValidate, Description: "Run a named project validation profile to evidence."}, s.validate)
}

// ValidateIn names a project profile. Budget, escalation and approval are
// version-error parameters (decision D-208): present means refusal, absent
// means the default flow.
type ValidateIn struct {
	Profile    string  `json:"profile"`
	Budget     *string `json:"budget,omitempty"`
	Escalation *string `json:"escalation,omitempty"`
	Approval   *string `json:"approval,omitempty"`
}

// EvidenceSummary is one recorded run, bound to its snapshot.
type EvidenceSummary struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Snapshot string `json:"snapshot"`
}

// ValidateOut is the profile's evidence rows.
type ValidateOut struct {
	Profile  string            `json:"profile"`
	Evidence []EvidenceSummary `json:"evidence"`
	Refusal  *Refusal          `json:"refusal,omitempty"`
}

func versionedParam(name string, value *string) *Refusal {
	if value == nil {
		return nil
	}
	return NotImplemented(
		name+" parameter",
		"default flow in 0.1",
		"Call without "+name+"; policy alternatives arrive after this version.")
}

func (s *Server) validate(ctx context.Context, _ *sdk.CallToolRequest, in ValidateIn) (*sdk.CallToolResult, ValidateOut, error) {
	if refusal := versionedParam("budget", in.Budget); refusal != nil {
		return nil, ValidateOut{Evidence: []EvidenceSummary{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("escalation", in.Escalation); refusal != nil {
		return nil, ValidateOut{Evidence: []EvidenceSummary{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("approval", in.Approval); refusal != nil {
		return nil, ValidateOut{Evidence: []EvidenceSummary{}, Refusal: refusal}, nil
	}
	if in.Profile == "" {
		return nil, ValidateOut{}, Invalid("validate needs a named profile")
	}
	profiles := s.app.Config().Config.Validation
	profile, ok := profiles[in.Profile]
	if !ok {
		return nil, ValidateOut{}, Invalid("unknown validation profile: " + in.Profile)
	}
	rows, err := s.valid.RunProfile(ctx, in.Profile,
		profile, s.root, s.app.Config().Config.Secrets.Env, "")
	if err != nil {
		return nil, ValidateOut{}, err
	}
	out := ValidateOut{Profile: in.Profile}
	for _, row := range rows {
		out.Evidence = append(out.Evidence, EvidenceSummary{
			ID: row.ID, Status: row.Status, Snapshot: row.SnapshotHash,
		})
	}
	if out.Evidence == nil {
		out.Evidence = []EvidenceSummary{}
	}
	return nil, out, nil
}
