package parser

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

var fixtures = map[string]string{
	"python":     "def greet():\n    return helper()\n",
	"javascript": "function greet() { return helper(); }",
	"typescript": "function greet(): number { return helper(); }",
	"tsx":        "function greet() { return <div>{helper()}</div>; }",
}

func registryForTest(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestRegistryExtensionsAndOrder(t *testing.T) {
	r := registryForTest(t)
	want := []string{"python", "javascript", "typescript", "tsx"}
	var got []string
	for _, entry := range r.Entries() {
		got = append(got, entry.Language)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order: %v", got)
	}
	wantEntries := []LanguageInfo{
		{"python", "v0.25.0", []string{".py"}},
		{"javascript", "v0.25.0", []string{".js", ".mjs", ".cjs"}},
		{"typescript", "v0.23.2", []string{".ts", ".mts", ".cts"}},
		{"tsx", "v0.23.2", []string{".tsx"}},
	}
	if !reflect.DeepEqual(r.Entries(), wantEntries) {
		t.Fatalf("registry: %+v", r.Entries())
	}
	for ext, language := range map[string]string{".py": "python", ".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".ts": "typescript", ".mts": "typescript", ".cts": "typescript", ".tsx": "tsx"} {
		a, ok := r.Lookup("dir/file" + ext)
		if !ok || a.Info().Language != language {
			t.Fatalf("%s: %v %v", ext, a, ok)
		}
	}
	for _, path := range []string{"file.go", "file", "file.TS", "x.py.txt"} {
		if _, ok := r.Lookup(path); ok {
			t.Fatalf("unsupported: %s", path)
		}
	}
	if ParseSchemaVersion != 2 {
		t.Fatalf("schema: %d", ParseSchemaVersion)
	}
	entries := r.Entries()
	entries[0].Extensions[0] = ".fake"
	if _, ok := r.Lookup("x.py"); !ok {
		t.Fatal("caller mutated registry")
	}
}

func TestRegistryFirstExtensionMatchWins(t *testing.T) {
	r := registryForTest(t)
	r.adapters[1].info.Extensions = append(r.adapters[1].info.Extensions, ".py")
	a, ok := r.Lookup("x.py")
	if !ok || a.Info().Language != "python" {
		t.Fatal("first match lost")
	}
}

func TestAdaptersParseAndExtract(t *testing.T) {
	r := registryForTest(t)
	for language, source := range fixtures {
		t.Run(language, func(t *testing.T) {
			a := r.adaptersByName(language)
			s, err := a.Parse(context.Background(), SourceFile{Content: []byte(source)})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			syms, err := a.Symbols(s)
			if err != nil || len(syms) != 1 || syms[0].Name != "greet" || syms[0].Kind != "function" {
				t.Fatalf("symbols: %+v, %v", syms, err)
			}
			refs, err := a.References(s)
			if err != nil || len(refs) != 1 || refs[0].Name != "helper" || refs[0].Kind != "call" {
				t.Fatalf("refs: %+v, %v", refs, err)
			}
			if syms[0].Range.StartByte != 0 || syms[0].Range.EndByte == 0 {
				t.Fatalf("range: %+v", syms[0])
			}
		})
	}
}

func TestBrokenSourceReturnsPartialTreeAndError(t *testing.T) {
	r := registryForTest(t)
	for language, valid := range fixtures {
		t.Run(language, func(t *testing.T) {
			a := r.adaptersByName(language)
			broken := valid + "\nfunction broken( {"
			if language == "python" {
				broken = valid + "\ndef broken(:\n"
			}
			s, err := a.Parse(context.Background(), SourceFile{Content: []byte(broken)})
			if s == nil || !errors.Is(err, ErrSyntax) {
				t.Fatalf("partial snapshot=%v error=%v", s, err)
			}
			defer s.Close()
			if !s.HasErrors() {
				t.Fatal("partial tree lost error state")
			}
			syms, err := a.Symbols(s)
			if err != nil || len(syms) == 0 || syms[0].Name != "greet" {
				t.Fatalf("intact declaration lost: %+v %v", syms, err)
			}
		})
	}
}

func TestAdapterRejectsInvalidAndCanceledInput(t *testing.T) {
	a := registryForTest(t).adapters[0]
	s, err := a.Parse(context.Background(), SourceFile{Content: []byte{0xff}})
	if s != nil || !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("invalid UTF8: %v %v", s, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err = a.Parse(ctx, SourceFile{})
	if s != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v %v", s, err)
	}
	s, err = a.Parse(ctx, SourceFile{Content: []byte{0xff}})
	if s != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must precede validation: %v %v", s, err)
	}
}

func TestSnapshotOwnershipAndClosedResources(t *testing.T) {
	r := registryForTest(t)
	a := r.adapters[0]
	src := []byte(fixtures["python"])
	s, err := a.Parse(context.Background(), SourceFile{Content: src})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := range src {
		src[i] = 'x'
	}
	syms, err := a.Symbols(s)
	if err != nil || syms[0].Name != "greet" {
		t.Fatalf("source alias: %+v %v", syms, err)
	}
	if _, err := r.adapters[1].Symbols(s); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("foreign snapshot: %v", err)
	}
	if _, err := a.References(nil); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("nil snapshot: %v", err)
	}
	s.Close()
	s.Close()
	if _, err := a.Symbols(s); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("closed snapshot: %v", err)
	}
	r.Close()
	r.Close()
	if _, err := a.References(s); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed query: %v", err)
	}
	if _, err := a.Parse(context.Background(), SourceFile{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed adapter: %v", err)
	}
}

func (r *Registry) adaptersByName(language string) *adapter {
	for _, a := range r.adapters {
		if a.info.Language == language {
			return a
		}
	}
	panic("unknown fixture language")
}
