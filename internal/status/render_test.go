package status

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/migration"
)

var updateGolden = flag.Bool("update", false, "rewrite the testdata goldens from the current output")

// TestStatusHumanNamesCommonDirAndWorktree is acceptance criterion 3 on the
// human surface: knowing the Git layout is worth nothing if the report does not
// say it out loud, and the common dir and the active worktree are different
// answers that a linked worktree makes visibly different.
func TestStatusHumanNamesCommonDirAndWorktree(t *testing.T) {
	subject := healthySubject()
	subject.Repo.CommonDir = "/repo/.git"
	subject.Repo.GitDir = "/repo/.git/worktrees/feature"
	subject.Repo.WorktreeRoot = "/repo-feature"
	subject.Repo.IsLinkedWorktree = true

	var buf bytes.Buffer
	if err := Build(subject, 3*time.Millisecond).RenderHuman(&buf, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"Common dir:",
		"/repo/.git",
		"Worktree root:",
		"/repo-feature",
		"Git dir:",
		"/repo/.git/worktrees/feature",
		"Linked worktree:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("human status does not name %q:\n%s", want, got)
		}
	}

	if strings.ContainsRune(got, 0x1b) {
		t.Error("human status contains an ANSI escape with color disabled")
	}
}

// TestStatusJSONParityWithHuman pins decision D-11's promise that `--json` is
// not a lesser view: everything the human report states has a key a machine can
// read, so no consumer is ever forced to scrape the prose.
func TestStatusJSONParityWithHuman(t *testing.T) {
	healthy := Build(healthySubject(), 3*time.Millisecond)
	blocked := Build(uninitializedSubject(), 3*time.Millisecond)

	rendered := map[string]string{
		"healthy": renderHuman(t, healthy),
		"blocked": renderHuman(t, blocked),
	}
	decoded := map[string]map[string]any{
		"healthy": decodeJSON(t, healthy),
		"blocked": decodeJSON(t, blocked),
	}

	tests := []struct {
		label string
		path  string
	}{
		{label: "Readiness:", path: "readiness"},
		{label: "Blocking component:", path: "blocking_component"},
		{label: "Next:", path: "next_action"},
		{label: "Common dir:", path: "repository.common_dir"},
		{label: "Worktree root:", path: "repository.worktree_root"},
		{label: "Git dir:", path: "repository.git_dir"},
		{label: "Linked worktree:", path: "repository.is_linked_worktree"},
		{label: "Git version:", path: "repository.git_version"},
		{label: "DB path:", path: "runtime.db_path"},
		{label: "Cache dir:", path: "runtime.cache_dir"},
		{label: "Schema version:", path: "runtime.schema_version"},
		{label: "Journal mode:", path: "runtime.journal_mode"},
		{label: "Initialized:", path: "runtime.initialized"},
		{label: "Present:", path: "knowledge.present"},
		{label: "Decisions:", path: "knowledge.decisions"},
		{label: "Invariants:", path: "knowledge.invariants"},
		{label: "Problems:", path: "knowledge.problems"},
		{label: "Findings:", path: "knowledge.findings"},
		{label: "Write schema version:", path: "knowledge.write_schema_version"},
		{label: "Readable schema versions:", path: "knowledge.readable_schema_versions"},
		{label: "Registered:", path: "workspace.registered"},
		{label: "Workspace id:", path: "workspace.workspace_id"},
		{label: "Project id:", path: "workspace.project_id"},
		{label: "Duration:", path: "duration_ms"},
		{label: "knowledge", path: "components.knowledge.state"},
		{label: "inventory", path: "components.inventory.state"},
		{label: "syntax", path: "components.syntax.state"},
		{label: "semantic", path: "components.semantic.state"},
		{label: "coverage_map", path: "components.coverage_map.state"},
		{label: "runtime_db", path: "components.runtime_db.state"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			shown := ""
			for _, name := range []string{"healthy", "blocked"} {
				if strings.Contains(rendered[name], tc.label) {
					shown = name
					break
				}
			}
			if shown == "" {
				t.Fatalf("no rendered report shows %q", tc.label)
			}
			if _, ok := lookup(decoded[shown], tc.path); !ok {
				t.Errorf("the %s report shows %q but its JSON has no %q", shown, tc.label, tc.path)
			}
		})
	}
}

// TestStatusRenderHumanGolden pins the layout of the report a human reads.
func TestStatusRenderHumanGolden(t *testing.T) {
	assertGolden(t, "status_ready_human.golden", renderHuman(t, Build(healthySubject(), 3*time.Millisecond)))
	assertGolden(t, "status_blocked_human.golden", renderHuman(t, Build(uninitializedSubject(), 3*time.Millisecond)))
}

