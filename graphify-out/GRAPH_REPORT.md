# Graph Report - Mindrail  (2026-09-06)

## Corpus Check
- 198 files · ~281,269 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 2507 nodes · 6850 edges · 160 communities (146 shown, 7 thin omitted)
- Extraction: 86% EXTRACTED · 14% INFERRED · 0% AMBIGUOUS · INFERRED: 933 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `3a1300a0`
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
- DB
- Interactive Performance Contract
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- io.Writer
- config/loader.go
- New
- 5. Test plan
- contract_test.go
- newStore
- New
- status/render_test.go
- App
- checks.go
- DefaultChecks
- PayloadOf
- What You Must Do When Invoked
- Migrator
- Check
- runner.go
- testing.T
- loader/loader.go
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
- smoke_test.go
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- adapter.go
- MR-006 Durable symbol_uid allocation and rename/move protection
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- Subject
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
- loader/loader_test.go
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
- ProbeWriteAccess
- NewError
- status
- status
- graphify reference: add a URL and watch a folder
- graphify reference: commit hook and native CLAUDE.md integration
- graphify reference: incremental update and cluster-only
- agreement_test.go
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
- Open
- run
- assertRetryablePayload
- Build
- WriteFailure
- app/arch_test.go
- InTx
- Root
- RootKind
- newTestRoot
- hostilefs_linux_test.go
- healthySubject
- Mindrail
- .emit
- Config
- runWith
- github.com/spf13/cobra.Command
- refuseUnrepresentableJSON
- coherence_test.go
- Probe
- runtime.go
- driver.go
- version.go
- main
- .openSQLite
- WriteIfAbsent
- initReportOf
- storage/access_other.go
- agreement_hostilefs_linux_test.go
- fulldisk_linux_test.go
- writable.go
- context.Context
- schema_ledger_test.go
- NoSpaceRefusal
- load.go
- NewLoader
- scaffold_containment_test.go
- checks_test.go
- ClassifyRefusal
- storage/arch_test.go
- requireGit
- Step
- scaffold_hostilefs_linux_test.go
- readonlyfs_linux_test.go
- Options
- FakeRunner
- runRacer
- .RoundTrip
- TxActive

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 118 edges
2. `NewError()` - 91 edges
3. `New()` - 51 edges
4. `NewAdapter()` - 50 edges
5. `Build()` - 48 edges
6. `ExitCode()` - 47 edges
7. `Open()` - 46 edges
8. `fixedClock()` - 45 edges
9. `App` - 43 edges
10. `run()` - 43 edges

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

## Communities (160 total, 7 thin omitted)

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

### Community 9 - "DB"
Cohesion: 0.14
Nodes (23): database/sql.DB, time.Duration, timeoutError(), busyBudget(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure() (+15 more)

### Community 10 - "Interactive Performance Contract"
Cohesion: 0.20
Nodes (16): AC-13, Performance Targets, Concurrency Model, Interactive Latency Engineering, Priority Scheduler, Resolver Pool Eviction, AC-25, AC-26 (+8 more)

### Community 11 - "ResolveRuntimePaths"
Cohesion: 0.17
Nodes (26): gitLayout, PathOptions, ResolveRuntimePaths(), assertMode(), newGitLayout(), TestEnsureDirsAndWriteFileUseRestrictiveModes(), TestEnsureDirsReportsUnwritableRuntimePath(), TestProbeWritableRejectsAFileInThePathOfTheRuntimeRoot() (+18 more)

### Community 12 - "ADR-0002 — SQLite driver"
Cohesion: 0.33
Nodes (5): ADR-0002 — SQLite driver, Consequences, Context, Decision, Open — benchmark matrix required before this ADR is accepted

### Community 13 - "ADR-0001 — Primary language and CLI stack"
Cohesion: 0.40
Nodes (4): ADR-0001 — Primary language and CLI stack, Consequences, Context, Decision

### Community 15 - "io.Writer"
Cohesion: 0.15
Nodes (18): Envelope, io.Writer, os.File, detailLines(), RenderError(), renderSection(), ColorEnabled(), Warning (+10 more)

### Community 16 - "config/loader.go"
Cohesion: 0.20
Nodes (13): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+5 more)

### Community 17 - "New"
Cohesion: 0.14
Nodes (49): testing/fstest.MapFS, FormatTime(), Load(), New(), embeddedSet(), fixedClock(), newDB(), tableExists() (+41 more)

