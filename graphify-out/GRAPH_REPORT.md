# Graph Report - Mindrail  (2026-09-23)

## Corpus Check
- 460 files · ~771,162 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 5927 nodes · 15782 edges · 322 communities (290 shown, 23 thin omitted)
- Extraction: 84% EXTRACTED · 16% INFERRED · 0% AMBIGUOUS · INFERRED: 2450 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `cc7f7b5c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- MINDRAIL PROTOCOL Managed Section
- mindrail-0.1-task-list.md
- mindrail-tech-stack.md
- mindrail-technical-specification-1.0.md
- 129. Acceptance criteria
- 19. Semantic resolver kurulum, lifecycle ve resource policy
- .replay
- newFixture
- MR-002 — findings
- 2. Decisions
- run
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- version.go
- config/loader.go
- MR-011 — Design
- TestBrokenGitFileDetectionDoesNotOverFire
- newInitializedRepo
- New
- New
- status/render_test.go
- 2. Decisions
- checks.go
- KnowledgeCheck
- NewAdapter
- What You Must Do When Invoked
- index/identity_test.go
- 2. Decisions
- RuntimePathCheck
- initReportOf
- newLoader
- Build
- validate.py
- Phase 3 — Doc ↔ Code consistency
- Finding
- status/render.go
- Final report
- properties
- Task Verification Report
- Test Suite Health & Redundancy Audit
- TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline
- RootKind
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- 2. Decisions
- 3. The findings in full
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- 2. Decisions
- runWith
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
- shadowedNames
- decision.v1.schema.json
- scope
- invariant.v1.schema.json
- scope
- Classification — Categories, Severity, Confidence, Fix Decisions
- graphify reference: query, path, explain
- Deletion Gates (Phases 14–17)
- MR-013 — Frozen requirements and acceptance criteria
- registryForTest
- status
- status
- graphify reference: add a URL and watch a folder
- graphify reference: commit hook and native CLAUDE.md integration
- graphify reference: incremental update and cluster-only
- Probe
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
- impacted_tests.py
- 4. The confirmed defects
- corruptState
- report.go
- WriteIfAbsent
- SyntaxSnapshot
- 47. Human approval kimlik ve yetkilendirme modeli
- NamedSession
- MR-005 — Bulgular ve kapı kaydı
- 2. Requirements
- validate_test.go
- Root
- Migrator
- WriteFailure
- time.Duration
- Facts
- NewRootWith
- testguard.go
- FreeSpace
- mindrail-0.1-kernel-scope.md
- pathsAt
- ProbeWriteAccess
- refuseUnrepresentableJSON
- record/errors.go
- NewLoader
- invalidInput
- NewRunner
- storage/access_other.go
- readBySchema
- Probe
- 1. Decisions this document freezes
- NewRoot
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- changesFixtureDB
- MR-004 — design: leases, idempotent mutations and optimistic revision
- TestBrokenSetupMatrix
- gate.go
- MR-003 — design: sequential agent handover
- ParseTime
- MR-002 — audit package
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- newServiceFixture
- App
- 121. Failure modes
- 1. Decisions this document freezes
- MEDIUM
- Config
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- loadStore
- 2. Mindrail'in temel problemi
- hostilefs_linux_test.go
- Open
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- errorStore
- created_at
- indexerFixture
- git/adapter.go
- Check
- .Record
- 2. Decisions
- git/runner.go
- newEvidenceFixture
- 3. IN Scope
- Warning
- agreement_test.go
- Code
- States
- TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore
- MR-004 — Findings and per-task record
- MR-003 — findings
- SnapshotScope
- 103. Test weakening ve false-green guard
- 28. Interactive performance contract
- 159. First Implementation Milestone
- 33. Durable Symbol Identity Implementation
- 56. Resolver Process Supervision
- 12. Content-addressed index cache
- 36. Impact explosion ve drill-down modeli
- 3. Mindrail ne değildir?
- validator_test.go
- newRefreshFixture
- .recordedID
- newFixture
- writable_test.go
- writability_test.go
- audit_scope.py
- schema_ledger_test.go
- newStore
- MR-006 — Design
- smoke_test.go
- runner_test.go
- github.com/tree-sitter/go-tree-sitter.Node
- TASK-01 dependency + schema.Validator
- TASK-02 internal/knowledge/record
- TASK-03 loader RecordRef.Body
- TASK-04+05 app codes + validate.Check
- TASK-06 bootstrap/doctor/status wiring
- TASK-07 CLI classifier + agreement matrix
- TASK-08 independent mutation sweep
- PayloadOf
- MR-008 — Design
- Store
- MR-002 audit package — appendix: implementer self-reports
- lifecycle_lease_test.go
- 5. Test plan
- New
- time.Time
- Migrations
- Invariant
- .resolveIdentitiesTx
- Audit scoping — making audit cost follow risk, not task count
- NewError
- MR-002 — Versioned Decision/Invariant knowledge lifecycle
- DomainError
- fulldisk_linux_test.go
- goldenReport
- MR-006 — Bulgular ve kapı kaydı
- Lease
- NewRegistry
- MR-011 — Frozen requirements and acceptance criteria
- Checkpoint
- .recordedID
- .recordedID
- .recordedID
- arrange
- MR-005 — Design
- NewID
- IsRegistered
- [x] MR-004 — Güvenli lease, idempotency ve optimistic revision
- [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi
- ClassifyRefusal
- Mindrail 0.1 — Uygulama Görev Listesi
- DB
- MR-012 — Design
- healthySubject
- context.Context
- cli/arch_test.go
- columns.go
- [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı
- MR-010 — Design
- 2. Decisions
- Görevler
- [x] MR-001 — Yerel repository bootstrap ve tanılama yolu
- [ ] MR-008 — Scope drift ve unregistered change ambiguity
- migratedIndexSchema
- [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri
- RootContext
- [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi
- exit.go
- .recordedID
- pythonAnalyzer
- Supersede
- newInvocation
- NewService
- MR-007 — Design
- [x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence
- [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü
- io.Writer
- Load
- .withOperation
- check_test.go
- [x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması
- [x] MR-007 — Reconcile-first gerçek değişiklik keşfi
- runStatus
- Check
- Root
- [ ] MR-011 — Kaynak değişince evidence geçersizleştirme
- newImpactFixture
- NewRegistry
- MR-009 — Design
- requireValid
- MR-009 — Bulgular ve kapı kaydı
- migratedChangesSchema
- MR-007 — Bulgular ve kapı kaydı
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- MR-012 — Bulgular ve kapı kaydı
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- newGuardService
- IsUnwritten
- testing.T
- MR-011 — Bulgular ve kapı kaydı
- MR-013 — Design
- TestAPreconditionReadThatFailsIsReportedAsAReadOfThatRow
- driver.go
- database/sql.DB
- NewValidator
- status/knowledge_findings_test.go
- Discover
- github.com/spf13/cobra.Command
- [ ] MR-013 — Yerel completion evidence gate
- NewStore
- adapter_test.go
- ProjectUnit
- runRacer
- scaffold_hostilefs_linux_test.go
- readonlyfs_linux_test.go
- [ ] MR-015 — MCP koordinasyon ve değişiklik araçları
- builder
- readiness
- .insertCheckpoint

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 165 edges
2. `NewError()` - 147 edges
3. `run()` - 106 edges
4. `Open()` - 96 edges
5. `Check()` - 92 edges
6. `newInitializedRepo()` - 83 edges
7. `New()` - 82 edges
8. `shippedValidator()` - 79 edges
9. `storeOf()` - 77 edges
10. `Load()` - 71 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `ExitCode()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/exit.go
- `migratedChangesSchema()` --calls--> `Load()`  [EXTRACTED]
  migrations/changes_constraints_test.go → internal/migration/load.go
- `migratedIndexSchema()` --calls--> `Load()`  [EXTRACTED]
  migrations/index_constraints_test.go → internal/migration/load.go
- `migratedChangesSchema()` --calls--> `New()`  [EXTRACTED]
  migrations/changes_constraints_test.go → internal/migration/migrator.go
