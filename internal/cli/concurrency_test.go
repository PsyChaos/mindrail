package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/status"
)

// This file closes round 4's fourth coverage gap: "concurrency was barely
// touched: one determinism check and one swap probe. Nothing ran two commands
// against one store at the same time."
//
// The two commands MR-002 ships that read the knowledge store are `status` and
// `doctor`, and both are pure readers — this milestone has no writer in the
// command tree. So the question is not who wins a write, which is MR-004's, but
// whether one repository read by several commands at once answers each of them
// the way it answers one command alone. Two things could break that and neither
// is visible to a single-threaded suite: shared state behind the read path, and
// a report assembled out of a store that changed under it.
//
// The determinism the first test asserts is byte-equality against a sequential
// baseline rather than agreement among the concurrent runs. Eight readers that
// all returned the same wrong answer would satisfy the weaker form.
//
// Both tests are worth far more under `-race`, which `make verify` runs and
// `make check` does not.

// concurrentInvariant is a valid invariant, so the store these tests read is
// mixed-kind. The knowledge read path walks two directories and merges them, and
// a store of one kind exercises the merge with one input.
const concurrentInvariant = `{"schema_version":1,"kind":"invariant","id":"INV-0001",` +
	`"status":"active","created_at":"2026-01-01T00:00:00Z",` +
	`"statement":"The knowledge store stays readable while it is read.",` +
	`"severity":"HIGH","scope":{"level":"PROJECT"}}`

// TestTwoCommandsReadOneStoreAtTheSameTime runs `status` and `doctor` against
// one repository from eight goroutines at once and requires every answer to be
// the one a single sequential run produced.
func TestTwoCommandsReadOneStoreAtTheSameTime(t *testing.T) {
	repo := newInitializedRepo(t)
	writeKnowledgeRecord(t, repo, "DEC-0001.json", supersededDecision)
	writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
	writeInvariantRecord(t, repo, "INV-0001.json", concurrentInvariant)

	// The baseline is taken twice. A command whose own output varies between two
	// sequential runs would otherwise be reported below as a concurrency defect,
	// which is a diagnosis this test is not entitled to make — and one member
	// does vary, which is why the comparison runs through withoutTimings.
	baseline := make(map[string]result, 2)
	for _, command := range []string{"status", "doctor"} {
		first := run(t, repo, command, "--json")
		first.requireExit(t, app.ExitSuccess)

		second := run(t, repo, command, "--json")
		if withoutTimings(t, second.stdout) != withoutTimings(t, first.stdout) {
			t.Fatalf("%s --json printed different reports on two sequential runs, so concurrency is not what this test could be measuring\nfirst:\n%s\nsecond:\n%s",
				command, first.stdout, second.stdout)
		}
		baseline[command] = first
	}

	const readers, rounds = 8, 6

	type observation struct {
		command string
		got     result
	}
	seen := make([][]observation, readers)

	var wg sync.WaitGroup
	for reader := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The offset staggers the two commands across the readers, so at any
			// moment some goroutines are in `status` and others in `doctor`
			// rather than all of them running the same code.
			for round := range rounds {
				command := "status"
				if (reader+round)%2 == 1 {
					command = "doctor"
				}
				seen[reader] = append(seen[reader], observation{
					command: command,
					got:     run(t, repo, command, "--json"),
				})
			}
		}()
	}
	wg.Wait()

	for reader, observations := range seen {
		for round, o := range observations {
			want := baseline[o.command]
			if o.got.code != want.code {
				t.Errorf("reader %d round %d: %s exited %d, and alone it exits %d (error %v)",
					reader, round, o.command, o.got.code, want.code, o.got.err)
			}
			if withoutTimings(t, o.got.stdout) != withoutTimings(t, want.stdout) {
				t.Errorf("reader %d round %d: %s printed\n%s\nand alone it prints\n%s",
					reader, round, o.command, o.got.stdout, want.stdout)
			}
		}
	}
}

