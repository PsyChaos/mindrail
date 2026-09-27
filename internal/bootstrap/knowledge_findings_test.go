package bootstrap_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/internal/moduletree"
	"github.com/PsyChaos/mindrail/schemas"
)

// validRecord is a Decision every rule of decision.v1 accepts, filed at the path
// step 6 requires for its id.
const validRecord = `{
  "schema_version": 1,
  "kind": "decision",
  "id": "DEC-0001",
  "status": "active",
  "created_at": "2026-03-14T09:26:53Z",
  "title": "Keep the runtime state inside the repository",
  "decision": "Runtime state lives under the Git common directory."
}`

// brokenRecord is readable JSON of the readable schema version, so the loader
// records no Problem for it, and wrong in a way only step 5 can see: the id does
// not match the pattern decision.v1 pins. That separation is the whole point of
// the pairing — the loader ignores it and the pipeline must not.
const brokenRecord = `{
  "schema_version": 1,
  "kind": "decision",
  "id": "DEC-0002",
  "status": "handwritten",
  "created_at": "yesterday",
  "title": "",
  "decision": "We chose this."
}`

// writeRecord files one decision where the loader walks for them.
func writeRecord(t *testing.T, repo, name, body string) string {
	t.Helper()

	rel := filepath.Join(".mindrail", "knowledge", "decisions", name)
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create the decisions directory: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return filepath.ToSlash(rel)
}

// TestInvalidRecordsAreFindingsAndDoNotHaltStartup is AC-08.3 and decision D-42.
//
// A store full of records this binary read and rejected is a repository
// condition the report describes, not a failure that stops the report being
// built. Setting KnowledgeErr here would halt the sequence and leave every later
// block saying it was never observed — publishing a fabricated absence in place
// of the finding this step actually made.
//
// The assertion that the sequence kept running is the step *after* knowledge
// having produced its result, not the absence of an error: an error swallowed
// and a step that ran look the same from the return value alone.
func TestInvalidRecordsAreFindingsAndDoNotHaltStartup(t *testing.T) {
	repo := newGitRepo(t)
	initialize(t, repo)

	writeRecord(t, repo, "DEC-0001.json", validRecord)
	broken := writeRecord(t, repo, "DEC-0002.json", brokenRecord)

	recorder := &stepRecorder{}
	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, func(o *bootstrap.Options) {
		o.Recorder = recorder
	}))
	defer func() {
		if err := application.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("a store with an invalid record halted startup: %v", err)
	}

	subject := application.Subject()

	if subject.KnowledgeErr != nil {
		t.Errorf("KnowledgeErr = %v; a record this binary read and rejected is data, not an error", subject.KnowledgeErr)
	}
	if len(subject.Knowledge.Problems) != 0 {
		t.Errorf("the loader recorded %d problems for records it could read: %+v",
			len(subject.Knowledge.Problems), subject.Knowledge.Problems)
	}
	if len(subject.KnowledgeFindings) == 0 {
		t.Fatal("the pipeline reported nothing about a record its own schema rejects")
	}

	for _, finding := range subject.KnowledgeFindings {
		if finding.Path != broken {
			t.Errorf("finding against %q, want %q: the valid record beside it must not be reported",
				finding.Path, broken)
		}
		if finding.Step != validate.StepSchema {
			t.Errorf("finding at step %d, want step %d", int(finding.Step), int(validate.StepSchema))
		}
	}

	// The step after the knowledge step is what proves the sequence did not
	// stop. Its recorded marker and its result are asserted together, because a
	// step can be recorded on entry and still have returned nothing.
	steps := recorder.steps()
	position := func(want bootstrap.Step) int {
		for i, recorded := range steps {
			if recorded == want {
				return i
			}
		}
		return -1
	}
	knowledgeAt := position(bootstrap.StepValidateKnowledge)
	workspaceAt := position(bootstrap.StepRegisterWorkspace)
	if knowledgeAt < 0 || workspaceAt < 0 {
		t.Fatalf("the sequence recorded %v, which does not contain both the knowledge and the workspace step", steps)
	}
	if workspaceAt < knowledgeAt {
		t.Fatalf("the workspace step ran before the knowledge step: %v", steps)
	}
	if subject.Workspace.ID == "" {
		t.Error("the workspace step ran and produced nothing, so the sequence did stop at the knowledge step")
	}
}

// TestAHealthyStoreProducesNoFindings is the over-fire guard for the wiring.
//
// The pipeline runs on every startup now, so the first thing it must not do is
// report a repository that is fine. A record every rule accepts, read through
// the real loader and judged by the real validator, must leave the subject
// carrying nothing at all.
func TestAHealthyStoreProducesNoFindings(t *testing.T) {
	repo := newGitRepo(t)
	initialize(t, repo)
	writeRecord(t, repo, "DEC-0001.json", validRecord)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	defer func() {
		if err := application.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}

	subject := application.Subject()
	if subject.Knowledge.Count() != 1 {
		t.Fatalf("the loader read %d records, want 1; the guard below would be about an empty store",
			subject.Knowledge.Count())
	}
	if len(subject.KnowledgeFindings) != 0 {
		t.Errorf("a record every rule accepts produced %d findings: %+v",
			len(subject.KnowledgeFindings), subject.KnowledgeFindings)
	}
}

