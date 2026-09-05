package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/storage"
)

// contenderEnv turns this test binary into one of the racers below.
//
// The contention this test is about only exists between processes. SQLite's
// locks are held per process, and the WAL conversion of a brand-new file
// happens once; two goroutines in one process therefore share the winner's
// connection state and never see the SQLITE_BUSY that a second `mindrail init`
// on the same repository sees.
const contenderEnv = "MINDRAIL_TEST_OPEN_CONTENDER_DB"

const (
	contenderOpened = 20 // Open returned a handle
	contenderFailed = 21 // Open gave up
)

// TestMain dispatches the re-execs before it runs anything.
//
// Two of the conditions this package classifies exist only between processes or
// only inside a namespace this one cannot enter: contention for a write lock,
// and a filesystem with nothing left on it. Both are reproduced by starting this
// same binary in the state that has them, so both have to be recognised here
// before the suite starts.
func TestMain(m *testing.M) {
	if path := os.Getenv(contenderEnv); path != "" {
		os.Exit(runContender(path))
	}
	if code, isChild := fullDiskChild(); isChild {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func runContender(path string) int {
	db, err := storage.Open(context.Background(), storage.Options{Path: path})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		return contenderFailed
	}
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close: %v\n", err)
		return contenderFailed
	}
	return contenderOpened
}

// TestOpenWaitsOutAConcurrentWALConversion is decision D-24 at the layer that
// actually breaks it. busy_timeout is a per-connection setting, so it governs
// statements on an existing connection and cannot govern the creation of one;
// the WAL conversion happens during creation, and the process that arrives
// second is refused after a few milliseconds unless Open waits.
//
// Every contender must come back with a handle. One that did not is `mindrail
// init` exiting 4 on a healthy repository because another process was half a
// millisecond ahead of it.
func TestOpenWaitsOutAConcurrentWALConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	const (
		rounds     = 5
		contenders = 5
	)

	for round := range rounds {
		path := filepath.Join(t.TempDir(), "mindrail.db")

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			codes  = make([]int, contenders)
			output = make([]string, contenders)
		)
		for contender := range contenders {
			wg.Add(1)
			go func() {
				defer wg.Done()

				cmd := exec.Command(binary)
				cmd.Env = append(os.Environ(), contenderEnv+"="+path)
				out, runErr := cmd.CombinedOutput()

				code := cmd.ProcessState.ExitCode()
				var exitErr *exec.ExitError
				if runErr != nil && !errors.As(runErr, &exitErr) {
					code = -1
					out = append(out, []byte("spawn: "+runErr.Error())...)
				}

				mu.Lock()
				codes[contender], output[contender] = code, strings.TrimSpace(string(out))
				mu.Unlock()
			}()
		}
		wg.Wait()

		for contender, code := range codes {
			if code != contenderOpened {
				t.Errorf("round %d contender %d exited %d, want %d: %s",
					round, contender, code, contenderOpened, output[contender])
			}
		}
	}
}

// TestOpenStopsWaitingWhenTheContextIsCancelled bounds the wait from the other
// side. A retry loop that outlived its caller's context would turn a Ctrl-C
// during a contended start into a five second pause before the same failure.
func TestOpenStopsWaitingWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	started := time.Now()
	db, err := storage.Open(ctx, storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(cancelled) = nil error, want a failure")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("Open(cancelled) took %v, want it to give up promptly", elapsed)
	}
}

// TestOpenDoesNotWaitForAPermanentFailure keeps the retry honest about what it
// is for. A corrupt file is not going to become valid within busy_timeout, and
// spending the whole budget before saying so would delay every diagnosis.
func TestOpenDoesNotWaitForAPermanentFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a database", 600)), 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}

	started := time.Now()
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(garbage) = nil error, want ErrCorrupt")
	}
	if !errors.Is(err, storage.ErrCorrupt) {
		t.Fatalf("Open(garbage) = %v, want errors.Is(err, ErrCorrupt)", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("Open(garbage) took %v, want an immediate verdict; the retry is for SQLITE_BUSY only", elapsed)
	}
}
