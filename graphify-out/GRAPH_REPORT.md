# Graph Report - Mindrail  (2026-09-05)

## Corpus Check
- 149 files · ~191,617 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 2010 nodes · 4959 edges · 117 communities (105 shown, 7 thin omitted)
- Extraction: 91% EXTRACTED · 9% INFERRED · 0% AMBIGUOUS · INFERRED: 471 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `a399c6c5`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- MCP Tool Surface
- Acceptance Criteria
- Impact Engine
- MR-004 Safe leases, idempotency and optimistic revision
- Mindrail Technology Stack and Implementation Guide
- Evidence Types
- MR-002 Versioned Decision/Invariant knowledge lifecycle
- Mindrail Technical Specification 1.0
- MR-007 Reconcile-first actual change discovery
- db.go
- Interactive Performance Contract
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- github.com/spf13/cobra.Command
- NewLoader
- Load
- 5. Test plan
- contract_test.go
- context.Context
- New
- status/render_test.go
- App
- checks.go
- DefaultChecks
- PayloadOf
- What You Must Do When Invoked
- envelope_test.go
- Verdict
- TestBrokenSetupMatrix
- app/arch_test.go
- loader/loader_test.go
- Build
- validate.py
- Phase 3 — Doc ↔ Code consistency
- Code
- status/render.go
- Final report
- properties
- Task Verification Report
- Test Suite Health & Redundancy Audit
- cli/arch_test.go
- testing.T
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- DomainError
- MR-006 Durable symbol_uid allocation and rename/move protection
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- Probe
- openTemp
- inventory.py
- Reconciliation — Refutation, Not Agreement
- Planning (Phases 0–7)
- Autonomous Engineering Orchestrator
- Analysis (Phases 0–7)
- Test Suite Audit & Redundancy Elimination
- State
- Reader Protocol — Conformance
- graphify reference: extra exports and benchmark
- Check
- required
- Documentation ↔ Codebase Consistency & Integrity Auditor
- Evidence (Phases 8–13)
- enum
- required
- Probe
- decision.v1.schema.json
- scope
- invariant.v1.schema.json
- scope
- Classification — Categories, Severity, Confidence, Fix Decisions
- graphify reference: query, path, explain
- Deletion Gates (Phases 14–17)
- goldenReport
- NewError
- status
- status
- graphify reference: add a URL and watch a folder
- graphify reference: commit hook and native CLAUDE.md integration
- graphify reference: incremental update and cluster-only
- Root
- created_at
- decision
- id
- schema_version
- id
- schema_version
- graphify reference: GitHub clone and cross-repo merge
- graphify reference: transcribe video and audio
- CLAUDE.md
- .claude/CLAUDE.md
- extraction-spec.md
- Component
- storage/arch_test.go
- Open
- io.Writer
- Mindrail
- report.go
- app.go
- halt
- InTx
- Options

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 72 edges
2. `NewError()` - 67 edges
3. `App` - 42 edges
4. `Build()` - 37 edges
5. `Open()` - 32 edges
6. `Mindrail Technology Stack and Implementation Guide` - 32 edges
7. `ExitCode()` - 30 edges
8. `DefaultChecks()` - 30 edges
9. `Mindrail Technical Specification 1.0` - 30 edges
10. `NewAdapter()` - 29 edges

## Surprising Connections (you probably didn't know these)
- `Managed Section Markers` --implements--> `Repository Layout`  [INFERRED]
  AGENTS.md → .docs/mindrail-technical-specification-1.0.md
- `Not Yet Active Status Note` --references--> `Mindrail 0.1 Kernel Scope`  [INFERRED]
  AGENTS.md → .docs/mindrail-0.1-kernel-scope.md
- `First Implementation Milestones` --conceptually_related_to--> `Dependency Waves 1-8`  [INFERRED]
  .docs/mindrail-tech-stack.md → .tasks/mindrail-0.1-task-list.md
- `Out Of Scope` --conceptually_related_to--> `0.1 Deferred Delivery Backlog`  [INFERRED]
  .docs/mindrail-0.1-kernel-scope.md → .tasks/mindrail-0.1-task-list.md
