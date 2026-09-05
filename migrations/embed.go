// Package migrations carries the numbered SQL migration files as an embed.FS.
//
// tech-stack §7 puts the migration files at the repository root and §111
// requires them to travel inside the executable. go:embed cannot reach outside
// its own directory, so this two-line package lives beside the .sql files
// rather than the files being duplicated under internal/ (decision D-33).
package migrations

import "embed"

// FS holds every migration file in this directory. internal/migration is the
// only intended consumer; it owns the filename grammar and the ordering rules.
//
//go:embed *.sql
var FS embed.FS
