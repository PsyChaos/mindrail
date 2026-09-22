package index

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// UnsupportedLanguage names an out-of-scope file. It is terminal rather than
// failed work: the caller records unsupported and excludes it from the queue.
func UnsupportedLanguage(path string) error {
	return app.NewError(app.CodeSyntaxLanguageUnsupported, app.KindFailed,
		fmt.Sprintf("%s has no structural parser in this version", path),
		"This file is out of scope for structural indexing and does not block readiness.",
		"Leave the file out of the structural index; only Python, TypeScript and JavaScript are supported.").WithMetadata("path", path)
}

// ParseFailed keeps a supported file pending while surfacing its exact path.
func ParseFailed(path string, cause error) error {
	return app.NewError(app.CodeSyntaxParseFailed, app.KindFailed,
		fmt.Sprintf("the structural parse of %s was incomplete", path),
		"Available partial facts were retained, but this file still needs indexing.",
		"Correct the syntax or extraction issue in "+path+" and retry indexing.").WithMetadata("path", path).WithCause(cause)
}

// IndexStateCorrupt is for damaged persisted index facts, not a syntax error.
// A healthy backup is safer than deleting rows while another subsystem may
// still refer to the same runtime database.
func IndexStateCorrupt(cause error) error {
	return app.NewError(app.CodeIndexStateCorrupt, app.KindFailed,
		"the persisted structural index could not be read consistently",
		"Index readiness and structural answers cannot be trusted until the state is repaired.",
		"Preserve the runtime database and restore its index state from a known-good backup before retrying.").WithCause(cause)
}

// AmbiguousIdentity keeps a removed symbol unmigrated when two or more added
// symbols meet the migration bar. It mints nothing and steals nothing: the
// candidates are named so a human or a later pass can decide.
func AmbiguousIdentity(removedKey string, candidateKeys []string) error {
	return app.NewError(app.CodeSymbolIdentityAmbiguous, app.KindFailed,
		fmt.Sprintf("the removed symbol %s matches %d candidates and was not migrated", removedKey, len(candidateKeys)),
		"Guessing would attach the symbol's invariant relations to the wrong lineage.",
		"Name the surviving symbol explicitly: "+strings.Join(candidateKeys, ", ")+".")
}

// OrphanedProtectedSymbol keeps an active invariant blocking when its symbol
// is gone with no confident heir. The invariant stays active and loud rather
// than going silent.
func OrphanedProtectedSymbol(invariantID, target string) error {
	return app.NewError(app.CodeOrphanedProtectedSymbol, app.KindFailed,
		fmt.Sprintf("invariant %s protects %s, which no longer resolves", invariantID, target),
		"The protected symbol is gone and its invariant relations have nowhere to attach.",
		"Migrate the identity, or supersede the invariant "+invariantID+" explicitly.")
}

func corruptState(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return IndexStateCorrupt(err)
}

func schemaBehind(applied int64) error {
	return app.NewError(app.CodeMigrationFailed, app.KindFailed,
		fmt.Sprintf("Schema is behind this binary: the runtime database has applied migrations up to %d and the index store needs %d", applied, TableSchemaVersion),
		"Structural index commands cannot run; nothing was read and nothing was written.",
		"Run `mindrail init` to apply the pending migrations, then re-run the command.").
		WithMetadata("applied_version", strconv.FormatInt(applied, 10)).
		WithMetadata("required_version", strconv.FormatInt(TableSchemaVersion, 10))
}