- `MR-001 Local repository bootstrap and diagnostics path` --implements--> `SQLite Migrations`  [EXTRACTED]
  .tasks/mindrail-0.1-task-list.md → .docs/mindrail-tech-stack.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Milestone Alignment Between Documents** — _docs_mindrail_tech_stack_milestone_mapping_table, _docs_mindrail_tech_stack_first_implementation_milestones, _docs_mindrail_tech_stack_reconcile_in_fifth_milestone, _docs_mindrail_technical_specification_1_0_milestone_4, _docs_mindrail_technical_specification_1_0_milestone_5, _docs_mindrail_technical_specification_1_0_milestone_8 [EXTRACTED 1.00]
- **Reconcile-First Correctness Spine** — _docs_mindrail_technical_specification_1_0_reconcile_workflow, _docs_mindrail_technical_specification_1_0_session_correctness_path, _docs_mindrail_technical_specification_1_0_unregistered_change, _docs_mindrail_tech_stack_reconcile_implementation_rule, _docs_mindrail_tech_stack_protocol_skipped_scenario, _docs_mindrail_0_1_kernel_scope_ac_05 [EXTRACTED 1.00]
- **Thirteen-Tool Agent Protocol Surface** — _docs_mindrail_technical_specification_1_0_mcp_tool_surface, _docs_mindrail_technical_specification_1_0_agent_protocol_instructions, _docs_mindrail_technical_specification_1_0_managed_section_tool_coverage, agents_mindrail_protocol_managed_section, agents_protocol_call_sequence, _docs_mindrail_0_1_kernel_scope_mcp_parameter_surface [EXTRACTED 1.00]

## Communities (117 total, 7 thin omitted)

### Community 0 - "MCP Tool Surface"
Cohesion: 0.17
Nodes (36): AC-14, AC-21, MCP Parameter Surface, Core E2E Workflow, MCP Contract Tests, MCP Go SDK, MCP Layering, Protocol Compliant Scenario (+28 more)

### Community 1 - "Acceptance Criteria"
Cohesion: 0.09
Nodes (49): AC-05, AC-09, AC-10, AC-11, AC-12, AC-19, AC-20, Acceptance Criteria (+41 more)

### Community 2 - "Impact Engine"
Cohesion: 0.07
Nodes (42): AC-22, Out Of Scope, Coverage Provider Interface, Explainability Storage, Impact Graph Storage, Milestone Mapping Table, Validation Budget Impl, AC-04 (+34 more)

### Community 3 - "MR-004 Safe leases, idempotency and optimistic revision"
Cohesion: 0.09
Nodes (31): AC-01, AC-03, AC-04, AC-15, AC-16, Agent Workspace Modes, CLI JSON Output, Cobra CLI (+23 more)

### Community 4 - "Mindrail Technology Stack and Implementation Guide"
Cohesion: 0.09
Nodes (35): ADR List, Application Wiring, CI Integration, Dependency Matrix, Mindrail Technology Stack and Implementation Guide, Durable Symbol Identity Impl, Explicit Non Choices, Fixture Repositories (+27 more)

### Community 5 - "Evidence Types"
Cohesion: 0.14
Nodes (24): AC-08, AC-17, Approval Provider Architecture, Filesystem Safety, Process Tree Cleanup, Secret Redaction Pipeline, Security Boundaries, TOML Config (+16 more)

### Community 6 - "MR-002 Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.18
Nodes (20): AC-02, Schema Policy v1, Compatibility Surfaces, JSON Schema Validation, Knowledge Schema Compatibility, AC-16, AC-17, AC-18 (+12 more)

### Community 7 - "Mindrail Technical Specification 1.0"
Cohesion: 0.12
Nodes (23): Managed Resolver Cache, Pyright Resolver, Resolver Interface, Semantic Resolver Out Of Process, TypeScript Resolver, AC-01, AC-02, AC-03 (+15 more)

### Community 8 - "MR-007 Reconcile-first actual change discovery"
Cohesion: 0.15
Nodes (22): AC-06, Cold Index Worker Model, Content Addressed Parse Cache, Fsnotify Watcher, Telemetry and GC, AC-13, AC-14, AC-15 (+14 more)

### Community 9 - "db.go"
Cohesion: 0.16
Nodes (20): busyBudget(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), DB, Options, notWALFailure(), openFailure() (+12 more)

### Community 10 - "Interactive Performance Contract"
Cohesion: 0.20
Nodes (16): AC-13, Performance Targets, Concurrency Model, Interactive Latency Engineering, Priority Scheduler, Resolver Pool Eviction, AC-25, AC-26 (+8 more)

