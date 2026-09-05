package doctor

import (
	"context"
	"slices"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Result is one check's reading.
//
// Diagnostic, Impact and NextAction are separate fields rather than one
// formatted paragraph for the same reason app.DomainError splits them: the CLI,
// the JSON envelope and a later MCP consumer each need to present them
// differently, and none of them should have to parse prose. Every result whose
// State is not OK carries all three (spec §84).
type Result struct {
	Name       string            `json:"name"`
	Section    string            `json:"section"`
	State      State             `json:"state"`
	Summary    string            `json:"summary"`
	Diagnostic string            `json:"diagnostic,omitempty"`
	Impact     string            `json:"impact,omitempty"`
	NextAction []string          `json:"next_action,omitempty"`
	Code       app.Code          `json:"code,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	DurationMS int64             `json:"duration_ms"`

	// Metadata is the machine-readable detail the layer that detected the
	// failure attached to it — the directory git was asked about, the path that
	// is occupied, the step that stopped the run. It is carried through the
	// Result rather than reconstructed later because the producer is the only
	// layer that knows it, and a report that dropped it leaves the reader
	// unable to answer "which directory did Mindrail actually look at?"
	// (acceptance criterion 3).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Check is one health reading. Run takes a context because the interface has to
// accommodate a check that talks to something slow; the MR-001 checks are pure
// functions of a Subject and never block.
type Check interface {
	Name() string
	Run(ctx context.Context) Result
}

// CheckFunc adapts a function to Check. It carries the identity so a check body
// only has to describe what it found, and the name and section can never
// disagree between Names() and the report.
type CheckFunc struct {
	CheckName    string
	CheckSection string
	Fn           func(context.Context) Result
}

func (c CheckFunc) Name() string { return c.CheckName }

// Run stamps the check's identity onto whatever the function reported. A check
// registered without a function reports itself as unavailable rather than
// passing as healthy: silence about a check that never ran is the one answer a
// health report must not give.
func (c CheckFunc) Run(ctx context.Context) Result {
	if c.Fn == nil {
		return Result{
			Name:       c.CheckName,
			Section:    c.CheckSection,
			State:      StateUnavailable,
			Summary:    "Check has no implementation",
			Diagnostic: "The check was registered without a function, so it never ran.",
			Impact:     "This part of the installation is unreported; its health is unknown.",
			NextAction: []string{"Register this check with a non-nil Fn."},
			// The same code a check blocked by an aborted startup carries: both
			// are readings that were never taken, and neither says anything
			// about the subsystem it names.
			Code: app.CodeStartupIncomplete,
		}
	}

	result := c.Fn(ctx)
	if result.Name == "" {
		result.Name = c.CheckName
	}
	if result.Section == "" {
		result.Section = c.CheckSection
	}
	return result
}

// Runner holds an ordered check set and produces one report from it.
type Runner struct {
	checks []Check
}

// NewRunner registers checks in report order. The slice is cloned so a caller
// that reuses its argument cannot rewrite the check set afterwards.
func NewRunner(checks ...Check) *Runner {
	return &Runner{checks: slices.Clone(checks)}
}

// Register appends a check. Order is the report order, so registration order is
// part of the output contract.
func (r *Runner) Register(c Check) {
	r.checks = append(r.checks, c)
}

// Names returns the registered check names in report order.
func (r *Runner) Names() []string {
	names := make([]string, 0, len(r.checks))
	for _, c := range r.checks {
		names = append(names, c.Name())
	}
	return names
}

// Run executes every check in order and returns the report.
//
// The context is passed to each check rather than consulted here: a cancelled
// run still reports every check, because a truncated health report is harder to
// act on than a complete one that says a probe was cancelled. Checks that can
// block are responsible for honouring the context themselves.
func (r *Runner) Run(ctx context.Context) Report {
	startedAll := time.Now()

	results := make([]Result, 0, len(r.checks))

	// A runner with no checks reports OK: it found nothing wrong because it
	// looked at nothing, and inventing a worse verdict would be a claim the run
	// did not earn. With checks, the worst reading wins outright — including
	// NOT_APPLICABLE, so a report made entirely of inapplicable readings does
	// not round itself up to healthy.
	worst := StateOK
	for i, c := range r.checks {
		started := time.Now()
		result := c.Run(ctx)
		result.DurationMS = time.Since(started).Milliseconds()

		if i == 0 || severity(result.State) > severity(worst) {
			worst = result.State
		}
		results = append(results, result)
	}

	return Report{
		Command:    "doctor",
		Checks:     results,
		WorstState: worst,
		DurationMS: time.Since(startedAll).Milliseconds(),
	}
}

// Report is one doctor run.
type Report struct {
	Command    string   `json:"command"`
	Checks     []Result `json:"checks"`
	WorstState State    `json:"worst_state"`
	DurationMS int64    `json:"duration_ms"`
}

// Err returns the domain error of the first ERROR check, or nil.
//
// DEGRADED, UNAVAILABLE and NOT_APPLICABLE never produce an error here
// (decision D-14): §84 forbids treating an absent optional capability as fatal,
// and a doctor that exited non-zero on a repository with no index would train
// every pipeline to ignore its exit code.
//
// It is the report-only half of the answer. A command asks Verdict, which adds
// the one case a report cannot see for itself: a startup sequence that never
// finished.
func (r Report) Err() error {
	for _, result := range r.Checks {
		if result.State != StateError {
			continue
		}
		return errorFrom(result)
	}
	return nil
}

// Verdict is the one value `ok`, the `error` object and the process exit code
// are all derived from (finding F11).
//
// Before it existed those three were computed in three places from three
// inputs: `ok` from the doctor report's ERROR checks, the exit code from
// whichever of that verdict and the startup error was non-nil, and the error
// object from the first of them alone. A git binary that is missing or hangs
// produces no ERROR check — decision D-30 makes it UNAVAILABLE so a slow
// network filesystem cannot fail an otherwise correct pipeline — so the
// envelope said {"ok": true} with no data while the process exited 4.
//
// Two things make a reading fatal, and only two.
//
// An ERROR reading is fatal wherever it appears (decision D-14): a check that
// looked and found something broken has to be visible to CI.
//
// A reading is also fatal when the tech-stack §87 startup sequence stopped at
// the step that check reports on. That is the discriminator D-14 needs and did
// not have: "the runtime database has not been created yet" and "git is not
// installed" are both UNAVAILABLE, but the first is a state a completed startup
// observed and reported — decision D-03's one zero-exit row — while the second
// is a sequence that never ran. A halted start means no command can do its work,
// whatever the halting reading's state word says.
//
// Nothing else is promoted. A DEGRADED cache directory, a knowledge record this
// binary cannot parse and an uninitialised repository all leave the sequence
// intact and all still exit 0.
func Verdict(s Subject, r Report) error {
	if err := r.Err(); err != nil {
		return err
	}

	h, halted := s.haltedAt()
	if !halted {
		return nil
	}

	for _, result := range r.Checks {
		if result.Name == h.check && result.State != StateOK {
			return errorFrom(result)
		}
	}
	// The halting check is always registered and always reports the failure that
	// stopped the run, so this is unreachable in practice. Returning nil anyway
	// would reopen exactly the hole this function closes, so the halt speaks for
	// itself instead.
	return haltError(h)
}

// errorFrom projects one non-OK reading onto the process error carrier.
//
// The producer's metadata is carried over before the check name is stamped on.
// Replacing it with `{"check": ...}` used to discard details only the failing
// layer could supply — the git adapter's start_dir most of all, which is the
// answer to "which directory did Mindrail probe?" and is unanswerable from the
// shell prompt when the binary runs under -C or from a Git hook.
func errorFrom(result Result) error {
	why := result.Diagnostic
	if why == "" {
		why = result.Summary
	}

	domain := app.NewError(result.Code, kindForCode(result.Code), why, result.Impact, result.NextAction...)
	for key, value := range result.Metadata {
		domain = domain.WithMetadata(key, value)
	}
	// Last, so a producer cannot shadow the identity of the check reporting it.
	return domain.WithMetadata("check", result.Name)
}

// haltError describes a startup that stopped without the halting check saying
// so. It is the backstop of Verdict and asserts nothing it cannot know.
func haltError(h halt) error {
	return app.NewError(
		h.code,
		kindForCode(h.code),
		"startup stopped at the "+string(h.step)+" step",
		"No command can run until the step that stopped the startup sequence completes.",
		"Run `mindrail doctor` and resolve the failure reported by the "+h.check+" check.",
	).WithMetadata("stopped_at_step", string(h.step)).
		WithMetadata("check", h.check)
}

// kindForCode maps a failure code onto its exit class (decision D-03).
//
// The mapping lives here rather than in app because app is a leaf that knows
// nothing about which subsystem raised a code; doctor is the first layer that
// sees them all together. Codes not listed are ordinary operation failures:
// a migration that did not run, a schema this binary cannot read, a workspace
// that could not be recorded. All of those ran and failed, which is exit 1.
func kindForCode(c app.Code) app.Kind {
	switch c {
	case app.CodeNotAGitRepository, app.CodeBareRepository, app.CodeConfigInvalid, app.CodePathEscapesRoot:
		// Deterministic, user-correctable placement or configuration.
		return app.KindUsage
	case app.CodeGitUnavailable, app.CodeGitTimeout, app.CodeRuntimePathUnwritable,
		app.CodeRuntimeDBUnavailable, app.CodeRuntimeDBCorrupt, app.CodeStartupIncomplete:
		// Environment conditions: a missing tool, an unwritable path, a database
		// that is locked or damaged.
		return app.KindUnavailable
	default:
		return app.KindFailed
	}
}
