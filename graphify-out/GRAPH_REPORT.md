# Graph Report - Mindrail  (2026-09-06)

## Corpus Check
- 199 files · ~286,956 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 2778 nodes · 6596 edges · 198 communities (178 shown, 13 thin omitted)
- Extraction: 87% EXTRACTED · 13% INFERRED · 0% AMBIGUOUS · INFERRED: 875 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `b04b1bcb`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- MINDRAIL PROTOCOL Managed Section
- Mindrail 0.1 — Uygulama Görev Listesi
- mindrail-tech-stack.md
- mindrail-technical-specification-1.0.md
- 129. Acceptance criteria
- 19. Semantic resolver kurulum, lifecycle ve resource policy
- exit.go
- 5. Test plan
- 3. IN Scope
- DB
- 2. Public API per package
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- io.Writer
- config/loader.go
- New
- MR-001 — Implementation design
- contract_test.go
- time.Time
- New
- status/render_test.go
- context.Context
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- Migrator
- Check
- load.go
- testing.T
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
- smoke_test.go
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- adapter.go
- assertRetryablePayload
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- Subject
- IsUnwritten
- inventory.py
- Reconciliation — Refutation, Not Agreement
- Planning (Phases 0–7)
- Autonomous Engineering Orchestrator
- Analysis (Phases 0–7)
- Test Suite Audit & Redundancy Elimination
- State
- Reader Protocol — Conformance
- graphify reference: extra exports and benchmark
- newStore
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
- Component
- 128. Hedef implementation workstream'leri
- Open
- writability_test.go
- TestBrokenSetupMatrix
- report.go
- WriteFailure
- goldenReport
- 47. Human approval kimlik ve yetkilendirme modeli
- Root
- RootKind
- newTestRoot
- hostilefs_linux_test.go
- RuntimePathCheck
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
- InitReport
- WriteIfAbsent
- runInit
- storage/access_other.go
- agreement_hostilefs_linux_test.go
- fulldisk_linux_test.go
- writable.go
- NewID
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- NoSpaceRefusal
- requireGit
- NewLoader
- TestRepoConfigDirObstructionDoesNotOverFire
- MEDIUM
- ClassifyRefusal
- mindrail-0.1-kernel-scope.md
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- scaffold_hostilefs_linux_test.go
- readonlyfs_linux_test.go
- 121. Failure modes
- FakeRunner
- LOW
- open_contention_test.go
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- InTx
- 2. Mindrail'in temel problemi
- [ ] MR-001 — Yerel repository bootstrap ve tanılama yolu
- Görevler
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision
- [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması
- [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi
- [ ] MR-008 — Scope drift ve unregistered change ambiguity
- [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi
- [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence
- [ ] MR-011 — Kaynak değişince evidence geçersizleştirme
- [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı
- [ ] MR-013 — Yerel completion evidence gate
- [ ] MR-014 — MCP bilgi ve bağlam araçları
- [ ] MR-015 — MCP koordinasyon ve değişiklik araçları
- [ ] MR-016 — MCP validation ve completion araçları
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- [ ] MR-018 — Hook bypass'a dayanıklı CI verification
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi
- [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri
- 103. Test weakening ve false-green guard
- 28. Interactive performance contract
- 159. First Implementation Milestone
- 33. Durable Symbol Identity Implementation
- 56. Resolver Process Supervision
- 12. Content-addressed index cache
- 36. Impact explosion ve drill-down modeli
- 3. Mindrail ne değildir?

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
- `main()` --calls--> `ExitCode()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/exit.go
- `main()` --calls--> `Execute()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/cli/root.go
- `main()` --calls--> `RootContext()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/context.go
- `TestParseTimeRejectsMalformedInput()` --calls--> `ParseTime()`  [INFERRED]
  internal/app/clock_test.go → internal/app/clock.go
- `TestDomainErrorCarriesFiveFields()` --calls--> `Code`  [INFERRED]
  internal/app/errors_test.go → internal/app/code.go

## Import Cycles
- None detected.

## Communities (198 total, 13 thin omitted)

### Community 0 - "MINDRAIL PROTOCOL Managed Section"
Cohesion: 0.33
Nodes (6): Graphify Protocol, Managed Section Markers, MINDRAIL PROTOCOL Managed Section, Not Yet Active Status Note, Protocol Call Sequence, Soft Enforcement Note

### Community 1 - "Mindrail 0.1 — Uygulama Görev Listesi"
Cohesion: 0.14
Nodes (10): 0.1 Dışında Tutulacak Backlog, 0.1 Kabul Kriteri İzlenebilirlik Matrisi, Bağımlılık Dalgaları, Kullanım, Mindrail 0.1 — Uygulama Görev Listesi, F01 — `internal/cli/init.go`, F02 — `internal/config/loader.go`, F03 — `internal/storage/driver.go` — **regression introduced during remediation** (+2 more)

### Community 2 - "mindrail-tech-stack.md"
Cohesion: 0.01
Nodes (167): 100. Dependency Update Policy, 101. Linting and Formatting, 102. Supply-Chain Checks, 103. Build, 104. Version Command, 105. Native Build Toolchain, 106. Release Matrix, 107. Release Artifacts (+159 more)

### Community 3 - "mindrail-technical-specification-1.0.md"
Cohesion: 0.02
Nodes (117): 100. before_change, 101. after_change, 102. Change classifications, 104. Public API, 105. Database/schema changes, 106. Security path, 107. Index/readiness health modeli, 108. Parse error (+109 more)

### Community 4 - "129. Acceptance criteria"
Cohesion: 0.06
Nodes (36): 129. Acceptance criteria, AC-01 Resolver diagnostics, AC-02 Resolver failure, AC-03 Managed fallback, AC-04 Coverage provider abstraction, AC-05 Future language capability, AC-06 Signed human approval, AC-07 Approval provenance (+28 more)

### Community 5 - "19. Semantic resolver kurulum, lifecycle ve resource policy"
Cohesion: 0.12
Nodes (17): 19. Semantic resolver kurulum, lifecycle ve resource policy, Capability vector, Config, Default budget türetme, Eviction ve thrash koruması, Generation, Managed resolver integrity, `mindrail doctor` resolver teşhisi (+9 more)

### Community 6 - "exit.go"
Cohesion: 0.19
Nodes (12): Error, TestDomainErrorCarriesFiveFields(), Denied(), exitCodeForKind(), Failed(), Kind, TestErrorMessageUsesCause(), TestErrorUnwrapsToCause() (+4 more)

### Community 7 - "5. Test plan"
Cohesion: 0.12
Nodes (16): 5.10 `internal/doctor` (Unit D) — 11, 5.11 `internal/status` (Unit D) — 8, 5.12 `internal/bootstrap` (Unit E) — 8, 5.13 `internal/cli` — CLI contract (Unit E) — 14, 5.14 Architecture + smoke (Unit E) — 6, 5.15 Acceptance-criteria mapping, 5.1 `internal/app` (Unit D) — 10, 5.2 `internal/filesystem` (Unit A) — 8 (+8 more)

### Community 8 - "3. IN Scope"
Cohesion: 0.12
Nodes (16): 3. IN Scope, Basic impact, Change discovery, Completion gate, Coordination, Core executable, Diagnostics, Engineering knowledge (+8 more)

### Community 9 - "DB"
Cohesion: 0.21
Nodes (16): database/sql.DB, busyBudget(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure(), DB, Options (+8 more)

### Community 10 - "2. Public API per package"
Cohesion: 0.14
Nodes (14): 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D, 2.2 `internal/filesystem` — Unit A, 2.3 `internal/git` — Unit A, 2.4 `internal/storage` — Unit B (+6 more)

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
Cohesion: 0.18
Nodes (15): Envelope, io.Writer, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice() (+7 more)

### Community 16 - "config/loader.go"
Cohesion: 0.20
Nodes (13): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+5 more)

### Community 17 - "New"
Cohesion: 0.09
Nodes (76): testing/fstest.MapFS, assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), TestMain(), Load(), keys(), TestInitialMigrationCreatesOnlyMR001Tables() (+68 more)

### Community 18 - "MR-001 — Implementation design"
Cohesion: 0.12
Nodes (17): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 3. Dependency direction, 4. Work units, 6. Explicit non-goals, 7. Open questions and decisions, Decisions (+9 more)

### Community 19 - "contract_test.go"
Cohesion: 0.13
Nodes (43): result, TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestEveryUninitialisedSpellingExitsZero(), TestHealthyRepositoryIsUntouchedByRemedyCoherence(), addWorktree(), assertFourErrorKeys(), assertGolden(), assertNoErrorEnvelope() (+35 more)

### Community 20 - "time.Time"
Cohesion: 0.20
Nodes (16): FixedClock, SystemClock, database/sql.Tx, time.Time, FormatTime(), ParseTime(), TestFormatTimeIsUTCRFC3339(), Registration (+8 more)

### Community 21 - "New"
Cohesion: 0.09
Nodes (45): Recorder, refusingTransport, stepRecorder, txProbe, net/http.Request, net/http.Response, sync/atomic.Bool, sync/atomic.Int64 (+37 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.21
Nodes (19): InitReport, assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport(), renderHuman() (+11 more)

### Community 23 - "context.Context"
Cohesion: 0.10
Nodes (14): InitResult, context.Context, io/fs.FS, sync.Once, asKnowledgeError(), asMigrationError(), asRepositoryError(), asRuntimePathError() (+6 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (32): ConfigCheck(), configResult(), describePath(), describeProblems(), describeRuntimeLocation(), describeStartDir(), describeUninitializedDB(), explain() (+24 more)

### Community 25 - "healthySubject"
Cohesion: 0.12
Nodes (42): App, Subject, NewRunner(), TestCheckFuncWithoutFunctionIsNotSilent(), TestRunnerRespectsContextCancellation(), Verdict(), DefaultChecks(), KnowledgeCheck() (+34 more)

### Community 26 - "PayloadOf"
Cohesion: 0.09
Nodes (84): ErrorPayload, PayloadOf(), ExitCode(), TestProbeWritableLeavesAHealthyRepositoryAlone(), TestUnwritablePathPrescribesTheRemedyThatClearsEachCondition(), TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed(), NewAdapter(), assertPayloadIsActionable() (+76 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "Migrator"
Cohesion: 0.17
Nodes (15): Migration, bookkeepingOccupiedFailure(), checksumFailure(), Applied, Migrator, highestVersion(), namesAbsentFrom(), namesPresentIn() (+7 more)

### Community 29 - "Check"
Cohesion: 0.28
Nodes (11): Check, Runner, okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestReportErrExitCodeFollowsTheFailureClass(), TestReportErrOnlyForErrorState() (+3 more)

### Community 30 - "load.go"
Cohesion: 0.25
Nodes (11): alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns(), createdObjects(), SchemaObject (+3 more)

### Community 31 - "testing.T"
Cohesion: 0.11
Nodes (30): go/ast.ImportSpec, testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestFixedClockSatisfiesClock() (+22 more)

### Community 32 - "loader/loader_test.go"
Cohesion: 0.13
Nodes (30): decisionJSON(), importsPackage(), invariantJSON(), makeKnowledgeDirs(), mustMkdirAll(), newLoader(), TestLoadAbsentKnowledgeTreeIsHealthyZero(), TestLoadCountsDecisionsAndInvariants() (+22 more)

### Community 33 - "Build"
Cohesion: 0.17
Nodes (29): Subject, TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), TestRepositoryBlockIsObservedOnAHealthyRepository(), TestRepositoryBlockSurvivesALaterStepFailing() (+21 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "Code"
Cohesion: 0.21
Nodes (13): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+5 more)

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
Cohesion: 0.05
Nodes (67): Adapter, ExecRunner, gitFileFault, standpoint, worktreeLinkage, os/exec.Cmd, time.Duration, bareRepositoryError() (+59 more)

### Community 49 - "assertRetryablePayload"
Cohesion: 0.33
Nodes (10): assertRetryablePayload(), contendedWriteError(), openForTest(), readOnlyWriteError(), TestWriteFailureRatesAnUnwritableDatabaseUnavailable(), TestWriteFailureRatesContentionUnavailable(), fullDatabaseWriteError(), TestIsDiskFullIgnoresTheConditionsBesideIt() (+2 more)

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
Cohesion: 0.20
Nodes (8): diagnosis, halt, step, codeOf(), Subject, Result, knowledgeDirRefusal(), offersInit()

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
Cohesion: 0.21
Nodes (7): Report, State, severity(), States(), TestStateEnumIsExactlyFiveValues(), TestStateUnmarshalRejectsUnknown(), TestStateUnmarshalRejectsUnknownInsideAResult()

### Community 67 - "Reader Protocol — Conformance"
Cohesion: 0.22
Nodes (8): R1 — Task normalization, R2 — Requirement verdicts, R3 — Scope compliance, R4 — Contract and documentation conformance, R5 — Evidence standard, R6 — Severity and confidence, R7 — Reader report, Reader Protocol — Conformance

### Community 68 - "graphify reference: extra exports and benchmark"
Cohesion: 0.22
Nodes (8): graphify reference: extra exports and benchmark, Step 6b - Wiki (only if --wiki flag), Step 7 - Neo4j export (only if --neo4j or --neo4j-push flag), Step 7a - FalkorDB export (only if --falkordb or --falkordb-push flag), Step 7b - SVG export (only if --svg flag), Step 7c - GraphML export (only if --graphml flag), Step 7d - MCP server (only if --mcp flag), Step 8 - Token reduction benchmark (only if total_words > 5000)

### Community 69 - "newStore"
Cohesion: 0.33
Nodes (15): Clock, NewStore(), newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace() (+7 more)

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
Cohesion: 0.17
Nodes (20): io/fs.FileInfo, io/fs.FileMode, absentFailure(), classify(), describeMode(), Presence, nonDirectoryAncestor(), Probe() (+12 more)

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
Cohesion: 0.26
Nodes (13): AdoptCause(), CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain() (+5 more)

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
Cohesion: 0.19
Nodes (28): answer, condition, agreeingCommands(), agreementConditions(), answerFrom(), answerOf(), assertExemptionsAreStillEarned(), assertSameAnswer() (+20 more)

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
Cohesion: 0.18
Nodes (11): Components(), ComponentName, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown(), TestTerminalStateSpellingIsTheSpecLiteral(), paintReadiness() (+3 more)

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "Open"
Cohesion: 0.15
Nodes (26): Open(), openTemp(), TestCloseIsIdempotent(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection(), TestOpenCreatesWALSidecar() (+18 more)

### Community 110 - "writability_test.go"
Cohesion: 0.29
Nodes (13): assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), checkNamed(), initGitRepo(), newRepoUnderAnUnrepresentablePath(), TestAHealthyDatabaseReportsThatItsWritabilityWasChecked(), TestANonUTF8RepositoryPathIsRefusedRatherThanMangled() (+5 more)

### Community 111 - "TestBrokenSetupMatrix"
Cohesion: 0.16
Nodes (29): envelopeScenario, strictEnvelope, os.FileMode, TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(), chmodForTest(), configPath(), corruptDatabase(), createUnmigratedDatabase() (+21 more)

### Community 112 - "report.go"
Cohesion: 0.17
Nodes (15): CheckFunc, Result, componentFrom(), currentSchemaVersion(), Report, observationOf(), repositoryObservation(), stateRank() (+7 more)

### Community 113 - "WriteFailure"
Cohesion: 0.21
Nodes (16): DomainError, bookkeepingCreateFailure(), busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), IsBusy(), IsDiskFull() (+8 more)

### Community 114 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "Root"
Cohesion: 0.21
Nodes (8): Modes, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput(), PrivateModes()

### Community 117 - "RootKind"
Cohesion: 0.21
Nodes (14): os.FileInfo, probeRepositoryDir(), ensureDir(), unwritableError(), dangling(), root, RootKind, RuntimePaths (+6 more)

### Community 118 - "newTestRoot"
Cohesion: 0.16
Nodes (18): TestScaffoldFailureNamesTheRepositoryConfigRoot(), TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRepositoryRootIsItsOwnKind(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot() (+10 more)

### Community 119 - "hostilefs_linux_test.go"
Cohesion: 0.18
Nodes (15): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+7 more)

### Community 120 - "RuntimePathCheck"
Cohesion: 0.28
Nodes (14): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+6 more)

### Community 121 - "Mindrail"
Cohesion: 0.33
Nodes (6): Build, Documentation, License, Mindrail, Repository-owned knowledge, Requirements

### Community 122 - ".emit"
Cohesion: 0.32
Nodes (8): globalFlags, humanRenderer, globalFlagsOf(), invocation, Options, newInvocation(), resolveStartDir(), startDirError()

### Community 123 - "Config"
Cohesion: 0.19
Nodes (16): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+8 more)

### Community 124 - "runWith"
Cohesion: 0.35
Nodes (4): brokenSetup, TestNoDocumentContradictsItsOwnErrorObject(), runWith(), TestHumanOutputIsNeverEmpty()

### Community 125 - "github.com/spf13/cobra.Command"
Cohesion: 0.27
Nodes (11): github.com/spf13/cobra.Command, Options, newDoctorCommand(), runDoctor(), Execute(), NewRootCommand(), NewRootCommandWith(), Options (+3 more)

### Community 126 - "refuseUnrepresentableJSON"
Cohesion: 0.31
Nodes (8): reflect.Value, firstUnrepresentable(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo(), TestRepresentableDocumentsAreNotRefused(), unrepresentableError(), walkForInvalidUTF8()

### Community 127 - "coherence_test.go"
Cohesion: 0.13
Nodes (24): remedy, remedyClass, encoding/json.RawMessage, io/fs.DirEntry, absolutePathsIn(), assertDocumentIsCoherent(), classOf(), contradictionsBetween() (+16 more)

### Community 128 - "Probe"
Cohesion: 0.23
Nodes (14): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject (+6 more)

### Community 129 - "runtime.go"
Cohesion: 0.67
Nodes (3): hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 130 - "driver.go"
Cohesion: 0.26
Nodes (10): diskFullCode(), driverResultCode(), dsn(), Options, hasPrimaryCode(), isBusyError(), isCorruptError(), isDiskFullError() (+2 more)

### Community 131 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 132 - "main"
Cohesion: 0.40
Nodes (3): main(), context.CancelFunc, RootContext()

### Community 133 - "InitReport"
Cohesion: 0.67
Nodes (3): Report, InitReport, TerminalState

### Community 134 - "WriteIfAbsent"
Cohesion: 0.19
Nodes (17): TestEmbeddedTemplateLoadsStrictly(), requireEmptyTree(), requireEscape(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository(), TestScaffoldKeepsRepositoryModes(), TestScaffoldRefusesToWriteThroughAnEscapingSymlink(), DefaultTemplate() (+9 more)

### Community 135 - "runInit"
Cohesion: 0.26
Nodes (10): log/slog.Logger, blockingSummary(), invocation, Options, initReportOf(), newInitCommand(), runInit(), schemaIsCurrent() (+2 more)

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
Cohesion: 0.16
Nodes (15): Probes, fallbackWriteRemedy(), unwritableDatabaseSummary(), TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError() (+7 more)

### Community 142 - "NewID"
Cohesion: 0.83
Nodes (3): encodeCrockford(), NewID(), nextEntropy()

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "NoSpaceRefusal"
Cohesion: 0.20
Nodes (9): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+1 more)

### Community 145 - "requireGit"
Cohesion: 0.31
Nodes (9): newNonRepositoryDir(), isolateEnvironment(), newBareRepo(), requireGit(), TestHumanErrorNamesTheProbedDirectory(), newRepoWithUnrepresentablePath(), TestInitRefusesAnUnrepresentableRepositoryBeforeWriting(), TestTheUnrepresentablePathRemediesBothWork() (+1 more)

### Community 146 - "NewLoader"
Cohesion: 0.29
Nodes (13): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), NewLoader(), TestInvalidColorValueIsRejected(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers(), TestProvenanceReportsSourceLayer() (+5 more)

### Community 147 - "TestRepoConfigDirObstructionDoesNotOverFire"
Cohesion: 0.52
Nodes (6): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission(), requireModeEnforcement()

### Community 148 - "MEDIUM"
Cohesion: 0.22
Nodes (9): F04 — `internal/app/exit.go`, F05 — `internal/cli/agreement_test.go`, F06 — `internal/cli/agreement_test.go`, F07 — `internal/cli/contract_test.go`, F08 — `internal/cli/root.go`, F09 — `internal/filesystem/refusal.go`, F10 — `internal/git/adapter.go`, F11 — `internal/git/adapter.go` (+1 more)

### Community 149 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 150 - "mindrail-0.1-kernel-scope.md"
Cohesion: 0.22
Nodes (8): 1. Goal, 2. Primary End-to-End Scenario, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition, Mindrail 0.1 — Kernel Scope

### Community 151 - "29. Large repository cold-index lifecycle"
Cohesion: 0.22
Nodes (9): 29. Large repository cold-index lifecycle, Cooperative scheduler, Foreground ve local worker, Kesilme, Priority aging, Progress, ProjectUnit reservation, Resumability (+1 more)

### Community 152 - "11. SQLite concurrency ve interactive write fairness"
Cohesion: 0.25
Nodes (8): 11. SQLite concurrency ve interactive write fairness, Cold-index write fairness, Idempotency key, Immutable/content-addressed rows, Neden global write broker yok?, Optimistic revision, Process-local writer scheduling, WAL + bounded busy retry

### Community 153 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 154 - "readonlyfs_linux_test.go"
Cohesion: 0.44
Nodes (8): checkReadOnlyRemedy(), openOnAReadOnlyFilesystem(), probeOnAReadOnlyFilesystem(), readOnlyChild(), readOnlyScenario(), remountReadOnly(), remountReadWrite(), runReadOnlyChild()

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "FakeRunner"
Cohesion: 0.32
Nodes (5): FakeResponse, Invocation, sync.Mutex, FakeRunner, stepClock

### Community 157 - "LOW"
Cohesion: 0.29
Nodes (7): F12 — `internal/bootstrap/app.go`, F13 — `internal/bootstrap/app.go`, F14 — `internal/cli/init.go`, F15 — `internal/config/loader.go`, F16 — `internal/git/adapter.go`, F17 — `internal/git/gitfile.go`, LOW

### Community 158 - "open_contention_test.go"
Cohesion: 0.33
Nodes (6): testing.M, runContender(), TestMain(), TestOpenDoesNotWaitForAPermanentFailure(), TestOpenStopsWaitingWhenTheContextIsCancelled(), TestOpenWaitsOutAConcurrentWALConversion()

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "InTx"
Cohesion: 0.53
Nodes (5): InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow()

### Community 163 - "2. Mindrail'in temel problemi"
Cohesion: 0.40
Nodes (5): 2.1 Context kaybı, 2.2 Semantic conflict, 2.3 Intent kaybı, 2.4 Kanıtsız tamamlanma, 2. Mindrail'in temel problemi

### Community 164 - "[ ] MR-001 — Yerel repository bootstrap ve tanılama yolu"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-001 — Yerel repository bootstrap ve tanılama yolu, Ne yapılacak

### Community 165 - "Görevler"
Cohesion: 0.50
Nodes (4): Görevler, Kabul kriterleri, [ ] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü, Ne yapılacak

### Community 166 - "122. Dynamic dispatch ve validation budget"
Cohesion: 0.50
Nodes (4): 122. Dynamic dispatch ve validation budget, Escalation, User control, Validation budget policy

### Community 167 - "14. Code intelligence tiers"
Cohesion: 0.50
Nodes (4): 14. Code intelligence tiers, FILE, FULL, STRUCTURAL

### Community 168 - "Full support"
Cohesion: 0.50
Nodes (4): 15. Hedef language scope, Full support, Python, TypeScript / JavaScript

### Community 169 - "20. Confidence artık global tek sayı değildir"
Cohesion: 0.50
Nodes (4): 20. Confidence artık global tek sayı değildir, Analysis coverage, Edge confidence, Finding confidence

### Community 170 - "61. Multi-agent çalışma modları"
Cohesion: 0.50
Nodes (4): 61. Multi-agent çalışma modları, Mode A — Same workspace, sequential agents, Mode B — Separate worktrees, concurrent agents, Mode C — Same workspace, concurrent agents

### Community 171 - "7. Data plane ayrımı"
Cohesion: 0.50
Nodes (4): 7.1 Engineering Knowledge, 7.2 Runtime Coordination, 7.3 Proof, 7. Data plane ayrımı

### Community 172 - "[ ] MR-004 — Güvenli lease, idempotency ve optimistic revision"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision, Ne yapılacak

### Community 173 - "[ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme, Ne yapılacak

### Community 174 - "[ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması, Ne yapılacak

### Community 175 - "[ ] MR-007 — Reconcile-first gerçek değişiklik keşfi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi, Ne yapılacak

### Community 176 - "[ ] MR-008 — Scope drift ve unregistered change ambiguity"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-008 — Scope drift ve unregistered change ambiguity, Ne yapılacak

### Community 177 - "[ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi, Ne yapılacak

### Community 178 - "[ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence, Ne yapılacak

### Community 179 - "[ ] MR-011 — Kaynak değişince evidence geçersizleştirme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-011 — Kaynak değişince evidence geçersizleştirme, Ne yapılacak

### Community 180 - "[ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı, Ne yapılacak

### Community 181 - "[ ] MR-013 — Yerel completion evidence gate"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, Ne yapılacak

### Community 182 - "[ ] MR-014 — MCP bilgi ve bağlam araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-014 — MCP bilgi ve bağlam araçları, Ne yapılacak

### Community 183 - "[ ] MR-015 — MCP koordinasyon ve değişiklik araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-015 — MCP koordinasyon ve değişiklik araçları, Ne yapılacak

### Community 184 - "[ ] MR-016 — MCP validation ve completion araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-016 — MCP validation ve completion araçları, Ne yapılacak

### Community 185 - "[ ] MR-017 — Staged değişiklik için yerel Git enforcement"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-017 — Staged değişiklik için yerel Git enforcement, Ne yapılacak

### Community 186 - "[ ] MR-018 — Hook bypass'a dayanıklı CI verification"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak

### Community 187 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 188 - "[ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi, Ne yapılacak

### Community 189 - "[ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri, Ne yapılacak

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

## Knowledge Gaps
- **904 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `invocation`, `Report`, `Report` (+899 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1012 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **13 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `Probe`, `exit.go`, `DB`, `writable.go`, `io.Writer`, `config/loader.go`, `time.Time`, `New`, `context.Context`, `healthySubject`, `Migrator`, `Build`, `Code`, `adapter.go`, `Probe`, `TestBrokenSetupMatrix`, `WriteFailure`, `Root`, `RootKind`, `RuntimePathCheck`, `.emit`, `Config`, `refuseUnrepresentableJSON`, `coherence_test.go`?**
  _High betweenness centrality (0.034) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `driver.go`, `WriteIfAbsent`, `runInit`, `DB`, `ResolveRuntimePaths`, `fulldisk_linux_test.go`, `io.Writer`, `New`, `NewLoader`, `contract_test.go`, `TestRepoConfigDirObstructionDoesNotOverFire`, `New`, `time.Time`, `context.Context`, `checks.go`, `scaffold_hostilefs_linux_test.go`, `healthySubject`, `readonlyfs_linux_test.go`, `Migrator`, `loader/loader_test.go`, `Code`, `adapter.go`, `assertRetryablePayload`, `Subject`, `newStore`, `Probe`, `ProbeWriteAccess`, `NewError`, `Open`, `TestBrokenSetupMatrix`, `newTestRoot`, `hostilefs_linux_test.go`, `runWith`, `refuseUnrepresentableJSON`?**
  _High betweenness centrality (0.027) - this node is a cross-community bridge._
- **Why does `FakeRunner` connect `FakeRunner` to `agreement_test.go`, `PayloadOf`, `TestBrokenSetupMatrix`?**
  _High betweenness centrality (0.012) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `invocation` to the rest of the system?**
  _904 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Mindrail 0.1 — Uygulama Görev Listesi` be split into smaller, more focused modules?**
  _Cohesion score 0.14285714285714285 - nodes in this community are weakly interconnected._