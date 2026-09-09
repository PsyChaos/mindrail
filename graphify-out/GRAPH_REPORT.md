# Graph Report - Mindrail  (2026-09-09)

## Corpus Check
- 285 files · ~504,166 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 3817 nodes · 9913 edges · 234 communities (211 shown, 15 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 1669 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `7ea8d837`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- MINDRAIL PROTOCOL Managed Section
- mindrail-0.1-task-list.md
- mindrail-tech-stack.md
- mindrail-technical-specification-1.0.md
- 129. Acceptance criteria
- 19. Semantic resolver kurulum, lifecycle ve resource policy
- bootstrap.App
- testing.T
- MR-002 — findings
- RootKind
- adapter.go
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- io.Writer
- config/loader.go
- New
- NewRoot
- contract_test.go
- pathsAt
- New
- status/render_test.go
- database/sql.DB
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- context.Context
- DefaultChecks
- RuntimePathCheck
- runInit
- newLoader
- Build
- validate.py
- Phase 3 — Doc ↔ Code consistency
- 1. Decisions this document freezes
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
- coordination/store_test.go
- 3. The findings in full
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- Root
- hostilefs_linux_test.go
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
- TestBrokenSetupMatrix
- storeFile
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
- 128. Hedef implementation workstream'leri
- db.go
- Finding
- check_test.go
- report.go
- NewError
- SQLiteCheck
- 47. Human approval kimlik ve yetkilendirme modeli
- Store
- States
- Check
- IsRegistered
- storage/arch_test.go
- WriteFailure
- TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline
- Config
- ProbeWriteAccess
- Root
- goldenReport
- coherence_test.go
- 5. Test plan
- 3. IN Scope
- shutdown
- refuseUnrepresentableJSON
- record/errors.go
- 1. Decisions this document freezes
- WriteIfAbsent
- NewID
- storage/access_other.go
- agreement_hostilefs_linux_test.go
- columns.go
- TestRepoConfigDirObstructionDoesNotOverFire
- newStore
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- FreeSpace
- time.Time
- NewLoader
- 2. Public API per package
- MR-003 — design: sequential agent handover
- app.go
- MR-002 — audit package
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- scaffold_hostilefs_linux_test.go
- Probe
- 121. Failure modes
- runWith
- LOW
- Görevler
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- 4. Work units
- 2. Mindrail'in temel problemi
- [ ] MR-016 — MCP validation ve completion araçları
- [x] MR-001 — Yerel repository bootstrap ve tanılama yolu
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- DomainError
- created_at
- github.com/spf13/cobra.Command
- MR-002 — Versioned Decision/Invariant knowledge lifecycle
- Store
- runtimeDBPath
- Applied
- payloadOf
- timestamp_ground_test.go
- newInvocation
- Migration
- 2. Requirements
- InTx
- MEDIUM
- validate_test.go
- steps_test.go
- MR-003 — findings
- version.go
- 103. Test weakening ve false-green guard
- 28. Interactive performance contract
- 159. First Implementation Milestone
- 33. Durable Symbol Identity Implementation
- 56. Resolver Process Supervision
- 12. Content-addressed index cache
- 36. Impact explosion ve drill-down modeli
- 3. Mindrail ne değildir?
- Audit round 1
- D-52 — a supersede node is owned by the record filed at the path its id names
- [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü
- open_contention_test.go
- validator_test.go
- [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision
- RootContext
- status/knowledge_findings_test.go
- Open
- [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- decodeData
- [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- TASK-01 dependency + schema.Validator
- TASK-02 internal/knowledge/record
- TASK-03 loader RecordRef.Body
- TASK-04+05 app codes + validate.Check
- TASK-06 bootstrap/doctor/status wiring
- TASK-07 CLI classifier + agreement matrix
- TASK-08 independent mutation sweep
- IsUnwritten
- Options
- MR-002 audit package — appendix: implementer self-reports
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- scope
- [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri
- [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması
- [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi
- [ ] MR-008 — Scope drift ve unregistered change ambiguity
- [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi
- [ ] MR-011 — Kaynak değişince evidence geçersizleştirme
- [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı
- [ ] MR-014 — MCP bilgi ve bağlam araçları
- [ ] MR-015 — MCP koordinasyon ve değişiklik araçları
- [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 128 edges
2. `NewError()` - 105 edges
3. `Check()` - 92 edges
4. `storeOf()` - 77 edges
5. `shippedValidator()` - 69 edges
6. `decisionDoc()` - 68 edges
7. `healthySubject()` - 67 edges
8. `decisionAt()` - 67 edges
9. `run()` - 64 edges
10. `New()` - 54 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `ExitCode()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/exit.go
- `runCoordination()` --calls--> `newInvocation()`  [INFERRED]
  internal/cli/coordination.go → internal/cli/output.go
- `runCoordination()` --calls--> `shutdown()`  [INFERRED]
  internal/cli/coordination.go → internal/cli/output.go
- `NewRootWith()` --calls--> `newSessionCommand()`  [INFERRED]
  internal/cli/root.go → internal/cli/coordination.go
- `startupHaltedConditions()` --calls--> `dropRuntimeTables()`  [INFERRED]
  internal/cli/coordination_agreement_test.go → internal/cli/agreement_test.go

## Import Cycles
- None detected.

## Communities (234 total, 15 thin omitted)

### Community 0 - "MINDRAIL PROTOCOL Managed Section"
Cohesion: 0.33
Nodes (6): Graphify Protocol, Managed Section Markers, MINDRAIL PROTOCOL Managed Section, Not Yet Active Status Note, Protocol Call Sequence, Soft Enforcement Note

### Community 1 - "mindrail-0.1-task-list.md"
Cohesion: 0.21
Nodes (6): 0.1 Dışında Tutulacak Backlog, 0.1 Kabul Kriteri İzlenebilirlik Matrisi, Bağımlılık Dalgaları, Kullanım, Mindrail 0.1 — Uygulama Görev Listesi, Remediation brief

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

### Community 6 - "bootstrap.App"
Cohesion: 0.11
Nodes (8): bootstrap.App, InitResult, txProbe, sync/atomic.Bool, sync/atomic.Int64, sync.Once, asRepositoryError(), CommandRunner

### Community 7 - "testing.T"
Cohesion: 0.09
Nodes (43): testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestFixedClockSatisfiesClock(), TestFormatTimeIsUTCRFC3339() (+35 more)

### Community 8 - "MR-002 — findings"
Cohesion: 0.05
Nodes (41): 1. The number that matters, 2. Round 1: what was actually wrong, 3. Round 2: what closing them cost, 4. Status, 5. What generalises to MR-003, A containment gap MR-002 made load-bearing, Appendix A — round 1 findings, as confirmed, Appendix B — what the remediation did (+33 more)

### Community 9 - "RootKind"
Cohesion: 0.19
Nodes (13): os.FileInfo, ensureDir(), unwritableError(), dangling(), root, RootKind, RuntimePaths, Writability (+5 more)

### Community 10 - "adapter.go"
Cohesion: 0.05
Nodes (69): Adapter, ExecRunner, gitFileFault, standpoint, worktreeLinkage, os/exec.Cmd, time.Duration, bareRepositoryError() (+61 more)

### Community 11 - "ResolveRuntimePaths"
Cohesion: 0.19
Nodes (22): gitLayout, PathOptions, hasParentSegment(), overrideOrDefault(), requireAbsolute(), ResolveRuntimePaths(), assertMode(), newGitLayout() (+14 more)

### Community 12 - "ADR-0002 — SQLite driver"
Cohesion: 0.33
Nodes (5): ADR-0002 — SQLite driver, Consequences, Context, Decision, Open — benchmark matrix required before this ADR is accepted

### Community 13 - "ADR-0001 — Primary language and CLI stack"
Cohesion: 0.40
Nodes (4): ADR-0001 — Primary language and CLI stack, Consequences, Context, Decision

### Community 15 - "io.Writer"
Cohesion: 0.15
Nodes (16): Envelope, taskListResult, io.Writer, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal() (+8 more)

### Community 16 - "config/loader.go"
Cohesion: 0.20
Nodes (14): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+6 more)

### Community 17 - "New"
Cohesion: 0.08
Nodes (81): testing/fstest.MapFS, testing.M, FormatTime(), assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), TestMain(), Load() (+73 more)

### Community 18 - "NewRoot"
Cohesion: 0.16
Nodes (19): TestScaffoldFailureNamesTheRepositoryConfigRoot(), TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestModeVariantsStillRefuseAnEscape(), TestRepositoryRootIsItsOwnKind(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize() (+11 more)

### Community 19 - "contract_test.go"
Cohesion: 0.13
Nodes (45): result, addWorktree(), assertFourErrorKeys(), assertGolden(), assertShape(), findCheck(), isEmptyValue(), join() (+37 more)

### Community 20 - "pathsAt"
Cohesion: 0.20
Nodes (21): TestAtMostOneRecordCanBeCanonicalForAnId(), TestOnlyAByteIdenticalFileNameOwnsAnId(), TestOwnershipFollowsTheNameTheDirectoryHolds(), TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions(), TestAnInvariantCycleIsFatalAndNamesOnlyInvariants(), TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord(), TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge(), TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose() (+13 more)

### Community 21 - "New"
Cohesion: 0.10
Nodes (45): Recorder, refusingTransport, stepRecorder, net/http.Request, net/http.Response, New(), assertNoRuntimeState(), corruptDatabase() (+37 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.20
Nodes (20): InitReport, TestKnowledgeFindingsAreCountedFromTheSubject(), assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport() (+12 more)

### Community 23 - "database/sql.DB"
Cohesion: 0.38
Nodes (7): database/sql.Conn, database/sql.DB, boundCheckpointWait(), Checkpoint(), checkpointFailure(), checkpointOutcome(), CheckpointResult

### Community 24 - "checks.go"
Cohesion: 0.14
Nodes (36): allEscapeRoot(), alsoHeading(), degradedAccount(), degradedImpact(), degradedSummary(), describeBounded(), describeFindings(), describeProblems() (+28 more)

### Community 25 - "healthySubject"
Cohesion: 0.11
Nodes (56): tail, KnowledgeCheck(), assertDiagnosable(), Result, healthySubject(), TestAnUnusableKnowledgeDirectoryIsReportedAsAPathCondition(), TestGitCheckBranches(), TestKnowledgeCheckAbsentTreeIsOK() (+48 more)

### Community 26 - "PayloadOf"
Cohesion: 0.07
Nodes (103): Error, FakeResponse, Invocation, ErrorPayload, PayloadOf(), Denied(), ExitCode(), exitCodeForKind() (+95 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "context.Context"
Cohesion: 0.26
Nodes (3): context.Context, bookkeepingCreateFailure(), Migrator

### Community 29 - "DefaultChecks"
Cohesion: 0.13
Nodes (32): App, Runner, Subject, NewRunner(), TestRunnerRespectsContextCancellation(), Verdict(), DefaultChecks(), assertActionable() (+24 more)

### Community 30 - "RuntimePathCheck"
Cohesion: 0.28
Nodes (14): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+6 more)

### Community 31 - "runInit"
Cohesion: 0.18
Nodes (15): blockingSummary(), Options, initReportOf(), initVerdict(), newInitCommand(), runInit(), schemaIsCurrent(), TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee() (+7 more)

### Community 32 - "newLoader"
Cohesion: 0.06
Nodes (76): encoding/json.RawMessage, io/fs.DirEntry, reflect.Type, testing.B, decisionRef(), decodeObjects(), describeRefs(), everyKey() (+68 more)

### Community 33 - "Build"
Cohesion: 0.15
Nodes (32): Subject, TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), TestRepositoryBlockIsObservedOnAHealthyRepository(), TestRepositoryBlockSurvivesALaterStepFailing() (+24 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "1. Decisions this document freezes"
Cohesion: 0.07
Nodes (29): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's four acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-53 — the coordination tables are a second migration, and the reported runtime schema version becomes 2, D-54 — "Checkpoint" means two things in this repository, and neither is renamed (+21 more)

### Community 37 - "status/render.go"
Cohesion: 0.14
Nodes (21): strings.Builder, Report, Result, paint(), writeBlock(), writeResult(), Component, ComponentName (+13 more)

### Community 38 - "Final report"
Cohesion: 0.11
Nodes (17): 1. Task, 2. Requirements, 3. Teams, 4. Execution, 5. Implementation, 6. Validation, 7. Dual-agent audit, 8. Remaining issues (+9 more)

### Community 39 - "properties"
Cohesion: 0.15
Nodes (13): const, type, properties, kind, rationale, scope, statement, description (+5 more)

### Community 40 - "Task Verification Report"
Cohesion: 0.12
Nodes (15): 1. Original Task, 2. Requirement Matrix, 3. Confirmed Findings, 4. Refuted Findings, 5. Open Items, 6. Off-Spec Headings, 7. Scope Compliance, 8. Test & Verification Assessment (+7 more)

### Community 41 - "Test Suite Health & Redundancy Audit"
Cohesion: 0.12
Nodes (15): 10. Cleanup Plan, 11. Before / After Comparison, 12. Final Confidence Assessment, 1. Executive Summary, 2. Test Inventory, 3. Behavior Coverage Map, 4. Redundancy Clusters, 5. Deletion Candidates (+7 more)

### Community 42 - "cli/arch_test.go"
Cohesion: 0.31
Nodes (14): firstPartyPackage, assertMainDeclaresOnlyMain(), assertNoNetworkInLinkedPackages(), fileImports(), firstPartyPackages(), goSources(), isStandardLibrary(), matchForbidden() (+6 more)

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

### Community 48 - "coordination/store_test.go"
Cohesion: 0.28
Nodes (19): fixture, ids(), newFixture(), newStepClock(), openFixture(), TestABlockCarriesItsReasonAndLeavingClearsIt(), TestAnUnknownSessionIsRefusedByEveryWriter(), TestAnUnknownTaskIsRefusedByNameRatherThanByAConstraint() (+11 more)

### Community 49 - "3. The findings in full"
Cohesion: 0.07
Nodes (28): 3. The findings in full, F01 (also F41) — MEDIUM. After upgrading, a registered worktree is told it is not registered, F02 (also F11, F31, F38) — MEDIUM. A refused write leaves a session row and says nothing was written, F04 — MEDIUM. The write-failure ordering AC-04.5 asks for is correct and unguarded, F06 (also F25) — LOW. AC-04.1 names a method the store does not have, F07 — LOW. Two of six error constructors put the offending id in a remedy, F09 (also F29, F36, F43) — HIGH. A coordination command reports a halted startup as a missing database, F10 (also F03, F39, F45) — MEDIUM. `mindrail init` says startup stopped, three lines under READY (+20 more)

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

### Community 58 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 59 - "hostilefs_linux_test.go"
Cohesion: 0.20
Nodes (15): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+7 more)

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
Cohesion: 0.16
Nodes (8): Report, State, severity(), States(), TestStateEnumIsExactlyFiveValues(), TestStateUnmarshalRejectsUnknown(), TestStateUnmarshalRejectsUnknownInsideAResult(), Component

### Community 67 - "Reader Protocol — Conformance"
Cohesion: 0.22
Nodes (8): R1 — Task normalization, R2 — Requirement verdicts, R3 — Scope compliance, R4 — Contract and documentation conformance, R5 — Evidence standard, R6 — Severity and confidence, R7 — Reader report, Reader Protocol — Conformance

### Community 68 - "graphify reference: extra exports and benchmark"
Cohesion: 0.22
Nodes (8): graphify reference: extra exports and benchmark, Step 6b - Wiki (only if --wiki flag), Step 7 - Neo4j export (only if --neo4j or --neo4j-push flag), Step 7a - FalkorDB export (only if --falkordb or --falkordb-push flag), Step 7b - SVG export (only if --svg flag), Step 7c - GraphML export (only if --graphml flag), Step 7d - MCP server (only if --mcp flag), Step 8 - Token reduction benchmark (only if total_words > 5000)

### Community 69 - "Check"
Cohesion: 0.17
Nodes (59): TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(), TestADecisionAndAnInvariantNeverShareAGraphNode(), TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle(), TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages(), TestAnIdNoFileIsNamedAfterSuppliesNoEdges(), TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries(), TestAValidStoreIsStillSilentWithAStrayDraftBesideIt(), TestRoundThreesMisfiledVoucherDoesNotDeleteTheCycle() (+51 more)

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

### Community 83 - "TestBrokenSetupMatrix"
Cohesion: 0.12
Nodes (36): envelopeScenario, strictEnvelope, discoveryConditions(), newNonRepositoryDir(), unreadableRepositoryGit(), TestEveryUninitialisedSpellingExitsZero(), configPath(), corruptDatabase() (+28 more)

### Community 84 - "storeFile"
Cohesion: 0.24
Nodes (14): TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone(), combine(), holdsTwoReportableSelfLoops(), oracleCycleMembers(), recordsFor(), renderStore(), storesOfOneKind(), TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore() (+6 more)

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
Cohesion: 0.24
Nodes (24): condition, agreeingCommands(), agreementConditions(), assertRemedyIsWhatTheRowExpects(), chmodForRemedy(), dropRuntimeTables(), emptyMigrationLedger(), execOnRuntimeDB() (+16 more)

### Community 91 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

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
Cohesion: 0.26
Nodes (8): Components(), ComponentName, Readiness, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown(), TestTerminalStateSpellingIsTheSpecLiteral()

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "db.go"
Cohesion: 0.14
Nodes (24): classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure(), Options, notWALFailure(), openFailure(), openOnce() (+16 more)

### Community 110 - "Finding"
Cohesion: 0.10
Nodes (36): compareNodes(), ids(), newGraph(), facts(), TestAStepsMessageCarriesTheFactsItsRemedyNeeds(), TestEveryStepsMessageIsCovered(), expectedPath(), idFromPath() (+28 more)

### Community 111 - "check_test.go"
Cohesion: 0.27
Nodes (11): okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestCheckFuncWithoutFunctionIsNotSilent(), TestHaltErrorPointsAtTheCheckThatExplainsTheHalt(), TestReportErrExitCodeFollowsTheFailureClass(), TestReportErrOnlyForErrorState() (+3 more)

### Community 112 - "report.go"
Cohesion: 0.19
Nodes (18): classify(), componentFrom(), coordinationInfo(), Component, ComponentName, Readiness, Report, observationOf() (+10 more)

### Community 113 - "NewError"
Cohesion: 0.13
Nodes (19): CheckFunc, AdoptCause(), CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause() (+11 more)

### Community 114 - "SQLiteCheck"
Cohesion: 0.23
Nodes (12): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject (+4 more)

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "Store"
Cohesion: 0.27
Nodes (11): database/sql.Tx, Registration, Store, NewStore(), scanWorkspace(), upsertProject(), upsertWorkspace(), validateRegistration() (+3 more)

### Community 117 - "States"
Cohesion: 0.27
Nodes (10): CanTransition(), ParseState(), States(), TestARefusalCanSayWhatIsAvailableInstead(), TestATaskNeverMovesToTheStateItIsAlreadyIn(), TestEveryOrderedPairOfStatesGetsTheAnswerTheSpecificationGives(), TestParseStateAcceptsTheSevenAndRefusesEverythingElse(), TestTheTerminalStatesAreTerminalAndNothingElseIs() (+2 more)

### Community 118 - "Check"
Cohesion: 0.25
Nodes (20): Check, ConfigCheck(), configResult(), describePath(), describeUninitializedDB(), explain(), failure(), GitCheck() (+12 more)

### Community 119 - "IsRegistered"
Cohesion: 0.29
Nodes (10): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+2 more)

### Community 120 - "storage/arch_test.go"
Cohesion: 0.14
Nodes (20): go/ast.ImportSpec, moduleRoot(), TestTheKnowledgePipelineHasExactlyOneCallSite(), lifecycleLiteralsOutsideThisPackage(), moduleRoot(), TestNoOtherPackageStatesTheLifecycle(), Root(), SkipDir() (+12 more)

### Community 121 - "WriteFailure"
Cohesion: 0.27
Nodes (16): IsBusy(), IsDiskFull(), IsReadOnly(), assertRetryablePayload(), contendedWriteError(), openForTest(), readOnlyWriteError(), TestWriteFailureNamesNothingItCannotIdentify() (+8 more)

### Community 122 - "TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline"
Cohesion: 0.06
Nodes (69): regexp.Regexp, TestAConstructorNamesThePositionOfANilOption(), TestOptionsThatAreNotNilAreStillApplied(), TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(), TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(), Decision, NewDecision(), NewInvariant() (+61 more)

### Community 123 - "Config"
Cohesion: 0.19
Nodes (16): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+8 more)

### Community 124 - "ProbeWriteAccess"
Cohesion: 0.06
Nodes (65): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable() (+57 more)

### Community 125 - "Root"
Cohesion: 0.17
Nodes (10): classifiedArgs, Root, github.com/spf13/cobra.Completion, github.com/spf13/cobra.PositionalArgs, github.com/spf13/cobra.ShellCompDirective, Execute(), helpTopics(), NewRoot() (+2 more)

### Community 126 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 127 - "coherence_test.go"
Cohesion: 0.19
Nodes (18): remedy, remedyClass, agreementRemedyClass(), TestEveryRemedyThatNamesAPathIsClassified(), absolutePathsIn(), assertDocumentIsCoherent(), classOf(), contradictionsBetween() (+10 more)

### Community 128 - "5. Test plan"
Cohesion: 0.12
Nodes (16): 5.10 `internal/doctor` (Unit D) — 11, 5.11 `internal/status` (Unit D) — 8, 5.12 `internal/bootstrap` (Unit E) — 8, 5.13 `internal/cli` — CLI contract (Unit E) — 14, 5.14 Architecture + smoke (Unit E) — 6, 5.15 Acceptance-criteria mapping, 5.1 `internal/app` (Unit D) — 10, 5.2 `internal/filesystem` (Unit A) — 8 (+8 more)

### Community 129 - "3. IN Scope"
Cohesion: 0.06
Nodes (30): 1. Goal, 2. Primary End-to-End Scenario, 3. IN Scope, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition (+22 more)

### Community 130 - "shutdown"
Cohesion: 0.27
Nodes (9): log/slog.Logger, Options, newDoctorCommand(), runDoctor(), shutdown(), Options, newStatusCommand(), readinessOf() (+1 more)

### Community 131 - "refuseUnrepresentableJSON"
Cohesion: 0.24
Nodes (9): reflect.Value, invocation, firstUnrepresentable(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo(), TestRepresentableDocumentsAreNotRefused(), unrepresentableError() (+1 more)

### Community 132 - "record/errors.go"
Cohesion: 0.20
Nodes (3): DuplicateItemError, EmptyFieldError, OptionKindError

### Community 133 - "1. Decisions this document freezes"
Cohesion: 0.12
Nodes (16): 0. Baseline evidence, 1. Decisions this document freezes, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation, D-43 — a Finding gets its own codes, because D-38 says it is not a Problem, D-44 — a fatal loader Problem outranks every Finding (+8 more)

### Community 134 - "WriteIfAbsent"
Cohesion: 0.21
Nodes (16): requireEmptyTree(), requireEscape(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository(), TestScaffoldKeepsRepositoryModes(), TestScaffoldRefusesToWriteThroughAnEscapingSymlink(), DefaultTemplate(), EnsureKnowledgeDirs() (+8 more)

### Community 135 - "NewID"
Cohesion: 0.27
Nodes (9): encodeCrockford(), compareRandomness(), TestABackwardsClockStillMintsAscendingIds(), TestTheEntropyCounterCarriesAcrossAByte(), NewID(), nextEntropy(), TestAnIdIsPrefixedUniqueAndSortsInMintOrder(), TestAnIdLeaksNeitherTheClockNorTheCaller() (+1 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "agreement_hostilefs_linux_test.go"
Cohesion: 0.17
Nodes (26): answer, hostileCondition, agreeingHostileCommands(), assertTheLoopTerminates(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo(), hostileAvailableBytes() (+18 more)

### Community 140 - "columns.go"
Cohesion: 0.43
Nodes (7): alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns(), unquote()

### Community 141 - "TestRepoConfigDirObstructionDoesNotOverFire"
Cohesion: 0.52
Nodes (6): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission(), requireModeEnforcement()

### Community 142 - "newStore"
Cohesion: 0.38
Nodes (13): newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace(), TestRegisteredIDsAreOpaqueAndPrefixed(), TestRegisterIsIdempotent() (+5 more)

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "FreeSpace"
Cohesion: 0.13
Nodes (16): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+8 more)

### Community 145 - "time.Time"
Cohesion: 0.27
Nodes (6): FixedClock, SystemClock, stepClock, sync.Mutex, time.Time, stepClock

### Community 146 - "NewLoader"
Cohesion: 0.27
Nodes (14): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers() (+6 more)

### Community 147 - "2. Public API per package"
Cohesion: 0.08
Nodes (23): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+15 more)

### Community 148 - "MR-003 — design: sequential agent handover"
Cohesion: 0.17
Nodes (12): 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000002_coordination.sql`, 6. The state table, 7. Command surface, 8. Status integration (+4 more)

### Community 149 - "app.go"
Cohesion: 0.16
Nodes (13): Mode, Options, io/fs.FS, asKnowledgeError(), asMigrationError(), asMigrationSetError(), asRuntimePathError(), asWorkspaceError() (+5 more)

### Community 150 - "MR-002 — audit package"
Cohesion: 0.15
Nodes (12): 10. The independent mutation sweep, and what it found, 11. What the audit is asked to establish, 1. Original request, verbatim, 2. The task, 3. Requirements and acceptance criteria, 4. Design contract, 5. Implementation summary, 6. Change references (+4 more)

### Community 151 - "29. Large repository cold-index lifecycle"
Cohesion: 0.22
Nodes (9): 29. Large repository cold-index lifecycle, Cooperative scheduler, Foreground ve local worker, Kesilme, Priority aging, Progress, ProjectUnit reservation, Resumability (+1 more)

### Community 152 - "11. SQLite concurrency ve interactive write fairness"
Cohesion: 0.25
Nodes (8): 11. SQLite concurrency ve interactive write fairness, Cold-index write fairness, Idempotency key, Immutable/content-addressed rows, Neden global write broker yok?, Optimistic revision, Process-local writer scheduling, WAL + bounded busy retry

### Community 153 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 154 - "Probe"
Cohesion: 0.16
Nodes (13): diagnosis, halt, Probes, step, codeOf(), fallbackWriteRemedy(), Subject, Result (+5 more)

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "runWith"
Cohesion: 0.22
Nodes (11): brokenSetup, condition, answerOf(), runWith(), commandName(), coordinationCommands(), startupHaltedConditions(), TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes() (+3 more)

### Community 157 - "LOW"
Cohesion: 0.11
Nodes (17): F01 — `mindrail init` reported READY over a repository the next command refused, F02 — an unopenable `config.toml` was remedied as a malformed one, F03 — a size limit was reported as a full disk, F12 — a defective build blamed the repository, F13 — the rebuild remedy did not name the database, F14 — three unexercised claims in the init report path, F15 — the containment remedy named no path, F16 — a guard that could not fail (+9 more)

### Community 158 - "Görevler"
Cohesion: 0.29
Nodes (7): Görevler, Kabul kriterleri, Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak, Ne yapılacak

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "4. Work units"
Cohesion: 0.25
Nodes (8): 4. Work units, File-ownership summary, Unit A — Repository & filesystem, Unit B — Runtime store, Unit C — Config & knowledge, Unit D — Error model & diagnostics, Unit-D T0 handshake, Unit E — Wiring (final, sequential)

### Community 163 - "2. Mindrail'in temel problemi"
Cohesion: 0.40
Nodes (5): 2.1 Context kaybı, 2.2 Semantic conflict, 2.3 Intent kaybı, 2.4 Kanıtsız tamamlanma, 2. Mindrail'in temel problemi

### Community 164 - "[ ] MR-016 — MCP validation ve completion araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-016 — MCP validation ve completion araçları, Ne yapılacak

### Community 165 - "[x] MR-001 — Yerel repository bootstrap ve tanılama yolu"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-001 — Yerel repository bootstrap ve tanılama yolu

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

### Community 172 - "DomainError"
Cohesion: 0.25
Nodes (11): DomainError, busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), MainDatabaseFile(), mainDatabaseFile(), readOnlyMediaWriteFailure() (+3 more)

### Community 173 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

### Community 174 - "github.com/spf13/cobra.Command"
Cohesion: 0.34
Nodes (14): github.com/spf13/cobra.Command, humanRenderer, Options, newCheckpointCommand(), newCheckpointWriteCommand(), runCoordination(), Options, newTaskCommand() (+6 more)

### Community 175 - "MR-002 — Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.22
Nodes (9): 0. What MR-001 left here, exactly, 1. Scope decision: no CLI write surface, 2. Package plan, 3. Frozen signatures, 4. Decisions, 5. Test plan, 6. Explicit non-goals, 7. Acceptance criteria traceability (+1 more)

### Community 176 - "Store"
Cohesion: 0.09
Nodes (26): checkpointResult, handoverResult, taskResult, coordination.Checkpoint, coordination.CheckpointRef, coordination.Handover, rowScanner, coordination.Summary (+18 more)

### Community 177 - "runtimeDBPath"
Cohesion: 0.29
Nodes (14): chmodForTest(), runtimeDBPath(), assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), checkNamed(), newRepoUnderAnUnrepresentablePath(), TestAHealthyDatabaseReportsThatItsWritabilityWasChecked() (+6 more)

### Community 178 - "Applied"
Cohesion: 0.40
Nodes (7): checksumFailure(), Applied, highestVersion(), namesAbsentFrom(), readLedgerRow(), schemaAheadFailure(), Result

### Community 179 - "payloadOf"
Cohesion: 0.33
Nodes (8): TestMachineLocalRootsStillOfferTheirOverride(), requireModeEnforcement(), TestObstructedDirNamesOnlyWhatCanNeverBeADirectory(), TestProbeRepoConfigReportsAWorktreeThatRefusesTheScaffold(), TestProbeRepoConfigSeparatesNoInputFromNoPermission(), TestRemedyNamesAPathTheUserCanActuallyChmod(), payloadOf(), TestUnwritablePathRemedyMatchesTheObstruction()

### Community 180 - "timestamp_ground_test.go"
Cohesion: 0.18
Nodes (18): createdAtDescription(), everyDescriptionClaim(), everyRefusedSpelling(), measuredGround(), TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(), TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(), TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(), TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository() (+10 more)

### Community 181 - "newInvocation"
Cohesion: 0.32
Nodes (8): globalFlags, humanRenderer, globalFlagsOf(), invocation, Options, newInvocation(), resolveStartDir(), startDirError()

### Community 182 - "Migration"
Cohesion: 0.42
Nodes (6): createdObjects(), Migration, SchemaObject, schemaEffects(), namesPresentIn(), SchemaEffect

### Community 183 - "2. Requirements"
Cohesion: 0.15
Nodes (13): 2. Requirements, REQ-01 — `internal/knowledge/record`: the typed records and their lifecycle, REQ-02 — `internal/knowledge/schema.Validator`: step 5's mechanism, REQ-03 — `internal/knowledge/validate.Check`: steps 5–11, REQ-04 — `internal/knowledge/loader` carries the body, REQ-05 — the code vocabulary, REQ-06 — `internal/doctor` reports findings distinctly from problems, REQ-07 — `internal/status` publishes the count (+5 more)

### Community 184 - "InTx"
Cohesion: 0.36
Nodes (6): InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow(), txKeyType

### Community 185 - "MEDIUM"
Cohesion: 0.25
Nodes (8): F04, F08 — a rejected command line exited 1 and wrote no envelope, F05 — the agreement matrix never read the remedy it claimed to carry out, F06 — no matrix row for a `config.toml` that cannot be opened, F07 — the linked-worktree cross-check compared two fields of four, F09 — the read-only-media sentinel had no row in the classifier's table, F10 — `sameDirectory`'s identity comparison was untested, F11 — the linked-worktree back-pointer check was untested, MEDIUM

### Community 186 - "validate_test.go"
Cohesion: 0.15
Nodes (20): RecordKind, RecordRef, suppressions(), TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided(), TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded(), TestStepSevenIsExemptFromTheSuppression(), TestSuppressionKeysOnTheFileTheReferencedIdWouldOccupy(), everyConditionStore() (+12 more)

### Community 187 - "steps_test.go"
Cohesion: 0.33
Nodes (9): detections(), overFires(), TestEachStepDetectsItsCondition(), TestEveryStepHasAnOverFireGuard(), TestEveryStepIsRepresentedInTheDetectionTable(), TestFindingsAreSortedByPathThenStep(), TestNoStepFiresOnTheAdjacentHealthyCondition(), detection (+1 more)

### Community 188 - "MR-003 — findings"
Cohesion: 0.17
Nodes (12): 1. What the milestone does now, 2. Two defects the implementation found in itself, 3. Mutations run, and what each turned red, 4. What an audit should look at first, 5. Audit round 1, MR-003 — findings, The commands could not write, against a database whose permissions were fine, The five HIGH findings (+4 more)

### Community 189 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

### Community 198 - "Audit round 1"
Cohesion: 0.29
Nodes (7): 1. The verdict, 2. The confirmed findings, 4. What was refuted, and why, 5. What this round did not look at, 6. What generalises, Audit round 1, Where the round disagreed with itself about severity

### Community 199 - "D-52 — a supersede node is owned by the record filed at the path its id names"
Cohesion: 0.29
Nodes (7): D-52 — a supersede node is owned by the record filed at the path its id names, Outcome, Review, The history this decision exists to end, The rule, What holds it, Why this is not a fourth patch

### Community 200 - "[x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

### Community 201 - "open_contention_test.go"
Cohesion: 0.40
Nodes (5): runContender(), TestMain(), TestOpenDoesNotWaitForAPermanentFailure(), TestOpenStopsWaitingWhenTheContextIsCancelled(), TestOpenWaitsOutAConcurrentWALConversion()

### Community 202 - "validator_test.go"
Cohesion: 0.07
Nodes (60): NewRegistry(), mustReadEmbedded(), TestEmbeddedSchemaDocumentIsACopy(), TestEmbeddedSchemasPresent(), TestNewRegistryAcceptsExtraDocuments(), TestNewRegistryRejectsIncompleteOrCorruptFS(), TestSupportsRejectsNewerVersion(), TestVersionWindowIsWriteOneReadableOne() (+52 more)

### Community 203 - "[ ] MR-004 — Güvenli lease, idempotency ve optimistic revision"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision, Ne yapılacak

### Community 204 - "RootContext"
Cohesion: 0.40
Nodes (3): main(), context.CancelFunc, RootContext()

### Community 205 - "status/knowledge_findings_test.go"
Cohesion: 0.47
Nodes (5): findingOn(), rowValue(), TestACycleBlocksAndAnInvalidRecordDoesNot(), TestStatusBuildsNoValidatorOfItsOwn(), TestTheFindingsCountIsQualifiedByObservation()

### Community 206 - "Open"
Cohesion: 0.12
Nodes (31): fileSize(), TestAFailedCheckpointIsReportedAsOne(), TestCheckpointGivesUpOnAHeldDatabaseRatherThanWaitingForIt(), TestCheckpointHandsTheConnectionBackAsItFoundIt(), TestCheckpointTruncatesTheLogItWroteBack(), Open(), openTemp(), TestCloseIsIdempotent() (+23 more)

### Community 207 - "[ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme, Ne yapılacak

### Community 208 - "decodeData"
Cohesion: 0.16
Nodes (24): TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestHealthyRepositoryIsUntouchedByRemedyCoherence(), assertNoErrorEnvelope(), decodeData(), openTask(), remedyMentions(), sessionCount(), sessionID() (+16 more)

### Community 209 - "[ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence, Ne yapılacak

### Community 210 - "[ ] MR-017 — Staged değişiklik için yerel Git enforcement"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-017 — Staged değişiklik için yerel Git enforcement, Ne yapılacak

### Community 211 - "TASK-01 dependency + schema.Validator"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-01 dependency + schema.Validator, Tests added

### Community 212 - "TASK-02 internal/knowledge/record"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-02 internal/knowledge/record, Tests added

### Community 213 - "TASK-03 loader RecordRef.Body"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-03 loader RecordRef.Body, Tests added

### Community 214 - "TASK-04+05 app codes + validate.Check"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-04+05 app codes + validate.Check, Tests added

### Community 215 - "TASK-06 bootstrap/doctor/status wiring"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-06 bootstrap/doctor/status wiring, Tests added

### Community 216 - "TASK-07 CLI classifier + agreement matrix"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-07 CLI classifier + agreement matrix, Tests added

### Community 217 - "TASK-08 independent mutation sweep"
Cohesion: 0.29
Nodes (7): AC coverage claimed, Deviations from the frozen requirements, as reported, Files, Mutations the implementer claims to have performed, Risks flagged, TASK-08 independent mutation sweep, Tests added

### Community 224 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 232 - "scope"
Cohesion: 0.25
Nodes (8): scope, sessionResult, github.com/PsyChaos/mindrail/internal/workspace.Workspace, coordinationScope(), coordinationUnavailable(), newSessionCommand(), newSessionOpenCommand(), Options

### Community 233 - "[ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri, Ne yapılacak

### Community 234 - "[ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması, Ne yapılacak

### Community 235 - "[ ] MR-007 — Reconcile-first gerçek değişiklik keşfi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi, Ne yapılacak

### Community 236 - "[ ] MR-008 — Scope drift ve unregistered change ambiguity"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-008 — Scope drift ve unregistered change ambiguity, Ne yapılacak

### Community 237 - "[ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi, Ne yapılacak

### Community 238 - "[ ] MR-011 — Kaynak değişince evidence geçersizleştirme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-011 — Kaynak değişince evidence geçersizleştirme, Ne yapılacak

### Community 239 - "[ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı, Ne yapılacak

### Community 240 - "[ ] MR-014 — MCP bilgi ve bağlam araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-014 — MCP bilgi ve bağlam araçları, Ne yapılacak

### Community 241 - "[ ] MR-015 — MCP koordinasyon ve değişiklik araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-015 — MCP koordinasyon ve değişiklik araçları, Ne yapılacak

### Community 242 - "[ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi, Ne yapılacak

## Knowledge Gaps
- **1115 isolated node(s):** `0.1 Dışında Tutulacak Backlog`, `0.1 Kabul Kriteri İzlenebilirlik Matrisi`, `Bağımlılık Dalgaları`, `Kullanım`, `Remediation brief` (+1110 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1268 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **15 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `refuseUnrepresentableJSON`, `bootstrap.App`, `RootKind`, `adapter.go`, `io.Writer`, `config/loader.go`, `app.go`, `New`, `database/sql.DB`, `healthySubject`, `PayloadOf`, `context.Context`, `DefaultChecks`, `RuntimePathCheck`, `newLoader`, `Build`, `DomainError`, `github.com/spf13/cobra.Command`, `Store`, `Applied`, `newInvocation`, `Migration`, `Root`, `Probe`, `TestBrokenSetupMatrix`, `scope`, `db.go`, `SQLiteCheck`, `Store`, `IsRegistered`, `Config`, `ProbeWriteAccess`, `Root`?**
  _High betweenness centrality (0.039) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `refuseUnrepresentableJSON`, `bootstrap.App`, `WriteIfAbsent`, `adapter.go`, `ResolveRuntimePaths`, `TestRepoConfigDirObstructionDoesNotOverFire`, `newStore`, `io.Writer`, `FreeSpace`, `New`, `NewLoader`, `contract_test.go`, `NewRoot`, `app.go`, `New`, `scaffold_hostilefs_linux_test.go`, `Probe`, `runWith`, `DefaultChecks`, `runInit`, `newLoader`, `Store`, `coordination/store_test.go`, `Applied`, `payloadOf`, `hostilefs_linux_test.go`, `Probe`, `Open`, `TestBrokenSetupMatrix`, `db.go`, `check_test.go`, `NewError`, `Store`, `Check`, `WriteFailure`, `ProbeWriteAccess`?**
  _High betweenness centrality (0.016) - this node is a cross-community bridge._
- **Why does `activeDecision()` connect `TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline` to `testing.T`?**
  _High betweenness centrality (0.010) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **Are the 8 inferred relationships involving `Check()` (e.g. with `newGraph()` and `stepDuplicateActiveLineage()`) actually correct?**
  _`Check()` has 8 INFERRED edges - model-reasoned connections that need verification._
- **What connects `0.1 Dışında Tutulacak Backlog`, `0.1 Kabul Kriteri İzlenebilirlik Matrisi`, `Bağımlılık Dalgaları` to the rest of the system?**
  _1115 weakly-connected nodes found - possible documentation gaps or missing edges._