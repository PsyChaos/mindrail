package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteJSONContainsNoANSI holds tech-stack §12: JSON never carries ANSI, and
// stdout carries exactly one envelope so `--json` is parseable without filtering.
func TestWriteJSONContainsNoANSI(t *testing.T) {
	hostile := map[string]string{
		"colored": "\x1b[31mred\x1b[0m",
		"control": "tab\there",
	}

	tests := []struct {
		name     string
		data     any
		warnings []Warning
		err      error
		wantOK   bool
	}{
		{
			name:   "success envelope",
			data:   hostile,
			wantOK: true,
		},
		{
			name:     "warnings ride along with success",
			data:     hostile,
			warnings: []Warning{{Code: CodeConfigUnknownEnvVar, Message: "\x1b[33mMINDRAIL_NOPE\x1b[0m is not a known key"}},
			wantOK:   true,
		},
		{
			name: "domain error envelope",
			err: NewError(CodeConfigInvalid, KindUsage, "\x1b[31munknown key\x1b[0m", "config cannot be trusted",
				"remove the unknown key from .mindrail/config.toml"),
			wantOK: false,
		},
		{
			name:   "plain error still produces an error envelope",
			err:    errors.New("\x1b[31mboom\x1b[0m"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, "status", tt.data, tt.warnings, tt.err); err != nil {
				t.Fatalf("WriteJSON: %v", err)
			}

			out := buf.Bytes()
			if bytes.ContainsRune(out, 0x1b) {
				t.Errorf("output contains a raw ESC byte: %q", out)
			}
			if !bytes.HasSuffix(out, []byte("\n")) {
				t.Errorf("output does not end with a newline: %q", out)
			}
			if n := bytes.Count(out, []byte("\n")); n != 1 {
				t.Errorf("output has %d newlines, want exactly 1: %q", n, out)
			}

			var envelope Envelope
			if err := json.Unmarshal(out, &envelope); err != nil {
				t.Fatalf("output is not a single JSON object (%v): %q", err, out)
			}
			if envelope.Command != "status" {
				t.Errorf("command = %q, want %q", envelope.Command, "status")
			}
			if envelope.OK != tt.wantOK {
				t.Errorf("ok = %v, want %v", envelope.OK, tt.wantOK)
			}
			if tt.err == nil && envelope.Error != nil {
				t.Errorf("error = %+v on a success envelope, want nil", envelope.Error)
			}
			if tt.err != nil && envelope.Error == nil {
				t.Errorf("error = nil on a failure envelope, want a payload")
			}
			if len(envelope.Warnings) != len(tt.warnings) {
				t.Errorf("warnings = %v, want %d entries", envelope.Warnings, len(tt.warnings))
			}
		})
	}
}

// TestWriteJSONOmitsAbsentSections keeps decision D-11's envelope small enough
// that a healthy run is not padded with nulls.
func TestWriteJSONOmitsAbsentSections(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, "version", nil, nil, nil); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, key := range []string{"command", "ok"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("envelope %s is missing the always-present key %q", buf.String(), key)
		}
	}
	for _, key := range []string{"data", "error", "warnings"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("envelope %s carries an empty %q key", buf.String(), key)
		}
	}
}

// TestColorEnabledRespectsNoColor covers the tech-stack §12 rule in full: JSON
// mode, NO_COLOR and an explicit "never" all win over terminal detection.
func TestColorEnabledRespectsNoColor(t *testing.T) {
	terminal := openCharDevice(t)

	tests := []struct {
		name      string
		w         func() *os.File
		environ   []string
		colorPref string
		jsonMode  bool
		want      bool
	}{
		{"terminal with no overrides", terminal, nil, "auto", false, true},
		{"json mode disables color", terminal, nil, "auto", true, false},
		{"json mode wins over always", terminal, nil, "always", true, false},
		{"NO_COLOR present disables color", terminal, []string{"PATH=/usr/bin", "NO_COLOR=1"}, "auto", false, false},
		{"empty NO_COLOR still disables color", terminal, []string{"NO_COLOR="}, "auto", false, false},
		{"NO_COLOR wins over always", terminal, []string{"NO_COLOR=1"}, "always", false, false},
		{"never disables color", terminal, nil, "never", false, false},
		{"unrelated env var is ignored", terminal, []string{"NO_COLORS=1", "COLORTERM=truecolor"}, "auto", false, true},
		{"non terminal writer disables color", func() *os.File { return regularFile(t) }, nil, "auto", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.w()
			if w == nil {
				t.Skip("no character device available on this platform")
			}
			if got := ColorEnabled(w, tt.environ, tt.colorPref, tt.jsonMode); got != tt.want {
				t.Errorf("ColorEnabled(%v, %q, %v) = %v, want %v", tt.environ, tt.colorPref, tt.jsonMode, got, tt.want)
			}
		})
	}

	t.Run("a non-file writer is never a terminal", func(t *testing.T) {
		if ColorEnabled(&bytes.Buffer{}, nil, "always", false) {
			t.Errorf("ColorEnabled(buffer) = true, want false")
		}
	})
}

// openCharDevice returns a writer that Stat()s as a character device, which is
// the only terminal-shaped file a test can rely on without a pty.
func openCharDevice(t *testing.T) func() *os.File {
	t.Helper()
	return func() *os.File {
		if runtime.GOOS == "windows" {
			return nil
		}
		f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return nil
		}
		t.Cleanup(func() { _ = f.Close() })

		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return nil
		}
		return f
	}
}

func regularFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TestWriteJSONDoesNotEscapeHTML keeps repository paths and shell snippets
// readable in the envelope instead of turning `<` into an escape sequence.
func TestWriteJSONDoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, "doctor", map[string]string{"hint": "a<b&c>d"}, nil, nil); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(buf.String(), "a<b&c>d") {
		t.Errorf("WriteJSON escaped HTML characters: %s", buf.String())
	}
}
