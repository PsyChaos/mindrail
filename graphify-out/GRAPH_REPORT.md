# Graph Report - Mindrail  (2026-09-05)

## Corpus Check
- 184 files · ~255,608 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 2366 nodes · 6352 edges · 139 communities (126 shown, 6 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 826 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `8111c321`
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
- Resolver Manager
- MR-005 Python/TypeScript/JavaScript structural indexing
- DB
- Interactive Performance Contract
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- io.Writer
- NewLoader
- New
- 5. Test plan
- contract_test.go
- newStore
- New
- status/render_test.go
- App
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- Migrator
- Verdict
- Reconcile Workflow
- storage/arch_test.go
- loader/loader_test.go
- healthySubject
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
- context.Context
- Completion Gate
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- explain
- Open
- inventory.py
- Reconciliation — Refutation, Not Agreement
- Planning (Phases 0–7)
- Autonomous Engineering Orchestrator
- Analysis (Phases 0–7)
- Test Suite Audit & Redundancy Elimination
- State
- Reader Protocol — Conformance
- graphify reference: extra exports and benchmark
- check_test.go
- required
- Documentation ↔ Codebase Consistency & Integrity Auditor
- Evidence (Phases 8–13)
- enum
- required
- ProbeWriteAccess
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
- TestBrokenSetupMatrix
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
- Readiness
- gitfile.go
- open_contention_test.go
- newInitializedRepo
- WriteFailure
- Build
- DomainError
- loader/loader.go
- InTx
- Root
- RootKind
- newTestRoot
- exit.go
- RuntimePathCheck
- load.go
- .emit
- agreementConditions
- runWith
- github.com/spf13/cobra.Command
- refuseUnrepresentableJSON
- coherence_test.go
- Probe
- runtime.go
- driver.go
- version.go
- main
- runStatus
- assertNoErrorEnvelope
- initReportOf
- storage/access_other.go

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 101 edges
2. `NewError()` - 86 edges
3. `New()` - 51 edges
4. `Build()` - 48 edges
5. `fixedClock()` - 45 edges
6. `run()` - 44 edges
7. `App` - 43 edges
8. `NewAdapter()` - 43 edges
9. `newDB()` - 43 edges
10. `Open()` - 43 edges

## Surprising Connections (you probably didn't know these)
- `First Implementation Milestones` --conceptually_related_to--> `Dependency Waves 1-8`  [INFERRED]
  .docs/mindrail-tech-stack.md → .tasks/mindrail-0.1-task-list.md
- `Not Yet Active Status Note` --references--> `Mindrail 0.1 Kernel Scope`  [INFERRED]
  AGENTS.md → .docs/mindrail-0.1-kernel-scope.md
- `MR-001 Local repository bootstrap and diagnostics path` --implements--> `SQLite Migrations`  [EXTRACTED]
  .tasks/mindrail-0.1-task-list.md → .docs/mindrail-tech-stack.md
- `Managed Section Markers` --implements--> `Repository Layout`  [INFERRED]
  AGENTS.md → .docs/mindrail-technical-specification-1.0.md
- `Out Of Scope` --conceptually_related_to--> `0.1 Deferred Delivery Backlog`  [INFERRED]
  .docs/mindrail-0.1-kernel-scope.md → .tasks/mindrail-0.1-task-list.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Milestone Alignment Between Documents** — _docs_mindrail_tech_stack_milestone_mapping_table, _docs_mindrail_tech_stack_first_implementation_milestones, _docs_mindrail_tech_stack_reconcile_in_fifth_milestone, _docs_mindrail_technical_specification_1_0_milestone_4, _docs_mindrail_technical_specification_1_0_milestone_5, _docs_mindrail_technical_specification_1_0_milestone_8 [EXTRACTED 1.00]
- **Reconcile-First Correctness Spine** — _docs_mindrail_technical_specification_1_0_reconcile_workflow, _docs_mindrail_technical_specification_1_0_session_correctness_path, _docs_mindrail_technical_specification_1_0_unregistered_change, _docs_mindrail_tech_stack_reconcile_implementation_rule, _docs_mindrail_tech_stack_protocol_skipped_scenario, _docs_mindrail_0_1_kernel_scope_ac_05 [EXTRACTED 1.00]
- **Thirteen-Tool Agent Protocol Surface** — _docs_mindrail_technical_specification_1_0_mcp_tool_surface, _docs_mindrail_technical_specification_1_0_agent_protocol_instructions, _docs_mindrail_technical_specification_1_0_managed_section_tool_coverage, agents_mindrail_protocol_managed_section, agents_protocol_call_sequence, _docs_mindrail_0_1_kernel_scope_mcp_parameter_surface [EXTRACTED 1.00]

## Communities (139 total, 6 thin omitted)

### Community 0 - "MCP Tool Surface"
Cohesion: 0.20
Nodes (33): AC-14, AC-21, MCP Parameter Surface, Core E2E Workflow, MCP Contract Tests, MCP Go SDK, MCP Layering, Protocol Compliant Scenario (+25 more)

### Community 1 - "Acceptance Criteria"
Cohesion: 0.16
Nodes (24): AC-10, AC-12, AC-19, AC-20, Acceptance Criteria, CI In Scope Rationale, CI Integration, Git Hooks (+16 more)

### Community 2 - "Impact Engine"
Cohesion: 0.09
Nodes (36): AC-22, Non Goals, Structural Precision Limits, Coverage Provider Interface, Explainability Storage, Impact Graph Storage, AC-04, AC-05 (+28 more)

### Community 3 - "MR-004 Safe leases, idempotency and optimistic revision"
Cohesion: 0.16
Nodes (20): AC-04, AC-15, AC-16, Fixture Repositories, Release Quality Gate, SQLite Concurrency Tests, Testing Stack, AC-09 (+12 more)

### Community 4 - "Mindrail Technology Stack and Implementation Guide"
Cohesion: 0.09
Nodes (32): Mindrail 0.1 Kernel Scope, Application Wiring, CLI JSON Output, Cobra CLI, Dependency Matrix, Mindrail Technology Stack and Implementation Guide, Explicit Non Choices, Go Primary Language (+24 more)

### Community 5 - "Evidence Types"
Cohesion: 0.13
Nodes (27): AC-08, AC-17, Approval Provider Architecture, Filesystem Safety, Process Tree Cleanup, Secret Redaction Pipeline, Security Boundaries, TOML Config (+19 more)

### Community 6 - "MR-002 Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.20
Nodes (18): AC-02, Schema Policy v1, Compatibility Surfaces, JSON Schema Validation, Knowledge Schema Compatibility, AC-16, AC-17, AC-18 (+10 more)

### Community 7 - "Resolver Manager"
Cohesion: 0.11
Nodes (23): Out Of Scope, Managed Resolver Cache, Pyright Resolver, Resolver Interface, Semantic Resolver Out Of Process, TypeScript Resolver, AC-01, AC-02 (+15 more)

### Community 8 - "MR-005 Python/TypeScript/JavaScript structural indexing"
Cohesion: 0.09
Nodes (33): AC-01, AC-06, Cold Index Worker Model, Concurrency Model, Content Addressed Parse Cache, Doctor Checks, Error Model, Fsnotify Watcher (+25 more)

### Community 9 - "DB"
Cohesion: 0.23
Nodes (15): busyBudget(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure(), DB, Options, notWALFailure() (+7 more)

### Community 10 - "Interactive Performance Contract"
Cohesion: 0.18
Nodes (17): AC-13, Performance Targets, ADR List, Interactive Latency Engineering, Resolver Pool Eviction, Tree Sitter CGO Build, AC-25, AC-26 (+9 more)

### Community 11 - "ResolveRuntimePaths"
Cohesion: 0.17
Nodes (25): gitLayout, ResolveRuntimePaths(), assertMode(), newGitLayout(), TestEnsureDirsAndWriteFileUseRestrictiveModes(), TestEnsureDirsReportsUnwritableRuntimePath(), TestProbeWritableRejectsAFileInThePathOfTheRuntimeRoot(), TestProbeWritableReportsAnUnwritableRuntimePathWithoutCreatingIt() (+17 more)

### Community 12 - "ADR-0002 — SQLite driver"
Cohesion: 0.33
Nodes (5): ADR-0002 — SQLite driver, Consequences, Context, Decision, Open — benchmark matrix required before this ADR is accepted

### Community 13 - "ADR-0001 — Primary language and CLI stack"
Cohesion: 0.40
Nodes (4): ADR-0001 — Primary language and CLI stack, Consequences, Context, Decision

### Community 15 - "io.Writer"
Cohesion: 0.18
Nodes (15): Envelope, io.Writer, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice() (+7 more)

### Community 16 - "NewLoader"
Cohesion: 0.05
Nodes (66): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, OutputConfig, ProjectConfig, Provenance (+58 more)

### Community 17 - "New"
Cohesion: 0.09
Nodes (76): testing/fstest.MapFS, FormatTime(), assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), Load(), keys(), TestInitialMigrationCreatesOnlyMR001Tables() (+68 more)

### Community 18 - "5. Test plan"
Cohesion: 0.04
Nodes (47): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+39 more)