// TestStatusRenderHumanColorStaysOutOfTheContent proves the coloured and plain
// renderings differ only in escape sequences.
func TestStatusRenderHumanColorStaysOutOfTheContent(t *testing.T) {
	report := Build(uninitializedSubject(), 3*time.Millisecond)

	var coloured bytes.Buffer
	if err := report.RenderHuman(&coloured, true); err != nil {
		t.Fatalf("RenderHuman(color=true): %v", err)
	}
	if !strings.ContainsRune(coloured.String(), 0x1b) {
		t.Error("coloured rendering emitted no escape sequences")
	}
	if stripped := stripANSI(coloured.String()); stripped != renderHuman(t, report) {
		t.Errorf("colour changed the content:\n--- stripped ---\n%s", stripped)
	}
}

// TestInitReportEndsWithTerminalState pins spec §82: init's last line is the
// literal a human and a CI grep for, and nothing prints after it.
func TestInitReportEndsWithTerminalState(t *testing.T) {
	tests := []struct {
		name     string
		report   InitReport
		wantLast string
		absent   string
	}{
		{
			name:     "ready",
			report:   readyInitReport(),
			wantLast: "READY FOR TARGETED WORK",
			absent:   "BLOCKED:",
		},
		{
			name:     "blocked",
			report:   blockedInitReport(),
			wantLast: "BLOCKED: the runtime database has not been created",
			absent:   "READY FOR TARGETED WORK",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tc.report.RenderHuman(&buf, false); err != nil {
				t.Fatalf("RenderHuman: %v", err)
			}
			got := buf.String()

			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			if last := lines[len(lines)-1]; last != tc.wantLast {
				t.Errorf("last line = %q, want %q\n%s", last, tc.wantLast, got)
			}
			if strings.Contains(got, tc.absent) {
				t.Errorf("output contains %q, which contradicts the terminal state:\n%s", tc.absent, got)
			}
			if strings.ContainsRune(got, 0x1b) {
				t.Error("init report contains an ANSI escape with color disabled")
			}
		})
	}
}

// TestInitReportNamesThePreservedConfig pins spec §82's "existing config is not
// silently overwritten": a second init has to say which file it left alone,
// otherwise "nothing happened" and "your settings were kept" look identical.
func TestInitReportNamesThePreservedConfig(t *testing.T) {
	report := readyInitReport()
	report.ConfigCreated = false
	report.KnowledgeDirsCreated = nil
	report.MigrationsApplied = nil

	var buf bytes.Buffer
	if err := report.RenderHuman(&buf, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, report.ConfigPath) {
		t.Errorf("output does not name the config file %q:\n%s", report.ConfigPath, got)
	}
	if !strings.Contains(got, "preserved") {
		t.Errorf("output does not say the existing config was preserved:\n%s", got)
	}
}

// TestInitReportNeverClaimsAScaffoldItDidNotEstablish is finding H13.
//
// "Nothing was created" has the same two meanings on the config line and the
// knowledge-directories line that it had on the migrations line one line below
// them, and only the migrations line was ever taught to tell them apart
// (finding F14). A run blocked before the scaffold step created no config file
// and no directories, and reported "Config: (preserved)" and "Knowledge
// directories: already present" about a `.mindrail` that is not on disk — the
// two sentences a user checks to find out whether their own settings survived.
func TestInitReportNeverClaimsAScaffoldItDidNotEstablish(t *testing.T) {
	report := blockedInitReport()
	report.ConfigCreated = false
	report.ConfigPresent = false
	report.KnowledgeDirsCreated = nil
	report.KnowledgeDirsPresent = false

	var buf bytes.Buffer
	if err := report.RenderHuman(&buf, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	got := buf.String()

	for _, forbidden := range []string{"preserved", "already present"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("init reports %q about a scaffold it never wrote:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "Config: ") || !strings.Contains(got, "Knowledge directories: ") {
		t.Errorf("init dropped the scaffold lines instead of qualifying them:\n%s", got)
	}
}

// TestInitReportStillReportsAScaffoldItDidEstablish is the over-fire guard for
// finding H13, in both directions.
//
// A second init has to say the existing config was preserved — spec §82's
// promise is unverifiable otherwise — and a first init has to say it created
// one. Qualifying the lines must not cost either sentence.
func TestInitReportStillReportsAScaffoldItDidEstablish(t *testing.T) {
	tests := map[string]struct {
		mutate func(*InitReport)
		want   []string
		absent []string
	}{
		"a second init that changed nothing": {
			mutate: func(r *InitReport) {
				r.ConfigCreated = false
				r.KnowledgeDirsCreated = nil
			},
			want:   []string{"preserved", "already present"},
			absent: []string{"not written", "not created"},
		},
		"a first init that created everything": {
			mutate: func(*InitReport) {},
			want:   []string{"created", ".mindrail/knowledge/decisions"},
			absent: []string{"not written", "not created", "already present"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			report := readyInitReport()
			tc.mutate(&report)

			var buf bytes.Buffer
			if err := report.RenderHuman(&buf, false); err != nil {
				t.Fatalf("RenderHuman: %v", err)
			}
			got := buf.String()

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("output does not say %q:\n%s", want, got)
				}
			}
			for _, forbidden := range tc.absent {
				if strings.Contains(got, forbidden) {
					t.Errorf("output says %q about work it did do:\n%s", forbidden, got)
				}
			}
		})
	}
}

