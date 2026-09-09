# Graph Report - Mindrail  (2026-09-09)

## Corpus Check
- 280 files · ~487,262 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 3752 nodes · 9819 edges · 243 communities (221 shown, 14 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 1663 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `ce0c7088`
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
- .requireExit
- New
- status/render_test.go
- database/sql.DB
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- context.Context
- NewError
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
- ProbeWriteAccess
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
- runWith
- MEDIUM
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
- subject
- Check
- report.go
- validator_test.go
- Probe
- 47. Human approval kimlik ve yetkilendirme modeli
- Clock
- time.Time
- State
- IsRegistered
- NewRegistry
- WriteFailure
- TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline
- Config
- Appendix E — what the third remediation did
- NewRootWith
- goldenReport
- coherence_test.go
- 5. Test plan
- mindrail-0.1-kernel-scope.md
- shutdown
- refuseUnrepresentableJSON
- record/errors.go
- 2. Requirements
- WriteIfAbsent
- NewID
- storage/access_other.go
- agreement_hostilefs_linux_test.go
- 3. IN Scope
- TestRepoConfigDirObstructionDoesNotOverFire
- schema_ledger_test.go
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- FreeSpace
- writable.go
- NewLoader
- 2. Public API per package
- NewDecision
- fulldisk_linux_test.go
- MR-002 — audit package
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- scaffold_hostilefs_linux_test.go
- explain
- 121. Failure modes
- Supersede
- LOW
- [ ] MR-013 — Yerel completion evidence gate
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- MR-001 — Implementation design
- 2. Mindrail'in temel problemi
- [ ] MR-016 — MCP validation ve completion araçları
- [x] MR-001 — Yerel repository bootstrap ve tanılama yolu
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- requireValid
- created_at
- runCoordination
- MR-002 — Versioned Decision/Invariant knowledge lifecycle
- Store
- newStore
- scaffold_containment_test.go
- builder
- timestamp_ground_test.go
- newInvocation
- ClassifyRefusal
- storage/arch_test.go
- github.com/spf13/cobra.Command
- NewValidator
- load.go
- runtimeDBPath
- [ ] MR-018 — Hook bypass'a dayanıklı CI verification
- version.go
- 103. Test weakening ve false-green guard
- 28. Interactive performance contract
- 159. First Implementation Milestone
- 33. Durable Symbol Identity Implementation
- 56. Resolver Process Supervision
- 12. Content-addressed index cache
- 36. Impact explosion ve drill-down modeli
- 3. Mindrail ne değildir?
- graph
- TestStartupStepOrderMatchesSpec
- [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü
- 3. Round 2: what closing them cost
- validator.go
- Görevler
- runtime.go
- status/knowledge_findings_test.go
- Open
- [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- run
- [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- TASK-01 dependency + schema.Validator
- TASK-02 internal/knowledge/record
- TASK-03 loader RecordRef.Body
- TASK-04+05 app codes + validate.Check
- TASK-06 bootstrap/doctor/status wiring
- TASK-07 CLI classifier + agreement matrix
- TASK-08 independent mutation sweep
- TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed
- TestEveryStartFailureReachesTheVerdict
- TestNoTransactionOpenDuringGitOrFilesystemWork
- MR-002 audit package — appendix: implementer self-reports
- load_test.go
- InTx
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- 2. Round 1: what was actually wrong
- FakeRunner
- TestAReaderStaysCoherentWhileTheStoreIsRewritten
- TestNoGoGitDependency
- runRacer
- .RoundTrip
- newCheckpointCommand
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
- `sessionCount()` --calls--> `runtimeDBPath()`  [INFERRED]
  internal/cli/coordination_test.go → internal/cli/contract_test.go
- `TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer()` --calls--> `observationNote()`  [INFERRED]
  internal/status/observation_test.go → internal/status/render.go
- `TestACycleBlocksAndAnInvalidRecordDoesNot()` --calls--> `Build()`  [INFERRED]
  internal/status/knowledge_findings_test.go → internal/status/report.go
- `TestKnowledgeFindingsAreCountedFromTheSubject()` --calls--> `Build()`  [INFERRED]
  internal/status/knowledge_findings_test.go → internal/status/report.go
- `TestTheFindingsCountIsQualifiedByObservation()` --calls--> `Build()`  [INFERRED]
  internal/status/knowledge_findings_test.go → internal/status/report.go

## Import Cycles
- None detected.

## Communities (243 total, 14 thin omitted)

### Community 0 - "MINDRAIL PROTOCOL Managed Section"
Cohesion: 0.33
Nodes (6): Graphify Protocol, Managed Section Markers, MINDRAIL PROTOCOL Managed Section, Not Yet Active Status Note, Protocol Call Sequence, Soft Enforcement Note

### Community 1 - "mindrail-0.1-task-list.md"
Cohesion: 0.28
Nodes (5): 0.1 Dışında Tutulacak Backlog, 0.1 Kabul Kriteri İzlenebilirlik Matrisi, Bağımlılık Dalgaları, Kullanım, Mindrail 0.1 — Uygulama Görev Listesi

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
Cohesion: 0.09
Nodes (17): App, InitResult, Mode, Options, io/fs.FS, sync.Once, CauseOf(), asKnowledgeError() (+9 more)

### Community 7 - "testing.T"
Cohesion: 0.09
Nodes (38): testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestFixedClockSatisfiesClock(), TestFormatTimeIsUTCRFC3339() (+30 more)

### Community 8 - "MR-002 — findings"
Cohesion: 0.07
Nodes (27): 1. The number that matters, 4. Status, 5. What generalises to MR-003, Appendix A — round 1 findings, as confirmed, Appendix B — what the remediation did, Appendix C — round 2 findings, as confirmed, Appendix D — round 3 findings, as confirmed, Appendix F — round 4 findings, as confirmed (+19 more)

### Community 9 - "RootKind"
Cohesion: 0.19
Nodes (17): os.FileInfo, knowledgeDirRefusal(), TestRepositoryRootIsItsOwnKind(), ensureDir(), unwritableError(), dangling(), root, RootKind (+9 more)

### Community 10 - "adapter.go"
Cohesion: 0.07
Nodes (52): Adapter, ExecRunner, gitFileFault, standpoint, worktreeLinkage, os/exec.Cmd, bareRepositoryError(), detachedWorktreeError() (+44 more)

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
Cohesion: 0.13
Nodes (17): Envelope, sessionResult, taskListResult, io.Writer, os.File, ColorEnabled(), Warning, hasEnvKey() (+9 more)

### Community 16 - "config/loader.go"
Cohesion: 0.20
Nodes (14): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+6 more)

### Community 17 - "New"
Cohesion: 0.13
Nodes (52): testing/fstest.MapFS, FormatTime(), Load(), New(), embeddedSet(), fixedClock(), newDB(), tableExists() (+44 more)

### Community 18 - "NewRoot"
Cohesion: 0.15
Nodes (18): TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize(), newTestRoot() (+10 more)

### Community 19 - "contract_test.go"
Cohesion: 0.12
Nodes (41): newNonRepositoryDir(), TestEveryUninitialisedSpellingExitsZero(), addWorktree(), assertGolden(), corruptDatabase(), createUnmigratedDatabase(), denyAccess(), denyWrites() (+33 more)

### Community 20 - ".requireExit"
Cohesion: 0.17
Nodes (15): brokenSetup, result, assertFourErrorKeys(), assertShape(), isEmptyValue(), runBare(), TestANearMissIsOfferedTheCommandItAlmostSpelled(), TestEveryCommandInTheTreeRejectsABadLineTheSameWay() (+7 more)

### Community 21 - "New"
Cohesion: 0.21
Nodes (28): New(), assertNoRuntimeState(), corruptDatabase(), exists(), initialize(), newGitRepo(), options(), productDirUnder() (+20 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.20
Nodes (20): InitReport, TestKnowledgeFindingsAreCountedFromTheSubject(), assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport() (+12 more)

### Community 23 - "database/sql.DB"
Cohesion: 0.23
Nodes (12): database/sql.Conn, database/sql.DB, boundCheckpointWait(), Checkpoint(), checkpointFailure(), checkpointOutcome(), fileSize(), TestAFailedCheckpointIsReportedAsOne() (+4 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (33): allEscapeRoot(), alsoHeading(), degradedAccount(), degradedImpact(), degradedSummary(), describeBounded(), describeFindings(), describeProblems() (+25 more)

### Community 25 - "healthySubject"
Cohesion: 0.12
Nodes (54): tail, KnowledgeCheck(), assertDiagnosable(), Result, healthySubject(), TestKnowledgeCheckAbsentTreeIsOK(), TestSchemaUnsupportedRemediesAreActions(), TestUnreadableRecordRemediesNameTheFiles() (+46 more)

### Community 26 - "PayloadOf"
Cohesion: 0.06
Nodes (112): Error, main(), context.CancelFunc, RootContext(), ErrorPayload, PayloadOf(), Denied(), ExitCode() (+104 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "context.Context"
Cohesion: 0.18
Nodes (12): context.Context, Migration, bookkeepingCreateFailure(), checksumFailure(), Applied, Migrator, highestVersion(), namesAbsentFrom() (+4 more)

### Community 29 - "NewError"
Cohesion: 0.09
Nodes (47): AdoptCause(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain(), TestDomainErrorCarriesFiveFields() (+39 more)

### Community 30 - "RuntimePathCheck"
Cohesion: 0.27
Nodes (14): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+6 more)

### Community 31 - "runInit"
Cohesion: 0.20
Nodes (14): blockingSummary(), Options, initReportOf(), initVerdict(), newInitCommand(), runInit(), schemaIsCurrent(), TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee() (+6 more)

### Community 32 - "newLoader"
Cohesion: 0.08
Nodes (75): encoding/json.RawMessage, io/fs.DirEntry, reflect.Type, testing.B, decisionRef(), decodeObjects(), describeRefs(), everyKey() (+67 more)

### Community 33 - "Build"
Cohesion: 0.14
Nodes (32): github.com/PsyChaos/mindrail/internal/workspace.Workspace, Subject, TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), TestRepositoryBlockIsObservedOnAHealthyRepository() (+24 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "1. Decisions this document freezes"
Cohesion: 0.05
Nodes (41): 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000002_coordination.sql`, 6. The state table, 7. Command surface, 8. Status integration (+33 more)

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

### Community 48 - "coordination/store_test.go"
Cohesion: 0.24
Nodes (21): taskResult, fixture, Task, ids(), newFixture(), newStepClock(), openFixture(), TestABlockCarriesItsReasonAndLeavingClearsIt() (+13 more)

### Community 49 - "ProbeWriteAccess"
Cohesion: 0.29
Nodes (21): ProbeWriteAccess(), assertProbeMatchesSQLite(), assertUnwritablePayload(), chmodForTest(), dirEntryNames(), initialisedDatabase(), requireModeBitsAreEnforced(), TestProbeWriteAccessAgreesWithSQLite() (+13 more)

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
Cohesion: 0.18
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
Cohesion: 0.21
Nodes (6): Report, State, severity(), States(), TestStateEnumIsExactlyFiveValues(), Component

### Community 67 - "Reader Protocol — Conformance"
Cohesion: 0.22
Nodes (8): R1 — Task normalization, R2 — Requirement verdicts, R3 — Scope compliance, R4 — Contract and documentation conformance, R5 — Evidence standard, R6 — Severity and confidence, R7 — Reader report, Reader Protocol — Conformance

### Community 68 - "graphify reference: extra exports and benchmark"
Cohesion: 0.22
Nodes (8): graphify reference: extra exports and benchmark, Step 6b - Wiki (only if --wiki flag), Step 7 - Neo4j export (only if --neo4j or --neo4j-push flag), Step 7a - FalkorDB export (only if --falkordb or --falkordb-push flag), Step 7b - SVG export (only if --svg flag), Step 7c - GraphML export (only if --graphml flag), Step 7d - MCP server (only if --mcp flag), Step 8 - Token reduction benchmark (only if total_words > 5000)

### Community 69 - "Check"
Cohesion: 0.07
Nodes (127): TestAtMostOneRecordCanBeCanonicalForAnId(), TestOnlyAByteIdenticalFileNameOwnsAnId(), TestOwnershipFollowsTheNameTheDirectoryHolds(), TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(), TestADecisionAndAnInvariantNeverShareAGraphNode(), TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle(), TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages(), TestAnIdNoFileIsNamedAfterSuppliesNoEdges() (+119 more)

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

### Community 83 - "runWith"
Cohesion: 0.20
Nodes (15): strictEnvelope, configPath(), recordFutureMigration(), runWith(), assertEnvelopeCoherent(), decodeStrictEnvelope(), envelopeScenarios(), lineWithPrefix() (+7 more)

### Community 84 - "MEDIUM"
Cohesion: 0.25
Nodes (8): F04, F08 — a rejected command line exited 1 and wrote no envelope, F05 — the agreement matrix never read the remedy it claimed to carry out, F06 — no matrix row for a `config.toml` that cannot be opened, F07 — the linked-worktree cross-check compared two fields of four, F09 — the read-only-media sentinel had no row in the classifier's table, F10 — `sameDirectory`'s identity comparison was untested, F11 — the linked-worktree back-pointer check was untested, MEDIUM

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
Cohesion: 0.18
Nodes (31): answer, condition, agreeingCommands(), agreementConditions(), answerFrom(), answerOf(), assertExemptionsAreStillEarned(), assertRemedyIsWhatTheRowExpects() (+23 more)

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
Cohesion: 0.36
Nodes (6): Components(), ComponentName, Readiness, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues()

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "db.go"
Cohesion: 0.12
Nodes (27): time.Duration, timeoutError(), busyBudget(), classifyOpenError(), classifyPragmaMismatch(), corruptFailure(), DB, Options (+19 more)

### Community 110 - "subject"
Cohesion: 0.16
Nodes (25): expectedPath(), idFromPath(), activeSubjects(), Step, invalid(), nameList(), plural(), schemaClause() (+17 more)

### Community 111 - "Check"
Cohesion: 0.12
Nodes (23): Check, CheckFunc, findCheck(), checkNamed(), errorFrom(), Report, Result, Runner (+15 more)

### Community 112 - "report.go"
Cohesion: 0.19
Nodes (18): classify(), componentFrom(), coordinationInfo(), Component, ComponentName, Readiness, Report, observationOf() (+10 more)

### Community 113 - "validator_test.go"
Cohesion: 0.23
Nodes (21): mustReadEmbedded(), embeddedValidator(), located(), registryFrom(), TestBothDocumentsStateTheSameCreatedAtRule(), TestValidatorAcceptsAnExtraSchemaDocumentAlongsideV1(), TestValidatorAcceptsARecordThatSatisfiesItsSchema(), TestValidatorAssertsTheCreatedAtPattern() (+13 more)

### Community 114 - "Probe"
Cohesion: 0.16
Nodes (18): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject (+10 more)

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "Clock"
Cohesion: 0.27
Nodes (11): Clock, Registration, Store, NewStore(), scanWorkspace(), upsertProject(), upsertWorkspace(), validateRegistration() (+3 more)

### Community 117 - "time.Time"
Cohesion: 0.18
Nodes (12): FixedClock, SystemClock, stepClock, sync.Mutex, time.Time, everyTimestampSpelling(), readByInvariantRecord(), readByRecord() (+4 more)

### Community 118 - "State"
Cohesion: 0.19
Nodes (14): lifecycleLiteralsOutsideThisPackage(), moduleRoot(), CanTransition(), State, ParseState(), States(), TestARefusalCanSayWhatIsAvailableInstead(), TestATaskNeverMovesToTheStateItIsAlreadyIn() (+6 more)

### Community 119 - "IsRegistered"
Cohesion: 0.27
Nodes (11): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+3 more)

### Community 120 - "NewRegistry"
Cohesion: 0.16
Nodes (9): Registry, NewRegistry(), TestEmbeddedSchemaDocumentIsACopy(), TestEmbeddedSchemasPresent(), TestNewRegistryAcceptsExtraDocuments(), TestNewRegistryRejectsIncompleteOrCorruptFS(), TestSupportsRejectsNewerVersion(), TestVersionWindowIsWriteOneReadableOne() (+1 more)

### Community 121 - "WriteFailure"
Cohesion: 0.14
Nodes (27): DomainError, busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), IsBusy(), IsDiskFull(), IsReadOnly() (+19 more)

### Community 122 - "TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline"
Cohesion: 0.29
Nodes (22): TestOptionsThatAreNotNilAreStillApplied(), invariantProperties(), propertyOf(), TestConstructorsAcceptEveryShapeTheirSchemasAllow(), TestConstructorsCopyEverythingTheCallerStillHolds(), TestNewDecisionRefusesWhatItsSchemaWouldReject(), TestNewInvariantHasNoParameterForAPropertyItsSchemaDoesNotDeclare(), TestNewInvariantRefusesWhatItsSchemaWouldReject() (+14 more)

### Community 123 - "Config"
Cohesion: 0.20
Nodes (15): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+7 more)

### Community 124 - "Appendix E — what the third remediation did"
Cohesion: 0.40
Nodes (5): Appendix E — what the third remediation did, Graded against the whole state space, not a fixture set, The rule, in two lines of code, The twelve, and what each one took, What was deliberately not done

### Community 125 - "NewRootWith"
Cohesion: 0.14
Nodes (14): classifiedArgs, envelopeScenario, Root, github.com/spf13/cobra.Completion, github.com/spf13/cobra.PositionalArgs, github.com/spf13/cobra.ShellCompDirective, TestOneTreeCanBeExecutedTwice(), Execute() (+6 more)

### Community 126 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 127 - "coherence_test.go"
Cohesion: 0.21
Nodes (17): remedy, remedyClass, agreementRemedyClass(), TestEveryRemedyThatNamesAPathIsClassified(), absolutePathsIn(), assertDocumentIsCoherent(), classOf(), contradictionsBetween() (+9 more)

### Community 128 - "5. Test plan"
Cohesion: 0.12
Nodes (16): 5.10 `internal/doctor` (Unit D) — 11, 5.11 `internal/status` (Unit D) — 8, 5.12 `internal/bootstrap` (Unit E) — 8, 5.13 `internal/cli` — CLI contract (Unit E) — 14, 5.14 Architecture + smoke (Unit E) — 6, 5.15 Acceptance-criteria mapping, 5.1 `internal/app` (Unit D) — 10, 5.2 `internal/filesystem` (Unit A) — 8 (+8 more)

### Community 129 - "mindrail-0.1-kernel-scope.md"
Cohesion: 0.12
Nodes (14): 1. Goal, 2. Primary End-to-End Scenario, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition, Mindrail 0.1 — Kernel Scope (+6 more)

### Community 130 - "shutdown"
Cohesion: 0.27
Nodes (9): log/slog.Logger, Options, newDoctorCommand(), runDoctor(), shutdown(), Options, newStatusCommand(), readinessOf() (+1 more)

### Community 131 - "refuseUnrepresentableJSON"
Cohesion: 0.24
Nodes (9): reflect.Value, invocation, firstUnrepresentable(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs(), TestRefusalReadsTheErrorPayloadToo(), TestRepresentableDocumentsAreNotRefused(), unrepresentableError() (+1 more)

### Community 132 - "record/errors.go"
Cohesion: 0.20
Nodes (3): DuplicateItemError, EmptyFieldError, OptionKindError

### Community 133 - "2. Requirements"
Cohesion: 0.06
Nodes (36): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation, D-43 — a Finding gets its own codes, because D-38 says it is not a Problem (+28 more)

### Community 134 - "WriteIfAbsent"
Cohesion: 0.28
Nodes (10): DefaultTemplate(), EnsureKnowledgeDirs(), scaffoldError(), TestDefaultTemplateIsNotEmptyAndIsACopy(), TestEnsureKnowledgeDirsCreatesGitkeep(), TestEnsureKnowledgeDirsIsIdempotent(), TestScaffoldDoesNotCreateOutOfScopeFiles(), TestWriteIfAbsentCreatesFromEmbeddedTemplate() (+2 more)

### Community 135 - "NewID"
Cohesion: 0.27
Nodes (9): encodeCrockford(), compareRandomness(), TestABackwardsClockStillMintsAscendingIds(), TestTheEntropyCounterCarriesAcrossAByte(), NewID(), nextEntropy(), TestAnIdIsPrefixedUniqueAndSortsInMintOrder(), TestAnIdLeaksNeitherTheClockNorTheCaller() (+1 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "agreement_hostilefs_linux_test.go"
Cohesion: 0.21
Nodes (22): hostileCondition, agreeingHostileCommands(), assertTheLoopTerminates(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo(), hostileAvailableBytes(), hostileConditions() (+14 more)

### Community 140 - "3. IN Scope"
Cohesion: 0.12
Nodes (16): 3. IN Scope, Basic impact, Change discovery, Completion gate, Coordination, Core executable, Diagnostics, Engineering knowledge (+8 more)

### Community 141 - "TestRepoConfigDirObstructionDoesNotOverFire"
Cohesion: 0.60
Nodes (5): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission()

### Community 142 - "schema_ledger_test.go"
Cohesion: 0.28
Nodes (15): openDB(), assertRemedyIsNotTheFailingCommand(), assertUserFacing(), assertUserFacingCode(), holdWriteLock(), mustFailUp(), openContendedDB(), TestAGenuineMigrationFailureIsStillAMigrationFailure() (+7 more)

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "FreeSpace"
Cohesion: 0.13
Nodes (18): FreeSpace, TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal(), statFreeSpace(), ProbeFreeSpace() (+10 more)

### Community 145 - "writable.go"
Cohesion: 0.21
Nodes (12): TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError(), conditionBlocker(), WriteAccess, WriteBlocker (+4 more)

### Community 146 - "NewLoader"
Cohesion: 0.36
Nodes (12): NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestLoadWithNoFilesUsesDefaults(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers(), TestProvenanceReportsSourceLayer(), TestStrictDecodeRejectsUnknownKey() (+4 more)

### Community 147 - "2. Public API per package"
Cohesion: 0.14
Nodes (14): 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D, 2.2 `internal/filesystem` — Unit A, 2.3 `internal/git` — Unit A, 2.4 `internal/storage` — Unit B (+6 more)

### Community 148 - "NewDecision"
Cohesion: 0.33
Nodes (8): TestAConstructorNamesThePositionOfANilOption(), Decision, NewDecision(), NewInvariant(), stampedTime(), TestAConstructorAgreesWithScopeValidateAboutTheDriveBoundary(), TestEveryInstantTheWriterCanStampRendersAsASpellingTheContractAccepts(), TestTheWriterStillRefusesTheInstantItAlwaysRefused()

### Community 149 - "fulldisk_linux_test.go"
Cohesion: 0.15
Nodes (24): afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero(), fullDiskChild(), fullDiskScenario(), initialiseOnDisk() (+16 more)

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

### Community 154 - "explain"
Cohesion: 0.11
Nodes (28): diagnosis, halt, step, configResult(), describePath(), describeRuntimeLocation(), describeStartDir(), describeUninitializedDB() (+20 more)

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "Supersede"
Cohesion: 0.23
Nodes (12): TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(), TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(), Decision, Decision, Supersede(), activeDecision(), TestSupersedeDoesNotRepeatATargetItAlreadyCarries(), TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout() (+4 more)

### Community 157 - "LOW"
Cohesion: 0.11
Nodes (17): F01 — `mindrail init` reported READY over a repository the next command refused, F02 — an unopenable `config.toml` was remedied as a malformed one, F03 — a size limit was reported as a full disk, F12 — a defective build blamed the repository, F13 — the rebuild remedy did not name the database, F14 — three unexercised claims in the init report path, F15 — the containment remedy named no path, F16 — a guard that could not fail (+9 more)

### Community 158 - "[ ] MR-013 — Yerel completion evidence gate"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, Ne yapılacak

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "MR-001 — Implementation design"
Cohesion: 0.11
Nodes (17): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 3. Dependency direction, 4. Work units, 6. Explicit non-goals, 7. Open questions and decisions, Decisions (+9 more)

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

### Community 172 - "requireValid"
Cohesion: 0.29
Nodes (9): createdAtOf(), encode(), readGolden(), requireValid(), TestConstructorsSerializeCreatedAtAsUTCRFC3339(), TestNewDecisionStampsWhatTheSchemaRequires(), TestNewInvariantStampsWhatTheSchemaRequires(), TestGoldenRecordsRoundTripByteForByte() (+1 more)

### Community 173 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

### Community 174 - "runCoordination"
Cohesion: 0.38
Nodes (8): scope, humanRenderer, coordinationScope(), coordinationUnavailable(), Options, newSessionCommand(), newSessionOpenCommand(), runCoordination()

### Community 175 - "MR-002 — Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.22
Nodes (9): 0. What MR-001 left here, exactly, 1. Scope decision: no CLI write surface, 2. Package plan, 3. Frozen signatures, 4. Decisions, 5. Test plan, 6. Explicit non-goals, 7. Acceptance criteria traceability (+1 more)

### Community 176 - "Store"
Cohesion: 0.10
Nodes (23): checkpointResult, handoverResult, coordination.Checkpoint, coordination.CheckpointRef, coordination.Handover, rowScanner, coordination.Summary, database/sql.Tx (+15 more)

### Community 177 - "newStore"
Cohesion: 0.32
Nodes (14): newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace(), TestRegisteredIDsAreOpaqueAndPrefixed(), TestRegisterIsIdempotent() (+6 more)

### Community 178 - "scaffold_containment_test.go"
Cohesion: 0.23
Nodes (11): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), requireEmptyTree(), requireEscape(), requireModeEnforcement(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository() (+3 more)

### Community 179 - "builder"
Cohesion: 0.16
Nodes (12): regexp.Regexp, newBuilder(), Decision, checkRepoRelative(), isScopeLevel(), isWindowsDriveRooted(), builder, Invariant (+4 more)

### Community 180 - "timestamp_ground_test.go"
Cohesion: 0.29
Nodes (11): createdAtDescription(), everyDescriptionClaim(), everyRefusedSpelling(), measuredGround(), TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(), TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(), TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(), TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository() (+3 more)

### Community 181 - "newInvocation"
Cohesion: 0.28
Nodes (9): globalFlags, humanRenderer, blockedReason(), globalFlagsOf(), invocation, Options, newInvocation(), resolveStartDir() (+1 more)

### Community 182 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 183 - "storage/arch_test.go"
Cohesion: 0.33
Nodes (10): go/ast.ImportSpec, fileImports(), importPath(), moduleRoot(), packageImports(), shouldSkipDir(), TestDriverImportConfinedToStorage(), TestDriverImportUnreachableFromOtherPackages() (+2 more)

### Community 184 - "github.com/spf13/cobra.Command"
Cohesion: 0.60
Nodes (9): github.com/spf13/cobra.Command, Options, newTaskCommand(), newTaskListCommand(), newTaskOpenCommand(), newTaskShowCommand(), newTaskStateCommand(), parseStateFlag() (+1 more)

### Community 185 - "NewValidator"
Cohesion: 0.27
Nodes (13): documentID(), registryWith(), TestDocumentIDAcceptsAnOrdinaryDocument(), TestDocumentIDRefusesADocumentThatNamesItselfNothing(), TestNewValidatorAcceptsARegistryCarryingEveryDocumentItReads(), TestNewValidatorAcceptsARegistryWhoseTwoHalvesAgree(), TestNewValidatorCompilesEverySchemaAtConstruction(), TestNewValidatorFailsWhenARequiredDocumentIsAbsent() (+5 more)

### Community 186 - "load.go"
Cohesion: 0.25
Nodes (11): alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns(), createdObjects(), SchemaObject (+3 more)

### Community 187 - "runtimeDBPath"
Cohesion: 0.36
Nodes (11): chmodForTest(), runtimeDBPath(), assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), newRepoUnderAnUnrepresentablePath(), TestANonUTF8RepositoryPathIsRefusedRatherThanMangled(), TestAnUnwritableRuntimeDatabaseIsNotReportedHealthy() (+3 more)

### Community 188 - "[ ] MR-018 — Hook bypass'a dayanıklı CI verification"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak

### Community 189 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

### Community 198 - "graph"
Cohesion: 0.38
Nodes (5): compareNodes(), ids(), newGraph(), graph, node

### Community 199 - "TestStartupStepOrderMatchesSpec"
Cohesion: 0.29
Nodes (6): Recorder, stepRecorder, TestStartupStepOrderMatchesSpec(), App, Step, Steps()

### Community 200 - "[x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

### Community 201 - "3. Round 2: what closing them cost"
Cohesion: 0.50
Nodes (4): 3. Round 2: what closing them cost, 🟠 The containment fix cost a third of the status budget, 🔴 The one that matters — a stray draft file makes a healthy repository BLOCKED, The remaining nine

### Community 202 - "validator.go"
Cohesion: 0.12
Nodes (24): mustCompileSchemas(), describe(), findingsFor(), shippedValidator(), TestGoldenRecordsSatisfyTheirShippedSchemaDocument(), collectFindings(), escapePointerToken(), Finding (+16 more)

### Community 203 - "Görevler"
Cohesion: 0.50
Nodes (4): Görevler, Kabul kriterleri, [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision, Ne yapılacak

### Community 204 - "runtime.go"
Cohesion: 0.67
Nodes (3): hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 205 - "status/knowledge_findings_test.go"
Cohesion: 0.38
Nodes (6): findingOn(), rowValue(), TestACycleBlocksAndAnInvalidRecordDoesNot(), TestStatusBuildsNoValidatorOfItsOwn(), TestTheFindingsCountIsQualifiedByObservation(), subjectWithKnowledgeFinding()

### Community 206 - "Open"
Cohesion: 0.10
Nodes (34): testing.M, IsUnwritten(), Open(), openTemp(), TestCloseIsIdempotent(), TestExpectedPragmasHonoursBusyTimeout(), TestForeignKeysAreEnforced(), TestOpenAppliesPragmasOnConcurrentConnections() (+26 more)

### Community 207 - "[ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme, Ne yapılacak

### Community 208 - "run"
Cohesion: 0.17
Nodes (33): TestAnOccupiedCacheDirectoryIsNotAnObstruction(), TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(), TestHealthyRepositoryIsUntouchedByRemedyCoherence(), assertNoErrorEnvelope(), decodeData(), newInitializedRepo(), run(), TestDoctorJSONContract() (+25 more)

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

### Community 219 - "TestEveryStartFailureReachesTheVerdict"
Cohesion: 0.39
Nodes (8): blockAccess(), blockCacheCreation(), createUnmigratedDatabase(), TestConcurrentFirstRunIsNotAWorkspaceFailure(), TestEveryStartFailureReachesTheVerdict(), TestInitFinishesWhenOnlyTheCacheDirectoryIsUnusable(), TestInitStopsWhenTheRuntimeRootIsUnusable(), writeConfig()

### Community 220 - "TestNoTransactionOpenDuringGitOrFilesystemWork"
Cohesion: 0.29
Nodes (6): txProbe, sync/atomic.Bool, sync/atomic.Int64, TestNoTransactionOpenDuringGitOrFilesystemWork(), CommandRunner, TxActive()

### Community 222 - "load_test.go"
Cohesion: 0.29
Nodes (7): keys(), TestEachMigrationCreatesOnlyItsMilestonesTables(), TestLoadEmbeddedMigrationsAreOrderedAndUnique(), TestLoadIgnoresNonSQLFiles(), TestLoadRejectsMalformedSets(), TestLoadSortsAscendingRegardlessOfDirectoryOrder(), TestNoDownMigrationsExist()

### Community 223 - "InTx"
Cohesion: 0.36
Nodes (6): InTx(), TestInTxCommitsAndRollsBack(), TestInTxTakesTheWriteLockAtBegin(), TestParameterizedSQLSurvivesInjectionLiteral(), TestTxActiveTracksTheWriteWindow(), txKeyType

### Community 224 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 225 - "2. Round 1: what was actually wrong"
Cohesion: 0.40
Nodes (5): 2. Round 1: what was actually wrong, A containment gap MR-002 made load-bearing, The fabricated claim — step 8 denied a file the same report was correcting, The fail-open — one unknown JSON property disabled the milestone's only fatal check, The measured costs

### Community 226 - "FakeRunner"
Cohesion: 0.60
Nodes (3): FakeResponse, Invocation, FakeRunner

### Community 228 - "TestAReaderStaysCoherentWhileTheStoreIsRewritten"
Cohesion: 0.70
Nodes (4): TestAReaderStaysCoherentWhileTheStoreIsRewritten(), TestTwoCommandsReadOneStoreAtTheSameTime(), withoutTimings(), writeInvariantRecord()

### Community 229 - "TestNoGoGitDependency"
Cohesion: 0.70
Nodes (4): importsOf(), packageSources(), stringLiteralsOf(), TestNoGoGitDependency()

### Community 230 - "runRacer"
Cohesion: 0.60
Nodes (4): assertLedgerAppliedOnce(), runRacer(), TestConcurrentInitAcrossProcessesWaits(), TestMain()

### Community 231 - ".RoundTrip"
Cohesion: 0.50
Nodes (3): refusingTransport, net/http.Request, net/http.Response

### Community 232 - "newCheckpointCommand"
Cohesion: 0.83
Nodes (3): Options, newCheckpointCommand(), newCheckpointWriteCommand()

### Community 233 - "[ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri, Ne yapılacak

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
- **1072 isolated node(s):** `Report`, `InitReport`, `0.1 Dışında Tutulacak Backlog`, `0.1 Kabul Kriteri İzlenebilirlik Matrisi`, `Bağımlılık Dalgaları` (+1067 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1224 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **14 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `refuseUnrepresentableJSON`, `App`, `RootKind`, `adapter.go`, `io.Writer`, `config/loader.go`, `FreeSpace`, `writable.go`, `contract_test.go`, `New`, `database/sql.DB`, `PayloadOf`, `context.Context`, `RuntimePathCheck`, `newLoader`, `Build`, `runCoordination`, `Store`, `newInvocation`, `github.com/spf13/cobra.Command`, `Root`, `Probe`, `TestEveryStartFailureReachesTheVerdict`, `db.go`, `Check`, `Probe`, `Clock`, `IsRegistered`, `WriteFailure`, `Config`, `NewRootWith`?**
  _High betweenness centrality (0.028) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `refuseUnrepresentableJSON`, `App`, `RootKind`, `ResolveRuntimePaths`, `TestRepoConfigDirObstructionDoesNotOverFire`, `schema_ledger_test.go`, `io.Writer`, `FreeSpace`, `New`, `NewLoader`, `contract_test.go`, `.requireExit`, `New`, `NewRoot`, `database/sql.DB`, `fulldisk_linux_test.go`, `scaffold_hostilefs_linux_test.go`, `explain`, `context.Context`, `NewError`, `newLoader`, `Store`, `coordination/store_test.go`, `scaffold_containment_test.go`, `ProbeWriteAccess`, `newStore`, `newInvocation`, `hostilefs_linux_test.go`, `Probe`, `Open`, `runWith`, `TestUnwritablePathStillNamesTheObstructionsItAlreadyNamed`, `TestEveryStartFailureReachesTheVerdict`, `db.go`, `Check`, `Clock`, `WriteFailure`?**
  _High betweenness centrality (0.019) - this node is a cross-community bridge._
- **Why does `Build()` connect `Build` to `shutdown`, `Readiness`, `status/knowledge_findings_test.go`, `db.go`, `Check`, `report.go`, `Probe`, `status/render_test.go`, `checks.go`, `healthySubject`, `explain`, `NewError`, `RuntimePathCheck`, `runInit`?**
  _High betweenness centrality (0.017) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **Are the 8 inferred relationships involving `Check()` (e.g. with `newGraph()` and `stepDuplicateActiveLineage()`) actually correct?**
  _`Check()` has 8 INFERRED edges - model-reasoned connections that need verification._
- **What connects `Report`, `InitReport`, `0.1 Dışında Tutulacak Backlog` to the rest of the system?**
  _1072 weakly-connected nodes found - possible documentation gaps or missing edges._