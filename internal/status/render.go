package status

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
)

// RenderHuman writes the readable form of a status report.
//
// Every field it prints has a JSON counterpart (decision D-11): `--json` is the
// same report, not a lesser one, so no consumer is ever pushed into scraping
// this text. The whole report is assembled before a single write, so a failing
// writer cannot leave half a report on a terminal.
func (r Report) RenderHuman(w io.Writer, color bool) error {
	var b strings.Builder

	b.WriteString("Mindrail Status\n\n")
	b.WriteString("Readiness: ")
	b.WriteString(paintReadiness(r.Readiness, string(r.Readiness), color))
	b.WriteString("\n")

	if r.StoppedAtStep != "" {
		b.WriteString("Startup stopped at: ")
		b.WriteString(r.StoppedAtStep)
		b.WriteString(" (everything after it is unreported, not absent)\n")
	}
	if r.BlockingComponent != "" {
		b.WriteString("Blocking component: ")
		b.WriteString(string(r.BlockingComponent))
		b.WriteString("\n")
	}
	writeLines(&b, "Next", r.NextAction)

	writeSection(&b, "Repository", []field{
		{"Observation", observationNote(r.Repository.Observation)},
		{"Common dir", r.Repository.CommonDir},
		{"Worktree root", r.Repository.WorktreeRoot},
		{"Git dir", r.Repository.GitDir},
		{"Linked worktree", observed(r.Repository.Observation, strconv.FormatBool(r.Repository.IsLinkedWorktree))},
		{"Git version", r.Repository.GitVersion},
	})
	writeSection(&b, "Runtime", []field{
		{"Observation", observationNote(r.Runtime.Observation)},
		{"DB path", r.Runtime.DBPath},
		{"Cache dir", r.Runtime.CacheDir},
		{"Schema version", observed(r.Runtime.Observation, strconv.FormatInt(r.Runtime.SchemaVersion, 10))},
		{"Journal mode", r.Runtime.JournalMode},
		{"Initialized", observed(r.Runtime.Observation, strconv.FormatBool(r.Runtime.Initialized))},
	})
	writeSection(&b, "Knowledge", []field{
		{"Observation", observationNote(r.Knowledge.Observation)},
		{"Present", observed(r.Knowledge.Observation, strconv.FormatBool(r.Knowledge.Present))},
		{"Decisions", observed(r.Knowledge.Observation, strconv.Itoa(r.Knowledge.Decisions))},
		{"Invariants", observed(r.Knowledge.Observation, strconv.Itoa(r.Knowledge.Invariants))},
		{"Problems", observed(r.Knowledge.Observation, strconv.Itoa(r.Knowledge.Problems))},
		{"Findings", observed(r.Knowledge.Observation, strconv.Itoa(r.Knowledge.Findings))},
		{"Write schema version", strconv.Itoa(r.Knowledge.WriteSchemaVersion)},
		{"Readable schema versions", joinInts(r.Knowledge.ReadableSchemaVersions)},
	})
	writeSection(&b, "Workspace", []field{
		{"Observation", observationNote(r.Workspace.Observation)},
		{"Registered", observed(r.Workspace.Observation, strconv.FormatBool(r.Workspace.Registered))},
		{"Workspace id", r.Workspace.ID},
		{"Project id", r.Workspace.ProjectID},
	})
	writeSection(&b, "Coordination", []field{
		{"Observation", observationNote(r.Coordination.Observation)},
		{"Tasks open", observed(r.Coordination.Observation, strconv.Itoa(r.Coordination.TasksOpen))},
		{"Tasks in progress", observed(r.Coordination.Observation, strconv.Itoa(r.Coordination.TasksInProgress))},
		{"Tasks blocked", observed(r.Coordination.Observation, strconv.Itoa(r.Coordination.TasksBlocked))},
		{"Leases active", observed(r.Coordination.Observation, strconv.Itoa(r.Coordination.LeasesActive))},
		{"Last checkpoint", lastCheckpointNote(r.Coordination)},
	})

	b.WriteString("\nComponents\n")
	for _, name := range componentOrder {
		writeComponent(&b, name, r.Components[name], color)
	}

	b.WriteString("\nDuration: ")
	b.WriteString(strconv.FormatInt(r.DurationMS, 10))
	b.WriteString("ms\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// RenderHuman writes what init did and ends with the spec §82 terminal line.
// Nothing is printed after that line: it is the sentence a human reads last and
// a CI job greps for, so anything below it would be noise in the one place that
// must be unambiguous.
func (r InitReport) RenderHuman(w io.Writer, color bool) error {
	var b strings.Builder

	b.WriteString("Mindrail Init\n\n")

	// "Absent" has two meanings on all three of these lines, and only one of
	// them was told apart. A run that found the config already there wrote
	// nothing, and so did a run that stopped before the scaffold step — and the
	// second one printed "Config: (preserved)" and "Knowledge directories:
	// already present" about files that are not on disk at all (finding H13),
	// which is the same defect the migration line below was fixed for
	// (finding F14). ConfigPresent and KnowledgeDirsPresent are what tell them
	// apart; each is set only when the step that establishes it returned.
	switch {
	case r.ConfigCreated:
		fmt.Fprintf(&b, "Config: %s (created)\n", r.ConfigPath)
	case r.ConfigPresent:
		fmt.Fprintf(&b, "Config: %s (preserved)\n", r.ConfigPath)
	case r.ConfigPath != "":
		fmt.Fprintf(&b, "Config: %s (not written)\n", r.ConfigPath)
	default:
		b.WriteString("Config: not written (init stopped before the repository scaffold step)\n")
	}

	switch {
	case len(r.KnowledgeDirsCreated) > 0:
		b.WriteString("Knowledge directories created:\n")
		for _, dir := range r.KnowledgeDirsCreated {
			b.WriteString("  ")
			b.WriteString(dir)
			b.WriteString("\n")
		}
	case r.KnowledgeDirsPresent:
		b.WriteString("Knowledge directories: already present\n")
	case r.ConfigPresent:
		// "Absent" has a third meaning this line did not have a word for. The
		// configuration is written and the knowledge directories are created by
		// one step, in that order, so a run that established the first and not the
		// second stopped *inside* the scaffold step — and saying it stopped
		// "before" that step tells the reader to look for a failure one stage
		// earlier than the one the report itself is about. It is the same defect
		// as H13 and F14, in the branch those fixes left behind.
		b.WriteString("Knowledge directories: not created (init stopped while laying down the repository scaffold)\n")
	default:
		b.WriteString("Knowledge directories: not created (init stopped before the repository scaffold step)\n")
	}

	// "none" has two meanings and they are not interchangeable. A run that
	// found the schema already complete applied nothing, and so did a run that
	// stopped before a database existed — and the second one printed "schema
	// already current" about a schema nobody had established (finding F14).
	// SchemaCurrent is what tells them apart; it is set only when the ledger was
	// actually read and found complete.
	switch {
	case len(r.MigrationsApplied) > 0:
		b.WriteString("Migrations applied:\n")
		for _, applied := range r.MigrationsApplied {
			fmt.Fprintf(&b, "  %06d %s  %s\n", applied.Version, applied.Name, app.FormatTime(applied.AppliedAt))
		}
	case r.SchemaCurrent:
		b.WriteString("Migrations applied: none (schema already current)\n")
	default:
		b.WriteString("Migrations applied: none (the runtime schema was not established)\n")
	}

	b.WriteString("\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return err
	}
	if err := r.Status.RenderHuman(w, color); err != nil {
		return err
	}

	terminal := string(r.TerminalState)
	if r.TerminalState == TerminalBlocked && r.Reason != "" {
		terminal += ": " + r.Reason
	}
	_, err := io.WriteString(w, "\n"+terminal+"\n")
	return err
}

// field is one labelled value in a section.
type field struct {
	label string
	value string
}

// writeSection prints a titled block with the labels aligned. A field with no
// value is skipped rather than printed as an empty colon: the JSON omits the
// same keys, so the two views stay in agreement about what is unknown.
func writeSection(b *strings.Builder, title string, fields []field) {
	present := make([]field, 0, len(fields))
	width := 0
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		present = append(present, f)
		if len(f.label) > width {
			width = len(f.label)
		}
	}
	if len(present) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString(title)
	b.WriteString("\n")
	for _, f := range present {
		b.WriteString("  ")
		b.WriteString(pad(f.label+":", width+1))
		b.WriteString(" ")
		b.WriteString(f.value)
		b.WriteString("\n")
	}
}

// componentNameWidth aligns the component column. It is derived rather than
// hard-coded so a component added by a later MR cannot silently break the
// alignment.
var componentNameWidth = func() int {
	width := 0
	for _, name := range componentOrder {
		if len(name) > width {
			width = len(name)
		}
	}
	return width
}()

// stateWordWidth reserves the column the state word occupies, so the summaries
// of OK and non-OK components line up in the same place.
var stateWordWidth = len(doctor.StateNotApplicable)

// writeComponent prints one component line and, when it is not healthy, the
// remedy that goes with it. Per decision D-21 the state word is printed next to
// the glyph for everything except OK, so nothing about the report's meaning
// depends on the glyph rendering.
func writeComponent(b *strings.Builder, name ComponentName, c Component, color bool) {
	word := ""
	if c.State != doctor.StateOK {
		word = string(c.State)
	}

	b.WriteString("  ")
	b.WriteString(paint(c.State, c.State.Marker(), color))
	b.WriteString(" ")
	b.WriteString(pad(string(name), componentNameWidth))
	b.WriteString("  ")
	b.WriteString(pad(word, stateWordWidth))
	b.WriteString("  ")
	b.WriteString(c.Summary)
	b.WriteString("\n")

	for _, action := range c.NextAction {
		b.WriteString("      Next: ")
		b.WriteString(action)
		b.WriteString("\n")
	}
}

// writeLines prints a labelled list, or nothing when there is nothing to say.
func writeLines(b *strings.Builder, label string, lines []string) {
	if len(lines) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString(label)
	b.WriteString(":\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// observationNote labels a block whose values are not findings. It is empty for
// an observed block, so writeSection drops the row: a line on every report
// saying the healthy answer is trustworthy would be noise on the one answer
// nobody has to double-check.
//
// The two non-observed cases print different sentences because they are
// different situations, and the remedy differs with them: nobody looked yet, or
// somebody looked and the subsystem refused to answer.
func observationNote(o Observation) string {
	switch o {
	case NotObserved:
		return "not observed — startup stopped before this subsystem was read"
	case Indeterminate:
		return "indeterminate — this subsystem was read and could not answer"
	default:
		return ""
	}
}

// observed qualifies a value the report cannot vouch for. Printing the zero of a
// field nobody filled in as a bare fact is how a report ends up asserting
// "initialized: false" about a repository whose database it merely failed to
// open (finding F15).
//
// It used to replace the value with the word "unknown", and that produced a
// second defect one layer over: `mindrail status` printed `Initialized: unknown`
// while `mindrail status --json`, on the same repository at the same instant,
// published `"initialized": false`. Two renderings of one report stated
// different things, and a reader who compared them had to work out from the
// source which of the two the tool actually believed.
//
// So the value is printed, and the observation is printed beside it, in the
// exact spelling the JSON member carries. Both views now say the same value and
// the same qualifier, and a reader can diff one against the other.
//
// Emitting JSON null for these members was the other way to make them agree, and
// it is the better answer in the abstract: null is what "unknown" is in JSON.
// It was not taken here because it turns six report members into pointers on a
// wire contract that consumers already read correctly through `observation` —
// which the type documentation has said qualifies every field beside it since
// the field existed — and this pass was not the one to spend a contract change
// on. The disagreement is what was reported, and the disagreement is gone.
func observed(o Observation, value string) string {
	if o.Known() {
		return value
	}
	return value + " (" + string(o) + ")"
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func joinInts(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ", ")
}

// ANSI attributes. Colour is applied only to the state glyph and the readiness
// word, so stripping the escapes leaves the report byte-identical.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"
	ansiDim    = "\x1b[2m"
)

func paint(state doctor.State, text string, color bool) string {
	if !color {
		return text
	}

	switch state {
	case doctor.StateOK:
		return ansiGreen + text + ansiReset
	case doctor.StateDegraded:
		return ansiYellow + text + ansiReset
	case doctor.StateError:
		return ansiRed + text + ansiReset
	case doctor.StateUnavailable:
		return ansiBlue + text + ansiReset
	case doctor.StateNotApplicable:
		return ansiDim + text + ansiReset
	default:
		return text
	}
}

func paintReadiness(readiness Readiness, text string, color bool) string {
	if !color {
		return text
	}

	switch readiness {
	case ReadinessReady:
		return ansiGreen + text + ansiReset
	case ReadinessPartialReady, ReadinessDegraded:
		return ansiYellow + text + ansiReset
	case ReadinessBlocked:
		return ansiRed + text + ansiReset
	default:
		return text
	}
}

// lastCheckpointNote renders the newest handover note's provenance for a human.
//
// It names the task and the session rather than quoting the note. status is a
// fixed-size report and an agent's free text is the one value in it with no
// bound on its length; `mindrail task show` is where the note itself belongs.
func lastCheckpointNote(info CoordinationInfo) string {
	if !info.Observation.Known() {
		// "unknown", not "". observed() concatenates value + " (" + observation
		// + ")", so an empty value left the line reading `Last checkpoint:
		// (not_observed)` — four spaces after the colon against the section's
		// three, on every `mindrail init` (finding F49). The word is also the
		// honest one: with no reading there is no checkpoint to report and no
		// claim that there is none.
		return observed(info.Observation, "unknown")
	}
	if info.LastCheckpoint == nil {
		return "none"
	}
	return info.LastCheckpoint.TaskID + " by " + info.LastCheckpoint.SessionID
}