// TestInitReportGolden pins both terminal outcomes byte for byte.
func TestInitReportGolden(t *testing.T) {
	for name, report := range map[string]InitReport{
		"init_ready_human.golden":   readyInitReport(),
		"init_blocked_human.golden": blockedInitReport(),
	} {
		var buf bytes.Buffer
		if err := report.RenderHuman(&buf, false); err != nil {
			t.Fatalf("RenderHuman: %v", err)
		}
		assertGolden(t, name, buf.String())
	}
}

// TestInitReportJSONKeys pins the machine-readable init contract.
func TestInitReportJSONKeys(t *testing.T) {
	encoded, err := json.MarshalIndent(readyInitReport(), "", "  ")
	if err != nil {
		t.Fatalf("marshal init report: %v", err)
	}
	assertGolden(t, "init_ready_json.golden", string(encoded)+"\n")

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode init report: %v", err)
	}
	for _, key := range []string{
		"command", "terminal_state", "config_path", "config_created",
		"knowledge_dirs_created", "migrations_applied", "schema_current",
		"status", "duration_ms",
	} {
		if _, ok := document[key]; !ok {
			t.Errorf("init report has no %q key", key)
		}
	}
	if document["command"] != "init" {
		t.Errorf("command = %v, want %q", document["command"], "init")
	}
	if document["terminal_state"] != string(TerminalReady) {
		t.Errorf("terminal_state = %v, want %q", document["terminal_state"], TerminalReady)
	}
}

func readyInitReport() InitReport {
	return InitReport{
		Command:       "init",
		TerminalState: TerminalReady,
		ConfigPath:    "/repo/.mindrail/config.toml",
		ConfigCreated: true,
		// The scaffold step ran and returned, which is what licenses the report
		// to describe the config file and the knowledge directories as being on
		// disk at all (finding H13). A fixture that left these false was
		// describing a run that never reached the scaffold while asserting it
		// had created one.
		ConfigPresent: true,
		KnowledgeDirsCreated: []string{
			".mindrail/knowledge/decisions",
			".mindrail/knowledge/invariants",
		},
		KnowledgeDirsPresent: true,
		MigrationsApplied: []migration.Applied{{
			Version:   1,
			Name:      "initial",
			Checksum:  "3c0d6b1f",
			AppliedAt: fixedInstant,
		}},
		SchemaCurrent: true,
		Status:        Build(healthySubject(), 3*time.Millisecond),
		DurationMS:    12,
	}
}

func blockedInitReport() InitReport {
	return InitReport{
		Command:              "init",
		TerminalState:        TerminalBlocked,
		Reason:               "the runtime database has not been created",
		ConfigPath:           "/repo/.mindrail/config.toml",
		ConfigCreated:        true,
		ConfigPresent:        true,
		KnowledgeDirsCreated: nil,
		KnowledgeDirsPresent: true,
		MigrationsApplied:    nil,
		Status:               Build(uninitializedSubject(), 3*time.Millisecond),
		DurationMS:           9,
	}
}

func renderHuman(t *testing.T, report Report) string {
	t.Helper()

	var buf bytes.Buffer
	if err := report.RenderHuman(&buf, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	return buf.String()
}

func decodeJSON(t *testing.T, report Report) map[string]any {
	t.Helper()

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	// Tech-stack §12: structured output never carries terminal escapes, so a
	// consumer never has to strip them before parsing.
	if bytes.ContainsRune(encoded, 0x1b) {
		t.Errorf("JSON report contains an ANSI escape: %s", encoded)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return document
}

// lookup walks a dotted path through a decoded JSON document.
func lookup(document map[string]any, path string) (any, bool) {
	var current any = document
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run `go test ./internal/status -update` to create it): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			out.WriteByte(s[i])
			continue
		}
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return out.String()
}

