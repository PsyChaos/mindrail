package app

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// Envelope is the single structured-output shape for every command (decision
// D-11). One serializer for the CLI now and MCP later means "JSON never
// contains ANSI escape codes" (tech-stack §12) is guaranteed in one place
// rather than re-argued per command.
type Envelope struct {
	Command  string        `json:"command"`
	OK       bool          `json:"ok"`
	Data     any           `json:"data,omitempty"`
	Error    *ErrorPayload `json:"error,omitempty"`
	Warnings []Warning     `json:"warnings,omitempty"`
}

// Warning is a non-fatal condition worth surfacing — an unknown MINDRAIL_ env
// var, say. Warnings never change the exit code; that is what makes them
// warnings.
type Warning struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// WriteJSON emits exactly one Envelope followed by a newline. Callers pass the
// command's error rather than pre-classifying it, so ok and the error payload
// can never disagree.
func WriteJSON(w io.Writer, command string, data any, warnings []Warning, err error) error {
	envelope := Envelope{
		Command:  command,
		OK:       err == nil,
		Data:     data,
		Warnings: warnings,
	}

	if err != nil {
		payload, ok := PayloadOf(err)
		if !ok {
			// An error that reached the boundary without a code is a defect,
			// but dropping it would be worse than reporting it uncoded.
			payload = ErrorPayload{Why: err.Error(), NextAction: []string{}}
		}
		envelope.Error = &payload
	}

	encoder := json.NewEncoder(w)
	// Repository paths and shell snippets stay readable; < helps nobody.
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

// ColorEnabled implements the tech-stack §12 colour rule. It is false when the
// output is machine-consumed (jsonMode), when the environment opts out
// (NO_COLOR), when the user opts out (colorPref "never"), or when the writer is
// not a terminal — in that last case the escapes would end up in a pipe or a
// log file, which is exactly where they do damage.
//
// environ is passed in rather than read from the process so that callers, and
// tests, control it.
func ColorEnabled(w io.Writer, environ []string, colorPref string, jsonMode bool) bool {
	if jsonMode {
		return false
	}
	if hasEnvKey(environ, "NO_COLOR") {
		return false
	}
	if colorPref == colorNever {
		return false
	}
	return isTerminal(w)
}

// colorNever is the only colour preference this package has to recognise; the
// remaining values ("auto", "always") all defer to terminal detection.
const colorNever = "never"

// hasEnvKey reports presence, not truth: NO_COLOR is an opt-out flag, so an
// empty value still counts as set.
func hasEnvKey(environ []string, key string) bool {
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if name == key || (!found && entry == key) {
			return true
		}
	}
	return false
}

// isTerminal detects a character device, which is as close as the standard
// library gets to "a human is looking at this".
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok || file == nil {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