// TestSchemasThisBinaryCannotCompileAreADefectInTheBinary is AC-08.2.
//
// schema.NewValidator failing means the documents this binary embeds do not
// compile. That is not a condition of the repository, and it must not be dressed
// up as one: KnowledgeErr renders as KNOWLEDGE_UNREADABLE with "Make
// .mindrail/knowledge readable." beside it, which would diagnose a repository
// that is fine and prescribe a remedy that cannot work. There is no registered
// code for "this binary cannot build its own validator" — REQ-05 freezes the two
// codes MR-002 adds — so the failure is raised in the binary's own voice.
//
// The recovered value is matched with errors.Is rather than on its wording, so
// the assertion is about which failure was raised and not about how it reads.
func TestSchemasThisBinaryCannotCompileAreADefectInTheBinary(t *testing.T) {
	repo := newGitRepo(t)
	initialize(t, repo)
	writeRecord(t, repo, "DEC-0001.json", validRecord)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, func(o *bootstrap.Options) {
		o.SchemaFS = uncompilableSchemas(t)
	}))
	defer func() {
		if err := application.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	raised := func() (recovered any) {
		defer func() { recovered = recover() }()
		if err := application.Start(t.Context()); err != nil {
			t.Errorf("Start returned %v; a binary whose schemas do not compile must not report a repository condition", err)
		}
		return nil
	}()

	if raised == nil {
		t.Fatal("a binary whose schema documents do not compile started as if they did")
	}
	failure, isError := raised.(error)
	if !isError {
		t.Fatalf("the failure was raised as %T, which carries no cause to match on: %v", raised, raised)
	}
	if !errors.Is(failure, schema.ErrSchemaNotShipped) {
		t.Errorf("the failure does not carry the compile error it came from: %v", failure)
	}

	subject := application.Subject()
	if subject.KnowledgeErr != nil {
		t.Errorf("KnowledgeErr = %v; a defect in this binary was filed as a condition of the repository", subject.KnowledgeErr)
	}
	if len(subject.KnowledgeFindings) != 0 {
		t.Errorf("findings were published from a validator that was never built: %+v", subject.KnowledgeFindings)
	}
}

// uncompilableSchemas ships both required documents, one of them registered
// under an $id nothing compiles against.
//
// NewRegistry accepts it — it asks only that the documents are present and are
// JSON — and NewValidator does not, which is exactly the split decision D-28
// describes and the only way this failure is reachable without shipping a broken
// binary.
func uncompilableSchemas(t *testing.T) fs.FS {
	t.Helper()

	invariant, err := fs.ReadFile(schemas.KnowledgeFS, "knowledge/"+schema.InvariantSchemaName)
	if err != nil {
		t.Fatalf("read the shipped invariant document: %v", err)
	}

	return fstest.MapFS{
		"knowledge/" + schema.DecisionSchemaName: &fstest.MapFile{Data: []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://example.invalid/a-document-nothing-compiles.json",
  "type": "object"
}`)},
		"knowledge/" + schema.InvariantSchemaName: &fstest.MapFile{Data: invariant},
	}
}

// TestTheKnowledgePipelineHasExactlyTwoAuthorizedCallSites retains AC-08.1's
// reporting boundary and explicitly adds remediation REQ-003's safety boundary.
//
// Bootstrap validates the store once for startup reports (D-42); doctor and
// status remain pure readers of that observation (D-41). Completion independently
// reloads and validates current knowledge before a safety-critical verdict, since
// a long-lived process's startup snapshot may now be stale. These are exactly
// two authorized callers, not permission for validation in any reporting layer.
func TestTheKnowledgePipelineHasExactlyTwoAuthorizedCallSites(t *testing.T) {
	root := moduleRoot(t)

	callers := make(map[string]int)
	inspected := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if moduletree.SkipDir(root, path) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		inspected++

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			t.Fatalf("relativise %s: %v", path, relErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Check" {
				return true
			}
			pkg, isIdent := selector.X.(*ast.Ident)
			if !isIdent || pkg.Name != "validate" {
				return true
			}
			callers[filepath.ToSlash(rel)]++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if inspected == 0 {
		t.Fatal("no production source was parsed, so this proves nothing")
	}

	want := map[string]int{
		"internal/bootstrap/app.go":      1,
		"internal/completion/compose.go": 1,
	}
	if len(callers) != len(want) {
		t.Fatalf("validate.Check is called from %v, want exactly %v", callers, want)
	}
	for file, count := range want {
		if callers[file] != count {
			t.Errorf("validate.Check is called %d times in %s, want %d", callers[file], file, count)
		}
	}
}

// moduleRoot walks up to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()

	working, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := moduletree.Root(working)
	if err != nil {
		t.Fatalf("module root above %s: %v", working, err)
	}
	return root
}