### Community 18 - "5. Test plan"
Cohesion: 0.04
Nodes (47): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+39 more)

### Community 19 - "contract_test.go"
Cohesion: 0.15
Nodes (25): result, addWorktree(), assertFourErrorKeys(), assertGolden(), assertShape(), isEmptyValue(), join(), jsonShape() (+17 more)

### Community 20 - "newStore"
Cohesion: 0.11
Nodes (33): FixedClock, SystemClock, database/sql.Tx, sync.Mutex, time.Time, Clock, encodeCrockford(), NewID() (+25 more)

### Community 21 - "New"
Cohesion: 0.20
Nodes (29): New(), assertNoRuntimeState(), corruptDatabase(), exists(), initialize(), newGitRepo(), options(), productDirUnder() (+21 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.17
Nodes (22): InitReport, assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport(), renderHuman() (+14 more)

### Community 23 - "App"
Cohesion: 0.10
Nodes (13): InitResult, io/fs.FS, sync.Once, asKnowledgeError(), asMigrationError(), asRepositoryError(), asRuntimePathError(), asWorkspaceError() (+5 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (32): ConfigCheck(), configResult(), describePath(), describeProblems(), describeRuntimeLocation(), describeStartDir(), describeUninitializedDB(), explain() (+24 more)

### Community 25 - "DefaultChecks"
Cohesion: 0.16
Nodes (27): App, Subject, NewRunner(), TestRunnerRespectsContextCancellation(), Verdict(), DefaultChecks(), haltedSubject(), TestBrokenConfigDoesNotUnresolveTheRepository() (+19 more)

### Community 26 - "PayloadOf"
Cohesion: 0.06
Nodes (107): Error, ErrorPayload, PayloadOf(), Denied(), ExitCode(), exitCodeForKind(), Failed(), Kind (+99 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "Migrator"
Cohesion: 0.15
Nodes (17): ParseTime(), Migration, bookkeepingCreateFailure(), bookkeepingOccupiedFailure(), checksumFailure(), Applied, Migrator, highestVersion() (+9 more)

### Community 29 - "Check"
Cohesion: 0.13
Nodes (20): Check, CheckFunc, findCheck(), errorFrom(), Report, Result, Runner, haltError() (+12 more)

### Community 30 - "runner.go"
Cohesion: 0.27
Nodes (8): ExecRunner, os/exec.Cmd, hasAnyPrefix(), SanitizedEnv(), unavailableError(), underlyingError(), workingDirectoryError(), workingDirectoryFault()

### Community 31 - "testing.T"
Cohesion: 0.09
Nodes (39): testing.T, TestFixedClockSatisfiesClock(), TestFormatTimeIsUTCRFC3339(), TestParseTimeRejectsMalformedInput(), TestShutdownTimeoutMatchesBusyTimeout(), TestSystemClockIsUTC(), assertGolden(), Report (+31 more)

### Community 32 - "loader/loader.go"
Cohesion: 0.13
Nodes (12): encoding/json.RawMessage, io/fs.DirEntry, Loader, isRecordFile(), readFailureReason(), recordPath(), stringField(), unreadableRecord() (+4 more)

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
Cohesion: 0.38
Nodes (9): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+1 more)

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

### Community 43 - "smoke_test.go"
Cohesion: 0.23
Nodes (17): assertSQLiteCheckHealthy(), shellQuote(), TestGracefulShutdownOnSIGINT(), writeSlowGit(), addWorktree(), buildBinary(), decodeEnvelope(), entriesIn() (+9 more)

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

### Community 48 - "adapter.go"
Cohesion: 0.18
Nodes (22): standpoint, worktreeLinkage, bareRepositoryError(), detachedWorktreeError(), firstLine(), foreignRelinkCaveat(), foreignWorktreeError(), gitReportedNoRepository() (+14 more)

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

### Community 58 - "Subject"
Cohesion: 0.18
Nodes (9): diagnosis, halt, step, codeOf(), fallbackWriteRemedy(), Subject, Result, probeRepositoryDir() (+1 more)

### Community 59 - "openTemp"
Cohesion: 0.24
Nodes (16): openTemp(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection(), TestOpenCreatesWALSidecar(), TestOpenReadOnlyAfterWriterClosed(), TestOpenReadOnlyRejectsWrites() (+8 more)

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

### Community 69 - "loader/loader_test.go"
Cohesion: 0.30
Nodes (21): decisionJSON(), importsPackage(), invariantJSON(), makeKnowledgeDirs(), mustMkdirAll(), newLoader(), TestLoadAbsentKnowledgeTreeIsHealthyZero(), TestLoadCountsDecisionsAndInvariants() (+13 more)

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
Cohesion: 0.16
Nodes (21): Probes, io/fs.FileInfo, io/fs.FileMode, absentFailure(), classify(), describeMode(), Presence, nonDirectoryAncestor() (+13 more)

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

### Community 83 - "ProbeWriteAccess"
Cohesion: 0.29
Nodes (21): ProbeWriteAccess(), assertProbeMatchesSQLite(), assertUnwritablePayload(), chmodForTest(), dirEntryNames(), initialisedDatabase(), requireModeBitsAreEnforced(), TestProbeWriteAccessAgreesWithSQLite() (+13 more)

### Community 84 - "NewError"
Cohesion: 0.24
Nodes (11): AdoptCause(), CauseOf(), NewError(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain(), TestDomainErrorCarriesFiveFields(), TestDomainErrorWrappingSurvivesErrorsIsAndAs(), TestErrorPayloadJSONHasAllFourKeys() (+3 more)

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

### Community 90 - "agreement_test.go"
Cohesion: 0.14
Nodes (46): answer, condition, agreeingCommands(), agreementConditions(), answerFrom(), answerOf(), assertExemptionsAreStillEarned(), assertSameAnswer() (+38 more)

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
Cohesion: 0.23
Nodes (15): gitFileFault, joinStderr(), brokenGitFileFault(), danglingGitFileError(), decorate(), findDanglingGitFile(), gitReportedBrokenIndirection(), inspectGitEntry() (+7 more)

### Community 109 - "Open"
Cohesion: 0.15
Nodes (15): testing.M, Open(), TestCloseIsIdempotent(), TestOpenReadOnlyDoesNotCreateFile(), TestOpenRejectsCorruptDatabase(), TestOpenRejectsRelativePath(), runContender(), TestMain() (+7 more)

### Community 110 - "run"
Cohesion: 0.17
Nodes (30): TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(), TestEveryUninitialisedSpellingExitsZero(), TestHealthyRepositoryIsUntouchedByRemedyCoherence(), assertNoErrorEnvelope(), decodeData(), newRepo(), run() (+22 more)

### Community 111 - "assertRetryablePayload"
Cohesion: 0.33
Nodes (10): assertRetryablePayload(), contendedWriteError(), openForTest(), readOnlyWriteError(), TestWriteFailureRatesAnUnwritableDatabaseUnavailable(), TestWriteFailureRatesContentionUnavailable(), fullDatabaseWriteError(), TestIsDiskFullIgnoresTheConditionsBesideIt() (+2 more)

### Community 112 - "Build"
Cohesion: 0.21
Nodes (16): Build(), classify(), componentFrom(), currentSchemaVersion(), Report, observationOf(), repositoryObservation(), stateRank() (+8 more)

### Community 113 - "WriteFailure"
Cohesion: 0.23
Nodes (15): DomainError, busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), IsBusy(), IsDiskFull(), IsReadOnly() (+7 more)

### Community 114 - "app/arch_test.go"
Cohesion: 0.67
Nodes (5): importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib()

### Community 115 - "InTx"
Cohesion: 0.53
Nodes (5): InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow()

### Community 116 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 117 - "RootKind"
Cohesion: 0.19
Nodes (17): os.FileInfo, knowledgeDirRefusal(), TestRepositoryRootIsItsOwnKind(), ensureDir(), unwritableError(), dangling(), root, RootKind (+9 more)

### Community 118 - "newTestRoot"
Cohesion: 0.20
Nodes (15): TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize(), newTestRoot() (+7 more)

### Community 119 - "hostilefs_linux_test.go"
Cohesion: 0.18
Nodes (15): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+7 more)

### Community 120 - "healthySubject"
Cohesion: 0.25
Nodes (17): refusalCause, RuntimePathCheck(), Subject, healthySubject(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding() (+9 more)

### Community 121 - "Mindrail"
Cohesion: 0.33
Nodes (6): Build, Documentation, License, Mindrail, Repository-owned knowledge, Requirements

### Community 122 - ".emit"
Cohesion: 0.26
Nodes (10): globalFlags, humanRenderer, log/slog.Logger, globalFlagsOf(), invocation, Options, newInvocation(), resolveStartDir() (+2 more)

### Community 123 - "Config"
Cohesion: 0.19
Nodes (16): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+8 more)

### Community 124 - "runWith"
Cohesion: 0.20
Nodes (10): brokenSetup, envelopeScenario, strictEnvelope, runWith(), assertEnvelopeCoherent(), decodeStrictEnvelope(), lineWithPrefix(), sectionOf() (+2 more)

### Community 125 - "github.com/spf13/cobra.Command"
Cohesion: 0.26
Nodes (12): github.com/spf13/cobra.Command, Options, newDoctorCommand(), runDoctor(), Execute(), Options, NewRootCommand(), NewRootCommandWith() (+4 more)

### Community 126 - "refuseUnrepresentableJSON"
Cohesion: 0.24
Nodes (9): reflect.Value, invocation, firstUnrepresentable(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo(), TestRepresentableDocumentsAreNotRefused(), unrepresentableError() (+1 more)

### Community 127 - "coherence_test.go"
Cohesion: 0.30
Nodes (14): remedy, remedyClass, absolutePathsIn(), assertDocumentIsCoherent(), classOf(), contradictionsBetween(), contradictoryPathRemedies(), mindrailCommandsIn() (+6 more)

### Community 128 - "Probe"
Cohesion: 0.17
Nodes (17): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject, subjectAt() (+9 more)

### Community 129 - "runtime.go"
Cohesion: 0.67
Nodes (3): hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 130 - "driver.go"
Cohesion: 0.46
Nodes (7): diskFullCode(), driverResultCode(), hasPrimaryCode(), isBusyError(), isCorruptError(), isDiskFullError(), isReadOnlyError()

### Community 131 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 132 - "main"
Cohesion: 0.40
Nodes (3): main(), context.CancelFunc, RootContext()

### Community 133 - ".openSQLite"
Cohesion: 0.33
Nodes (3): integrityFailure(), IsUnwritten(), TestIsUnwrittenSeparatesAnUnfinishedFileFromADatabase()

### Community 134 - "WriteIfAbsent"
Cohesion: 0.22
Nodes (15): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission(), DefaultTemplate(), EnsureKnowledgeDirs(), scaffoldError() (+7 more)

### Community 135 - "initReportOf"
Cohesion: 0.26
Nodes (10): blockingSummary(), Options, initReportOf(), newInitCommand(), runInit(), schemaIsCurrent(), blockedReason(), Report (+2 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "agreement_hostilefs_linux_test.go"
Cohesion: 0.24
Nodes (16): hostileCondition, agreeingHostileCommands(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo(), hostileAvailableBytes(), hostileConditions(), hostileFillerPath() (+8 more)

### Community 140 - "fulldisk_linux_test.go"
Cohesion: 0.22
Nodes (16): afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero(), fullDiskChild(), fullDiskScenario(), initialiseOnDisk() (+8 more)

### Community 141 - "writable.go"
Cohesion: 0.21
Nodes (12): TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError(), conditionBlocker(), WriteAccess, WriteBlocker (+4 more)

### Community 142 - "context.Context"
Cohesion: 0.33
Nodes (7): Adapter, context.Context, propagate(), stderrSummary(), trimOutputTerminator(), wrapUnexpected(), TestRevParseStripsOnlyTheOutputTerminator()

### Community 143 - "schema_ledger_test.go"
Cohesion: 0.28
Nodes (15): openDB(), assertRemedyIsNotTheFailingCommand(), assertUserFacing(), assertUserFacingCode(), holdWriteLock(), mustFailUp(), openContendedDB(), TestAGenuineMigrationFailureIsStillAMigrationFailure() (+7 more)

### Community 144 - "NoSpaceRefusal"
Cohesion: 0.20
Nodes (9): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+1 more)

### Community 145 - "load.go"
Cohesion: 0.25
Nodes (11): alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns(), createdObjects(), SchemaObject (+3 more)

### Community 146 - "NewLoader"
Cohesion: 0.35
Nodes (11): NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers(), TestProvenanceReportsSourceLayer(), TestStrictDecodeRejectsUnknownKey(), TestUnknownFlagKeyIsRejected() (+3 more)

### Community 147 - "scaffold_containment_test.go"
Cohesion: 0.23
Nodes (11): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), requireEmptyTree(), requireEscape(), requireModeEnforcement(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository() (+3 more)

### Community 148 - "checks_test.go"
Cohesion: 0.26
Nodes (12): KnowledgeCheck(), assertActionable(), assertDiagnosable(), Result, TestAnUnusableKnowledgeDirectoryIsReportedAsAPathCondition(), TestDefaultChecksRegisterNoAbsentSubsystem(), TestGitCheckBranches(), TestKnowledgeCheckAbsentTreeIsOK() (+4 more)

### Community 149 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 150 - "storage/arch_test.go"
Cohesion: 0.33
Nodes (10): go/ast.ImportSpec, fileImports(), importPath(), moduleRoot(), packageImports(), shouldSkipDir(), TestDriverImportConfinedToStorage(), TestDriverImportUnreachableFromOtherPackages() (+2 more)

### Community 151 - "requireGit"
Cohesion: 0.33
Nodes (9): newNonRepositoryDir(), isolateEnvironment(), newBareRepo(), requireGit(), TestHumanErrorNamesTheProbedDirectory(), newRepoWithUnrepresentablePath(), TestInitRefusesAnUnrepresentableRepositoryBeforeWriting(), TestTheUnrepresentablePathRemediesBothWork() (+1 more)

### Community 152 - "Step"
Cohesion: 0.31
Nodes (5): Recorder, stepRecorder, App, Step, Steps()

### Community 153 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 154 - "readonlyfs_linux_test.go"
Cohesion: 0.44
Nodes (8): checkReadOnlyRemedy(), openOnAReadOnlyFilesystem(), probeOnAReadOnlyFilesystem(), readOnlyChild(), readOnlyScenario(), remountReadOnly(), remountReadWrite(), runReadOnlyChild()

### Community 155 - "Options"
Cohesion: 0.25
Nodes (6): txProbe, sync/atomic.Bool, sync/atomic.Int64, Mode, Options, CommandRunner

### Community 156 - "FakeRunner"
Cohesion: 0.47
Nodes (4): FakeResponse, Invocation, unreadableRepositoryGit(), FakeRunner

### Community 157 - "runRacer"
Cohesion: 0.60
Nodes (4): assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), TestMain()

### Community 158 - ".RoundTrip"
Cohesion: 0.50
Nodes (3): refusingTransport, net/http.Request, net/http.Response

## Knowledge Gaps
- **425 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `invocation`, `Report`, `Report` (+420 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 542 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **7 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `Probe`, `.openSQLite`, `DB`, `writable.go`, `context.Context`, `io.Writer`, `config/loader.go`, `checks_test.go`, `New`, `newStore`, `App`, `DefaultChecks`, `PayloadOf`, `Migrator`, `Check`, `runner.go`, `loader/loader.go`, `healthySubject`, `Code`, `adapter.go`, `Probe`, `agreement_test.go`, `gitfile.go`, `WriteFailure`, `Root`, `RootKind`, `healthySubject`, `.emit`, `Config`, `refuseUnrepresentableJSON`?**
  _High betweenness centrality (0.041) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `WriteIfAbsent`, `initReportOf`, `DB`, `ResolveRuntimePaths`, `fulldisk_linux_test.go`, `io.Writer`, `schema_ledger_test.go`, `New`, `NewLoader`, `contract_test.go`, `scaffold_containment_test.go`, `New`, `newStore`, `App`, `checks.go`, `scaffold_hostilefs_linux_test.go`, `DefaultChecks`, `readonlyfs_linux_test.go`, `Migrator`, `Check`, `Subject`, `loader/loader_test.go`, `Probe`, `ProbeWriteAccess`, `NewError`, `agreement_test.go`, `Open`, `assertRetryablePayload`, `RootKind`, `newTestRoot`, `hostilefs_linux_test.go`, `runWith`, `refuseUnrepresentableJSON`?**
  _High betweenness centrality (0.031) - this node is a cross-community bridge._
- **Why does `NoSpaceRefusal()` connect `NoSpaceRefusal` to `RootKind`, `writable.go`?**
  _High betweenness centrality (0.018) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `invocation` to the rest of the system?**
  _425 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Acceptance Criteria` be split into smaller, more focused modules?**
  _Cohesion score 0.09306122448979592 - nodes in this community are weakly interconnected._