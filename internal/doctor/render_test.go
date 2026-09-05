package doctor

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
)

var updateGolden = flag.Bool("update", false, "rewrite the testdata goldens from the current output")

// TestReportRenderHumanGolden pins the spec §83 layout. The report is rendered
// from a subject that is healthy except for two checks, so the golden carries
// both the clean line shape and the Diagnostic/Impact/Next block.
func TestReportRenderHumanGolden(t *testing.T) {
	report := goldenReport(t)

	var buf bytes.Buffer
	if err := report.RenderHuman(&buf, false); err != nil {
		t.Fatalf("RenderHuman: %v", err)
	}
	got := buf.String()

	assertGolden(t, "doctor_report_human.golden", got)

	for _, want := range []string{
		"Mindrail Doctor",
		"Repository",
		"✓ Git common-dir detected",
		"Diagnostic:",
		"Impact:",
		"Next:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("human report does not contain %q:\n%s", want, got)
		}
	}

	// Decision D-21: the state vocabulary is asserted on the word, never on the
	// glyph, so the assertion survives a font, a locale or a terminal that
	// renders the markers differently.
	for _, result := range report.Checks {
		if result.State == StateOK {
			continue
		}
		if !strings.Contains(got, string(result.State)) {
			t.Errorf("check %q reported %q but the word is absent from the human report", result.Name, result.State)
		}
	}
	if !strings.Contains(got, string(report.WorstState)) {
		t.Errorf("worst state %q is absent from the human report", report.WorstState)
	}

	if strings.ContainsRune(got, 0x1b) {
		t.Error("human report contains an ANSI escape with color disabled")
	}
}

// TestRenderHumanColorStaysOutOfTheContent proves the coloured and uncoloured
// renderings differ only in escape sequences: a pipe and a terminal must read
// the same report.
func TestRenderHumanColorStaysOutOfTheContent(t *testing.T) {
	report := goldenReport(t)

	var plain, coloured bytes.Buffer
	if err := report.RenderHuman(&plain, false); err != nil {
		t.Fatalf("RenderHuman(color=false): %v", err)
	}
	if err := report.RenderHuman(&coloured, true); err != nil {
		t.Fatalf("RenderHuman(color=true): %v", err)
	}

	if !strings.ContainsRune(coloured.String(), 0x1b) {
		t.Error("coloured rendering emitted no escape sequences")
	}
	if stripped := stripANSI(coloured.String()); stripped != plain.String() {
		t.Errorf("colour changed the content:\n--- stripped ---\n%s\n--- plain ---\n%s", stripped, plain.String())
	}
}

// TestReportJSONHasNoANSIAndStableKeys pins the machine-readable shape. Agents
// and CI branch on these keys, so a rename is a breaking change and has to fail
// here rather than in a consumer.
func TestReportJSONHasNoANSIAndStableKeys(t *testing.T) {
	report := goldenReport(t)

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	got := string(encoded) + "\n"

	assertGolden(t, "doctor_report_json.golden", got)

	if bytes.ContainsRune(encoded, 0x1b) {
		t.Error("JSON report contains an ANSI escape")
	}

	var envelope struct {
		Command    string `json:"command"`
		WorstState string `json:"worst_state"`
		DurationMS *int64 `json:"duration_ms"`
		Checks     []struct {
			Name       string            `json:"name"`
			Section    string            `json:"section"`
			State      string            `json:"state"`
			Summary    string            `json:"summary"`
			Diagnostic string            `json:"diagnostic"`
			Impact     string            `json:"impact"`
			NextAction []string          `json:"next_action"`
			Code       string            `json:"code"`
			Details    map[string]string `json:"details"`
			DurationMS *int64            `json:"duration_ms"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	if envelope.Command != "doctor" {
		t.Errorf("command = %q, want %q", envelope.Command, "doctor")
	}
	if envelope.WorstState != string(report.WorstState) {
		t.Errorf("worst_state = %q, want %q", envelope.WorstState, report.WorstState)
	}
	if envelope.DurationMS == nil {
		t.Error("duration_ms is absent; decision D-20 requires it")
	}
	if len(envelope.Checks) != len(report.Checks) {
		t.Fatalf("decoded %d checks, want %d", len(envelope.Checks), len(report.Checks))
	}
	for i, check := range envelope.Checks {
		if check.Name == "" || check.Section == "" || check.State == "" || check.Summary == "" {
			t.Errorf("checks[%d] is missing an always-present key: %+v", i, check)
		}
		if check.DurationMS == nil {
			t.Errorf("checks[%d] has no duration_ms", i)
		}
		if check.State == string(StateOK) {
			continue
		}
		if check.Diagnostic == "" || check.Impact == "" || len(check.NextAction) == 0 {
			t.Errorf("checks[%d] reported %q without a full diagnosis: %+v", i, check.State, check)
		}
	}
}

// TestReportJSONRoundTripsThroughTheStrictDecoder proves the emitted document is
// one this binary would also accept, which is what makes the state enum a
// contract rather than a convention.
func TestReportJSONRoundTripsThroughTheStrictDecoder(t *testing.T) {
	encoded, err := json.Marshal(goldenReport(t))
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode own output: %v", err)
	}
	if decoded.WorstState != goldenReport(t).WorstState {
		t.Errorf("worst_state round-tripped to %q", decoded.WorstState)
	}
}

// goldenReport builds a deterministic report: a healthy repository whose
// knowledge store has one unreadable record and whose configuration warns.
// Durations are zeroed because a wall-clock value cannot be a golden.
func goldenReport(t *testing.T) Report {
	t.Helper()

	subject := healthySubject()
	subject.Config.Warnings = []app.Warning{{
		Code:    app.CodeConfigUnknownEnvVar,
		Message: `unknown environment variable "MINDRAIL_NOT_A_KEY"`,
	}}
	subject.Knowledge.Problems = []loader.Problem{{
		Path:    ".mindrail/knowledge/decisions/dec-9.json",
		Code:    app.CodeKnowledgeUnreadable,
		Message: "invalid character '}' looking for beginning of object key string",
	}}

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())
	report.DurationMS = 0
	for i := range report.Checks {
		report.Checks[i].DurationMS = 0
	}
	return report
}

// assertGolden compares got against testdata/name, rewriting it under -update.
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
		t.Fatalf("read golden %s (run `go test ./internal/doctor -update` to create it): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// stripANSI removes CSI sequences so a coloured rendering can be compared with
// a plain one.
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
