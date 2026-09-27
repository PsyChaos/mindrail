// Package agent contains assets used by Mindrail's optional coding-agent
// integrations. The assets are embedded so an installed binary is the single
// source of executable adapter code.
package agent

import _ "embed"

// jevRouteSource is the canonical JEV routing adapter.
//
//go:embed jev_route.py
var jevRouteSource string

// JevRouteSource returns the embedded, canonical JEV adapter source.
func JevRouteSource() string { return jevRouteSource }