### Community 19 - "contract_test.go"
Cohesion: 0.14
Nodes (34): result, addWorktree(), assertFourErrorKeys(), assertGolden(), assertShape(), findCheck(), isEmptyValue(), join() (+26 more)

### Community 20 - "newStore"
Cohesion: 0.10
Nodes (36): FixedClock, SystemClock, database/sql.DB, database/sql.Tx, sync.Mutex, time.Time, Clock, ParseTime() (+28 more)

### Community 21 - "New"
Cohesion: 0.09
Nodes (44): Recorder, refusingTransport, stepRecorder, txProbe, net/http.Request, net/http.Response, sync/atomic.Bool, sync/atomic.Int64 (+36 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.17
Nodes (22): InitReport, assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport(), renderHuman() (+14 more)

### Community 23 - "App"
Cohesion: 0.07
Nodes (19): InitResult, io/fs.FS, log/slog.Logger, sync.Once, asKnowledgeError(), asMigrationError(), asRepositoryError(), asRuntimePathError() (+11 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (30): Check, Runner, ConfigCheck(), configResult(), describePath(), describeProblems(), describeRuntimeLocation(), describeStartDir() (+22 more)

### Community 25 - "healthySubject"
Cohesion: 0.15
Nodes (38): NewRunner(), TestCheckFuncWithoutFunctionIsNotSilent(), TestRunnerRespectsContextCancellation(), DefaultChecks(), KnowledgeCheck(), assertActionable(), assertDiagnosable(), Result (+30 more)

### Community 26 - "PayloadOf"
Cohesion: 0.06
Nodes (95): ExecRunner, FakeResponse, Invocation, os/exec.Cmd, time.Duration, ErrorPayload, PayloadOf(), ExitCode() (+87 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "Migrator"
Cohesion: 0.16
Nodes (16): Migration, bookkeepingCreateFailure(), bookkeepingOccupiedFailure(), checksumFailure(), Applied, Migrator, highestVersion(), namesAbsentFrom() (+8 more)

### Community 29 - "Verdict"
Cohesion: 0.29
Nodes (8): CheckFunc, errorFrom(), Report, Result, Subject, haltError(), kindForCode(), Verdict()

### Community 30 - "Reconcile Workflow"
Cohesion: 0.13
Nodes (26): AC-03, AC-05, In Scope, Agent Workspace Modes, First Implementation Milestones, Milestone Mapping Table, Reconcile Implementation Rule, Reconcile In Fifth Milestone (+18 more)

### Community 31 - "storage/arch_test.go"
Cohesion: 0.33
Nodes (10): go/ast.ImportSpec, fileImports(), importPath(), moduleRoot(), packageImports(), shouldSkipDir(), TestDriverImportConfinedToStorage(), TestDriverImportUnreachableFromOtherPackages() (+2 more)

### Community 32 - "loader/loader_test.go"
Cohesion: 0.13
Nodes (30): decisionJSON(), importsPackage(), invariantJSON(), makeKnowledgeDirs(), mustMkdirAll(), newLoader(), TestLoadAbsentKnowledgeTreeIsHealthyZero(), TestLoadCountsDecisionsAndInvariants() (+22 more)

### Community 33 - "healthySubject"
Cohesion: 0.17
Nodes (24): Subject, TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), TestRepositoryBlockIsObservedOnAHealthyRepository(), TestRepositoryBlockSurvivesALaterStepFailing(), assertActionable() (+16 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "Code"
Cohesion: 0.33
Nodes (10): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+2 more)

### Community 37 - "status/render.go"
Cohesion: 0.17
Nodes (17): strings.Builder, Report, Result, paint(), writeBlock(), writeResult(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), Report (+9 more)

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
Cohesion: 0.11
Nodes (35): assertSQLiteCheckHealthy(), shellQuote(), TestGracefulShutdownOnSIGINT(), writeSlowGit(), addWorktree(), buildBinary(), decodeEnvelope(), entriesIn() (+27 more)

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

### Community 48 - "context.Context"
Cohesion: 0.36
Nodes (7): Adapter, context.Context, parseGitBool(), propagate(), stderrSummary(), wrapUnexpected(), TestBooleanAnswersStillTolerateWhitespace()

### Community 49 - "Completion Gate"
Cohesion: 0.12
Nodes (28): AC-07, AC-09, AC-11, AC-18, Exit Condition, Goal Hypothesis, Primary E2E Scenario, Deterministic Core (+20 more)

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

### Community 58 - "explain"
Cohesion: 0.20
Nodes (10): diagnosis, halt, Probes, step, explain(), Subject, Result, offersInit() (+2 more)

### Community 59 - "Open"
Cohesion: 0.14
Nodes (27): Open(), openTemp(), TestCloseIsIdempotent(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection(), TestOpenCreatesWALSidecar() (+19 more)

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

### Community 69 - "check_test.go"
Cohesion: 0.31
Nodes (10): okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestHaltErrorPointsAtTheCheckThatExplainsTheHalt(), TestReportErrExitCodeFollowsTheFailureClass(), TestReportErrOnlyForErrorState(), TestRunnerConstructorDoesNotAliasItsArgument() (+2 more)

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

### Community 75 - "ProbeWriteAccess"
Cohesion: 0.06
Nodes (66): io/fs.FileInfo, io/fs.FileMode, fallbackWriteRemedy(), afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero() (+58 more)

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
Cohesion: 0.12
Nodes (27): AdoptCause(), CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain() (+19 more)

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

### Community 90 - "TestBrokenSetupMatrix"
Cohesion: 0.16
Nodes (31): strictEnvelope, encoding/json.RawMessage, coherenceSetups(), TestEveryUninitialisedSpellingExitsZero(), configPath(), corruptDatabase(), createUnmigratedDatabase(), denyAccess() (+23 more)

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

### Community 107 - "Readiness"
Cohesion: 0.23
Nodes (9): Components(), ComponentName, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown(), TestTerminalStateSpellingIsTheSpecLiteral(), paintReadiness() (+1 more)

### Community 108 - "gitfile.go"
Cohesion: 0.21
Nodes (16): gitFileFault, joinStderr(), brokenGitFileFault(), danglingGitFileError(), decorate(), findDanglingGitFile(), gitReportedBrokenIndirection(), inspectGitEntry() (+8 more)

### Community 109 - "open_contention_test.go"
Cohesion: 0.29
Nodes (7): testing.M, TestMain(), runContender(), TestMain(), TestOpenDoesNotWaitForAPermanentFailure(), TestOpenStopsWaitingWhenTheContextIsCancelled(), TestOpenWaitsOutAConcurrentWALConversion()

### Community 110 - "newInitializedRepo"
Cohesion: 0.22
Nodes (20): TestHealthyRepositoryIsUntouchedByRemedyCoherence(), chmodForTest(), decodeData(), newInitializedRepo(), TestInitNeverClaimsASchemaItDidNotEstablish(), TestUnusableCacheDirectoryLeavesTheRepositoryUsable(), assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain() (+12 more)

### Community 111 - "WriteFailure"
Cohesion: 0.27
Nodes (16): IsBusy(), IsDiskFull(), IsReadOnly(), assertRetryablePayload(), contendedWriteError(), openForTest(), readOnlyWriteError(), TestWriteFailureNamesNothingItCannotIdentify() (+8 more)

### Community 112 - "Build"
Cohesion: 0.21
Nodes (16): Build(), classify(), componentFrom(), currentSchemaVersion(), Report, observationOf(), repositoryObservation(), stateRank() (+8 more)

### Community 113 - "DomainError"
Cohesion: 0.26
Nodes (8): DomainError, busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), mainDatabaseFile(), readOnlyWriteFailure(), Querier

### Community 114 - "loader/loader.go"
Cohesion: 0.20
Nodes (12): io/fs.DirEntry, Loader, Problem, Store, isRecordFile(), readFailureReason(), recordPath(), stringField() (+4 more)

### Community 115 - "InTx"
Cohesion: 0.36
Nodes (6): InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow(), txKeyType

### Community 116 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 117 - "RootKind"
Cohesion: 0.23
Nodes (13): RootKind, os.FileInfo, unwritableError(), dangling(), root, RuntimePaths, Writability, nearestExisting() (+5 more)

### Community 118 - "newTestRoot"
Cohesion: 0.18
Nodes (17): TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRepositoryRootIsItsOwnKind(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize() (+9 more)

### Community 119 - "exit.go"
Cohesion: 0.21
Nodes (11): Error, Denied(), exitCodeForKind(), Failed(), Kind, TestErrorMessageUsesCause(), TestErrorUnwrapsToCause(), TestExitCode() (+3 more)

### Community 120 - "RuntimePathCheck"
Cohesion: 0.29
Nodes (13): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+5 more)

### Community 121 - "load.go"
Cohesion: 0.25
Nodes (11): alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns(), createdObjects(), SchemaObject (+3 more)

### Community 122 - ".emit"
Cohesion: 0.32
Nodes (8): globalFlags, humanRenderer, globalFlagsOf(), invocation, Options, newInvocation(), resolveStartDir(), startDirError()

### Community 123 - "agreementConditions"
Cohesion: 0.57
Nodes (6): answer, condition, agreementConditions(), answerOf(), assertSameAnswer(), TestOneConditionIsNamedTheSameWayByEveryCommand()

### Community 124 - "runWith"
Cohesion: 0.32
Nodes (5): brokenSetup, envelopeScenario, runWith(), TestHumanOutputIsNeverEmpty(), Options

### Community 125 - "github.com/spf13/cobra.Command"
Cohesion: 0.32
Nodes (10): github.com/spf13/cobra.Command, Options, newDoctorCommand(), runDoctor(), Options, newInitCommand(), runInit(), Execute() (+2 more)

### Community 126 - "refuseUnrepresentableJSON"
Cohesion: 0.31
Nodes (8): reflect.Value, firstUnrepresentable(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo(), TestRepresentableDocumentsAreNotRefused(), unrepresentableError(), walkForInvalidUTF8()

### Community 127 - "coherence_test.go"
Cohesion: 0.31
Nodes (9): coherenceSetup, remedy, assertDocumentIsCoherent(), offersInitRemedy(), remediesIn(), stringsOf(), TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestAPopulatedNonWALDatabaseIsNotCalledUninitialised() (+1 more)

### Community 128 - "Probe"
Cohesion: 0.13
Nodes (19): App, SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload() (+11 more)

### Community 129 - "runtime.go"
Cohesion: 0.24
Nodes (6): PathOptions, ensureDir(), RuntimePaths, hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 130 - "driver.go"
Cohesion: 0.33
Nodes (9): diskFullCode(), driverResultCode(), dsn(), Options, hasPrimaryCode(), isBusyError(), isCorruptError(), isDiskFullError() (+1 more)

### Community 131 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 132 - "main"
Cohesion: 0.40
Nodes (3): main(), context.CancelFunc, RootContext()

### Community 133 - "runStatus"
Cohesion: 0.70
Nodes (4): Options, newStatusCommand(), readinessOf(), runStatus()

### Community 134 - "assertNoErrorEnvelope"
Cohesion: 0.53
Nodes (5): assertNoErrorEnvelope(), newRepoWithUnrepresentablePath(), TestARepresentableRepositoryIsUnaffectedByThePreflight(), TestInitRefusesAnUnrepresentableRepositoryBeforeWriting(), TestTheUnrepresentablePathRemediesBothWork()

### Community 135 - "initReportOf"
Cohesion: 0.31
Nodes (7): blockingSummary(), initReportOf(), schemaIsCurrent(), blockedReason(), Report, InitReport, TerminalState

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

## Knowledge Gaps
- **425 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `invocation`, `Report`, `Report` (+420 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 539 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `Probe`, `DB`, `io.Writer`, `NewLoader`, `newStore`, `New`, `App`, `healthySubject`, `PayloadOf`, `Migrator`, `Verdict`, `healthySubject`, `Code`, `context.Context`, `ProbeWriteAccess`, `TestBrokenSetupMatrix`, `gitfile.go`, `DomainError`, `loader/loader.go`, `Root`, `RootKind`, `exit.go`, `RuntimePathCheck`, `.emit`, `refuseUnrepresentableJSON`?**
  _High betweenness centrality (0.055) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `initReportOf`, `DB`, `ResolveRuntimePaths`, `io.Writer`, `NewLoader`, `New`, `contract_test.go`, `newStore`, `New`, `App`, `healthySubject`, `Migrator`, `loader/loader_test.go`, `Code`, `explain`, `Open`, `check_test.go`, `ProbeWriteAccess`, `NewError`, `TestBrokenSetupMatrix`, `WriteFailure`, `newTestRoot`, `runWith`, `refuseUnrepresentableJSON`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Why does `App` connect `App` to `runtime.go`, `healthySubject`, `initReportOf`, `DB`, `io.Writer`, `NewLoader`, `newStore`, `New`, `checks.go`?**
  _High betweenness centrality (0.010) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `invocation` to the rest of the system?**
  _425 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Impact Engine` be split into smaller, more focused modules?**
  _Cohesion score 0.08888888888888889 - nodes in this community are weakly interconnected._