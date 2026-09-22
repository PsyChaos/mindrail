# Graph Report - Mindrail  (2026-09-22)

## Corpus Check
- 400 files · ~718,333 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 5303 nodes · 14425 edges · 298 communities (268 shown, 21 thin omitted)
- Extraction: 84% EXTRACTED · 16% INFERRED · 0% AMBIGUOUS · INFERRED: 2311 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `0b97cd62`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- MINDRAIL PROTOCOL Managed Section
- mindrail-0.1-task-list.md
- mindrail-tech-stack.md
- mindrail-technical-specification-1.0.md
- 129. Acceptance criteria
- 19. Semantic resolver kurulum, lifecycle ve resource policy
- App
- indexStore
- MR-002 — findings
- 2. Decisions
- git/adapter.go
- ResolveRuntimePaths
- ADR-0002 — SQLite driver
- ADR-0001 — Primary language and CLI stack
- github.com/PsyChaos/mindrail
- io.Writer
- config/loader.go
- git/classify_test.go
- newFixture
- run
- errorStore
- New
- status/render_test.go
- InTx
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- context.Context
- Discover
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
- testing.T
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- Subject
- 3. The findings in full
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- newFixture
- agreement_hostilefs_linux_test.go
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
- cli/arch_test.go
- registryForTest
- status
- status
- graphify reference: add a URL and watch a folder
- graphify reference: commit hook and native CLAUDE.md integration
- graphify reference: incremental update and cluster-only
- DefaultChecks
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
- Subject
- 128. Hedef implementation workstream'leri
- impacted_tests.py
- 4. The confirmed defects
- DB
- report.go
- WriteIfAbsent
- SyntaxSnapshot
- 47. Human approval kimlik ve yetkilendirme modeli
- runner.go
- MR-005 — Bulgular ve kapı kaydı
- 2. Requirements
- pathsAt
- Root
- WriteFailure
- RootKind
- time.Duration
- Facts
- NewRootWith
- index/identity_test.go
- FreeSpace
- mindrail-0.1-kernel-scope.md
- Clock
- ProbeWriteAccess
- refuseUnrepresentableJSON
- record/errors.go
- newRefreshFixture
- NewRegistry
- hostilefs_linux_test.go
- storage/access_other.go
- readBySchema
- Probe
- 2. Requirements
- NewRoot
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- explain
- MR-004 — design: leases, idempotent mutations and optimistic revision
- runWith
- Code
- MR-003 — design: sequential agent handover
- 1. Decisions this document freezes
- MR-002 — audit package
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- newServiceFixture
- builder
- 121. Failure modes
- newStore
- MEDIUM
- adapter_test.go
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- validator.go
- 2. Mindrail'in temel problemi
- invalidInput
- Open
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- Supersede
- created_at
- indexerFixture
- Config
- NewRegistry
- Probe
- 2. Decisions
- Indexer
- requireValid
- 3. IN Scope
- Warning
- ExitCode
- New
- leaseFixture
- TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore
- MR-004 — Findings and per-task record
- MR-003 — findings
- readonlyfs_linux_test.go
- 103. Test weakening ve false-green guard
- 28. Interactive performance contract
- 159. First Implementation Milestone
- 33. Durable Symbol Identity Implementation
- 56. Resolver Process Supervision
- 12. Content-addressed index cache
- 36. Impact explosion ve drill-down modeli
- 3. Mindrail ne değildir?
- newRepo
- TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository
- .recordedID
- NewID
- writable_test.go
- .withOperation
- audit_scope.py
- Appendix B — what the remediation did
- NamedSession
- MR-006 — Design
- smoke_test.go
- runner_test.go
- validator_internal_test.go
- TASK-01 dependency + schema.Validator
- TASK-02 internal/knowledge/record
- TASK-03 loader RecordRef.Body
- TASK-04+05 app codes + validate.Check
- TASK-06 bootstrap/doctor/status wiring
- TASK-07 CLI classifier + agreement matrix
- TASK-08 independent mutation sweep
- coherence_test.go
- task.go
- runtime.go
- MR-002 audit package — appendix: implementer self-reports
- scaffold_hostilefs_linux_test.go
- 5. Test plan
- New
- time.Time
- Migrations
- States
- .resolveIdentitiesTx
- Audit scoping — making audit cost follow risk, not task count
- 2. Requirements
- MR-002 — Versioned Decision/Invariant knowledge lifecycle
- contract_test.go
- fulldisk_linux_test.go
- goldenReport
- MR-006 — Bulgular ve kapı kaydı
- NewError
- TestAStepsMessageCarriesTheFactsItsRemedyNeeds
- [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi
- validator_test.go
- .recordedID
- .recordedID
- .recordedID
- execOnRuntimeDB
- MR-005 — Design
- NewLoader
- newInvocation
- writability_test.go
- Step
- ClassifyRefusal
- Mindrail 0.1 — Uygulama Görev Listesi
- TestEveryStartFailureReachesTheVerdict
- assertNoErrorEnvelope
- MintFor
- TestWriteModeMigratesNothingAndRegistersNothing
- Check
- 1. Decisions this document freezes
- database/sql.DB
- .RoundTrip
- 2. Decisions
- arrange
- [x] MR-001 — Yerel repository bootstrap ve tanılama yolu
- [ ] MR-008 — Scope drift ve unregistered change ambiguity
- version.go
- [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri
- [ ] MR-013 — Yerel completion evidence gate
- [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi
- .recordedID
- ProjectUnit
- TestStepSevenIsExemptFromTheSuppression
- MR-007 — Design
- [x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- [x] MR-004 — Güvenli lease, idempotency ve optimistic revision
- [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü
- Tracker
- loadStore
- [x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması
- [x] MR-007 — Reconcile-first gerçek değişiklik keşfi
- Görevler
- [ ] MR-011 — Kaynak değişince evidence geçersizleştirme
- [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı
- [ ] MR-014 — MCP bilgi ve bağlam araçları
- [ ] MR-015 — MCP koordinasyon ve değişiklik araçları
- MR-007 — Bulgular ve kapı kaydı
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- [ ] MR-018 — Hook bypass'a dayanıklı CI verification
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- migratedIndexSchema
- TestRepoConfigDirObstructionDoesNotOverFire
- Lease
- github.com/spf13/cobra.Command
- subjectWithoutRepository
- newRepoFixtureNamed
- status/knowledge_findings_test.go
- .insertCheckpoint
- NewStore

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 164 edges
2. `NewError()` - 139 edges
3. `run()` - 104 edges
4. `Check()` - 92 edges
5. `Open()` - 91 edges
6. `newInitializedRepo()` - 81 edges
7. `shippedValidator()` - 79 edges
8. `New()` - 79 edges
9. `storeOf()` - 77 edges
10. `decisionDoc()` - 68 edges

## Surprising Connections (you probably didn't know these)
- `migratedChangesSchema()` --calls--> `Load()`  [EXTRACTED]
  migrations/changes_constraints_test.go → internal/migration/load.go
- `migratedIndexSchema()` --calls--> `Load()`  [EXTRACTED]
  migrations/index_constraints_test.go → internal/migration/load.go
- `migratedChangesSchema()` --calls--> `New()`  [EXTRACTED]
  migrations/changes_constraints_test.go → internal/migration/migrator.go
- `migratedIndexSchema()` --calls--> `New()`  [EXTRACTED]
  migrations/index_constraints_test.go → internal/migration/migrator.go
- `migratedChangesSchema()` --calls--> `Open()`  [EXTRACTED]
  migrations/changes_constraints_test.go → internal/storage/db.go

## Import Cycles
- None detected.

## Communities (298 total, 21 thin omitted)

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

### Community 6 - "App"
Cohesion: 0.07
Nodes (20): InitResult, txProbe, io/fs.FS, sync/atomic.Bool, sync/atomic.Int64, AdoptCause(), asKnowledgeError(), asMigrationError() (+12 more)

### Community 7 - "indexStore"
Cohesion: 0.12
Nodes (35): casHash(), casSymbol(), TestCASCompletionResolvesOnlyUniqueSameKey(), TestCompletionCASRejectsStaleH1BeforeDeletingH2Facts(), TestConcurrentRegistrationsFromOneObservationOnlyOneWins(), TestFailedObservationMustRegisterRetryBeforeCompletion(), TestLegacyRegistrationCannotRecreateOldCASTokenAfterStateABA(), TestRegisterCASRejectsMalformedHashWithoutRows() (+27 more)

### Community 8 - "MR-002 — findings"
Cohesion: 0.06
Nodes (35): 1. The number that matters, 2. Round 1: what was actually wrong, 3. Round 2: what closing them cost, 4. Status, 5. What generalises to MR-003, A containment gap MR-002 made load-bearing, Appendix A — round 1 findings, as confirmed, Appendix C — round 2 findings, as confirmed (+27 more)

### Community 9 - "2. Decisions"
Cohesion: 0.07
Nodes (30): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-114 — the domain lives in `internal/changes/`, schema version 6 (+22 more)

### Community 10 - "git/adapter.go"
Cohesion: 0.15
Nodes (23): Adapter, standpoint, worktreeLinkage, bareRepositoryError(), detachedWorktreeError(), firstLine(), foreignRelinkCaveat(), foreignWorktreeError() (+15 more)

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
Cohesion: 0.14
Nodes (12): attributed, checkpointResult, leaseListResult, leaseResult, sessionResult, taskResult, io.Writer, detailLines() (+4 more)

### Community 16 - "config/loader.go"
Cohesion: 0.19
Nodes (14): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+6 more)

### Community 17 - "git/classify_test.go"
Cohesion: 0.27
Nodes (12): exit128(), mentionsGitInit(), requirePermissionsAreEnforced(), TestExit128IsClassifiedByWhatGitSaid(), TestGitStderrAdviceDoesNotSpillIntoWhy(), TestGitStderrReachesTheReader(), TestLayoutProbeFailureKeepsStderr(), TestSecondProbeFailureIsClassifiedToo() (+4 more)

### Community 18 - "newFixture"
Cohesion: 0.20
Nodes (24): Task, State, fixture, ids(), newFixture(), newStepClock(), openFixture(), TestABlockCarriesItsReasonAndLeavingClearsIt() (+16 more)

### Community 19 - "run"
Cohesion: 0.09
Nodes (72): child, racedTask, readiness, bytes.Buffer, io.WriteCloser, os/exec.Cmd, sync.Once, TestHealthyRepositoryIsUntouchedByRemedyCoherence() (+64 more)

### Community 20 - "errorStore"
Cohesion: 0.14
Nodes (21): database/sql/driver.Conn, database/sql/driver.Driver, database/sql/driver.NamedValue, database/sql/driver.Result, database/sql/driver.Rows, database/sql/driver.Tx, database/sql/driver.TxOptions, database/sql/driver.Value (+13 more)

### Community 21 - "New"
Cohesion: 0.17
Nodes (37): New(), assertNoRuntimeState(), corruptDatabase(), exists(), initialize(), itoa(), newGitRepo(), options() (+29 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.20
Nodes (20): InitReport, TestKnowledgeFindingsAreCountedFromTheSubject(), assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport() (+12 more)

### Community 23 - "InTx"
Cohesion: 0.07
Nodes (46): database/sql.NullString, Ambiguity, Binding, FileFacts, FileIndexState, fileScanner, FileStateObservation, fileStateToken (+38 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (33): allEscapeRoot(), alsoHeading(), degradedAccount(), degradedImpact(), degradedSummary(), describeBounded(), describeFindings(), describeProblems() (+25 more)

### Community 25 - "healthySubject"
Cohesion: 0.11
Nodes (56): tail, KnowledgeCheck(), assertDiagnosable(), Result, healthySubject(), TestAnUnusableKnowledgeDirectoryIsReportedAsAPathCondition(), TestGitCheckBranches(), TestKnowledgeCheckAbsentTreeIsOK() (+48 more)

### Community 26 - "PayloadOf"
Cohesion: 0.19
Nodes (52): ErrorPayload, PayloadOf(), NewAdapter(), assertPayloadIsActionable(), assertRemedyDestinationsAreReal(), changeIntoDestinations(), TestAdapterResolveBareRepository(), TestAdapterResolveNotARepository() (+44 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "context.Context"
Cohesion: 0.18
Nodes (12): context.Context, Migration, bookkeepingCreateFailure(), checksumFailure(), Applied, Migrator, highestVersion(), namesAbsentFrom() (+4 more)

### Community 29 - "Discover"
Cohesion: 0.15
Nodes (23): Files(), File, TestFilesCancellationBeforeWalk(), TestFilesDeterministicallyAssignsNestedUnitsAndSkipsUnsafePaths(), CanonicalRoot(), contains(), Discover(), isLinkedWorktreeRoot() (+15 more)

### Community 30 - "RuntimePathCheck"
Cohesion: 0.26
Nodes (15): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+7 more)

### Community 31 - "initReportOf"
Cohesion: 0.20
Nodes (14): blockingSummary(), Options, initReportOf(), initVerdict(), newInitCommand(), runInit(), schemaIsCurrent(), TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee() (+6 more)

### Community 32 - "newLoader"
Cohesion: 0.07
Nodes (77): operationRecord, encoding/json.RawMessage, io/fs.DirEntry, reflect.Type, testing.B, Session, decisionRef(), decodeObjects() (+69 more)

### Community 33 - "Build"
Cohesion: 0.14
Nodes (36): findingOn(), TestACycleBlocksAndAnInvalidRecordDoesNot(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsObservedOnAHealthyRepository(), TestRepositoryBlockSurvivesALaterStepFailing(), TestHumanReportCarriesTheObservationMarker(), TestHumanReportOmitsTheObservationMarkerWhenItLooked() (+28 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "Finding"
Cohesion: 0.12
Nodes (32): compareNodes(), ids(), newGraph(), expectedPath(), idFromPath(), activeSubjects(), Step, invalid() (+24 more)

### Community 37 - "status/render.go"
Cohesion: 0.17
Nodes (17): strings.Builder, Report, Result, State, paint(), writeBlock(), writeResult(), Report (+9 more)

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

### Community 43 - "testing.T"
Cohesion: 0.05
Nodes (63): go/ast.ImportSpec, testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestFixedClockSatisfiesClock() (+55 more)

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

### Community 48 - "Subject"
Cohesion: 0.16
Nodes (11): diagnosis, halt, Probes, step, codeOf(), fallbackWriteRemedy(), Subject, Result (+3 more)

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

### Community 58 - "newFixture"
Cohesion: 0.28
Nodes (21): mkunit(), newFixture(), seedPending(), TestEnqueueP0PromotesQueuedPath(), TestEnqueueRejectsRelativeAndUncleanPaths(), TestEvictedPathsRefillAfterPrioritize(), TestFillColdOrdersByPathAndReportsMore(), TestPrioritizeEnqueuesUnqueuedUnitFiles() (+13 more)

### Community 59 - "agreement_hostilefs_linux_test.go"
Cohesion: 0.16
Nodes (28): answer, hostileCondition, agreeingHostileCommands(), assertTheLoopTerminates(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo(), hostileAvailableBytes() (+20 more)

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
Cohesion: 0.21
Nodes (7): Report, State, severity(), States(), TestStateEnumIsExactlyFiveValues(), TestStateUnmarshalRejectsUnknown(), TestStateUnmarshalRejectsUnknownInsideAResult()

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
Cohesion: 0.14
Nodes (28): github.com/tree-sitter/go-tree-sitter.Node, Import, adapter, textField(), unquoteModule(), bindingExtent(), bindingNames(), bindingsContain() (+20 more)

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

### Community 83 - "cli/arch_test.go"
Cohesion: 0.16
Nodes (25): firstPartyPackage, moduleRoot(), TestTheKnowledgePipelineHasExactlyOneCallSite(), assertMainDeclaresOnlyMain(), assertNoNetworkInLinkedPackages(), fileImports(), firstPartyPackages(), goSources() (+17 more)

### Community 84 - "registryForTest"
Cohesion: 0.13
Nodes (32): LocalKey(), extractForTest(), TestAnonymousAndLocalBindingsDoNotInventTargets(), TestBareMethodCallUsesLexicalScopeAndAmbiguityStaysUnresolved(), TestCompletePackageExtraction(), TestExtractionFailureAndCancellationCloseSnapshots(), TestExtractPreservesPartialFactsAndCancellation(), TestFingerprintSeparatesBodyAndSignature() (+24 more)

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

### Community 90 - "DefaultChecks"
Cohesion: 0.15
Nodes (28): App, NewRunner(), TestRunnerRespectsContextCancellation(), DefaultChecks(), assertActionable(), Subject, haltedSubject(), TestBrokenConfigDoesNotUnresolveTheRepository() (+20 more)

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

### Community 107 - "Subject"
Cohesion: 0.13
Nodes (17): Subject, Components(), ComponentName, Readiness, intPtr(), Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues() (+9 more)

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "impacted_tests.py"
Cohesion: 0.22
Nodes (20): changed_files(), git(), is_e2e(), is_test(), js_imports(), load_coverage_contexts(), load_explicit_map(), main() (+12 more)

### Community 110 - "4. The confirmed defects"
Cohesion: 0.08
Nodes (26): 1. The verdict, 2. The base rate, which is why this round exists, 3. The three deviations, graded, 4.10 — LOW. §4's corrected F17 bullet credits the fix to a test that does not contain it, 4.11 — LOW. The F48 figure attaches the 1,000-task measurement to 20,000 tasks, 4.12 — LOW. A minted session now starts after the row it attributes, 4.13 — LOW. The `EXISTS` rewrite degrades to a full checkpoint scan for a project with no checkpoints, 4.14 — LOW. Item 11 is recorded as "(test only)" but its commit changes production behaviour (+18 more)

### Community 111 - "DB"
Cohesion: 0.12
Nodes (26): classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), diskFullOpenFailure(), DB, Options, notWALFailure(), openFailure() (+18 more)

### Community 112 - "report.go"
Cohesion: 0.27
Nodes (13): coordinationInfo(), Report, observationOf(), repositoryObservation(), stateRank(), stoppedAtStep(), worst(), CoordinationInfo (+5 more)

### Community 113 - "WriteIfAbsent"
Cohesion: 0.21
Nodes (16): requireEmptyTree(), requireEscape(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository(), TestScaffoldKeepsRepositoryModes(), TestScaffoldRefusesToWriteThroughAnEscapingSymlink(), DefaultTemplate(), EnsureKnowledgeDirs() (+8 more)

### Community 114 - "SyntaxSnapshot"
Cohesion: 0.10
Nodes (14): github.com/tree-sitter/go-tree-sitter.Language, github.com/tree-sitter/go-tree-sitter.Query, github.com/tree-sitter/go-tree-sitter.Tree, sync.RWMutex, Import, adapter, Reference, Symbol (+6 more)

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "runner.go"
Cohesion: 0.16
Nodes (15): ExecRunner, insideGitDirectoryError(), stderrSummary(), TestAdapterPropagatesRunnerFailures(), worktreeHoldingGitDir(), directoryEntryFault(), hasAnyPrefix(), SanitizedEnv() (+7 more)

### Community 117 - "MR-005 — Bulgular ve kapı kaydı"
Cohesion: 0.05
Nodes (38): AC evidence and RED/GREEN, Başlangıç ve çalışma ağacı, D-89 — referans hedefi silinince `ON DELETE SET NULL`, D-90 — cross-file çözümleme dosya-kapsamlıdır, birim-kapsamlı değil, D-91 — TASK-06 scheduler'ı sürmez, readiness'i bağlar, D-92 — süreç-ölçeği kanıtı yeniden-açılan handle'lardır, Deliberate guard mutations (all restored), Denetim artefaktları (+30 more)

### Community 118 - "2. Requirements"
Cohesion: 0.06
Nodes (36): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation, D-43 — a Finding gets its own codes, because D-38 says it is not a Problem (+28 more)

### Community 119 - "pathsAt"
Cohesion: 0.13
Nodes (32): TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions(), TestAnInvariantCycleIsFatalAndNamesOnlyInvariants(), TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord(), TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge(), TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose(), detections(), overFires(), TestAKindThatDisagreesWithItsDirectoryIsReportedByStepFive() (+24 more)

### Community 120 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 121 - "WriteFailure"
Cohesion: 0.13
Nodes (29): DomainError, TestMigrationThreeLeavesTheObjectsTheDesignNames(), TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget(), busyFailure(), describeDatabaseFile(), diskFullFailure(), IsBusy(), IsDiskFull() (+21 more)

### Community 122 - "RootKind"
Cohesion: 0.21
Nodes (13): os.FileInfo, ensureDir(), unwritableError(), dangling(), root, RootKind, RuntimePaths, Writability (+5 more)

### Community 123 - "time.Duration"
Cohesion: 0.17
Nodes (19): time.Duration, genuineBusyError(), TestABusyOpenIsNamedOnce(), TestAnOpenRefusedForTheWholeBudgetIsRetryable(), TestAnOpenThatIsNotContendedIsNotRewritten(), TestTheLadderStepsByTheSpecsExample(), TestWaitBusyReturnsSuccessAndPermanentFailuresAtOnce(), TestWaitBusySleepsByTheLadderAndStopsAtTheBudget() (+11 more)

### Community 124 - "Facts"
Cohesion: 0.23
Nodes (17): Facts, Range, decodeExact(), Cache, Computer, readEntry(), sha256Hex(), validFacts() (+9 more)

### Community 125 - "NewRootWith"
Cohesion: 0.15
Nodes (13): classifiedArgs, Root, github.com/spf13/cobra.Completion, github.com/spf13/cobra.PositionalArgs, github.com/spf13/cobra.ShellCompDirective, Execute(), helpTopics(), NewRoot() (+5 more)

### Community 126 - "index/identity_test.go"
Cohesion: 0.18
Nodes (33): allocDatabase(), allocWorker(), ambiguityRows(), identityCount(), identityFixture(), identityUnit(), migrationFixture(), sourceFile() (+25 more)

### Community 127 - "FreeSpace"
Cohesion: 0.14
Nodes (14): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+6 more)

### Community 128 - "mindrail-0.1-kernel-scope.md"
Cohesion: 0.12
Nodes (14): 1. Goal, 2. Primary End-to-End Scenario, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition, Mindrail 0.1 — Kernel Scope (+6 more)

### Community 129 - "Clock"
Cohesion: 0.30
Nodes (9): Clock, Registration, Store, Workspace, NewStore(), scanWorkspace(), upsertWorkspace(), validateRegistration() (+1 more)

### Community 130 - "ProbeWriteAccess"
Cohesion: 0.18
Nodes (16): describeRefusedPaths(), TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError(), conditionBlocker(), WriteAccess (+8 more)

### Community 131 - "refuseUnrepresentableJSON"
Cohesion: 0.24
Nodes (11): offender, reflect.StructField, reflect.Value, firstUnrepresentable(), memberName(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo() (+3 more)

### Community 132 - "record/errors.go"
Cohesion: 0.20
Nodes (3): DuplicateItemError, EmptyFieldError, OptionKindError

### Community 133 - "newRefreshFixture"
Cohesion: 0.30
Nodes (18): activeInvariant(), newRefreshFixture(), symbolPackageDir(), TestAmbiguousTwinsBlockBySeverity(), TestEndToEndProtectRenameAmbiguousDelete(), TestOrphanBlocksOnCritical(), TestOutsideUnitTargetIsOrphaned(), TestRefreshBindingsWritesBoundRows() (+10 more)

### Community 134 - "NewRegistry"
Cohesion: 0.15
Nodes (10): Registry, NewRegistry(), TestEmbeddedSchemaDocumentIsACopy(), TestEmbeddedSchemasPresent(), TestNewRegistryAcceptsExtraDocuments(), TestNewRegistryRejectsIncompleteOrCorruptFS(), TestSupportsRejectsNewerVersion(), TestVersionWindowIsWriteOneReadableOne() (+2 more)

### Community 135 - "hostilefs_linux_test.go"
Cohesion: 0.25
Nodes (14): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+6 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "readBySchema"
Cohesion: 0.19
Nodes (19): createdAtDescription(), everyDescriptionClaim(), everyRefusedSpelling(), measuredGround(), TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(), TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(), TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(), TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository() (+11 more)

### Community 140 - "Probe"
Cohesion: 0.17
Nodes (20): io/fs.FileInfo, io/fs.FileMode, absentFailure(), classify(), describeMode(), Presence, nonDirectoryAncestor(), Probe() (+12 more)

### Community 141 - "2. Requirements"
Cohesion: 0.11
Nodes (18): 0. Baseline evidence, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, MR-004 — Frozen requirements and acceptance criteria, REQ-01 — `internal/storage`: the bounded wait, named and measured, REQ-02 — `migrations/000003_lease_idempotency.sql` (+10 more)

### Community 142 - "NewRoot"
Cohesion: 0.14
Nodes (20): TestScaffoldFailureNamesTheRepositoryConfigRoot(), TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRepositoryRootIsItsOwnKind(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot() (+12 more)

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "explain"
Cohesion: 0.18
Nodes (24): ConfigCheck(), configResult(), describePath(), describeRuntimeLocation(), describeStartDir(), describeUninitializedDB(), explain(), failure() (+16 more)

### Community 145 - "MR-004 — design: leases, idempotent mutations and optimistic revision"
Cohesion: 0.13
Nodes (15): 10. Command surface, 11. Status integration, 12. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000003_lease_idempotency.sql` (+7 more)

### Community 146 - "runWith"
Cohesion: 0.11
Nodes (26): brokenSetup, coordinationRefusal, envelopeScenario, semanticDamageRow, strictEnvelope, assertFourErrorKeys(), isEmptyValue(), runWith() (+18 more)

### Community 147 - "Code"
Cohesion: 0.35
Nodes (10): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+2 more)

### Community 148 - "MR-003 — design: sequential agent handover"
Cohesion: 0.17
Nodes (12): 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000002_coordination.sql`, 6. The state table, 7. Command surface, 8. Status integration (+4 more)

### Community 149 - "1. Decisions this document freezes"
Cohesion: 0.12
Nodes (17): 1. Decisions this document freezes, D-64 — one `leases` table for every target kind, one row per tenure, inside `internal/coordination`, D-65 — active means unreleased and unexpired by the store's clock, judged in Go, and expiry is lazy, D-66 — a task lease is acquired only by moving the task; renew and release by id work for any kind, D-67 — the lease is judged before the state table, and an expired lease is taken over out loud, D-68 — `claimed_by` is the holder while a lease is active, and attribution afterwards (amends D-58), D-69 — the TTL is twenty minutes, a constant, D-70 — stale-lease recovery is expiry; operator override is outside 0.1 (+9 more)

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
Cohesion: 0.06
Nodes (70): serviceFixture, FakeResponse, Invocation, RenameEntry, changesFixture(), changesFixtureDB(), seedTaskChain(), shaOf() (+62 more)

### Community 154 - "builder"
Cohesion: 0.17
Nodes (10): regexp.Regexp, Decision, Severity, checkRepoRelative(), ScopeLevel, isScopeLevel(), isWindowsDriveRooted(), builder (+2 more)

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "newStore"
Cohesion: 0.38
Nodes (13): newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace(), TestRegisteredIDsAreOpaqueAndPrefixed(), TestRegisterIsIdempotent() (+5 more)

### Community 157 - "MEDIUM"
Cohesion: 0.08
Nodes (25): F01 — `mindrail init` reported READY over a repository the next command refused, F02 — an unopenable `config.toml` was remedied as a malformed one, F03 — a size limit was reported as a full disk, F04, F08 — a rejected command line exited 1 and wrote no envelope, F05 — the agreement matrix never read the remedy it claimed to carry out, F06 — no matrix row for a `config.toml` that cannot be opened, F07 — the linked-worktree cross-check compared two fields of four, F09 — the read-only-media sentinel had no row in the classifier's table (+17 more)

### Community 158 - "adapter_test.go"
Cohesion: 0.36
Nodes (7): fakeAsked(), healthyResponses(), TestAdapterResolveGoldenArgv(), TestAdapterResolveInsideGitDirectory(), TestAdapterResolveLinkedWorktreeFlag(), TestAdapterVersion(), TestFakeRunnerRecordsInvocations()

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "validator.go"
Cohesion: 0.23
Nodes (14): collectFindings(), escapePointerToken(), Finding, RecordKind, Validator, keywordOf(), leafMessage(), pointerOf() (+6 more)

### Community 163 - "2. Mindrail'in temel problemi"
Cohesion: 0.40
Nodes (5): 2.1 Context kaybı, 2.2 Semantic conflict, 2.3 Intent kaybı, 2.4 Kanıtsız tamamlanma, 2. Mindrail'in temel problemi

### Community 164 - "invalidInput"
Cohesion: 0.08
Nodes (27): BaselineSummary, FileChange, SymbolChange, Operation, recorded, contentHash(), Change, Store (+19 more)

### Community 165 - "Open"
Cohesion: 0.09
Nodes (39): IsUnwritten(), Open(), openTemp(), TestCloseIsIdempotent(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections(), TestOpenAppliesPragmasOnEveryConnection() (+31 more)

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

### Community 172 - "Supersede"
Cohesion: 0.23
Nodes (12): TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(), TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(), Decision, Decision, Supersede(), activeDecision(), TestSupersedeDoesNotRepeatATargetItAlreadyCarries(), TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout() (+4 more)

### Community 173 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

### Community 174 - "indexerFixture"
Cohesion: 0.20
Nodes (28): sourceStillMatches(), indexerFixture(), semanticFileFacts(), sourceFile(), TestIndexFileCancellationLeavesTargetPendingAndRetries(), TestIndexFileCannotStealChildOwnedIndexedFile(), TestIndexFileChangedReplacesOnlyOwnFacts(), TestIndexFileFailedSameHashRetriesAndClassifiesParseFailure() (+20 more)

### Community 175 - "Config"
Cohesion: 0.19
Nodes (16): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+8 more)

### Community 176 - "NewRegistry"
Cohesion: 0.17
Nodes (15): timedAdapter, RuntimePaths, adapter, Registry, NewRegistry(), newRegistry(), SyntaxAdapter, gitCommand() (+7 more)

### Community 177 - "Probe"
Cohesion: 0.23
Nodes (14): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject (+6 more)

### Community 178 - "2. Decisions"
Cohesion: 0.06
Nodes (32): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's four acceptance criteria, D-100 — backfill mints, never migrates (+24 more)

### Community 179 - "Indexer"
Cohesion: 0.27
Nodes (10): IndexResult, sourceVersion, Timing, contentHash(), Indexer, Store, Symbol, NewIndexer() (+2 more)

### Community 180 - "requireValid"
Cohesion: 0.25
Nodes (13): createdAtOf(), describe(), encode(), findingsFor(), readGolden(), requireValid(), shippedValidator(), TestConstructorsSerializeCreatedAtAsUTCRFC3339() (+5 more)

### Community 181 - "3. IN Scope"
Cohesion: 0.12
Nodes (16): 3. IN Scope, Basic impact, Change discovery, Completion gate, Coordination, Core executable, Diagnostics, Engineering knowledge (+8 more)

### Community 182 - "Warning"
Cohesion: 0.21
Nodes (13): Envelope, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice(), regularFile() (+5 more)

### Community 183 - "ExitCode"
Cohesion: 0.11
Nodes (20): Error, main(), context.CancelFunc, RootContext(), Denied(), ExitCode(), exitCodeForKind(), Failed() (+12 more)

### Community 184 - "New"
Cohesion: 0.06
Nodes (106): testing/fstest.MapFS, testing.M, TestChangesSchemaVersionNamesItsCreatingMigration(), queryPlan(), schemaOnlyDatabase(), TestTheNewestCheckpointQueryDoesNotSortTheProject(), TestIndexSchemaVersionNamesItsCreatingMigration(), TestStoreDistinguishesNeverInitializedFromMissingLedger() (+98 more)

### Community 185 - "leaseFixture"
Cohesion: 0.30
Nodes (20): settableClock, fixture, leaseFixture(), mustFile(), newSettableClock(), readLeaseRow(), requireCode(), sameRow() (+12 more)

### Community 186 - "TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore"
Cohesion: 0.25
Nodes (13): TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone(), combine(), holdsTwoReportableSelfLoops(), oracleCycleMembers(), recordsFor(), renderStore(), storesOfOneKind(), TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore() (+5 more)

### Community 187 - "MR-004 — Findings and per-task record"
Cohesion: 0.04
Nodes (46): 1. TASK-01 — the bounded wait, named and measured, 2. TASK-02 — migration 000003, and a ledger that can read `ADD COLUMN`, 3. TASK-03 — the lease: one row per tenure, and the three verbs over a file, 4. TASK-04 — the lease and the revision in front of every task move, 5. TASK-05 — a repeated operation is answered from its record, 6. TASK-06 — the command surface, 7. TASK-07 — `status` publishes the leases held right now, 8. TASK-08 — two processes, eight sessions, and the proof under load (+38 more)

### Community 188 - "MR-003 — findings"
Cohesion: 0.10
Nodes (21): 1. What the milestone does now, 2. Two defects the implementation found in itself, 3. Mutations run, and what each turned red, 4. What an audit should look at first, 5. Audit round 1, 6. The round-1 remediation, 7. The round-2 remediation, MR-003 — findings (+13 more)

### Community 189 - "readonlyfs_linux_test.go"
Cohesion: 0.44
Nodes (8): checkReadOnlyRemedy(), openOnAReadOnlyFilesystem(), probeOnAReadOnlyFilesystem(), readOnlyChild(), readOnlyScenario(), remountReadOnly(), remountReadWrite(), runReadOnlyChild()

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

### Community 198 - "newRepo"
Cohesion: 0.12
Nodes (55): condition, agreeingCommands(), agreementConditions(), assertRemedyIsWhatTheRowExpects(), chmodForRemedy(), discoveryConditions(), dropRuntimeTables(), emptyMigrationLedger() (+47 more)

### Community 199 - "TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository"
Cohesion: 0.16
Nodes (21): gitFileFault, joinStderr(), linkedWorktreeOfAdminDir(), notARepositoryError(), brokenGitFileFault(), danglingGitFileError(), decorate(), findDanglingGitFile() (+13 more)

### Community 201 - "NewID"
Cohesion: 0.27
Nodes (9): encodeCrockford(), compareRandomness(), TestABackwardsClockStillMintsAscendingIds(), TestTheEntropyCounterCarriesAcrossAByte(), NewID(), nextEntropy(), TestAnIdIsPrefixedUniqueAndSortsInMintOrder(), TestAnIdLeaksNeitherTheClockNorTheCaller() (+1 more)

### Community 202 - "writable_test.go"
Cohesion: 0.31
Nodes (18): assertProbeMatchesSQLite(), assertUnwritablePayload(), chmodForTest(), dirEntryNames(), initialisedDatabase(), requireModeBitsAreEnforced(), TestProbeWriteAccessAgreesWithSQLite(), TestProbeWriteAccessAgreesWithSQLiteAcrossTheSidecarMatrix() (+10 more)

### Community 203 - ".withOperation"
Cohesion: 0.43
Nodes (3): Store, operationConflict(), requestHash()

### Community 204 - "audit_scope.py"
Cohesion: 0.32
Nodes (12): assign_cluster(), build_plan(), changed_files(), classify(), diff_lines(), git(), load_tasks(), main() (+4 more)

### Community 205 - "Appendix B — what the remediation did"
Cohesion: 0.33
Nodes (6): Appendix B — what the remediation did, FIX-A `internal/knowledge/validate` — reported `GREEN`, FIX-B `internal/doctor` — reported `GREEN_WITH_DEVIATIONS`, FIX-C `internal/knowledge/loader` — reported `GREEN_WITH_DEVIATIONS`, FIX-D `internal/knowledge/record` + `schemas/` — reported `GREEN`, FIX-E sweep, docs, re-audit — reported `GREEN_WITH_DEVIATIONS`

### Community 206 - "NamedSession"
Cohesion: 0.19
Nodes (18): TaskTarget(), activeTaskLease(), assertInvariant(), fixture, TestACheckpointIsStampedUnderTheWriteLock(), TestACheckpointOnADamagedTaskRowIsRefused(), TestAHoldersCheckpointRenewsAndAHandoffReleases(), TestAMoveOnAnotherSessionsHeldTaskIsRefusedForEveryDestination() (+10 more)

### Community 207 - "MR-006 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000005_symbol_identity.sql`, 6. Allocation, 7. Rename/move matching (+3 more)

### Community 208 - "smoke_test.go"
Cohesion: 0.23
Nodes (17): assertSQLiteCheckHealthy(), shellQuote(), TestGracefulShutdownOnSIGINT(), writeSlowGit(), addWorktree(), buildBinary(), decodeEnvelope(), entriesIn() (+9 more)

### Community 209 - "runner_test.go"
Cohesion: 0.24
Nodes (13): countEnv(), helperArgv(), helperRunner(), lookupEnv(), processAlive(), readPID(), TestDefaultTimeoutIsBounded(), TestExecRunnerCancellationKillsChild() (+5 more)

### Community 210 - "validator_internal_test.go"
Cohesion: 0.19
Nodes (14): documentID(), Registry, registryWith(), TestCollectFindingsDescendsToLeaves(), TestCollectFindingsNeverTurnsAFailureIntoSilence(), TestDocumentIDAcceptsAnOrdinaryDocument(), TestDocumentIDRefusesADocumentThatNamesItselfNothing(), TestNewValidatorAcceptsARegistryCarryingEveryDocumentItReads() (+6 more)

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

### Community 218 - "coherence_test.go"
Cohesion: 0.21
Nodes (17): remedy, remedyClass, agreementRemedyClass(), TestEveryRemedyThatNamesAPathIsClassified(), absolutePathsIn(), assertDocumentIsCoherent(), classOf(), contradictionsBetween() (+9 more)

### Community 219 - "task.go"
Cohesion: 0.42
Nodes (9): taskListResult, Options, newTaskCommand(), newTaskListCommand(), newTaskOpenCommand(), newTaskShowCommand(), newTaskStateCommand(), parseStateFlag() (+1 more)

### Community 220 - "runtime.go"
Cohesion: 0.67
Nodes (3): hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 222 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 223 - "5. Test plan"
Cohesion: 0.04
Nodes (47): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+39 more)

### Community 224 - "New"
Cohesion: 0.35
Nodes (20): New(), cachePaths(), entryFiles(), fixtureFacts(), pythonInfo(), runFIFOProbe(), TestCanceledContextBypassesDiskAndCompute(), TestComputeErrorAndCancellationAreNeverMasked() (+12 more)

### Community 225 - "time.Time"
Cohesion: 0.06
Nodes (65): Acquisition, FixedClock, SystemClock, handoverResult, lockProbingClock, rowScanner, stepClock, database/sql.Tx (+57 more)

### Community 228 - "States"
Cohesion: 0.18
Nodes (15): TestTheTableIsUnchangedByTheLease(), lifecycleLiteralsOutsideThisPackage(), moduleRoot(), CanTransition(), State, ParseState(), States(), TestARefusalCanSayWhatIsAvailableInstead() (+7 more)

### Community 229 - ".resolveIdentitiesTx"
Cohesion: 0.16
Nodes (16): DivergenceEntry, ReconcileResult, ancestorRow, stagedKey, fileHintsFor(), Service, joinRoot(), RenameHint (+8 more)

### Community 230 - "Audit scoping — making audit cost follow risk, not task count"
Cohesion: 0.20
Nodes (9): Audit scoping — making audit cost follow risk, not task count, Rule 1 — One audit per delivery, never one per task, Rule 2 — Depth is set per cluster by risk, Rule 3 — Mutation runs targeted tests, the full suite runs once, Rule 4 — Checkpoint audits hide audit time behind implementation time, Rule 5 — Re-audits are delta-only, The audit plan artifact, The problem this solves (+1 more)

### Community 231 - "2. Requirements"
Cohesion: 0.17
Nodes (12): 2. Requirements, REQ-01 — `internal/identity`: the shared id minter, REQ-02 — `migrations/000002_coordination.sql`, REQ-03 — `internal/coordination`: the domain, REQ-04 — `internal/coordination.Store`: persistence, REQ-05 — the code vocabulary, REQ-06 — the command surface, REQ-07 — `internal/status` publishes the coordination state (+4 more)

### Community 232 - "MR-002 — Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.22
Nodes (9): 0. What MR-001 left here, exactly, 1. Scope decision: no CLI write surface, 2. Package plan, 3. Frozen signatures, 4. Decisions, 5. Test plan, 6. Explicit non-goals, 7. Acceptance criteria traceability (+1 more)

### Community 233 - "contract_test.go"
Cohesion: 0.09
Nodes (40): result, addWorktree(), assertGolden(), assertShape(), findCheck(), join(), jsonShape(), newRepoWithCommit() (+32 more)

### Community 234 - "fulldisk_linux_test.go"
Cohesion: 0.22
Nodes (16): afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero(), fullDiskChild(), fullDiskScenario(), initialiseOnDisk() (+8 more)

### Community 235 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 236 - "MR-006 — Bulgular ve kapı kaydı"
Cohesion: 0.05
Nodes (41): Başlangıç ve çalışma ağacı, D-112 — eşleştirme transaction içinde Store metodudur (D-93 arıtması), D-113 — ata kümesi dosya+ihattur, global tarama yok, MR-006 — Bulgular ve kapı kaydı, Reader / Breaker bulguları ve giderim, Reader / Breaker bulguları ve giderim — TASK-01 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı (+33 more)

### Community 237 - "NewError"
Cohesion: 0.13
Nodes (24): CauseOf(), NewError(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain(), TestDomainErrorCarriesFiveFields(), TestDomainErrorWrappingSurvivesErrorsIsAndAs(), TestErrorPayloadJSONHasAllFourKeys(), TestPayloadCarriesTheCause() (+16 more)

### Community 238 - "TestAStepsMessageCarriesTheFactsItsRemedyNeeds"
Cohesion: 0.70
Nodes (4): facts(), TestAStepsMessageCarriesTheFactsItsRemedyNeeds(), TestEveryStepsMessageIsCovered(), fact

### Community 239 - "[ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi, Ne yapılacak

### Community 240 - "validator_test.go"
Cohesion: 0.20
Nodes (26): mustReadEmbedded(), Registry, NewValidator(), embeddedValidator(), located(), registryFrom(), TestBothDocumentsStateTheSameCreatedAtRule(), TestNewValidatorRejectsANilRegistry() (+18 more)

### Community 244 - "execOnRuntimeDB"
Cohesion: 0.35
Nodes (11): execOnRuntimeDB(), downgradeToSchemaFive(), downgradeToSchemaFour(), downgradeToSchemaOne(), downgradeToSchemaThree(), downgradeToSchemaTwo(), TestAnUpgradedDatabaseGainsChangesWithoutLosingIdentities(), TestAnUpgradedDatabaseGainsSymbolIdentityWithoutLosingSymbols() (+3 more)

### Community 245 - "MR-005 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000004_index.sql`, 6. Parsing rules, 7. Incremental reindex and resume (+3 more)

### Community 246 - "NewLoader"
Cohesion: 0.24
Nodes (14): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers() (+6 more)

### Community 247 - "newInvocation"
Cohesion: 0.13
Nodes (19): globalFlags, humanRenderer, log/slog.Logger, Options, newDoctorCommand(), runDoctor(), invocation, blockedReason() (+11 more)

### Community 248 - "writability_test.go"
Cohesion: 0.33
Nodes (11): assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), checkNamed(), initGitRepo(), newRepoUnderAnUnrepresentablePath(), TestANonUTF8RepositoryPathIsRefusedRatherThanMangled(), TestAnUnwritableRuntimeDatabaseIsNotReportedHealthy() (+3 more)

### Community 249 - "Step"
Cohesion: 0.31
Nodes (5): Recorder, stepRecorder, App, Step, Steps()

### Community 250 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 251 - "Mindrail 0.1 — Uygulama Görev Listesi"
Cohesion: 0.40
Nodes (5): 0.1 Dışında Tutulacak Backlog, 0.1 Kabul Kriteri İzlenebilirlik Matrisi, Bağımlılık Dalgaları, Kullanım, Mindrail 0.1 — Uygulama Görev Listesi

### Community 252 - "TestEveryStartFailureReachesTheVerdict"
Cohesion: 0.39
Nodes (8): blockAccess(), blockCacheCreation(), createUnmigratedDatabase(), TestConcurrentFirstRunIsNotAWorkspaceFailure(), TestEveryStartFailureReachesTheVerdict(), TestInitFinishesWhenOnlyTheCacheDirectoryIsUnusable(), TestInitStopsWhenTheRuntimeRootIsUnusable(), writeConfig()

### Community 253 - "assertNoErrorEnvelope"
Cohesion: 0.36
Nodes (7): TestAnOccupiedCacheDirectoryIsNotAnObstruction(), assertNoErrorEnvelope(), TestFirstRunInFlightIsReportedAsNotInitialized(), newRepoWithUnrepresentablePath(), TestARepresentableRepositoryIsUnaffectedByThePreflight(), TestInitRefusesAnUnrepresentableRepositoryBeforeWriting(), TestTheUnrepresentablePathRemediesBothWork()

### Community 254 - "MintFor"
Cohesion: 0.33
Nodes (3): TestTheRequestHashIsStableAndSensitive(), TestAMintedSessionStartsNoLaterThanTheRowItAttributes(), MintFor()

### Community 255 - "TestWriteModeMigratesNothingAndRegistersNothing"
Cohesion: 0.67
Nodes (5): countRows(), execOnDB(), takeTheSchemaBackOneMigration(), TestWriteModeMigratesNothingAndRegistersNothing(), unregisterEveryWorktree()

### Community 256 - "Check"
Cohesion: 0.13
Nodes (23): Check, CheckFunc, errorFrom(), Report, Result, Runner, State, Subject (+15 more)

### Community 257 - "1. Decisions this document freezes"
Cohesion: 0.12
Nodes (17): 0. Baseline evidence, 1. Decisions this document freezes, 3. Traceability to the task list's four acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-53 — the coordination tables are a second migration, and the reported runtime schema version becomes 2, D-54 — "Checkpoint" means two things in this repository, and neither is renamed, D-55 — the task state machine is a total function, stated once, and an illegal transition is a named refusal (+9 more)

### Community 258 - "database/sql.DB"
Cohesion: 0.18
Nodes (21): database/sql.Conn, database/sql.DB, boundCheckpointWait(), Checkpoint(), checkpointFailure(), checkpointOutcome(), fileSize(), TestAFailedCheckpointIsReportedAsOne() (+13 more)

### Community 259 - ".RoundTrip"
Cohesion: 0.50
Nodes (3): refusingTransport, net/http.Request, net/http.Response

### Community 260 - "2. Decisions"
Cohesion: 0.08
Nodes (24): 0. Baseline evidence, 1. The scenario, in one paragraph, 2. Decisions, 3. Requirements and acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, 6. Traceability to the task list's five acceptance criteria, D-80 — the index lands in `internal/index/`, and the deviation from §7's tree is named once (+16 more)

### Community 261 - "arrange"
Cohesion: 0.32
Nodes (12): idempotentWriter, Session, arrange(), countRows(), fixture, TestAnOperationIDMustBeAnIdentifier(), TestARecordThatNamesNoEntityIsDamageNotAFreshMint(), TestARefusedOperationRecordsNothing() (+4 more)

### Community 262 - "[x] MR-001 — Yerel repository bootstrap ve tanılama yolu"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-001 — Yerel repository bootstrap ve tanılama yolu

### Community 263 - "[ ] MR-008 — Scope drift ve unregistered change ambiguity"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-008 — Scope drift ve unregistered change ambiguity, Ne yapılacak

### Community 264 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 265 - "[x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri

### Community 266 - "[ ] MR-013 — Yerel completion evidence gate"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, Ne yapılacak

### Community 268 - "[ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi, Ne yapılacak

### Community 271 - "ProjectUnit"
Cohesion: 0.12
Nodes (15): Scheduler, New(), ProjectUnit, blocks(), Service, bindFileUIDs(), Service, splitTarget() (+7 more)

### Community 274 - "TestStepSevenIsExemptFromTheSuppression"
Cohesion: 0.43
Nodes (7): suppressions(), TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided(), TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded(), TestStepSevenIsExemptFromTheSuppression(), TestSuppressionKeysOnTheFileTheReferencedIdWouldOccupy(), problemAt(), suppression

### Community 275 - "MR-007 — Design"
Cohesion: 0.18
Nodes (11): 10. Test plan, 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000006_changes.sql`, 6. Discovery flows, 7. Symbol delta core (+3 more)

### Community 276 - "[x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme

### Community 277 - "[x] MR-004 — Güvenli lease, idempotency ve optimistic revision"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-004 — Güvenli lease, idempotency ve optimistic revision

### Community 278 - "[x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

### Community 280 - "Tracker"
Cohesion: 0.17
Nodes (8): unsafe.Pointer, Install(), TestCanceledAfterParseClosesTree(), TestEveryEmbeddedQueryCompilesAndGrammarABIIsCompatible(), TestNativeResourcesReturnToBaselineAfter1000CyclesPerLanguage(), TestRegistryConstructionFailureClosesEarlierQueries(), Tracker, canceledAfterEntry

### Community 282 - "loadStore"
Cohesion: 0.44
Nodes (7): TestAtMostOneRecordCanBeCanonicalForAnId(), TestOwnershipFollowsTheNameTheDirectoryHolds(), assertSameRefs(), loadStore(), TestTheFixtureStoreIsTheOneTheLoaderProduces(), TestTheRecordsTheLoaderIgnoresAreTheOnesTheValidatorFinds(), writeKnowledgeRecord()

### Community 283 - "[x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması

### Community 284 - "[x] MR-007 — Reconcile-first gerçek değişiklik keşfi"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-007 — Reconcile-first gerçek değişiklik keşfi

### Community 285 - "Görevler"
Cohesion: 0.29
Nodes (7): Görevler, Kabul kriterleri, Kabul kriterleri, [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence, [ ] MR-016 — MCP validation ve completion araçları, Ne yapılacak, Ne yapılacak

### Community 288 - "[ ] MR-011 — Kaynak değişince evidence geçersizleştirme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-011 — Kaynak değişince evidence geçersizleştirme, Ne yapılacak

### Community 289 - "[ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı, Ne yapılacak

### Community 291 - "[ ] MR-014 — MCP bilgi ve bağlam araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-014 — MCP bilgi ve bağlam araçları, Ne yapılacak

### Community 292 - "[ ] MR-015 — MCP koordinasyon ve değişiklik araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-015 — MCP koordinasyon ve değişiklik araçları, Ne yapılacak

### Community 295 - "MR-007 — Bulgular ve kapı kaydı"
Cohesion: 0.06
Nodes (35): Başlangıç ve çalışma ağacı, D-131 — yakınsama bağımsız keşifler üzerinedir (AC-05.1 açıklaması), Ek pinler (bu tur), MR-007 — Bulgular ve kapı kaydı, Reader / Breaker bulguları ve giderim — TASK-01 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-02 kapısı, Reader / Breaker bulguları ve giderim — TASK-03 kapısı (+27 more)

### Community 296 - "[ ] MR-017 — Staged değişiklik için yerel Git enforcement"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-017 — Staged değişiklik için yerel Git enforcement, Ne yapılacak

### Community 297 - "[ ] MR-018 — Hook bypass'a dayanıklı CI verification"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak

### Community 298 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 299 - "migratedIndexSchema"
Cohesion: 0.49
Nodes (9): migratedIndexSchema(), mustIndexSQL(), seedIndexConstraintRows(), TestIdentityAllocationKeyIsUniquePerProjectUnitLanguageKey(), TestIdentityBindingStatusAndSymbolUidAreConstrained(), TestIdentityContainerUidRejectsDanglingParent(), TestIndexStateCheckRejectsUnknownValue(), TestIndexTablesDeclarePrimaryKeys() (+1 more)

### Community 300 - "TestRepoConfigDirObstructionDoesNotOverFire"
Cohesion: 0.52
Nodes (6): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission(), requireModeEnforcement()

### Community 301 - "Lease"
Cohesion: 0.24
Nodes (11): LeaseStatus, TargetKind, FileTarget(), fileTargetInvalid(), Acquisition, Lease, Target, Task (+3 more)

### Community 303 - "github.com/spf13/cobra.Command"
Cohesion: 0.18
Nodes (26): scope, github.com/spf13/cobra.Command, Options, newCheckpointCommand(), newCheckpointWriteCommand(), coordinationScope(), coordinationUnavailable(), invocation (+18 more)

### Community 306 - "subjectWithoutRepository"
Cohesion: 0.47
Nodes (5): TestBlockingComponentIsNeverOneNobodyInspected(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), observationNote(), subjectWithoutRepository()

### Community 307 - "newRepoFixtureNamed"
Cohesion: 0.47
Nodes (5): newRepoFixtureNamed(), TestAbsolutePathKeepsTrailingWhitespace(), TestBooleanAnswersStillTolerateWhitespace(), TestResolveRealRepositoryOverFireOnUnusualPathNames(), TestResolveRealRepositoryWithTrailingSpaceInItsPath()

### Community 308 - "status/knowledge_findings_test.go"
Cohesion: 0.67
Nodes (3): rowValue(), TestStatusBuildsNoValidatorOfItsOwn(), TestTheFindingsCountIsQualifiedByObservation()

### Community 313 - "NewStore"
Cohesion: 0.47
Nodes (5): NewStore(), TestAMoveIsStampedUnderTheWriteLock(), refuseWrites(), restrict(), TestAFailedWriteIsNamedByTheStorageLayerFirst()

## Knowledge Gaps
- **1441 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `invocation`, `invocation`, `racedTask` (+1436 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1657 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **21 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `Check`, `Clock`, `database/sql.DB`, `refuseUnrepresentableJSON`, `ProbeWriteAccess`, `App`, `git/adapter.go`, `Probe`, `io.Writer`, `config/loader.go`, `Code`, `New`, `InTx`, `healthySubject`, `context.Context`, `RuntimePathCheck`, `newLoader`, `Build`, `invalidInput`, `Lease`, `Config`, `github.com/spf13/cobra.Command`, `Probe`, `subjectWithoutRepository`, `Warning`, `ExitCode`, `newRepo`, `TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository`, `.withOperation`, `DefaultChecks`, `task.go`, `time.Time`, `DB`, `runner.go`, `newInvocation`, `Root`, `WriteFailure`, `RootKind`, `time.Duration`, `TestEveryStartFailureReachesTheVerdict`, `NewRootWith`?**
  _High betweenness centrality (0.042) - this node is a cross-community bridge._
- **Why does `fixtureContext()` connect `PayloadOf` to `newRepoFixtureNamed`, `testing.T`, `context.Context`, `TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository`?**
  _High betweenness centrality (0.015) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `Check`, `Clock`, `database/sql.DB`, `refuseUnrepresentableJSON`, `newRefreshFixture`, `App`, `hostilefs_linux_test.go`, `indexStore`, `git/adapter.go`, `ResolveRuntimePaths`, `Probe`, `NewRoot`, `io.Writer`, `explain`, `git/classify_test.go`, `runWith`, `newFixture`, `errorStore`, `New`, `InTx`, `newServiceFixture`, `newStore`, `context.Context`, `adapter_test.go`, `newLoader`, `invalidInput`, `Open`, `TestRepoConfigDirObstructionDoesNotOverFire`, `indexerFixture`, `github.com/spf13/cobra.Command`, `Subject`, `Warning`, `ExitCode`, `New`, `leaseFixture`, `NewStore`, `readonlyfs_linux_test.go`, `newRepo`, `TestDanglingGitFileIsNotDiagnosedAsAnAbsentRepository`, `writable_test.go`, `NamedSession`, `runner_test.go`, `DefaultChecks`, `scaffold_hostilefs_linux_test.go`, `time.Time`, `contract_test.go`, `fulldisk_linux_test.go`, `NewError`, `DB`, `WriteIfAbsent`, `runner.go`, `NewLoader`, `newInvocation`, `WriteFailure`, `time.Duration`, `TestEveryStartFailureReachesTheVerdict`, `index/identity_test.go`, `FreeSpace`?**
  _High betweenness centrality (0.014) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **Are the 76 inferred relationships involving `run()` (e.g. with `realGitRepo()` and `assertTheLoopTerminates()`) actually correct?**
  _`run()` has 76 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `invocation` to the rest of the system?**
  _1441 weakly-connected nodes found - possible documentation gaps or missing edges._