package validate

import (
	"path"
	"strings"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
)

// recordSuffix is the extension every knowledge record file carries. It is the
// loader's `recordSuffix`, which is unexported there, so this is a second
// spelling of one constant; TestTheExpectedPathIsTheOneTheLoaderProduces reads
// the loader's answer back out of a real load and compares, so the two cannot
// drift into disagreeing about where a record lives.
const recordSuffix = ".json"

// kindDirs binds each record kind to the directory the loader walks it in. Same
// caveat and same test as recordSuffix: loader.kindDirs is unexported, so this
// is a copy held honest by a pairing test rather than by hope.
var kindDirs = map[loader.RecordKind]string{
	loader.KindDecision:  "decisions",
	loader.KindInvariant: "invariants",
}

// expectedPath returns the repository-relative file an id of one kind must
// occupy.
//
// This is not a guess about the layout: it is exactly what step 6 requires, so
// the mapping decision D-45 needs to decide whether a reference is unverifiable
// is the same mapping step 6 enforces. If the two ever disagreed, one of them
// would be wrong about where a record lives.
//
// The second return is false for a kind this package cannot place. It cannot
// happen — the loader produces exactly the two kinds above — and callers treat
// it as "the file cannot be identified" rather than as "the file is absent",
// which is the direction D-45 points: never publish a claim about a file you
// could not name.
func expectedPath(kind loader.RecordKind, id string) (string, bool) {
	dir, ok := kindDirs[kind]
	if !ok {
		return "", false
	}
	return path.Join(loader.StoreRoot, dir, id+recordSuffix), true
}

// idFromPath returns the id a record's file name claims, which is the base name
// with the record suffix removed. Step 6 compares it against the id the document
// carries; they are the two halves of "the file name and the id have to agree".
func idFromPath(recordPath string) string {
	return strings.TrimSuffix(path.Base(recordPath), recordSuffix)
}
