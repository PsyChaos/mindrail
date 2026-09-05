package bootstrap

import "github.com/PsyChaos/mindrail/internal/doctor"

// Doctor returns the health runner for this process, constructing it at most
// once.
//
// It is lazy because tech-stack §87 step 9 is "initialise managers", not
// "initialise every manager": `mindrail status` never runs a check, and paying
// for a runner it will not use is exactly the kind of eager wiring that turns a
// warm path into a cold one as later milestones add managers behind the same
// step. The construction is guarded rather than merely nil-checked so two
// goroutines asking at once still get the same runner.
func (a *App) Doctor() *doctor.Runner {
	a.doctorOnce.Do(func() {
		a.managerInits.Add(1)
		// The probed subject is kept, not discarded: the probe errors are the
		// only other place in the process that still holds a cause chain, and
		// Diagnose needs them for the failures startup itself never saw — a
		// database path occupied by a directory looks, to startup, exactly like a
		// repository that was never initialised.
		a.doctorSubject = doctor.Probe(a.subject)
		a.doctorRunner = doctor.NewRunner(doctor.DefaultChecks(a.doctorSubject)...)
	})
	return a.doctorRunner
}

// ManagerInitCount reports how many lazy managers have been constructed.
//
// It exists for the laziness assertion and nothing else: "did status avoid
// building the doctor runner?" is otherwise unobservable from outside, and a
// laziness claim no test can fail is not a claim.
func (a *App) ManagerInitCount() int { return int(a.managerInits.Load()) }
