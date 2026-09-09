package workspace

// Identifier prefixes. They are part of the persisted value, so an id read out
// of a log or an MCP payload says what kind of thing it names without a lookup.
//
// They live here rather than in internal/identity because the prefix is what a
// package calls its own things; the format is what identity owns (decision
// D-57).
const (
	projectIDPrefix   = "PRJ"
	workspaceIDPrefix = "WS"
)
