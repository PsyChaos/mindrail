package dashboard

import "embed"

// assets contains the complete dashboard client. Keeping it in the binary makes
// `mindrail dashboard` work from an installed executable without a source tree.
//
//go:embed assets/*
var assets embed.FS
