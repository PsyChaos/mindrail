package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index/parser"
)

func cachePaths(t *testing.T) filesystem.RuntimePaths {
	t.Helper()
	root := t.TempDir()
	paths, err := filesystem.ResolveRuntimePaths(filesystem.PathOptions{
		CommonDir: filepath.Join(root, ".git"), WorktreeRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func pythonInfo(t *testing.T) parser.LanguageInfo {
	t.Helper()
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	adapter, ok := registry.Lookup("fixture.py")
	if !ok {
		t.Fatal("Python adapter missing")
	}
	return adapter.Info()
}

func fixtureFacts(t *testing.T, source []byte) Facts {
	t.Helper()
	r, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	a, _ := r.Lookup("fixture.py")
	facts, err := parser.Extract(t.Context(), a, parser.SourceFile{Content: source})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func entryFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}

func TestIdenticalContentSharesOneValidatedSnapshot(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	facts := fixtureFacts(t, source)
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) {
		calls++
		return facts, nil
	}
	for _, path := range []string{"a.py", "nested/b.py"} {
		file := filepath.Join(paths.WorktreeRoot, path)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, source, 0o600); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := cache.GetOrCompute(t.Context(), info, content, compute)
		if err != nil || !reflect.DeepEqual(got, facts) {
			t.Fatalf("snapshot = %+v, %v", got, err)
		}
	}
	if calls != 1 || len(entryFiles(t, paths.CacheDir)) != 1 {
		t.Fatalf("same bytes used %d computes and %d entries, want one each", calls, len(entryFiles(t, paths.CacheDir)))
	}
	entry, err := os.ReadFile(entryFiles(t, paths.CacheDir)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(entry, []byte(info.Language)) || !bytes.Contains(entry, []byte(info.GrammarVersion)) ||
		!bytes.Contains(entry, []byte("\"schema_version\":3")) ||
		!bytes.Contains(entry, []byte(contentHash(source))) {
		t.Fatalf("entry omits key identity: %s", entry)
	}
}

func TestEveryIdentityComponentSeparatesCacheEntries(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) { calls++; return Facts{}, nil }
	variants := []struct {
		info   parser.LanguageInfo
		source []byte
	}{
		{info, source},
		{parser.LanguageInfo{Language: "javascript", GrammarVersion: info.GrammarVersion}, source},
		{parser.LanguageInfo{Language: info.Language, GrammarVersion: "next-grammar"}, source},
		{info, []byte("def beta(): pass\n")},
	}
	for _, variant := range variants {
		if _, err := cache.GetOrCompute(t.Context(), variant.info, variant.source, compute); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(variants) || len(entryFiles(t, paths.CacheDir)) != len(variants) {
		t.Fatalf("identity variants produced %d computes and %d entries", calls, len(entryFiles(t, paths.CacheDir)))
	}
}

func TestInvalidIdentityAndMissingComputerAreRejected(t *testing.T) {
	cache := New(cachePaths(t))
	called := false
	compute := func(context.Context, []byte) (Facts, error) { called = true; return Facts{}, nil }
	for _, info := range []parser.LanguageInfo{{Language: "", GrammarVersion: "v1"}, {Language: "python", GrammarVersion: " "}} {
		_, err := cache.GetOrCompute(t.Context(), info, []byte("x"), compute)
		if !errors.Is(err, ErrIdentity) || called {
			t.Fatalf("invalid identity %+v: err=%v compute=%t", info, err, called)
		}
	}
	_, err := cache.GetOrCompute(t.Context(), pythonInfo(t), []byte("x"), nil)
	if !errors.Is(err, ErrNoComputer) {
		t.Fatalf("nil computer: %v", err)
	}
}

func contentHash(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

func TestDeletedCorruptAndUnavailableCacheRecomputes(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	want := fixtureFacts(t, source)
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) { calls++; return want, nil }
	get := func() {
		t.Helper()
		got, err := cache.GetOrCompute(t.Context(), info, source, compute)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("facts = %+v, %v", got, err)
		}
	}
	get() // cold
	get() // warm
	if calls != 1 {
		t.Fatalf("warm cache recomputed %d times", calls)
	}
	if err := os.RemoveAll(paths.CacheDir); err != nil {
		t.Fatal(err)
	}
	get() // deleted store
	if calls != 2 {
		t.Fatalf("deleted cache did not recompute: %d", calls)
	}
	entry := entryFiles(t, paths.CacheDir)[0]
	if err := os.WriteFile(entry, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	get() // invalid JSON
	if calls != 3 {
		t.Fatalf("corrupt cache did not recompute: %d", calls)
	}
	if err := os.RemoveAll(paths.CacheDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.CacheDir, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	get() // read and write unavailable
	get()
	if calls != 5 {
		t.Fatalf("unavailable cache did not recompute: %d", calls)
	}
}

func TestVersionBumpInvalidatesWithoutDatabaseMutation(t *testing.T) {
	paths := cachePaths(t)
	if err := os.MkdirAll(paths.RuntimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dbBytes := []byte("database sentinel")
	if err := os.WriteFile(paths.DBPath, dbBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	cache := New(paths)
	info := pythonInfo(t)
	content := []byte("def alpha(): pass\n")
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) { calls++; return Facts{}, nil }
	if _, err := cache.GetOrCompute(t.Context(), info, content, compute); err != nil {
		t.Fatal(err)
	}
	cache.schemaVersion++ // Simulates a future parser.ParseSchemaVersion bump.
	if _, err := cache.GetOrCompute(t.Context(), info, content, compute); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(entryFiles(t, paths.CacheDir)) != 2 {
		t.Fatalf("version bump used %d computes and %d entries", calls, len(entryFiles(t, paths.CacheDir)))
	}
	got, err := os.ReadFile(paths.DBPath)
	if err != nil || !bytes.Equal(got, dbBytes) {
		t.Fatalf("database changed: %q, %v", got, err)
	}
}

func TestCanceledContextBypassesDiskAndCompute(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	_, err := cache.GetOrCompute(ctx, pythonInfo(t), []byte("x"), func(context.Context, []byte) (Facts, error) {
		called = true
		return Facts{}, nil
	})
	if !errors.Is(err, context.Canceled) || called || len(entryFiles(t, paths.CacheDir)) != 0 {
		t.Fatalf("canceled call: err=%v compute=%t files=%v", err, called, entryFiles(t, paths.CacheDir))
	}
	// Cancellation takes precedence even over invalid metadata and a missing
	// callback, proving the entry guard runs before validation or disk access.
	_, err = cache.GetOrCompute(ctx, parser.LanguageInfo{}, []byte("x"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled invalid call = %v, want cancellation", err)
	}
}

func TestTamperedDigestAndMalformedFactsAreCacheMisses(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	want := fixtureFacts(t, source)
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) { calls++; return want, nil }
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil {
		t.Fatal(err)
	}
	entry := entryFiles(t, paths.CacheDir)[0]
	original, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(original, []byte("alpha"), []byte("omega"), 1)
	if bytes.Equal(tampered, original) {
		t.Fatal("fixture failed to tamper cached facts")
	}
	if err := os.WriteFile(entry, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := cache.GetOrCompute(t.Context(), info, source, compute)
	if err != nil || !reflect.DeepEqual(got, want) || calls != 2 {
		t.Fatalf("tampered cache returned %+v, %v after %d computes", got, err, calls)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(original, &envelope); err != nil {
		t.Fatal(err)
	}
	malformed, err := json.Marshal(Facts{Symbols: []parser.Symbol{{Name: "", Kind: "function", Range: parser.Range{StartByte: 0, EndByte: uint(len(source))}}}})
	if err != nil {
		t.Fatal(err)
	}
	envelope["facts"] = malformed
	sum := sha256.Sum256(malformed)
	digest, _ := json.Marshal(hex.EncodeToString(sum[:]))
	envelope["digest"] = digest
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = cache.GetOrCompute(t.Context(), info, source, compute)
	if err != nil || !reflect.DeepEqual(got, want) || calls != 3 {
		t.Fatalf("malformed facts returned %+v, %v after %d computes", got, err, calls)
	}
	// JSON null decodes to a zero struct unless the on-disk representation is
	// checked against the encoder's complete canonical Facts shape.
	envelope["facts"] = []byte("null")
	sum = sha256.Sum256(envelope["facts"])
	digest, _ = json.Marshal(hex.EncodeToString(sum[:]))
	envelope["digest"] = digest
	encoded, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = cache.GetOrCompute(t.Context(), info, source, compute)
	if err != nil || !reflect.DeepEqual(got, want) || calls != 4 {
		t.Fatalf("null facts returned %+v, %v after %d computes", got, err, calls)
	}
	badRange, err := json.Marshal(Facts{Symbols: []parser.Symbol{{Name: "alpha", Kind: "function", Range: parser.Range{StartByte: 0, EndByte: uint(len(source) + 1)}}}})
	if err != nil {
		t.Fatal(err)
	}
	envelope["facts"] = badRange
	sum = sha256.Sum256(badRange)
	digest, _ = json.Marshal(hex.EncodeToString(sum[:]))
	envelope["digest"] = digest
	encoded, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = cache.GetOrCompute(t.Context(), info, source, compute)
	if err != nil || !reflect.DeepEqual(got, want) || calls != 5 {
		t.Fatalf("out-of-source range returned %+v, %v after %d computes", got, err, calls)
	}
}

func TestMismatchedKeyAndOversizedEntryAreCacheMisses(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	calls := 0
	compute := func(context.Context, []byte) (Facts, error) { calls++; return Facts{}, nil }
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil {
		t.Fatal(err)
	}
	entry := entryFiles(t, paths.CacheDir)[0]
	original, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(original, &envelope); err != nil {
		t.Fatal(err)
	}
	var identity map[string]any
	if err := json.Unmarshal(envelope["identity"], &identity); err != nil {
		t.Fatal(err)
	}
	identity["schema_version"] = parser.ParseSchemaVersion + 1
	envelope["identity"], err = json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil || calls != 2 {
		t.Fatalf("mismatched key hit: err=%v computes=%d", err, calls)
	}
	oversized := make([]byte, maxEntryBytes+1)
	copy(oversized, original)
	for i := len(original); i < len(oversized); i++ {
		oversized[i] = ' '
	}
	if err := os.WriteFile(entry, oversized, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil || calls != 3 {
		t.Fatalf("oversized entry hit: err=%v computes=%d", err, calls)
	}
}

func TestComputeErrorAndCancellationAreNeverMasked(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	wantErr := errors.New("extract failed")
	partial := Facts{HasSyntaxErrors: true}
	got, err := cache.GetOrCompute(t.Context(), info, source, func(context.Context, []byte) (Facts, error) {
		return partial, wantErr
	})
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, partial) || len(entryFiles(t, paths.CacheDir)) != 0 {
		t.Fatalf("compute error was masked or cached: %+v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	got, err = cache.GetOrCompute(ctx, info, source, func(context.Context, []byte) (Facts, error) {
		cancel()
		return Facts{}, nil
	})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, Facts{}) || len(entryFiles(t, paths.CacheDir)) != 0 {
		t.Fatalf("mid-compute cancellation was masked or cached: %+v, %v", got, err)
	}
}

func TestMalformedComputedFactsAreRejectedBeforeWrite(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	_, err := cache.GetOrCompute(t.Context(), pythonInfo(t), []byte("x"), func(context.Context, []byte) (Facts, error) {
		return Facts{Symbols: []parser.Symbol{{Name: "x", Kind: "function", Range: parser.Range{EndByte: 2}}}}, nil
	})
	if !errors.Is(err, ErrInvalidFacts) || len(entryFiles(t, paths.CacheDir)) != 0 {
		t.Fatalf("malformed computed facts: err=%v entries=%v", err, entryFiles(t, paths.CacheDir))
	}
}

func TestFailedAtomicReplaceCleansTemporaryFile(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	compute := func(context.Context, []byte) (Facts, error) { return Facts{}, nil }
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil {
		t.Fatal(err)
	}
	entry := entryFiles(t, paths.CacheDir)[0]
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(entry, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil {
		t.Fatalf("cache rename failure blocked computation: %v", err)
	}
	entries, err := os.ReadDir(paths.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("atomic write left temporary entries: %v", entries)
	}
}

// The parent runs the dangerous probe in a bounded child process. Before the
// regular-file guard, os.Open on a FIFO blocks even after context cancellation.
func TestNonRegularCacheEntryDoesNotBlock(t *testing.T) {
	if kind := os.Getenv("MINDRAIL_SNAPSHOT_FIFO_CHILD"); kind != "" {
		runFIFOProbe(t, kind)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("FIFO fixture requires a Unix mkfifo command")
	}
	if _, err := exec.LookPath("mkfifo"); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	for _, kind := range []string{"direct", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNonRegularCacheEntryDoesNotBlock$")
			cmd.Env = append(os.Environ(), "MINDRAIL_SNAPSHOT_FIFO_CHILD="+kind)
			out, err := cmd.CombinedOutput()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("cache read blocked on %s FIFO until child timeout: %s", kind, out)
			}
			if err != nil {
				t.Fatalf("%s FIFO child failed: %v: %s", kind, err, out)
			}
		})
	}
}

func runFIFOProbe(t *testing.T, kind string) {
	t.Helper()
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("x")
	compute := func(context.Context, []byte) (Facts, error) { return Facts{}, nil }
	if _, err := cache.GetOrCompute(t.Context(), info, source, compute); err != nil {
		t.Fatal(err)
	}
	entry := entryFiles(t, paths.CacheDir)[0]
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	fifo := entry
	if kind == "symlink" {
		fifo = filepath.Join(paths.CacheDir, "target.fifo")
	}
	if out, err := exec.CommandContext(t.Context(), "mkfifo", fifo).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %v: %s", err, out)
	}
	if kind == "symlink" {
		if err := os.Symlink(fifo, entry); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	got, err := cache.GetOrCompute(ctx, info, source, func(context.Context, []byte) (Facts, error) {
		calls++
		return Facts{}, nil
	})
	if err != nil || !reflect.DeepEqual(got, Facts{}) || calls != 1 {
		t.Fatalf("nonregular entry %s: facts=%+v err=%v computes=%d", kind, got, err, calls)
	}
}

func TestConcurrentSameKeyWritersLeaveOneReadableEntry(t *testing.T) {
	paths := cachePaths(t)
	cache := New(paths)
	info := pythonInfo(t)
	source := []byte("def alpha(): pass\n")
	want := fixtureFacts(t, source)
	const writers = 16
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := cache.GetOrCompute(t.Context(), info, source, func(context.Context, []byte) (Facts, error) { return want, nil })
			if err == nil && !reflect.DeepEqual(got, want) {
				err = errors.New("concurrent writer returned different facts")
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := cache.GetOrCompute(t.Context(), info, source, func(context.Context, []byte) (Facts, error) {
		return Facts{}, errors.New("final entry was not readable")
	})
	if err != nil || !reflect.DeepEqual(got, want) || len(entryFiles(t, paths.CacheDir)) != 1 {
		t.Fatalf("concurrent final entry: facts=%+v err=%v entries=%v", got, err, entryFiles(t, paths.CacheDir))
	}
}