- `migratedIndexSchema()` --calls--> `New()`  [EXTRACTED]
  migrations/index_constraints_test.go → internal/migration/migrator.go

## Import Cycles
- None detected.

## Communities (322 total, 23 thin omitted)

### Community 0 - "MINDRAIL PROTOCOL Managed Section"
Cohesion: 0.33
Nodes (6): Graphify Protocol, Managed Section Markers, MINDRAIL PROTOCOL Managed Section, Not Yet Active Status Note, Protocol Call Sequence, Soft Enforcement Note

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

### Community 6 - ".replay"
Cohesion: 0.18
Nodes (6): Operation, recorded, operationConflict(), Lease, Store, ValidOperationID()

### Community 7 - "newFixture"
Cohesion: 0.18
Nodes (25): Task, State, TestAMintedSessionStartsNoLaterThanTheRowItAttributes(), fixture, ids(), newFixture(), newStepClock(), openFixture() (+17 more)

### Community 8 - "MR-002 — findings"
Cohesion: 0.05
Nodes (41): 1. The number that matters, 2. Round 1: what was actually wrong, 3. Round 2: what closing them cost, 4. Status, 5. What generalises to MR-003, A containment gap MR-002 made load-bearing, Appendix A — round 1 findings, as confirmed, Appendix B — what the remediation did (+33 more)

### Community 9 - "2. Decisions"
Cohesion: 0.07
Nodes (30): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-114 — the domain lives in `internal/changes/`, schema version 6 (+22 more)

### Community 10 - "run"
Cohesion: 0.08
Nodes (60): result, TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(), TestEveryUninitialisedSpellingExitsZero(), TestHealthyRepositoryIsUntouchedByRemedyCoherence(), addWorktree(), assertGolden(), assertNoErrorEnvelope() (+52 more)

### Community 11 - "ResolveRuntimePaths"
Cohesion: 0.14
Nodes (29): gitLayout, PathOptions, hasParentSegment(), overrideOrDefault(), requireAbsolute(), ResolveRuntimePaths(), assertMode(), newGitLayout() (+21 more)

### Community 12 - "ADR-0002 — SQLite driver"
Cohesion: 0.33
Nodes (5): ADR-0002 — SQLite driver, Consequences, Context, Decision, Open — benchmark matrix required before this ADR is accepted

### Community 13 - "ADR-0001 — Primary language and CLI stack"
Cohesion: 0.40
Nodes (4): ADR-0001 — Primary language and CLI stack, Consequences, Context, Decision

### Community 15 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 16 - "config/loader.go"
Cohesion: 0.16
Nodes (17): fileConfig, fileOutput, fileProject, fileSecrets, fileValidation, Loader, LoaderOptions, Provenance (+9 more)

### Community 17 - "MR-011 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema, 6. Evaluation flows, 7. Secret inputs (+3 more)

### Community 18 - "TestBrokenGitFileDetectionDoesNotOverFire"
Cohesion: 0.16
Nodes (22): gitFileFault, joinStderr(), linkedWorktreeOfAdminDir(), notARepositoryError(), brokenGitFileFault(), danglingGitFileError(), decorate(), findDanglingGitFile() (+14 more)

### Community 19 - "newInitializedRepo"
Cohesion: 0.08
Nodes (73): child, racedTask, io.WriteCloser, execOnRuntimeDB(), decodeData(), newInitializedRepo(), runtimeDBPath(), TestOneTreeCanBeExecutedTwice() (+65 more)

### Community 20 - "New"
Cohesion: 0.17
Nodes (38): New(), embeddedSet(), fixedClock(), newDB(), openDB(), tableExists(), TestAppliedAtIsUTCRFC3339(), TestChecksumMismatchIsRejected() (+30 more)