// TestHumanReportCarriesTheObservationMarker is finding W8.
//
// The Observation marker is the field that stops a reader taking "Registered:
// unknown" for "Registered: false". The JSON half of it was tested; the human
// half was not, and forcing observationNote to return the empty string deleted
// the whole line from every human report while the suite stayed green — which
// means the marker was, for the reader who actually reads the text output,
// untested.
//
// The sentence itself is asserted, not merely the label. The two non-observed
// cases are different situations with different remedies, and a marker that
// printed the same words for both would be a line that says nothing.
func TestHumanReportCarriesTheObservationMarker(t *testing.T) {
	cases := []struct {
		observation Observation
		want        string
	}{
		{NotObserved, "not observed — startup stopped before this subsystem was read"},
		{Indeterminate, "indeterminate — this subsystem was read and could not answer"},
	}

	for _, tc := range cases {
		t.Run(string(tc.observation), func(t *testing.T) {
			report := Build(healthySubject(), 3*time.Millisecond)
			report.Workspace.Observation = tc.observation
			report.Workspace.Registered = true

			var buf bytes.Buffer
			if err := report.RenderHuman(&buf, false); err != nil {
				t.Fatalf("RenderHuman: %v", err)
			}
			got := buf.String()

			if !strings.Contains(got, "Observation:") {
				t.Fatalf("human report has no Observation line for a %s block:\n%s", tc.observation, got)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("human report does not say %q for a %s block:\n%s", tc.want, tc.observation, got)
			}
			// The marker exists to stop the zero being read as a finding, so the
			// value it qualifies has to be hidden in the same breath.
			if strings.Contains(got, "Registered:  true") {
				t.Errorf("human report publishes Registered: true under a %s observation:\n%s",
					tc.observation, got)
			}
		})
	}
}

// TestHumanReportOmitsTheObservationMarkerWhenItLooked is the over-fire guard
// for the assertion above.
//
// A line on every report saying the healthy answer is trustworthy would be noise
// on the one answer nobody has to double-check, and a marker printed everywhere
// is a marker that distinguishes nothing. The blocked report is checked in the
// same test so that "no marker anywhere" cannot pass it.
func TestHumanReportOmitsTheObservationMarkerWhenItLooked(t *testing.T) {
	var healthy bytes.Buffer
	if err := Build(healthySubject(), 3*time.Millisecond).RenderHuman(&healthy, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	if strings.Contains(healthy.String(), "Observation:") {
		t.Errorf("a healthy report marks its own findings as unreliable:\n%s", healthy.String())
	}

	blocked := Build(healthySubject(), 3*time.Millisecond)
	blocked.Runtime.Observation = Indeterminate
	var out bytes.Buffer
	if err := blocked.RenderHuman(&out, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	if !strings.Contains(out.String(), "Observation:") {
		t.Fatalf("the marker never appears at all, so its absence above proves nothing:\n%s", out.String())
	}
}

// TestInitReportTellsTheTwoScaffoldSilencesApart is the W10 half of findings
// H13 and F14.
//
// "Not created" had two meanings and only one had a sentence. A run that never
// reached the scaffold step creates nothing, and so does a run that wrote the
// configuration and then failed on the knowledge directories — the two are one
// step apart on disk and were reported identically, sending a reader whose init
// failed *inside* the scaffold step to look for a failure before it.
//
// ConfigPresent is what tells them apart: the configuration and the knowledge
// directories are established by one step, in that order, so a report that has
// the first and not the second stopped between them.
func TestInitReportTellsTheTwoScaffoldSilencesApart(t *testing.T) {
	tests := map[string]struct {
		configPresent bool
		want          string
		absent        string
	}{
		// Both sentences are matched in full. `absent` used to be the fragment
		// "stopped before", which reads as this report's scaffold silence and is
		// also two words of the coordination block's "startup stopped before
		// this subsystem was read" — so once a fixture that genuinely has not
		// observed coordination reached this renderer, the assertion failed over
		// a sentence about a different subsystem.
		"stopped before the scaffold step": {
			configPresent: false,
			want:          "init stopped before the repository scaffold step",
			absent:        "init stopped while laying down the repository scaffold",
		},
		"stopped inside the scaffold step": {
			configPresent: true,
			want:          "init stopped while laying down the repository scaffold",
			absent:        "init stopped before the repository scaffold step",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			report := blockedInitReport()
			report.ConfigCreated = false
			report.ConfigPresent = tc.configPresent
			report.KnowledgeDirsCreated = nil
			report.KnowledgeDirsPresent = false

			var buf bytes.Buffer
			if err := report.RenderHuman(&buf, false); err != nil {
				t.Fatalf("RenderHuman: %v", err)
			}
			got := buf.String()

			if !strings.Contains(got, tc.want) {
				t.Errorf("output does not say %q:\n%s", tc.want, got)
			}
			if strings.Contains(got, tc.absent) {
				t.Errorf("output says %q about the other silence:\n%s", tc.absent, got)
			}
			// Neither silence may be dressed up as a scaffold that exists.
			if strings.Contains(got, "already present") {
				t.Errorf("output claims knowledge directories that were never created:\n%s", got)
			}
		})
	}
}
