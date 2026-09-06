package storage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestVerifyPragmasClassifiesAFailedConnection is finding E3, and the mutation
// finding E5 says survived.
//
// verifyPragmas opens every connection the pool may hand out, and the open of
// one of them can fail for exactly the reasons the open of the first one can:
// the file is not a database, the filesystem has no room to size the
// shared-memory index. The two lines that report those failures sat one
// statement apart and did not agree -- the pragma read-back went through
// classifyOpenError and the connection open went through openFailure -- so the
// same condition produced RUNTIME_DB_UNAVAILABLE with a dead-end remedy here and
// the correct diagnosis three lines below.
//
// A file that is not a database is the deterministic provocation: it needs no
// mounted filesystem, and it is classified into a different code and a different
// remedy by the two functions, so a regression cannot hide inside a shared one.
func TestVerifyPragmasClassifiesAFailedConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	if err := os.WriteFile(path, []byte("this is not an SQLite database at all"), 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}

	opts := Options{Path: path, MaxOpenConns: 1}
	sqlDB, err := sql.Open(driverName, dsn(opts))
	if err != nil {
		t.Fatalf("sql.Open = %v, want no error; the driver opens lazily", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	verifyErr := verifyPragmas(t.Context(), sqlDB, opts, 1)
	if verifyErr == nil {
		t.Fatal("verifyPragmas against a file that is not a database = nil, want a failure")
	}

	if !errors.Is(verifyErr, ErrCorrupt) {
		t.Errorf("verifyPragmas = %v, want errors.Is(err, ErrCorrupt); "+
			"the same condition classifyOpenError names a corrupt file must be named that here too", verifyErr)
	}

	payload, ok := app.PayloadOf(verifyErr)
	if !ok {
		t.Fatalf("verifyPragmas = %v, which carries no domain payload", verifyErr)
	}
	if payload.Code != app.CodeRuntimeDBCorrupt {
		t.Errorf("payload.Code = %q, want %q; %q is the generic open failure whose remedy "+
			"(\"check that the Git common directory exists and is writable\") is a dead end here",
			payload.Code, app.CodeRuntimeDBCorrupt, app.CodeRuntimeDBUnavailable)
	}
}
