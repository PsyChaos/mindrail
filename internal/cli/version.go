package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// Build metadata, overridden at link time with -ldflags "-X ...".
//
// The defaults describe an unstamped local build honestly rather than claiming
// a version it does not have: a binary that reports "0.1.0" because nobody set
// the flag is worse than one that says "dev".
var (
	version   = "dev"
	commit    = ""
	buildDate = ""
	dirty     = ""
)

// mcpCompatibility is what this binary offers over MCP. MR-014 through MR-016
// own the server; until then the honest answer is none, and an agent that reads
// this field must be able to trust it (decision D-09).
const mcpCompatibility = "none"

// versionInfo is the `mindrail version` payload.
//
// The knowledge schema window is part of it because it is the one compatibility
// question another tool has to answer before it writes a record: "will this
// binary read what I am about to produce?" (kernel-scope §3).
type versionInfo struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
	BuildDate              string `json:"build_date"`
	Dirty                  string `json:"dirty"`
	Go                     string `json:"go"`
	Platform               string `json:"platform"`
	WriteSchemaVersion     int    `json:"write_schema_version"`
	ReadableSchemaVersions []int  `json:"readable_schema_versions"`
	MCPCompatibility       string `json:"mcp_compatibility"`
}

// newVersionCommand builds `mindrail version`.
//
// It is the one command that starts nothing: reporting which binary this is has
// to work outside a repository, on an unwritable filesystem and before init has
// ever run, because those are the situations in which somebody asks.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Report this binary's version and compatibility",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inv, err := newInvocation(cmd, "version", Options{})
			if err != nil {
				return inv.emit(nil, nil, nil, "", err)
			}

			info := buildVersionInfo()
			return inv.emit(info, info.renderHuman, nil, "", nil)
		},
	}
}

func buildVersionInfo() versionInfo {
	return versionInfo{
		Version:                version,
		Commit:                 resolveCommit(),
		BuildDate:              resolveBuildDate(),
		Dirty:                  resolveDirty(),
		Go:                     runtime.Version(),
		Platform:               runtime.GOOS + "/" + runtime.GOARCH,
		WriteSchemaVersion:     schema.WriteVersion,
		ReadableSchemaVersions: schema.ReadableVersions(),
		MCPCompatibility:       mcpCompatibility,
	}
}

// resolveBuildDate reports the linker-stamped build date, or "unknown"
// for unstamped local builds — same honesty rule as version itself.
func resolveBuildDate() string {
	if buildDate != "" {
		return buildDate
	}
	return "unknown"
}

// resolveDirty reports the linker-stamped tree state. Only the stamper
// may claim "clean" or "dirty"; everything else says "unknown".
func resolveDirty() string {
	switch dirty {
	case "clean", "dirty":
		return dirty
	default:
		return "unknown"
	}
}

// resolveCommit prefers the linker-stamped value and falls back to the revision
// the Go toolchain embedded, so a `go install`-ed binary still identifies
// itself.
func resolveCommit() string {
	if commit != "" {
		return commit
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return setting.Value
		}
	}
	return "unknown"
}

// renderHuman writes the same fields as the JSON payload, in the same order.
// Colour is not used: there is nothing here to grade.
func (v versionInfo) renderHuman(w io.Writer, _ bool) error {
	var b strings.Builder

	fmt.Fprintf(&b, "mindrail %s\n", v.Version)
	fmt.Fprintf(&b, "  commit:                   %s\n", v.Commit)
	fmt.Fprintf(&b, "  build date:               %s\n", v.BuildDate)
	fmt.Fprintf(&b, "  dirty:                    %s\n", v.Dirty)
	fmt.Fprintf(&b, "  go:                       %s\n", v.Go)
	fmt.Fprintf(&b, "  platform:                 %s\n", v.Platform)
	fmt.Fprintf(&b, "  write schema version:     %d\n", v.WriteSchemaVersion)
	fmt.Fprintf(&b, "  readable schema versions: %s\n", joinInts(v.ReadableSchemaVersions))
	fmt.Fprintf(&b, "  mcp compatibility:        %s\n", v.MCPCompatibility)

	_, err := io.WriteString(w, b.String())
	return err
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
