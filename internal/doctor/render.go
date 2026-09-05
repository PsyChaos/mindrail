package doctor

import (
	"io"
	"maps"
	"slices"
	"strings"
)

// RenderHuman writes the spec §83 report: a title, one block per section, and
// for every reading a marker line followed by its details and, when something
// is wrong, the Diagnostic / Impact / Next triple in that order.
//
// The whole report is assembled before a single write, so a failing writer
// cannot leave half a health report on a terminal.
func (r Report) RenderHuman(w io.Writer, color bool) error {
	var b strings.Builder
	b.WriteString("Mindrail Doctor\n")

	section := ""
	for _, result := range r.Checks {
		if result.Section != section {
			section = result.Section
			b.WriteString("\n")
			b.WriteString(section)
			b.WriteString("\n")
		}
		writeResult(&b, result, color)
	}

	b.WriteString("\nOverall: ")
	b.WriteString(paint(r.WorstState, string(r.WorstState), color))
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// writeResult renders one reading.
//
// The state word is printed next to the glyph for everything except OK
// (decision D-21): the glyph carries the report's §83 look, the word carries
// its meaning, and no reader — human or golden test — has to depend on a font.
func writeResult(b *strings.Builder, result Result, color bool) {
	headline := result.State.Marker() + " " + result.Summary
	if result.State != StateOK {
		headline = result.State.Marker() + " " + string(result.State) + " " + result.Summary
	}
	b.WriteString(paint(result.State, headline, color))
	b.WriteString("\n")

	if result.Code != "" {
		b.WriteString("  code: ")
		b.WriteString(string(result.Code))
		b.WriteString("\n")
	}
	for _, key := range slices.Sorted(maps.Keys(result.Details)) {
		b.WriteString("  ")
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(result.Details[key])
		b.WriteString("\n")
	}

	writeBlock(b, "Diagnostic", strings.Split(result.Diagnostic, "\n"))
	writeBlock(b, "Impact", strings.Split(result.Impact, "\n"))
	writeBlock(b, "Next", result.NextAction)
}

// writeBlock writes one labelled section, one line per value. A block with
// nothing to say is skipped rather than printed as a label over emptiness,
// which mirrors app.RenderError so the two never look different.
func writeBlock(b *strings.Builder, label string, lines []string) {
	present := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			present = append(present, line)
		}
	}
	if len(present) == 0 {
		return
	}

	b.WriteString("\n  ")
	b.WriteString(label)
	b.WriteString(":\n")
	for _, line := range present {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// ANSI attributes for the five states. They are applied only to the marker line,
// so stripping them leaves the report byte-identical to the uncoloured one.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBlue   = "\x1b[34m"
	ansiDim    = "\x1b[2m"
)

// paint colours text for a state. Colour is a presentation decision made by the
// caller (app.ColorEnabled); this function only knows how.
func paint(state State, text string, color bool) string {
	if !color {
		return text
	}

	attribute := ""
	switch state {
	case StateOK:
		attribute = ansiGreen
	case StateDegraded:
		attribute = ansiYellow
	case StateError:
		attribute = ansiRed
	case StateUnavailable:
		attribute = ansiBlue
	case StateNotApplicable:
		attribute = ansiDim
	}
	if attribute == "" {
		return text
	}
	return attribute + text + ansiReset
}