### Community 11 - "ResolveRuntimePaths"
Cohesion: 0.09
Nodes (36): gitLayout, PathOptions, RootKind, os.FileInfo, ensureDir(), RuntimePaths, hasParentSegment(), overrideOrDefault() (+28 more)

### Community 12 - "ADR-0002 — SQLite driver"
Cohesion: 0.33
Nodes (5): ADR-0002 — SQLite driver, Consequences, Context, Decision, Open — benchmark matrix required before this ADR is accepted

### Community 13 - "ADR-0001 — Primary language and CLI stack"
Cohesion: 0.40
Nodes (4): ADR-0001 — Primary language and CLI stack, Consequences, Context, Decision

### Community 15 - "github.com/spf13/cobra.Command"
Cohesion: 0.07
Nodes (39): globalFlags, humanRenderer, invocation, versionInfo, main(), context.CancelFunc, github.com/spf13/cobra.Command, log/slog.Logger (+31 more)

### Community 16 - "NewLoader"
Cohesion: 0.07
Nodes (50): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, OutputConfig, ProjectConfig, Provenance (+42 more)

### Community 17 - "Load"
Cohesion: 0.12
Nodes (47): testing/fstest.MapFS, assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), Load(), keys(), TestInitialMigrationCreatesOnlyMR001Tables(), TestLoadEmbeddedMigrationsAreOrderedAndUnique() (+39 more)

### Community 18 - "5. Test plan"
Cohesion: 0.04
Nodes (47): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+39 more)

### Community 19 - "contract_test.go"
Cohesion: 0.15
Nodes (35): result, addWorktree(), assertFourErrorKeys(), assertGolden(), assertShape(), findCheck(), isEmptyValue(), join() (+27 more)

### Community 20 - "context.Context"
Cohesion: 0.05
Nodes (61): FixedClock, SystemClock, FakeResponse, Invocation, context.Context, database/sql.Tx, sync.Mutex, time.Time (+53 more)

