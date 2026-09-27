// Package gate unifies completion judgment: one ALLOW/DENY model over the
// rows, verdicts and findings earlier milestones produced (MR-013). The
// gate is a pure composer — it calls no store, reads no clock, and judges
// values only — so the same inputs decide byte-identical outputs. Drivers
// sequence discovery before it; MR-014's tools and MR-017's CI read it.
package gate

import (
	"sort"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// InvariantBlock is one invariant↔symbol binding with the scope that
// decides hardness. Denies iff the binding is unsettled (ambiguous or
// orphaned) under an active HIGH/CRITICAL invariant — the MR-006 D-98
// rule, judged here instead of stored anywhere.
type InvariantBlock struct {
	InvariantID string
	UID         string
	Status      string
	Severity    string
	Active      bool
}

// Ambiguity is one protected-symbol ambiguity: several candidates for one
// key, optionally watched by an invariant. Denies iff an active
// HIGH/CRITICAL invariant points at it (spec line 1467); warn-scope
// ambiguities stay outside the gate entirely (AC-01.5).
type Ambiguity struct {
	Key         string
	Candidates  []string
	InvariantID string
	Severity    string
	Active      bool
}

// Input is everything the gate judges, as plain values. Empty means clean.
type Input struct {
	Bindings    []InvariantBlock
	Ambiguities []Ambiguity
	Attribution []changes.Finding
	Coverage    validation.Coverage
	Guard       []testguard.Finding
}

// Denial is one reason completion is refused: a deterministic code,
// a human handle, a compact provenance pointer and remedies.
type Denial struct {
	Code       app.Code
	Reason     string
	Key        string
	Provenance string
	NextAction []string
}

// Decision is the verdict. No timestamp rides it: idempotency is
// structural (decision D-182).
type Decision struct {
	Allow   bool
	Denials []Denial
}

// Service evaluates completion. It holds nothing; construct once, share.
type Service struct{}

// New builds a Service.
func New() *Service {
	return &Service{}
}

// Evaluate collects denials in fixed family order, sorts for byte
// stability, and allows iff none remain. Pure: no I/O, no clock.
// Malformed blocking inputs (vacuous findings, unknown binding statuses)
// refuse loudly instead of denying vaguely or passing silently: a safety
// gate must never launder caller garbage in either direction.
func (s *Service) Evaluate(input Input) (Decision, error) {
	var denials []Denial
	invariant, err := invariantDenials(input.Bindings)
	if err != nil {
		return Decision{}, err
	}
	denials = append(denials, invariant...)
	denials = append(denials, ambiguityDenials(input.Ambiguities)...)
	attribution, err := attributionDenials(input.Attribution)
	if err != nil {
		return Decision{}, err
	}
	denials = append(denials, attribution...)
	denials = append(denials, evidenceDenials(input.Coverage)...)
	guard, err := guardDenials(input.Guard)
	if err != nil {
		return Decision{}, err
	}
	denials = append(denials, guard...)
	sort.Slice(denials, func(i, j int) bool {
		if denials[i].Code != denials[j].Code {
			return denials[i].Code < denials[j].Code
		}
		if denials[i].Key != denials[j].Key {
			return denials[i].Key < denials[j].Key
		}
		return denials[i].Reason < denials[j].Reason
	})
	denials = dedupe(denials)
	return Decision{Allow: len(denials) == 0, Denials: denials}, nil
}

// dedupe collapses byte-identical denials: drivers may report one fact
// twice, but the decision lists it once.
func dedupe(denials []Denial) []Denial {
	seen := map[string]bool{}
	var out []Denial
	for _, denial := range denials {
		flat := string(denial.Code) + "\x00" + denial.Reason + "\x00" + denial.Key + "\x00" +
			denial.Provenance + "\x00" + strings.Join(denial.NextAction, "\x00")
		if seen[flat] {
			continue
		}
		seen[flat] = true
		out = append(out, denial)
	}
	return out
}

func invariantDenials(bindings []InvariantBlock) ([]Denial, error) {
	var out []Denial
	for _, binding := range bindings {
		if binding.Status == index.BindingBound || !binding.Active || !hardSeverity(binding.Severity) {
			continue
		}
		code := app.CodeOrphanedProtectedSymbol
		reason := "protected symbol has no confident heir"
		remedy := "migrate, supersede or retire the invariant explicitly"
		switch binding.Status {
		case index.BindingOrphaned:
		case index.BindingAmbiguous:
			code = app.CodeSymbolIdentityAmbiguous
			reason = "protected symbol identity cannot be settled"
			remedy = "assign the identity explicitly among the candidates"
		default:
			return nil, invalidInput("unknown binding status: " + binding.Status)
		}
		out = append(out, Denial{
			Code:       code,
			Reason:     reason,
			Key:        binding.InvariantID,
			Provenance: "invariant binding " + binding.InvariantID + " over " + binding.UID,
			NextAction: []string{remedy + " for " + binding.InvariantID + "."},
		})
	}
	return out, nil
}

func ambiguityDenials(ambiguities []Ambiguity) []Denial {
	var out []Denial
	for _, ambiguity := range ambiguities {
		if len(ambiguity.Candidates) < 2 || !ambiguity.Active || !hardSeverity(ambiguity.Severity) {
			continue
		}
		out = append(out, Denial{
			Code:       app.CodeSymbolIdentityAmbiguous,
			Reason:     "ambiguous symbol watched by an active invariant",
			Key:        ambiguity.Key,
			Provenance: "ambiguity over " + ambiguity.Key + " (" + strconv.Itoa(len(ambiguity.Candidates)) + " candidates)",
			NextAction: []string{"Assign " + ambiguity.Key + " explicitly to one identity."},
		})
	}
	return out
}

func attributionDenials(findings []changes.Finding) ([]Denial, error) {
	var out []Denial
	for _, finding := range findings {
		if !finding.Blocking {
			continue
		}
		if finding.Code == "" || finding.Provenance.ChangeKey == "" {
			return nil, invalidInput("blocking attribution finding without code or key")
		}
		out = append(out, Denial{
			Code:       finding.Code,
			Reason:     finding.Detail,
			Key:        finding.Provenance.ChangeKey,
			Provenance: "change " + finding.Provenance.ChangeID + " of task " + finding.Provenance.TaskID,
			NextAction: finding.NextAction,
		})
	}
	return out, nil
}

func evidenceDenials(coverage validation.Coverage) []Denial {
	var out []Denial
	for _, profile := range coverage.Required {
		if coverage.Satisfied[profile] {
			continue
		}
		reason := "no current evidence"
		for _, item := range coverage.ReRun {
			if item.Profile == profile && item.Reason != "" {
				reason = item.Reason
				break
			}
		}
		out = append(out, Denial{
			Code:       app.CodeRequiredEvidenceNotCurrent,
			Reason:     reason,
			Key:        profile,
			Provenance: "required profile " + profile + " without current evidence",
			NextAction: []string{"Run the " + profile + " validation profile and re-check completion."},
		})
	}
	return out
}

func guardDenials(findings []testguard.Finding) ([]Denial, error) {
	var out []Denial
	for _, finding := range findings {
		if !finding.Blocking {
			continue
		}
		if finding.TestKey == "" {
			return nil, invalidInput("blocking guard finding without test key")
		}
		out = append(out, Denial{
			Code:       app.CodeTestGuardWeakened,
			Reason:     finding.Reason + " (" + finding.Signal + ")",
			Key:        finding.TestKey,
			Provenance: "guard finding over " + finding.TestKey,
			NextAction: []string{"Restore the weakened test or record an explicit allowance for " + finding.TestKey + "."},
		})
	}
	return out, nil
}

func hardSeverity(severity string) bool {
	return severity == "HIGH" || severity == "CRITICAL"
}

func invalidInput(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No completion facts were changed.", "Pass valid bindings, findings and coverage.")
}