### Community 21 - "New"
Cohesion: 0.08
Nodes (60): refusingTransport, stepRecorder, txProbe, net/http.Request, net/http.Response, sync/atomic.Bool, sync/atomic.Int64, New() (+52 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.20
Nodes (20): InitReport, TestKnowledgeFindingsAreCountedFromTheSubject(), assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport() (+12 more)

### Community 23 - "2. Decisions"
Cohesion: 0.10
Nodes (20): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-170 — pure service over before/after bytes, trigger as provenance (+12 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (32): allEscapeRoot(), alsoHeading(), degradedAccount(), degradedImpact(), degradedSummary(), describeBounded(), describeFindings(), describeProblems() (+24 more)

### Community 25 - "KnowledgeCheck"
Cohesion: 0.10
Nodes (53): tail, KnowledgeCheck(), assertActionable(), assertDiagnosable(), Result, TestSchemaUnsupportedRemediesAreActions(), TestUnreadableRecordRemediesNameTheFiles(), accountsIn() (+45 more)

### Community 26 - "NewAdapter"
Cohesion: 0.17
Nodes (53): ErrorPayload, NewAdapter(), assertPayloadIsActionable(), assertRemedyDestinationsAreReal(), changeIntoDestinations(), TestAdapterResolveInsideGitDirectory(), TestUnusableWorkingDirectorySurvivesTheAdapter(), TestWorkingDirectoryDetectionDoesNotOverFire() (+45 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "index/identity_test.go"
Cohesion: 0.18
Nodes (31): allocWorker(), ambiguityRows(), identityCount(), identityFixture(), identityUnit(), migrationFixture(), sourceFile(), TestBackfillRefusesOrphanSymbols() (+23 more)

### Community 29 - "2. Decisions"
Cohesion: 0.10
Nodes (21): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-155 — evidence lives in the runtime database, not in knowledge (+13 more)

### Community 30 - "RuntimePathCheck"
Cohesion: 0.26
Nodes (15): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+7 more)

### Community 31 - "initReportOf"
Cohesion: 0.15
Nodes (17): blockingSummary(), invocation, Options, initReportOf(), initVerdict(), newInitCommand(), runInit(), schemaIsCurrent() (+9 more)

### Community 32 - "newLoader"
Cohesion: 0.07
Nodes (76): operationRecord, encoding/json.RawMessage, io/fs.DirEntry, reflect.Type, testing.B, Session, decisionRef(), decodeObjects() (+68 more)

### Community 33 - "Build"
Cohesion: 0.12
Nodes (42): Subject, findingOn(), TestACycleBlocksAndAnInvalidRecordDoesNot(), TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable() (+34 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "Finding"
Cohesion: 0.09
Nodes (38): Validator, compareNodes(), ids(), newGraph(), facts(), TestAStepsMessageCarriesTheFactsItsRemedyNeeds(), TestEveryStepsMessageIsCovered(), expectedPath() (+30 more)

### Community 37 - "status/render.go"
Cohesion: 0.16
Nodes (18): strings.Builder, Report, Result, State, paint(), writeBlock(), writeResult(), Report (+10 more)

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

### Community 42 - "TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline"
Cohesion: 0.20
Nodes (31): TestAConstructorNamesThePositionOfANilOption(), TestOptionsThatAreNotNilAreStillApplied(), Decision, NewDecision(), NewInvariant(), stampedTime(), invariantProperties(), propertyOf() (+23 more)

### Community 43 - "RootKind"
Cohesion: 0.18
Nodes (17): Probes, knowledgeDirRefusal(), probeRepositoryDir(), TestRepositoryRootIsItsOwnKind(), ensureDir(), unwritableError(), dangling(), root (+9 more)

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

### Community 48 - "2. Decisions"
Cohesion: 0.08
Nodes (26): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-132 — declared scope is the baseline, nothing else (+18 more)

### Community 49 - "3. The findings in full"
Cohesion: 0.06
Nodes (35): 1. The verdict, 2. The confirmed findings, 3. The findings in full, 4. What was refuted, and why, 5. What this round did not look at, 6. What generalises, Audit round 1, F01 (also F41) — MEDIUM. After upgrading, a registered worktree is told it is not registered (+27 more)

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

### Community 58 - "2. Decisions"
Cohesion: 0.10
Nodes (20): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-146 — the graph is designed with the D-90 constraint, not around it (+12 more)

### Community 59 - "runWith"
Cohesion: 0.11
Nodes (34): answer, brokenSetup, hostileCondition, agreeingHostileCommands(), assertTheLoopTerminates(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo() (+26 more)

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
Cohesion: 0.17
Nodes (11): Autonomous Engineering Orchestrator, Completion bar, Evidence standard, How the audit scales, Lifecycle, Orchestrator rules, Step 1 — Right-size the ceremony, Step 2 — Adapt to the environment (+3 more)

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
Cohesion: 0.17
Nodes (60): TestOnlyAByteIdenticalFileNameOwnsAnId(), TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(), TestADecisionAndAnInvariantNeverShareAGraphNode(), TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle(), TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages(), TestAnIdNoFileIsNamedAfterSuppliesNoEdges(), TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries(), TestAValidStoreIsStillSilentWithAStrayDraftBesideIt() (+52 more)

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

### Community 75 - "shadowedNames"
Cohesion: 0.12
Nodes (28): github.com/tree-sitter/go-tree-sitter.Query, Import, adapter, textField(), unquoteModule(), bindingExtent(), bindingNames(), bindingsContain() (+20 more)

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

### Community 83 - "MR-013 — Frozen requirements and acceptance criteria"
Cohesion: 0.12
Nodes (16): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-179 — the gate composes values; reconcile-first is composition (+8 more)

### Community 84 - "registryForTest"
Cohesion: 0.08
Nodes (40): unsafe.Pointer, LocalKey(), extractForTest(), TestAnonymousAndLocalBindingsDoNotInventTargets(), TestBareMethodCallUsesLexicalScopeAndAmbiguityStaysUnresolved(), TestCompletePackageExtraction(), TestExtractionFailureAndCancellationCloseSnapshots(), TestExtractPreservesPartialFactsAndCancellation() (+32 more)

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

### Community 90 - "Probe"
Cohesion: 0.17
Nodes (17): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject, subjectAt() (+9 more)

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

### Community 107 - "Component"
Cohesion: 0.16
Nodes (15): Components(), ComponentName, Readiness, intPtr(), Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown() (+7 more)

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "impacted_tests.py"
Cohesion: 0.22
Nodes (20): changed_files(), git(), is_e2e(), is_test(), js_imports(), load_coverage_contexts(), load_explicit_map(), main() (+12 more)

### Community 110 - "4. The confirmed defects"
Cohesion: 0.08
Nodes (26): 1. The verdict, 2. The base rate, which is why this round exists, 3. The three deviations, graded, 4.10 — LOW. §4's corrected F17 bullet credits the fix to a test that does not contain it, 4.11 — LOW. The F48 figure attaches the 1,000-task measurement to 20,000 tasks, 4.12 — LOW. A minted session now starts after the row it attributes, 4.13 — LOW. The `EXISTS` rewrite degrades to a full checkpoint scan for a project with no checkpoints, 4.14 — LOW. Item 11 is recorded as "(test only)" but its commit changes production behaviour (+18 more)

### Community 111 - "corruptState"
Cohesion: 0.09
Nodes (22): LeaseView, Provenance, SymbolAttribution, TaskAttribution, Service, Store, Finding, Change (+14 more)

### Community 112 - "report.go"
Cohesion: 0.27
Nodes (13): coordinationInfo(), Report, observationOf(), repositoryObservation(), stateRank(), stoppedAtStep(), worst(), CoordinationInfo (+5 more)

### Community 113 - "WriteIfAbsent"
Cohesion: 0.13
Nodes (26): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission() (+18 more)

### Community 114 - "SyntaxSnapshot"
Cohesion: 0.10
Nodes (14): github.com/tree-sitter/go-tree-sitter.Language, github.com/tree-sitter/go-tree-sitter.Tree, sync.RWMutex, timedAdapter, Import, adapter, Reference, Symbol (+6 more)

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "NamedSession"
Cohesion: 0.25
Nodes (24): settableClock, leaseFixture(), mustFile(), newSettableClock(), requireCode(), TestAcquiringOverAnExpiredTenureClosesItAndSaysSo(), TestAnAcquisitionByTheHolderRenewsUnderTheSameID(), TestARemedyNamingAKeyIsACommandLineThatRuns() (+16 more)

### Community 117 - "MR-005 — Bulgular ve kapı kaydı"
Cohesion: 0.05
Nodes (38): AC evidence and RED/GREEN, Başlangıç ve çalışma ağacı, D-89 — referans hedefi silinince `ON DELETE SET NULL`, D-90 — cross-file çözümleme dosya-kapsamlıdır, birim-kapsamlı değil, D-91 — TASK-06 scheduler'ı sürmez, readiness'i bağlar, D-92 — süreç-ölçeği kanıtı yeniden-açılan handle'lardır, Deliberate guard mutations (all restored), Denetim artefaktları (+30 more)

### Community 118 - "2. Requirements"
Cohesion: 0.06
Nodes (36): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation, D-43 — a Finding gets its own codes, because D-38 says it is not a Problem (+28 more)

### Community 119 - "validate_test.go"
Cohesion: 0.15
Nodes (20): RecordKind, RecordRef, TestFindingsAreSortedByPathThenStep(), suppressions(), TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided(), TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded(), TestStepSevenIsExemptFromTheSuppression(), TestSuppressionKeysOnTheFileTheReferencedIdWouldOccupy() (+12 more)

### Community 120 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 121 - "Migrator"
Cohesion: 0.14
Nodes (14): createdObjects(), Migration, SchemaObject, schemaEffects(), checksumFailure(), Applied, Migrator, highestVersion() (+6 more)

### Community 122 - "WriteFailure"
Cohesion: 0.23
Nodes (19): TestMigrationThreeLeavesTheObjectsTheDesignNames(), TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget(), IsBusy(), IsDiskFull(), IsReadOnly(), assertRetryablePayload(), contendedWriteError(), openForTest() (+11 more)

### Community 123 - "time.Duration"
Cohesion: 0.17
Nodes (19): time.Duration, genuineBusyError(), TestABusyOpenIsNamedOnce(), TestAnOpenRefusedForTheWholeBudgetIsRetryable(), TestAnOpenThatIsNotContendedIsNotRewritten(), TestTheLadderStepsByTheSpecsExample(), TestWaitBusyReturnsSuccessAndPermanentFailuresAtOnce(), TestWaitBusySleepsByTheLadderAndStopsAtTheBudget() (+11 more)

### Community 124 - "Facts"
Cohesion: 0.23
Nodes (17): Facts, Range, decodeExact(), Cache, Computer, readEntry(), sha256Hex(), validFacts() (+9 more)

### Community 125 - "NewRootWith"
Cohesion: 0.15
Nodes (13): classifiedArgs, Root, github.com/spf13/cobra.Completion, github.com/spf13/cobra.PositionalArgs, github.com/spf13/cobra.ShellCompDirective, Execute(), helpTopics(), NewRoot() (+5 more)

### Community 126 - "testguard.go"
Cohesion: 0.20
Nodes (19): addedMarkers(), blocks(), code(), FileDelta, Finding, Request, Result, Service (+11 more)

### Community 127 - "FreeSpace"
Cohesion: 0.13
Nodes (16): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+8 more)

### Community 128 - "mindrail-0.1-kernel-scope.md"
Cohesion: 0.12
Nodes (14): 1. Goal, 2. Primary End-to-End Scenario, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition, Mindrail 0.1 — Kernel Scope (+6 more)

### Community 129 - "pathsAt"
Cohesion: 0.21
Nodes (21): TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions(), TestAnInvariantCycleIsFatalAndNamesOnlyInvariants(), TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord(), TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge(), TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose(), detections(), overFires(), TestAKindThatDisagreesWithItsDirectoryIsReportedByStepFive() (+13 more)

### Community 130 - "ProbeWriteAccess"
Cohesion: 0.19
Nodes (15): TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError(), conditionBlocker(), WriteAccess, WriteBlocker (+7 more)

### Community 131 - "refuseUnrepresentableJSON"
Cohesion: 0.24
Nodes (11): offender, reflect.StructField, reflect.Value, firstUnrepresentable(), memberName(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo() (+3 more)

### Community 132 - "record/errors.go"
Cohesion: 0.20
Nodes (3): DuplicateItemError, EmptyFieldError, OptionKindError

### Community 133 - "NewLoader"
Cohesion: 0.27
Nodes (15): NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestLoadWithNoFilesUsesDefaults(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers(), TestProvenanceReportsSourceLayer(), TestStrictDecodeRejectsUnknownKey() (+7 more)

### Community 134 - "invalidInput"
Cohesion: 0.09
Nodes (28): BaselineSummary, DivergenceEntry, FileChange, ReconcileResult, SymbolChange, contentHash(), entryKind(), excludedPath() (+20 more)

### Community 135 - "NewRunner"
Cohesion: 0.24
Nodes (12): Runner, invalidInput(), NewRunner(), newTestRunner(), TestHelperProcess(), TestRunnerBoundsOutput(), TestRunnerRefusesBeforeSpawn(), TestRunnerRefusesUnresolvableBinary() (+4 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "readBySchema"
Cohesion: 0.19
Nodes (19): createdAtDescription(), everyDescriptionClaim(), everyRefusedSpelling(), measuredGround(), TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(), TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(), TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(), TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository() (+11 more)

### Community 140 - "Probe"
Cohesion: 0.17
Nodes (20): io/fs.FileInfo, io/fs.FileMode, absentFailure(), classify(), describeMode(), Presence, nonDirectoryAncestor(), Probe() (+12 more)

### Community 141 - "1. Decisions this document freezes"
Cohesion: 0.06
Nodes (35): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-64 — one `leases` table for every target kind, one row per tenure, inside `internal/coordination`, D-65 — active means unreleased and unexpired by the store's clock, judged in Go, and expiry is lazy (+27 more)

### Community 142 - "NewRoot"
Cohesion: 0.19
Nodes (16): TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize(), newTestRoot() (+8 more)

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "changesFixtureDB"
Cohesion: 0.20
Nodes (26): changesFixture(), changesFixtureDB(), seedTaskChain(), shaOf(), TestBaselineClearingDiscipline(), TestCaptureBaselineEmptyHashForMissingAndNonRegular(), TestCaptureBaselineRefusesUnknownTask(), TestCaptureBaselineReplacesWithoutStacking() (+18 more)

### Community 145 - "MR-004 — design: leases, idempotent mutations and optimistic revision"
Cohesion: 0.13
Nodes (15): 10. Command surface, 11. Status integration, 12. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000003_lease_idempotency.sql` (+7 more)

### Community 146 - "TestBrokenSetupMatrix"
Cohesion: 0.13
Nodes (34): envelopeScenario, strictEnvelope, newNonRepositoryDir(), TestAReaderStaysCoherentWhileTheStoreIsRewritten(), TestTwoCommandsReadOneStoreAtTheSameTime(), withoutTimings(), writeInvariantRecord(), configPath() (+26 more)

### Community 147 - "gate.go"
Cohesion: 0.19
Nodes (24): Ambiguity, Denial, InvariantBlock, ambiguityDenials(), attributionDenials(), dedupe(), evidenceDenials(), Decision (+16 more)

### Community 148 - "MR-003 — design: sequential agent handover"
Cohesion: 0.17
Nodes (12): 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000002_coordination.sql`, 6. The state table, 7. Command surface, 8. Status integration (+4 more)

### Community 149 - "ParseTime"
Cohesion: 0.14
Nodes (24): handoverResult, rowScanner, ParseTime(), TestFormatTimeIsUTCRFC3339(), readFailed(), Checkpoint, CheckpointRef, Handover (+16 more)

### Community 150 - "MR-002 — audit package"
Cohesion: 0.15
Nodes (12): 10. The independent mutation sweep, and what it found, 11. What the audit is asked to establish, 1. Original request, verbatim, 2. The task, 3. Requirements and acceptance criteria, 4. Design contract, 5. Implementation summary, 6. Change references (+4 more)

### Community 151 - "29. Large repository cold-index lifecycle"
Cohesion: 0.22
Nodes (9): 29. Large repository cold-index lifecycle, Cooperative scheduler, Foreground ve local worker, Kesilme, Priority aging, Progress, ProjectUnit reservation, Resumability (+1 more)

### Community 152 - "11. SQLite concurrency ve interactive write fairness"
Cohesion: 0.25
Nodes (8): 11. SQLite concurrency ve interactive write fairness, Cold-index write fairness, Idempotency key, Immutable/content-addressed rows, Neden global write broker yok?, Optimistic revision, Process-local writer scheduling, WAL + bounded busy retry

### Community 153 - "newServiceFixture"
Cohesion: 0.09
Nodes (47): serviceFixture, RenameEntry, attributionSetup(), taskChangeID(), TestAttributeTaskDriftFiresPerFile(), TestAttributeTaskNeverInventsPaths(), TestAttributeTaskSingleCandidateAttributes(), TestAttributeTaskTwoCandidatesIsAmbiguous() (+39 more)

### Community 154 - "App"
Cohesion: 0.07
Nodes (23): InitResult, Recorder, scope, io/fs.FS, log/slog.Logger, AdoptCause(), asKnowledgeError(), asMigrationError() (+15 more)

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "1. Decisions this document freezes"
Cohesion: 0.07
Nodes (29): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's four acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-53 — the coordination tables are a second migration, and the reported runtime schema version becomes 2, D-54 — "Checkpoint" means two things in this repository, and neither is renamed (+21 more)

### Community 157 - "MEDIUM"
Cohesion: 0.08
Nodes (25): F01 — `mindrail init` reported READY over a repository the next command refused, F02 — an unopenable `config.toml` was remedied as a malformed one, F03 — a size limit was reported as a full disk, F04, F08 — a rejected command line exited 1 and wrote no envelope, F05 — the agreement matrix never read the remedy it claimed to carry out, F06 — no matrix row for a `config.toml` that cannot be opened, F07 — the linked-worktree cross-check compared two fields of four, F09 — the read-only-media sentinel had no row in the classifier's table (+17 more)

### Community 158 - "Config"
Cohesion: 0.17
Nodes (20): OutputConfig, ProjectConfig, RuntimeConfig, SecretsConfig, Defaults(), Config, ValidationProfile, quote() (+12 more)

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "loadStore"
Cohesion: 0.44
Nodes (7): TestAtMostOneRecordCanBeCanonicalForAnId(), TestOwnershipFollowsTheNameTheDirectoryHolds(), assertSameRefs(), loadStore(), TestTheFixtureStoreIsTheOneTheLoaderProduces(), TestTheRecordsTheLoaderIgnoresAreTheOnesTheValidatorFinds(), writeKnowledgeRecord()

### Community 163 - "2. Mindrail'in temel problemi"
Cohesion: 0.40
Nodes (5): 2.1 Context kaybı, 2.2 Semantic conflict, 2.3 Intent kaybı, 2.4 Kanıtsız tamamlanma, 2. Mindrail'in temel problemi

### Community 164 - "hostilefs_linux_test.go"
Cohesion: 0.25
Nodes (14): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+6 more)

### Community 165 - "Open"
Cohesion: 0.10
Nodes (37): Open(), openTemp(), TestCloseIsIdempotent(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection(), TestOpenCreatesWALSidecar() (+29 more)

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

### Community 172 - "errorStore"
Cohesion: 0.14
Nodes (21): database/sql/driver.Conn, database/sql/driver.Driver, database/sql/driver.NamedValue, database/sql/driver.Result, database/sql/driver.Rows, database/sql/driver.Tx, database/sql/driver.TxOptions, database/sql/driver.Value (+13 more)

### Community 173 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

### Community 174 - "indexerFixture"
Cohesion: 0.14
Nodes (37): os.FileInfo, IndexResult, sourceVersion, Timing, contentHash(), Indexer, Store, NewIndexer() (+29 more)

### Community 175 - "git/adapter.go"
Cohesion: 0.12
Nodes (28): Adapter, standpoint, worktreeLinkage, bareRepositoryError(), detachedWorktreeError(), firstLine(), foreignRelinkCaveat(), foreignWorktreeError() (+20 more)

### Community 176 - "Check"
Cohesion: 0.10
Nodes (32): Check, diagnosis, halt, step, Runner, ConfigCheck(), configResult(), describePath() (+24 more)

### Community 177 - ".Record"
Cohesion: 0.27
Nodes (9): corruptState(), Result, Evidence, Store, NewStore(), operationConflict(), requestHash(), schemaBehind() (+1 more)

### Community 178 - "2. Decisions"
Cohesion: 0.06
Nodes (32): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-100 — backfill mints, never migrates (+24 more)

### Community 179 - "git/runner.go"
Cohesion: 0.13
Nodes (14): ExecRunner, os/exec.Cmd, stderrSummary(), TestAdapterPropagatesRunnerFailures(), directoryEntryFault(), hasAnyPrefix(), SanitizedEnv(), timeoutError() (+6 more)

### Community 180 - "newEvidenceFixture"
Cohesion: 0.23
Nodes (12): NewRedactor(), TestRedactDefaultPatterns(), TestRedactExactValues(), validSecretName(), newEvidenceFixture(), TestEvidenceSchemaVersionGate(), TestOperationIDReplay(), TestRecordBindsEvidenceRow() (+4 more)

### Community 181 - "3. IN Scope"
Cohesion: 0.12
Nodes (16): 3. IN Scope, Basic impact, Change discovery, Completion gate, Coordination, Core executable, Diagnostics, Engineering knowledge (+8 more)

### Community 182 - "Warning"
Cohesion: 0.21
Nodes (13): Envelope, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice(), regularFile() (+5 more)

### Community 183 - "agreement_test.go"
Cohesion: 0.12
Nodes (41): condition, remedy, remedyClass, agreeingCommands(), agreementConditions(), agreementRemedyClass(), assertRemedyIsWhatTheRowExpects(), chmodForRemedy() (+33 more)

### Community 184 - "Code"
Cohesion: 0.15
Nodes (16): CheckFunc, Code, indexCodes(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+8 more)

### Community 185 - "States"
Cohesion: 0.24
Nodes (12): TestTheTableIsUnchangedByTheLease(), CanTransition(), State, ParseState(), States(), TestARefusalCanSayWhatIsAvailableInstead(), TestATaskNeverMovesToTheStateItIsAlreadyIn(), TestEveryOrderedPairOfStatesGetsTheAnswerTheSpecificationGives() (+4 more)

### Community 186 - "TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore"
Cohesion: 0.25
Nodes (13): TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone(), combine(), holdsTwoReportableSelfLoops(), oracleCycleMembers(), recordsFor(), renderStore(), storesOfOneKind(), TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore() (+5 more)

### Community 187 - "MR-004 — Findings and per-task record"
Cohesion: 0.04
Nodes (46): 1. TASK-01 — the bounded wait, named and measured, 2. TASK-02 — migration 000003, and a ledger that can read `ADD COLUMN`, 3. TASK-03 — the lease: one row per tenure, and the three verbs over a file, 4. TASK-04 — the lease and the revision in front of every task move, 5. TASK-05 — a repeated operation is answered from its record, 6. TASK-06 — the command surface, 7. TASK-07 — `status` publishes the leases held right now, 8. TASK-08 — two processes, eight sessions, and the proof under load (+38 more)

### Community 188 - "MR-003 — findings"
Cohesion: 0.10
Nodes (21): 1. What the milestone does now, 2. Two defects the implementation found in itself, 3. Mutations run, and what each turned red, 4. What an audit should look at first, 5. Audit round 1, 6. The round-1 remediation, 7. The round-2 remediation, MR-003 — findings (+13 more)

### Community 189 - "SnapshotScope"
Cohesion: 0.31
Nodes (6): TestRunProfileFlowEndToEnd(), hashFiles(), SnapshotScope(), TestSnapshotScopeDeterministic(), TestSnapshotScopeRefusesBeforeExecution(), Snapshot

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

### Community 198 - "validator_test.go"
Cohesion: 0.21
Nodes (23): mustReadEmbedded(), embeddedValidator(), located(), registryFrom(), TestBothDocumentsStateTheSameCreatedAtRule(), TestSchemaKindsAreSpeltTheSameWayAsTheLoaders(), TestValidatorAcceptsAnExtraSchemaDocumentAlongsideV1(), TestValidatorAcceptsARecordThatSatisfiesItsSchema() (+15 more)

### Community 199 - "newRefreshFixture"
Cohesion: 0.30
Nodes (18): activeInvariant(), newRefreshFixture(), symbolPackageDir(), TestAmbiguousTwinsBlockBySeverity(), TestEndToEndProtectRenameAmbiguousDelete(), TestOrphanBlocksOnCritical(), TestOutsideUnitTargetIsOrphaned(), TestRefreshBindingsWritesBoundRows() (+10 more)

### Community 201 - "newFixture"
Cohesion: 0.28
Nodes (21): mkunit(), newFixture(), seedPending(), TestEnqueueP0PromotesQueuedPath(), TestEnqueueRejectsRelativeAndUncleanPaths(), TestEvictedPathsRefillAfterPrioritize(), TestFillColdOrdersByPathAndReportsMore(), TestPrioritizeEnqueuesUnqueuedUnitFiles() (+13 more)

### Community 202 - "writable_test.go"
Cohesion: 0.31
Nodes (18): assertProbeMatchesSQLite(), assertUnwritablePayload(), chmodForTest(), dirEntryNames(), initialisedDatabase(), requireModeBitsAreEnforced(), TestProbeWriteAccessAgreesWithSQLite(), TestProbeWriteAccessAgreesWithSQLiteAcrossTheSidecarMatrix() (+10 more)

### Community 203 - "writability_test.go"
Cohesion: 0.30
Nodes (13): chmodForTest(), assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), checkNamed(), newRepoUnderAnUnrepresentablePath(), TestAHealthyDatabaseReportsThatItsWritabilityWasChecked(), TestANonUTF8RepositoryPathIsRefusedRatherThanMangled() (+5 more)

### Community 204 - "audit_scope.py"
Cohesion: 0.32
Nodes (12): assign_cluster(), build_plan(), changed_files(), classify(), diff_lines(), git(), load_tasks(), main() (+4 more)

### Community 205 - "schema_ledger_test.go"
Cohesion: 0.32
Nodes (13): assertRemedyIsNotTheFailingCommand(), assertUserFacing(), assertUserFacingCode(), holdWriteLock(), mustFailUp(), openContendedDB(), TestAGenuineMigrationFailureIsStillAMigrationFailure(), TestAnUnreadableLedgerIsNotReportedInTheDriversWords() (+5 more)

### Community 206 - "newStore"
Cohesion: 0.32
Nodes (14): newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace(), TestRegisteredIDsAreOpaqueAndPrefixed(), TestRegisterIsIdempotent() (+6 more)

### Community 207 - "MR-006 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000005_symbol_identity.sql`, 6. Allocation, 7. Rename/move matching (+3 more)

### Community 208 - "smoke_test.go"
Cohesion: 0.23
Nodes (17): assertSQLiteCheckHealthy(), shellQuote(), TestGracefulShutdownOnSIGINT(), writeSlowGit(), addWorktree(), buildBinary(), decodeEnvelope(), entriesIn() (+9 more)

### Community 209 - "runner_test.go"
Cohesion: 0.24
Nodes (13): countEnv(), helperArgv(), helperRunner(), lookupEnv(), processAlive(), readPID(), TestDefaultTimeoutIsBounded(), TestExecRunnerCancellationKillsChild() (+5 more)

### Community 210 - "github.com/tree-sitter/go-tree-sitter.Node"
Cohesion: 0.22
Nodes (18): github.com/tree-sitter/go-tree-sitter.Node, callbackArg(), enclosesNode(), hasMarker(), insideAny(), isTrivialCallback(), jsUnescape(), newEcmaAnalyzer() (+10 more)

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

### Community 218 - "PayloadOf"
Cohesion: 0.17
Nodes (21): PayloadOf(), ExitCode(), TestProbeWritableLeavesAHealthyRepositoryAlone(), TestUnwritablePathPrescribesTheRemedyThatClearsEachCondition(), TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed(), TestAdapterResolveBareRepository(), TestAdapterResolveNotARepository(), exit128() (+13 more)

### Community 219 - "MR-008 — Design"
Cohesion: 0.07
Nodes (26): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000007_scope_attribution.sql`, 6. Evaluation flows, 7. Lease guidance input (+18 more)

### Community 220 - "Store"
Cohesion: 0.33
Nodes (8): Registration, Store, Workspace, NewStore(), scanWorkspace(), upsertWorkspace(), validateRegistration(), rowScanner

### Community 222 - "lifecycle_lease_test.go"
Cohesion: 0.32
Nodes (12): fixture, readLeaseRow(), TaskTarget(), activeTaskLease(), assertInvariant(), fixture, TestAHoldersCheckpointRenewsAndAHandoffReleases(), TestAMoveOnAnotherSessionsHeldTaskIsRefusedForEveryDestination() (+4 more)

### Community 223 - "5. Test plan"
Cohesion: 0.04
Nodes (47): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+39 more)

### Community 224 - "New"
Cohesion: 0.26
Nodes (21): RuntimePaths, New(), cachePaths(), entryFiles(), fixtureFacts(), pythonInfo(), runFIFOProbe(), TestCanceledContextBypassesDiskAndCompute() (+13 more)

### Community 225 - "time.Time"
Cohesion: 0.07
Nodes (48): Acquisition, FixedClock, SystemClock, TargetKind, database/sql.Tx, time.Time, Ambiguity, Binding (+40 more)

### Community 228 - "Invariant"
Cohesion: 0.42
Nodes (5): blocks(), Service, Invariant, Outcome, Report

### Community 229 - ".resolveIdentitiesTx"
Cohesion: 0.25
Nodes (11): ancestorRow, stagedKey, RenameHint, Store, hintMovesFor(), localPart(), matchBar(), orderParentsFirst() (+3 more)

### Community 230 - "Audit scoping — making audit cost follow risk, not task count"
Cohesion: 0.20
Nodes (9): Audit scoping — making audit cost follow risk, not task count, Rule 1 — One audit per delivery, never one per task, Rule 2 — Depth is set per cluster by risk, Rule 3 — Mutation runs targeted tests, the full suite runs once, Rule 4 — Checkpoint audits hide audit time behind implementation time, Rule 5 — Re-audits are delta-only, The audit plan artifact, The problem this solves (+1 more)

### Community 231 - "NewError"
Cohesion: 0.13
Nodes (24): CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain(), TestDomainErrorCarriesFiveFields() (+16 more)

### Community 232 - "MR-002 — Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.22
Nodes (9): 0. What MR-001 left here, exactly, 1. Scope decision: no CLI write surface, 2. Package plan, 3. Frozen signatures, 4. Decisions, 5. Test plan, 6. Explicit non-goals, 7. Acceptance criteria traceability (+1 more)

### Community 233 - "DomainError"
Cohesion: 0.23
Nodes (12): DomainError, bookkeepingCreateFailure(), busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), MainDatabaseFile(), mainDatabaseFile() (+4 more)

### Community 234 - "fulldisk_linux_test.go"
Cohesion: 0.22
Nodes (16): afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero(), fullDiskChild(), fullDiskScenario(), initialiseOnDisk() (+8 more)

### Community 235 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 236 - "MR-006 — Bulgular ve kapı kaydı"
Cohesion: 0.05
Nodes (41): Başlangıç ve çalışma ağacı, D-112 — eşleştirme transaction içinde Store metodudur (D-93 arıtması), D-113 — ata kümesi dosya+ihattur, global tarama yok, MR-006 — Bulgular ve kapı kaydı, Reader / Breaker bulguları ve giderim, Reader / Breaker bulguları ve giderim — TASK-01 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı (+33 more)

### Community 237 - "Lease"
Cohesion: 0.27
Nodes (8): LeaseStatus, FileTarget(), fileTargetInvalid(), Acquisition, Lease, Task, isASCIILetter(), sameRow()

### Community 238 - "NewRegistry"
Cohesion: 0.16
Nodes (9): Registry, NewRegistry(), TestEmbeddedSchemaDocumentIsACopy(), TestEmbeddedSchemasPresent(), TestNewRegistryAcceptsExtraDocuments(), TestNewRegistryRejectsIncompleteOrCorruptFS(), TestSupportsRejectsNewerVersion(), TestVersionWindowIsWriteOneReadableOne() (+1 more)

### Community 239 - "MR-011 — Frozen requirements and acceptance criteria"
Cohesion: 0.13
Nodes (15): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-165 — staleness is computed, never stored (+7 more)

### Community 240 - "Checkpoint"
Cohesion: 0.27
Nodes (11): database/sql.Conn, boundCheckpointWait(), Checkpoint(), checkpointFailure(), checkpointOutcome(), fileSize(), TestAFailedCheckpointIsReportedAsOne(), TestCheckpointGivesUpOnAHeldDatabaseRatherThanWaitingForIt() (+3 more)

### Community 244 - "arrange"
Cohesion: 0.32
Nodes (12): idempotentWriter, Session, arrange(), countRows(), fixture, TestAnOperationIDMustBeAnIdentifier(), TestARecordThatNamesNoEntityIsDamageNotAFreshMint(), TestARefusedOperationRecordsNothing() (+4 more)

### Community 245 - "MR-005 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000004_index.sql`, 6. Parsing rules, 7. Incremental reindex and resume (+3 more)

### Community 246 - "NewID"
Cohesion: 0.27
Nodes (9): encodeCrockford(), compareRandomness(), TestABackwardsClockStillMintsAscendingIds(), TestTheEntropyCounterCarriesAcrossAByte(), NewID(), nextEntropy(), TestAnIdIsPrefixedUniqueAndSortsInMintOrder(), TestAnIdLeaksNeitherTheClockNorTheCaller() (+1 more)

### Community 247 - "IsRegistered"
Cohesion: 0.15
Nodes (21): coordinationRefusal, semanticDamageRow, IsRegistered(), TestTheKnowledgeCodesCarryTheSpellingConsumersBranchOn(), assertFourErrorKeys(), isEmptyValue(), assertPublishedFailureHasACode(), commandName() (+13 more)

### Community 248 - "[x] MR-004 — Güvenli lease, idempotency ve optimistic revision"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-004 — Güvenli lease, idempotency ve optimistic revision

### Community 249 - "[ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi, Ne yapılacak

### Community 250 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 251 - "Mindrail 0.1 — Uygulama Görev Listesi"
Cohesion: 0.40
Nodes (5): 0.1 Dışında Tutulacak Backlog, 0.1 Kabul Kriteri İzlenebilirlik Matrisi, Bağımlılık Dalgaları, Kullanım, Mindrail 0.1 — Uygulama Görev Listesi

### Community 252 - "DB"
Cohesion: 0.17
Nodes (18): queryPlan(), schemaOnlyDatabase(), TestTheNewestCheckpointQueryDoesNotSortTheProject(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure(), DB (+10 more)

### Community 253 - "MR-012 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema, 6. Analysis flows, 7. Secret inputs (+3 more)

### Community 254 - "healthySubject"
Cohesion: 0.14
Nodes (36): App, Subject, NewRunner(), TestRunnerRespectsContextCancellation(), Verdict(), DefaultChecks(), Subject, haltedSubject() (+28 more)

### Community 255 - "context.Context"
Cohesion: 0.07
Nodes (45): context.Context, database/sql.NullString, FileFacts, FileIndexState, fileScanner, FileStateObservation, fileStateToken, Identity (+37 more)

### Community 256 - "cli/arch_test.go"
Cohesion: 0.31
Nodes (15): firstPartyPackage, assertMainDeclaresOnlyMain(), assertNoNetworkInLinkedPackages(), fileImports(), firstPartyPackages(), goSources(), isStandardLibrary(), matchForbidden() (+7 more)

### Community 257 - "columns.go"
Cohesion: 0.42
Nodes (9): addedColumn(), alterations(), columnList(), columnName(), firstToken(), nextToken(), stripComments(), tableColumns() (+1 more)

### Community 258 - "[ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı, Ne yapılacak

### Community 259 - "MR-010 — Design"
Cohesion: 0.08
Nodes (22): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000008_evidence.sql`, 6. Execution flows, 7. Secret inputs (+14 more)

### Community 260 - "2. Decisions"
Cohesion: 0.08
Nodes (24): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-80 — the index lands in `internal/index/`, and the deviation from §7's tree is named once (+16 more)

### Community 261 - "Görevler"
Cohesion: 0.20
Nodes (10): Görevler, Kabul kriterleri, Kabul kriterleri, Kabul kriterleri, [ ] MR-014 — MCP bilgi ve bağlam araçları, [ ] MR-016 — MCP validation ve completion araçları, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak (+2 more)

### Community 262 - "[x] MR-001 — Yerel repository bootstrap ve tanılama yolu"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-001 — Yerel repository bootstrap ve tanılama yolu

### Community 263 - "[ ] MR-008 — Scope drift ve unregistered change ambiguity"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-008 — Scope drift ve unregistered change ambiguity, Ne yapılacak

### Community 264 - "migratedIndexSchema"
Cohesion: 0.49
Nodes (9): migratedIndexSchema(), mustIndexSQL(), seedIndexConstraintRows(), TestIdentityAllocationKeyIsUniquePerProjectUnitLanguageKey(), TestIdentityBindingStatusAndSymbolUidAreConstrained(), TestIdentityContainerUidRejectsDanglingParent(), TestIndexStateCheckRejectsUnknownValue(), TestIndexTablesDeclarePrimaryKeys() (+1 more)

### Community 265 - "[x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri

### Community 266 - "RootContext"
Cohesion: 0.40
Nodes (3): main(), context.CancelFunc, RootContext()

### Community 268 - "[ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi, Ne yapılacak

### Community 269 - "exit.go"
Cohesion: 0.21
Nodes (11): Error, Denied(), exitCodeForKind(), Failed(), Kind, TestErrorMessageUsesCause(), TestErrorUnwrapsToCause(), TestExitCode() (+3 more)

### Community 271 - "pythonAnalyzer"
Cohesion: 0.23
Nodes (9): github.com/tree-sitter/go-tree-sitter.Parser, allowedAbove(), encloses(), isTestName(), isTrivialBody(), newPythonAnalyzer(), normalizeDecorator(), pythonAnalyzer (+1 more)

### Community 272 - "Supersede"
Cohesion: 0.23
Nodes (12): TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(), TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(), Decision, Decision, Supersede(), activeDecision(), TestSupersedeDoesNotRepeatATargetItAlreadyCarries(), TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout() (+4 more)

### Community 273 - "newInvocation"
Cohesion: 0.21
Nodes (11): globalFlags, humanRenderer, Options, newDoctorCommand(), runDoctor(), globalFlagsOf(), invocation, Options (+3 more)

### Community 274 - "NewService"
Cohesion: 0.36
Nodes (5): TestFreshnessLifecycleEndToEnd(), Store, Service, NewService(), Runner

### Community 275 - "MR-007 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000006_changes.sql`, 6. Discovery flows, 7. Symbol delta core (+3 more)

### Community 276 - "[x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme

### Community 277 - "[ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence, Ne yapılacak

### Community 278 - "[x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

### Community 279 - "io.Writer"
Cohesion: 0.14
Nodes (10): attributed, checkpointResult, leaseListResult, leaseResult, sessionResult, taskListResult, taskResult, io.Writer (+2 more)

### Community 280 - "Load"
Cohesion: 0.10
Nodes (33): testing/fstest.MapFS, TestChangesSchemaVersionNamesItsCreatingMigration(), TestIndexSchemaVersionNamesItsCreatingMigration(), TestStoreDistinguishesNeverInitializedFromMissingLedger(), TestStoreGatesHealthyOlderSchemaBeforeAnyIndexOperation(), Load(), keys(), TestEachMigrationCreatesOnlyItsMilestonesTables() (+25 more)

### Community 281 - ".withOperation"
Cohesion: 0.43
Nodes (3): Store, operationConflict(), requestHash()

### Community 282 - "check_test.go"
Cohesion: 0.29
Nodes (11): State, okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestCheckFuncWithoutFunctionIsNotSilent(), TestReportErrExitCodeFollowsTheFailureClass(), TestReportErrOnlyForErrorState() (+3 more)

### Community 283 - "[x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması

### Community 284 - "[x] MR-007 — Reconcile-first gerçek değişiklik keşfi"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-007 — Reconcile-first gerçek değişiklik keşfi

### Community 285 - "runStatus"
Cohesion: 0.70
Nodes (4): Options, newStatusCommand(), readinessOf(), runStatus()

### Community 286 - "Check"
Cohesion: 0.23
Nodes (21): Check(), Coverage, provenanceScope(), rehashScope(), shortHash(), evidenceRow(), provenanceForTest(), rowScope() (+13 more)

### Community 287 - "Root"
Cohesion: 0.14
Nodes (22): go/ast.ImportSpec, moduleRoot(), TestTheKnowledgePipelineHasExactlyOneCallSite(), lifecycleLiteralsOutsideThisPackage(), moduleRoot(), TestNoOtherPackageStatesTheLifecycle(), Root(), SkipDir() (+14 more)

### Community 288 - "[ ] MR-011 — Kaynak değişince evidence geçersizleştirme"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-011 — Kaynak değişince evidence geçersizleştirme, Ne yapılacak

### Community 289 - "newImpactFixture"
Cohesion: 0.14
Nodes (31): Breadth, Entry, impactFixture, Input, Request, Result, targetFact, Via (+23 more)

### Community 290 - "NewRegistry"
Cohesion: 0.19
Nodes (14): helperKey(), TestAnalyzeRealRepositoryEndToEnd(), adapter, Registry, NewRegistry(), newRegistry(), SyntaxAdapter, gitCommand() (+6 more)

### Community 291 - "MR-009 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema, 6. Analysis flows, 7. Lease/scope inputs (+3 more)

### Community 292 - "requireValid"
Cohesion: 0.25
Nodes (13): createdAtOf(), describe(), encode(), findingsFor(), readGolden(), requireValid(), shippedValidator(), TestConstructorsSerializeCreatedAtAsUTCRFC3339() (+5 more)

### Community 293 - "MR-009 — Bulgular ve kapı kaydı"
Cohesion: 0.17
Nodes (12): Başlangıç ve çalışma ağacı, MR-009 — Bulgular ve kapı kaydı, MR-009 kapanış, TASK-01 guard mutasyon defteri (tamamı geri alındı), TASK-01 kabul kanıtı, TASK-01 kapı — Reader: 4/4 CONFIRMED + 1 gap; Breaker: 5/6 REFUTED + 1 BROKEN→KAPANDI, TASK-02 guard mutasyon defteri (tamamı geri alındı), TASK-02 kabul kanıtı (+4 more)

### Community 294 - "migratedChangesSchema"
Cohesion: 0.51
Nodes (9): migratedChangesSchema(), mustChangesSQL(), seedTaskChain(), TestBaselineGrainIsPerTaskPath(), TestChangeRowsEnforceForeignKeys(), TestChangeRowVocabularies(), TestChangeSymbolGrainIsPerKey(), TestOneOpenChangePerTask() (+1 more)

### Community 295 - "MR-007 — Bulgular ve kapı kaydı"
Cohesion: 0.06
Nodes (35): Başlangıç ve çalışma ağacı, D-131 — yakınsama bağımsız keşifler üzerinedir (AC-05.1 açıklaması), Ek pinler (bu tur), MR-007 — Bulgular ve kapı kaydı, Reader / Breaker bulguları ve giderim — TASK-01 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-03 kapısı (+27 more)

### Community 296 - "[ ] MR-017 — Staged değişiklik için yerel Git enforcement"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-017 — Staged değişiklik için yerel Git enforcement, Ne yapılacak

### Community 297 - "MR-012 — Bulgular ve kapı kaydı"
Cohesion: 0.18
Nodes (11): Başlangıç ve çalışma ağacı, MR-012 — Bulgular ve kapı kaydı, MR-012 kapanış, TASK-01 guard mutasyon defteri (tamamı geri alındı), TASK-01 kabul kanıtı, TASK-01 kapı — Reader: 5/5 CONFIRMED + 3 gözlem; Breaker: 6/6 REFUTED, TASK-02 guard mutasyon defteri (tamamı geri alındı), TASK-02 kabul kanıtı (+3 more)

### Community 298 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 299 - "newGuardService"
Cohesion: 0.19
Nodes (25): TestWeakeningScenarioEndToEnd(), ecmaDelta(), TestEcmaAddedOnlyQuiet(), TestEcmaDuplicateNamesAggregate(), TestEcmaEscapedNamesMap(), TestEcmaExpectDecrease(), TestEcmaExpectRemovedAndTestRemoved(), TestEcmaHatchSuppressesLoudly() (+17 more)

### Community 301 - "testing.T"
Cohesion: 0.04
Nodes (89): testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestFixedClockSatisfiesClock(), TestParseTimeRejectsMalformedInput() (+81 more)

### Community 302 - "MR-011 — Bulgular ve kapı kaydı"
Cohesion: 0.25
Nodes (8): Başlangıç ve çalışma ağacı, MR-011 — Bulgular ve kapı kaydı, MR-011 kapanış, TASK-01 guard mutasyon defteri (tamamı geri alındı), TASK-01 kabul kanıtı, TASK-01 kapı — Reader: 2 CONFIRMED + 2 REFUTED(harf) + 1 tension; Breaker: 3 BROKEN→KAPANDI + 3 REFUTED, TASK-02 kabul kanıtı, TASK-02 kapı — Reader: AC-02.1/02.2 CONFIRMED, ledger CONFIRMED; Breaker: 3 REFUTED + 1 zayıf-beat→GÜÇLENDİRİLDİ

### Community 303 - "MR-013 — Design"
Cohesion: 0.09
Nodes (19): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema, 6. Evaluation flows, 7. Secret inputs (+11 more)

### Community 305 - "driver.go"
Cohesion: 0.27
Nodes (9): diskFullCode(), driverResultCode(), dsn(), Options, hasPrimaryCode(), isCorruptError(), isDiskFullError(), isReadOnlyError() (+1 more)

### Community 306 - "database/sql.DB"
Cohesion: 0.23
Nodes (16): kernelFixture, database/sql.DB, Clock, Store, NewStore(), TestNewStoreRefusesNilHandle(), containsBoth(), fakeStatusRunner() (+8 more)

### Community 307 - "NewValidator"
Cohesion: 0.11
Nodes (30): collectFindings(), documentID(), escapePointerToken(), Registry, Finding, RecordKind, Registry, registryWith() (+22 more)

### Community 308 - "status/knowledge_findings_test.go"
Cohesion: 0.67
Nodes (3): rowValue(), TestStatusBuildsNoValidatorOfItsOwn(), TestTheFindingsCountIsQualifiedByObservation()

### Community 309 - "Discover"
Cohesion: 0.17
Nodes (23): Files(), TestFilesCancellationBeforeWalk(), TestFilesDeterministicallyAssignsNestedUnitsAndSkipsUnsafePaths(), CanonicalRoot(), contains(), Discover(), isLinkedWorktreeRoot(), Owner() (+15 more)

### Community 310 - "github.com/spf13/cobra.Command"
Cohesion: 0.19
Nodes (29): github.com/spf13/cobra.Command, Options, newCheckpointCommand(), newCheckpointWriteCommand(), invocation, Options, newSessionCommand(), newSessionOpenCommand() (+21 more)

### Community 311 - "[ ] MR-013 — Yerel completion evidence gate"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, Ne yapılacak

### Community 312 - "NewStore"
Cohesion: 0.18
Nodes (9): lockProbingClock, stepClock, sync.Mutex, TestACheckpointIsStampedUnderTheWriteLock(), NewStore(), TestAMoveIsStampedUnderTheWriteLock(), refuseWrites(), restrict() (+1 more)

### Community 314 - "adapter_test.go"
Cohesion: 0.24
Nodes (9): FakeResponse, Invocation, fakeAsked(), healthyResponses(), TestAdapterResolveGoldenArgv(), TestAdapterResolveLinkedWorktreeFlag(), TestAdapterVersion(), TestFakeRunnerRecordsInvocations() (+1 more)

### Community 316 - "ProjectUnit"
Cohesion: 0.14
Nodes (11): File, Scheduler, New(), ProjectUnit, bindFileUIDs(), Service, splitTarget(), Job (+3 more)

### Community 321 - "runRacer"
Cohesion: 0.43
Nodes (6): testing.M, appliedCount(), assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), TestMain()

### Community 322 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 323 - "readonlyfs_linux_test.go"
Cohesion: 0.44
Nodes (8): checkReadOnlyRemedy(), openOnAReadOnlyFilesystem(), probeOnAReadOnlyFilesystem(), readOnlyChild(), readOnlyScenario(), remountReadOnly(), remountReadWrite(), runReadOnlyChild()

### Community 324 - "[ ] MR-015 — MCP koordinasyon ve değişiklik araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-015 — MCP koordinasyon ve değişiklik araçları, Ne yapılacak

### Community 325 - "builder"
Cohesion: 0.17
Nodes (10): regexp.Regexp, Decision, Severity, checkRepoRelative(), ScopeLevel, isScopeLevel(), isWindowsDriveRooted(), builder (+2 more)

### Community 326 - "readiness"
Cohesion: 0.25
Nodes (4): readiness, bytes.Buffer, sync.Once, boundedBuffer

## Knowledge Gaps
- **1650 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `Provenance`, `invocation`, `invocation` (+1645 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1882 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **23 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `ProbeWriteAccess`, `refuseUnrepresentableJSON`, `.replay`, `invalidInput`, `NewRunner`, `Probe`, `exit.go`, `config/loader.go`, `newInvocation`, `TestBrokenSetupMatrix`, `gate.go`, `TestBrokenGitFileDetectionDoesNotOverFire`, `New`, `ParseTime`, `.withOperation`, `App`, `Config`, `RuntimePathCheck`, `newLoader`, `Build`, `RootKind`, `git/adapter.go`, `.Record`, `git/runner.go`, `github.com/spf13/cobra.Command`, `Warning`, `Code`, `Probe`, `Store`, `time.Time`, `DomainError`, `Lease`, `corruptState`, `Checkpoint`, `Root`, `Migrator`, `testguard.go`, `time.Duration`, `DB`, `NewRootWith`, `healthySubject`, `context.Context`?**
  _High betweenness centrality (0.035) - this node is a cross-community bridge._
- **Why does `SyntaxSnapshot` connect `SyntaxSnapshot` to `NewStore`, `shadowedNames`, `Facts`?**
  _High betweenness centrality (0.015) - this node is a cross-community bridge._
- **Why does `FormatTime()` connect `time.Time` to `Migrator`, `invalidInput`, `newFixture`, `.insertCheckpoint`, `newStore`, `corruptState`, `.Record`, `TestBrokenSetupMatrix`, `newInitializedRepo`, `NamedSession`, `ParseTime`, `New`, `io.Writer`, `.withOperation`, `Store`, `lifecycle_lease_test.go`, `context.Context`?**
  _High betweenness centrality (0.011) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **Are the 78 inferred relationships involving `run()` (e.g. with `realGitRepo()` and `assertTheLoopTerminates()`) actually correct?**
  _`run()` has 78 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `Provenance` to the rest of the system?**
  _1650 weakly-connected nodes found - possible documentation gaps or missing edges._