### Community 21 - "New"
Cohesion: 0.09
Nodes (45): Recorder, refusingTransport, stepRecorder, txProbe, net/http.Request, net/http.Response, sync/atomic.Bool, sync/atomic.Int64 (+37 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.24
Nodes (16): InitReport, assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport(), renderHuman() (+8 more)

### Community 23 - "App"
Cohesion: 0.12
Nodes (6): InitResult, io/fs.FS, sync.Once, asRepositoryError(), App, New()

### Community 24 - "checks.go"
Cohesion: 0.15
Nodes (32): diagnosis, ConfigCheck(), configResult(), describePath(), describeProblems(), describeStartDir(), explain(), failure() (+24 more)

### Community 25 - "DefaultChecks"
Cohesion: 0.16
Nodes (35): NewRunner(), TestRunnerRespectsContextCancellation(), DefaultChecks(), KnowledgeCheck(), assertActionable(), assertDiagnosable(), Result, Subject (+27 more)

### Community 26 - "PayloadOf"
Cohesion: 0.06
Nodes (78): Adapter, ExecRunner, os/exec.Cmd, time.Duration, ErrorPayload, PayloadOf(), ExitCode(), bareRepositoryError() (+70 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "envelope_test.go"
Cohesion: 0.22
Nodes (13): brokenSetup, envelopeScenario, strictEnvelope, assertNoErrorEnvelope(), decodeData(), runWith(), assertEnvelopeCoherent(), decodeStrictEnvelope() (+5 more)

### Community 29 - "Verdict"
Cohesion: 0.27
Nodes (8): CheckFunc, errorFrom(), Report, Result, Subject, haltError(), kindForCode(), Verdict()

### Community 30 - "TestBrokenSetupMatrix"
Cohesion: 0.18
Nodes (25): os.FileMode, chmodForTest(), configPath(), corruptDatabase(), createUnmigratedDatabase(), denyAccess(), denyWrites(), hangingGit() (+17 more)

### Community 31 - "app/arch_test.go"
Cohesion: 0.67
Nodes (5): importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib()

### Community 32 - "loader/loader_test.go"
Cohesion: 0.08
Nodes (41): encoding/json.RawMessage, io/fs.DirEntry, Loader, isRecordFile(), readFailureReason(), recordPath(), stringField(), decisionJSON() (+33 more)

### Community 33 - "Build"
Cohesion: 0.22
Nodes (22): Subject, TestStatusHumanNamesCommonDirAndWorktree(), Build(), currentSchemaVersion(), stoppedAtStep(), assertActionable(), buildSubjects(), haltedAtSQLiteSubject() (+14 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "Code"
Cohesion: 0.38
Nodes (9): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+1 more)

### Community 37 - "status/render.go"
Cohesion: 0.18
Nodes (16): strings.Builder, Report, Result, paint(), writeBlock(), writeResult(), Report, joinInts() (+8 more)

### Community 38 - "Final report"
Cohesion: 0.11
Nodes (17): 1. Task, 2. Requirements, 3. Teams, 4. Execution, 5. Implementation, 6. Validation, 7. Dual-agent audit, 8. Remaining issues (+9 more)

### Community 39 - "properties"
Cohesion: 0.12
Nodes (17): description, format, type, const, type, properties, created_at, kind (+9 more)

### Community 40 - "Task Verification Report"
Cohesion: 0.12
Nodes (15): 1. Original Task, 2. Requirement Matrix, 3. Confirmed Findings, 4. Refuted Findings, 5. Open Items, 6. Off-Spec Headings, 7. Scope Compliance, 8. Test & Verification Assessment (+7 more)

### Community 41 - "Test Suite Health & Redundancy Audit"
Cohesion: 0.12
Nodes (15): 10. Cleanup Plan, 11. Before / After Comparison, 12. Final Confidence Assessment, 1. Executive Summary, 2. Test Inventory, 3. Behavior Coverage Map, 4. Redundancy Clusters, 5. Deletion Candidates (+7 more)

### Community 42 - "cli/arch_test.go"
Cohesion: 0.29
Nodes (15): firstPartyPackage, assertMainDeclaresOnlyMain(), assertNoNetworkInLinkedPackages(), fileImports(), firstPartyPackages(), goSources(), isStandardLibrary(), matchForbidden() (+7 more)

### Community 43 - "testing.T"
Cohesion: 0.14
Nodes (28): assertSQLiteCheckHealthy(), shellQuote(), TestGracefulShutdownOnSIGINT(), writeSlowGit(), addWorktree(), buildBinary(), decodeEnvelope(), entriesIn() (+20 more)

### Community 44 - "Breaker Protocol — Falsification"
Cohesion: 0.13
Nodes (14): B0 — Isolation and safety, B1 — Mutation: delete each guard, B2 — A/B of deleted behavior, B3 — Exercise the written threat model, B4 — Upgrade path, not fresh install, B5 — Weakest satisfying input: read your own predicate, B6 — Dual readers: one question answered in two places, Backward compatibility (+6 more)

### Community 45 - "properties"
Cohesion: 0.13
Nodes (15): description, type, description, type, const, type, properties, consequences (+7 more)

### Community 46 - "Report Template"
Cohesion: 0.14
Nodes (13): 10. Final Integrity Matrix, 1. Executive Summary, 2. Documentation Inventory, 3. Documentation ↔ Documentation Findings, 4. Documentation ↔ Code Findings, 5. Detailed Findings, 6. Canonical Documentation Map, 7. Fix Roadmap (+5 more)

### Community 47 - "Dual-Agent Task Audit"
Cohesion: 0.14
Nodes (13): Consensus is inverted, Dual-Agent Task Audit, Environment adaptation, Evidence: a number measured twice, Getting started, Mandatory off-spec headings, No lead bucket, Orchestrator rules (+5 more)

### Community 48 - "DomainError"
Cohesion: 0.14
Nodes (12): DomainError, Error, Denied(), exitCodeForKind(), Failed(), Kind, TestErrorMessageUsesCause(), TestErrorUnwrapsToCause() (+4 more)

### Community 49 - "MR-006 Durable symbol_uid allocation and rename/move protection"
Cohesion: 0.24
Nodes (13): AC-07, AC-18, Stable Identifiers, AC-28, AC-29, Core Contract, Durable Symbol Identity, Orphan Protection (+5 more)

### Community 50 - "mutate.py"
Cohesion: 0.33
Nodes (12): added_lines(), c_if_condition(), changed_files(), find_guards(), is_test_path(), main(), mutate_line(), Path (+4 more)

### Community 51 - "enum"
Cohesion: 0.15
Nodes (13): FILE, MODULE, PACKAGE, PROJECT, SYMBOL, enum, type, level (+5 more)

### Community 52 - "enum"
Cohesion: 0.15
Nodes (13): FILE, MODULE, PACKAGE, PROJECT, SYMBOL, enum, type, level (+5 more)

### Community 53 - "Execution (Phases 8–17)"
Cohesion: 0.17
Nodes (11): Execution (Phases 8–17), Phase 10 — Parallel execution, Phase 11 — Coordination between agents, Phase 12 — Decision log, Phase 13 — Escalation, Phase 14 — Continuous task validation, Phase 15 — Testing strategy, Phase 16 — Integration (+3 more)

### Community 54 - "supersedes"
Cohesion: 0.17
Nodes (12): minLength, pattern, type, supersedes, tags, description, items, type (+4 more)

### Community 55 - "supersedes"
Cohesion: 0.17
Nodes (12): minLength, pattern, type, supersedes, tags, description, items, type (+4 more)

### Community 56 - "changeset.py"
Cohesion: 0.35
Nodes (10): build(), classify(), main(), parse_name_status(), parse_numstat(), print_summary(), Path, Rough module key so a source file and its test can be matched. (+2 more)

### Community 57 - "test_inventory.py"
Cohesion: 0.33
Nodes (10): build(), detect_framework(), extract_tests(), looks_like_test_path(), main(), normalize_body(), print_summary(), Path (+2 more)

### Community 58 - "Probe"
Cohesion: 0.27
Nodes (10): App, RuntimePathCheck(), SQLiteCheck(), Subject, subjectAt(), TestDirectoryAtTheDatabasePathIsNotAMissingDatabase(), TestProbeIsIdempotentAndSkipsUnreachedSteps(), TestProbeOnAHealthyRepositoryStaysOK() (+2 more)

### Community 59 - "openTemp"
Cohesion: 0.22
Nodes (17): openTemp(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection(), TestOpenCreatesWALSidecar(), TestOpenReadOnlyAfterWriterClosed(), TestOpenReadOnlyRejectsWrites() (+9 more)

### Community 60 - "inventory.py"
Cohesion: 0.47
Nodes (9): analyze_doc(), build_inventory(), classify_special(), extract_env_from_code(), is_excluded(), main(), print_summary(), Path (+1 more)

### Community 61 - "Reconciliation — Refutation, Not Agreement"
Cohesion: 0.20
Nodes (9): Class sweep closure, Final compliance decision, Final verdict, No unresolved bucket, Reconciliation — Refutation, Not Agreement, Severity reconciliation, The admission rule, The fix carries its own proof (+1 more)

### Community 62 - "Planning (Phases 0–7)"
Cohesion: 0.20
Nodes (9): Phase 0 — Task intake, Phase 1 — Project discovery, Phase 2 — Clarification gate, Phase 3 — Requirement normalization, Phase 4 — Acceptance criteria, Phase 5 — Solution design, Phase 6 — Work breakdown, Phase 7 — Dependency graph (+1 more)

### Community 63 - "Autonomous Engineering Orchestrator"
Cohesion: 0.20
Nodes (9): Autonomous Engineering Orchestrator, Completion bar, Evidence standard, Lifecycle, Orchestrator rules, Step 1 — Right-size the ceremony, Step 2 — Adapt to the environment, The one rule that matters most (+1 more)

### Community 64 - "Analysis (Phases 0–7)"
Cohesion: 0.20
Nodes (9): Analysis (Phases 0–7), Phase 0 — Suite inventory, Phase 1 — Behavior map, Phase 2 — Test classification, Phase 3 — Duplication clustering, Phase 4 — Implementation-detail tests, Phase 5 — Framework/library tests, Phase 6 — Parametric explosion (+1 more)

### Community 65 - "Test Suite Audit & Redundancy Elimination"
Cohesion: 0.20
Nodes (9): Completion bar, Core stance, Deletion is never automatic, Flaky tests, Getting started, Output, Scoping the expensive measurements, Test Suite Audit & Redundancy Elimination (+1 more)

### Community 66 - "State"
Cohesion: 0.29
Nodes (5): Report, State, severity(), States(), TestStateEnumIsExactlyFiveValues()

### Community 67 - "Reader Protocol — Conformance"
Cohesion: 0.22
Nodes (8): R1 — Task normalization, R2 — Requirement verdicts, R3 — Scope compliance, R4 — Contract and documentation conformance, R5 — Evidence standard, R6 — Severity and confidence, R7 — Reader report, Reader Protocol — Conformance

### Community 68 - "graphify reference: extra exports and benchmark"
Cohesion: 0.22
Nodes (8): graphify reference: extra exports and benchmark, Step 6b - Wiki (only if --wiki flag), Step 7 - Neo4j export (only if --neo4j or --neo4j-push flag), Step 7a - FalkorDB export (only if --falkordb or --falkordb-push flag), Step 7b - SVG export (only if --svg flag), Step 7c - GraphML export (only if --graphml flag), Step 7d - MCP server (only if --mcp flag), Step 8 - Token reduction benchmark (only if total_words > 5000)

### Community 69 - "Check"
Cohesion: 0.25
Nodes (12): Check, Runner, okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestCheckFuncWithoutFunctionIsNotSilent(), TestReportErrExitCodeFollowsTheFailureClass() (+4 more)

### Community 70 - "required"
Cohesion: 0.22
Nodes (9): scope, severity, statement, created_at, id, kind, schema_version, status (+1 more)

### Community 71 - "Documentation ↔ Codebase Consistency & Integrity Auditor"
Cohesion: 0.25
Nodes (7): Applying fixes (Phase 9), Completion bar, Core stance, Documentation ↔ Codebase Consistency & Integrity Auditor, Getting started, Output, Workflow

### Community 72 - "Evidence (Phases 8–13)"
Cohesion: 0.25
Nodes (7): Evidence (Phases 8–13), Phase 10 — Assertion strength, Phase 11 — Mock quality, Phase 12 — Git history and regression value, Phase 13 — Risk weighting, Phase 8 — Coverage contribution, Phase 9 — Mutation impact

### Community 73 - "enum"
Cohesion: 0.25
Nodes (8): CRITICAL, HIGH, LOW, MEDIUM, severity, description, enum, type

### Community 74 - "required"
Cohesion: 0.25
Nodes (8): decision, title, created_at, id, kind, schema_version, status, required

### Community 75 - "Probe"
Cohesion: 0.20
Nodes (16): io/fs.FileInfo, io/fs.FileMode, absentFailure(), classify(), describeMode(), Presence, Probe(), probeUnresolved() (+8 more)

### Community 76 - "decision.v1.schema.json"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 77 - "scope"
Cohesion: 0.29
Nodes (7): $defs, scope, level, additionalProperties, description, required, type

### Community 78 - "invariant.v1.schema.json"
Cohesion: 0.29
Nodes (6): additionalProperties, description, $id, $schema, title, type

### Community 79 - "scope"
Cohesion: 0.29
Nodes (7): $defs, scope, level, additionalProperties, description, required, type

### Community 80 - "Classification — Categories, Severity, Confidence, Fix Decisions"
Cohesion: 0.33
Nodes (5): Classification — Categories, Severity, Confidence, Fix Decisions, Confidence, Finding categories, Fix decisions, Severity

### Community 81 - "graphify reference: query, path, explain"
Cohesion: 0.33
Nodes (5): For /graphify explain, For /graphify path, graphify reference: query, path, explain, Step 0 — Constrained query expansion (REQUIRED before traversal), Step 1 — Traversal

### Community 82 - "Deletion Gates (Phases 14–17)"
Cohesion: 0.33
Nodes (5): Deletion Gates (Phases 14–17), Phase 14 — Deletion confidence, Phase 15 — Safety gate, Phase 16 — Controlled removal, Phase 17 — Consolidation

### Community 83 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 84 - "NewError"
Cohesion: 0.21
Nodes (14): AdoptCause(), CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain() (+6 more)

### Community 85 - "status"
Cohesion: 0.33
Nodes (6): active, superseded, status, description, enum, type

### Community 86 - "status"
Cohesion: 0.33
Nodes (6): active, superseded, status, description, enum, type

### Community 87 - "graphify reference: add a URL and watch a folder"
Cohesion: 0.50
Nodes (3): For /graphify add, For --watch, graphify reference: add a URL and watch a folder

### Community 88 - "graphify reference: commit hook and native CLAUDE.md integration"
Cohesion: 0.50
Nodes (3): For git commit hook, For native CLAUDE.md integration, graphify reference: commit hook and native CLAUDE.md integration

### Community 89 - "graphify reference: incremental update and cluster-only"
Cohesion: 0.50
Nodes (3): For --cluster-only, For --update (incremental re-extraction), graphify reference: incremental update and cluster-only

### Community 90 - "Root"
Cohesion: 0.16
Nodes (15): canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput(), NewRoot(), Normalize(), newTestRoot() (+7 more)

### Community 91 - "created_at"
Cohesion: 0.50
Nodes (4): description, format, type, created_at

### Community 92 - "decision"
Cohesion: 0.50
Nodes (4): description, minLength, type, decision

### Community 93 - "id"
Cohesion: 0.50
Nodes (4): description, pattern, type, id

### Community 94 - "schema_version"
Cohesion: 0.50
Nodes (4): schema_version, const, description, type

### Community 95 - "id"
Cohesion: 0.50
Nodes (4): description, pattern, type, id

### Community 96 - "schema_version"
Cohesion: 0.50
Nodes (4): schema_version, const, description, type

### Community 107 - "Component"
Cohesion: 0.16
Nodes (12): Components(), ComponentName, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown(), TestTerminalStateSpellingIsTheSpecLiteral(), paintReadiness() (+4 more)

### Community 108 - "storage/arch_test.go"
Cohesion: 0.33
Nodes (10): go/ast.ImportSpec, fileImports(), importPath(), moduleRoot(), packageImports(), shouldSkipDir(), TestDriverImportConfinedToStorage(), TestDriverImportUnreachableFromOtherPackages() (+2 more)

### Community 109 - "Open"
Cohesion: 0.14
Nodes (16): testing.M, TestMain(), Open(), TestCloseIsIdempotent(), TestOpenReadOnlyDoesNotCreateFile(), TestOpenRejectsCorruptDatabase(), TestOpenRejectsRelativePath(), runContender() (+8 more)

### Community 110 - "io.Writer"
Cohesion: 0.21
Nodes (14): Envelope, io.Writer, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice() (+6 more)

### Community 111 - "Mindrail"
Cohesion: 0.33
Nodes (6): Build, Documentation, License, Mindrail, Repository-owned knowledge, Requirements

### Community 112 - "report.go"
Cohesion: 0.35
Nodes (9): Report, observationOf(), stateRank(), worst(), KnowledgeInfo, Observation, RepositoryInfo, RuntimeInfo (+1 more)

### Community 113 - "app.go"
Cohesion: 0.31
Nodes (7): asKnowledgeError(), asMigrationError(), asRuntimePathError(), asWorkspaceError(), domainize(), integrityFailure(), verifyLedger()

### Community 114 - "halt"
Cohesion: 0.33
Nodes (5): halt, Probes, step, codeOf(), Subject

### Community 115 - "InTx"
Cohesion: 0.43
Nodes (6): database/sql.DB, InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow()

## Knowledge Gaps
- **424 isolated node(s):** `0. Citation audit`, `1.1 Why `internal/status` is added to §7`, `2.1 `internal/app` — Unit D`, `2.2 `internal/filesystem` — Unit A`, `2.3 `internal/git` — Unit A` (+419 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 532 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **7 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `db.go`, `ResolveRuntimePaths`, `github.com/spf13/cobra.Command`, `NewLoader`, `context.Context`, `New`, `App`, `DefaultChecks`, `PayloadOf`, `Verdict`, `TestBrokenSetupMatrix`, `loader/loader_test.go`, `Build`, `Code`, `DomainError`, `Probe`, `Root`, `io.Writer`, `app.go`?**
  _High betweenness centrality (0.032) - this node is a cross-community bridge._
- **Why does `App` connect `App` to `Build`, `Check`, `db.go`, `ResolveRuntimePaths`, `io.Writer`, `github.com/spf13/cobra.Command`, `NewLoader`, `app.go`, `NewError`, `New`, `context.Context`, `PayloadOf`, `openTemp`, `Verdict`?**
  _High betweenness centrality (0.018) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `db.go`, `ResolveRuntimePaths`, `github.com/spf13/cobra.Command`, `NewLoader`, `Load`, `contract_test.go`, `context.Context`, `New`, `App`, `checks.go`, `DefaultChecks`, `envelope_test.go`, `loader/loader_test.go`, `Probe`, `NewError`, `Root`, `Open`, `io.Writer`, `app.go`, `halt`?**
  _High betweenness centrality (0.016) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **What connects `0. Citation audit`, `1.1 Why `internal/status` is added to §7`, `2.1 `internal/app` — Unit D` to the rest of the system?**
  _424 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Acceptance Criteria` be split into smaller, more focused modules?**
  _Cohesion score 0.09306122448979592 - nodes in this community are weakly interconnected._