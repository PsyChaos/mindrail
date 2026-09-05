package bootstrap

import (
	"log/slog"
	"slices"
)

// Step names one stage of the tech-stack §87 startup sequence.
//
// The sequence is a contract, not an implementation detail: every later
// milestone adds work to one of these stages, and a diagnostic that says "we
// failed at open_sqlite" is only useful if the name means the same thing in
// every binary. The values are therefore the wire spelling, not a description
// of it.
type Step string

const (
	StepResolveRepository   Step = "resolve_repository"
	StepLoadConfig          Step = "load_config"
	StepResolveRuntimePaths Step = "resolve_runtime_paths"
	StepOpenSQLite          Step = "open_sqlite"
	StepMigrateDB           Step = "migrate_db"
	StepValidateKnowledge   Step = "validate_knowledge"
	StepRegisterWorkspace   Step = "register_workspace"
	StepLoadIndexState      Step = "load_index_state"
	StepInitManagers        Step = "init_managers"
	StepExecuteCommand      Step = "execute_command"
)

// orderedSteps is the §87 sequence itself. Order is the whole point of the
// list: repository discovery decides where the runtime state lives, so nothing
// that touches the database may be reordered above it.
var orderedSteps = []Step{
	StepResolveRepository,
	StepLoadConfig,
	StepResolveRuntimePaths,
	StepOpenSQLite,
	StepMigrateDB,
	StepValidateKnowledge,
	StepRegisterWorkspace,
	StepLoadIndexState,
	StepInitManagers,
	StepExecuteCommand,
}

// Steps returns the ten steps of tech-stack §87, in order, as a copy.
func Steps() []Step {
	return slices.Clone(orderedSteps)
}

// Recorder observes the startup sequence. It exists so a test can assert on the
// order the steps really ran in rather than on the state they happened to
// leave behind: a pipeline that produced the right result from the wrong order
// would still be wrong, because the ordering is what makes fail-fast honest.
type Recorder interface{ Step(s Step) }

// record announces one step. Every step goes to the log as well as to the
// recorder, so a user debugging a slow or failing start sees the same sequence
// the tests assert on rather than a separate, prettier story.
func (a *App) record(s Step) {
	a.logger.Debug("startup step", slog.String("step", string(s)), slog.String("mode", a.opts.Mode.String()))
	if a.opts.Recorder != nil {
		a.opts.Recorder.Step(s)
	}
}