// TestAReaderStaysCoherentWhileTheStoreIsRewritten is the harder half: readers
// running while the records under them are replaced, removed and left half
// written.
//
// It cannot assert *which* answer a reader gets — that depends on where the
// walk was when the file changed, and every one of the four states the writer
// cycles through is a legitimate reading of some instant. What it can assert is
// that the answer is a coherent one, and the three clauses below are the whole
// of the envelope's contract: the process exits zero exactly when the envelope
// says ok, the envelope carries an error exactly when it does not, and the
// readiness is one this project publishes.
//
// The writer alternates between an atomic replacement and a direct write on
// purpose. A rename is what a careful editor does and a reader never sees a
// partial file through it; a direct write is what a script with a shell
// redirection does, and it is the only way a reader here can meet a truncated
// record at all.
func TestAReaderStaysCoherentWhileTheStoreIsRewritten(t *testing.T) {
	repo := newInitializedRepo(t)
	writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
	writeInvariantRecord(t, repo, "INV-0001.json", concurrentInvariant)

	target := filepath.Join(repo, ".mindrail", "knowledge", "decisions", "DEC-0001.json")
	states := []string{supersededDecision, cyclicSupersededDecision, halfWrittenCycleMember}

	// The cap is a runaway guard rather than a plan: the writer stops when the
	// readers are done, and reaching the cap first would mean a reader took long
	// enough for the store to have been rewritten a hundred thousand times.
	const readers, rounds, cap = 4, 8, 100_000

	var rewrites atomic.Int64
	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for i := range cap {
			select {
			case <-stop:
				return
			default:
			}
			switch i % 5 {
			case 4:
				_ = os.Remove(target)
			case 3:
				// Atomically, as an editor that writes beside the file and
				// renames over it does.
				temp := target + ".tmp"
				if os.WriteFile(temp, []byte(states[i%3]), 0o644) == nil {
					_ = os.Rename(temp, target)
				}
			default:
				_ = os.WriteFile(target, []byte(states[i%3]), 0o644)
			}
			rewrites.Add(1)
			// Enough of a pause that this loop does not starve the readers it
			// exists to interfere with, and short enough that it rewrites the
			// store many times per command.
			time.Sleep(50 * time.Microsecond)
		}
	}()

	type reading struct {
		command string
		got     result
	}
	observed := make([][]reading, readers)
	var wg sync.WaitGroup
	for reader := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := range rounds {
				command := "status"
				if (reader+round)%2 == 1 {
					command = "doctor"
				}
				observed[reader] = append(observed[reader], reading{
					command: command,
					got:     run(t, repo, command, "--json"),
				})
			}
		}()
	}
	wg.Wait()
	close(stop)
	writer.Wait()

	// Without this the test passes vacuously on a machine where the writer
	// finished before the first reader started, which is what the first draft
	// did: 400 rewrites completed in under a millisecond and every reading saw
	// one settled store.
	if got := rewrites.Load(); got < int64(readers*rounds) {
		t.Errorf("the store was rewritten %d times under %d readings, so the readers may never have met a change",
			got, readers*rounds)
	}

	published := map[string]int{}
	for reader, readings := range observed {
		for round, r := range readings {
			var envelope struct {
				OK    bool            `json:"ok"`
				Error json.RawMessage `json:"error"`
				Data  struct {
					Readiness string `json:"readiness"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(r.got.stdout), &envelope); err != nil {
				t.Errorf("reader %d round %d (%s) printed something that is not an envelope: %v\n%s",
					reader, round, r.command, err, r.got.stdout)
				continue
			}

			if envelope.OK != (r.got.code == app.ExitSuccess) {
				t.Errorf("reader %d round %d (%s): the envelope says ok=%v and the process exited %d",
					reader, round, r.command, envelope.OK, r.got.code)
			}
			carriesError := len(envelope.Error) > 0 && string(envelope.Error) != "null"
			if envelope.OK == carriesError {
				t.Errorf("reader %d round %d (%s): ok=%v with error=%s",
					reader, round, r.command, envelope.OK, envelope.Error)
			}
			// Readiness is status's own summary; doctor publishes its checks
			// instead and carries no such member, so asking it for one would be
			// asserting against a command that never promised it.
			if r.command == "status" {
				switch status.Readiness(envelope.Data.Readiness) {
				case status.ReadinessReady, status.ReadinessPartialReady,
					status.ReadinessDegraded, status.ReadinessBlocked:
				default:
					t.Errorf("reader %d round %d published readiness %q, which is not one this project defines",
						reader, round, envelope.Data.Readiness)
				}
			}
			published[fmt.Sprintf("%s %s/%d", r.command, envelope.Data.Readiness, r.got.code)]++
		}
	}

	// What the race actually produced, so a run in which every reader happened to
	// meet the same state is visible in the output rather than passing silently
	// as evidence of more than it is.
	t.Logf("%d readings over a store rewritten %d times: %v",
		readers*rounds, rewrites.Load(), published)
}

// withoutTimings renders one report with every "duration_ms" removed, at any
// depth.
//
// The member is how long the command took, so two runs over one unchanged
// repository differ in it — 9 against 8 milliseconds, on the run that found
// this. Dropping it is what lets the rest be compared byte for byte, and the
// removal is by key name at every level rather than at the one place it is
// known to appear, so a second timing added elsewhere does not quietly turn this
// comparison into a flaky one.
func withoutTimings(t *testing.T, stdout string) string {
	t.Helper()

	var report any
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}

	var strip func(any) any
	strip = func(value any) any {
		switch typed := value.(type) {
		case map[string]any:
			for key, member := range typed {
				if key == "duration_ms" {
					delete(typed, key)
					continue
				}
				typed[key] = strip(member)
			}
			return typed
		case []any:
			for i, member := range typed {
				typed[i] = strip(member)
			}
			return typed
		default:
			return value
		}
	}

	// encoding/json sorts a map's keys, so the rendering is canonical and two
	// reports that differ in nothing compare equal whatever order they arrived
	// in.
	rendered, err := json.Marshal(strip(report))
	if err != nil {
		t.Fatalf("re-marshalling the report: %v", err)
	}
	return string(rendered)
}

// writeInvariantRecord is writeKnowledgeRecord's other directory. The helper it
// mirrors writes decisions, which is where the §95 reader looks first and which
// is therefore the only kind the CLI suite had fixtures for.
func writeInvariantRecord(t *testing.T, repo, name, content string) {
	t.Helper()

	dir := filepath.Join(repo, ".mindrail", "knowledge", "invariants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	writeFile(t, filepath.Join(dir, name), []byte(content))
}
