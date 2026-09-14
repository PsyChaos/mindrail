# Graph Report - Mindrail  (2026-09-14)

## Corpus Check
- 305 files · ~549,226 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 3990 nodes · 10499 edges · 244 communities (220 shown, 16 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 1834 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `77287b68`
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
- Warning
- config/loader.go
- New
- DefaultChecks
- run
- Root
- New
- status/render_test.go
- InTx
- checks.go
- healthySubject
- PayloadOf
- What You Must Do When Invoked
- context.Context
- check_test.go
- RuntimePathCheck
- initReportOf
- newLoader
- Build
- validate.py
- Phase 3 — Doc ↔ Code consistency
- subject
- status/render.go
- Final report
- properties
- Task Verification Report
- Test Suite Health & Redundancy Audit
- TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline
- smoke_test.go
- Breaker Protocol — Falsification
- properties
- Report Template
- Dual-Agent Task Audit
- newFixture
- 3. The findings in full
- mutate.py
- enum
- enum
- Execution (Phases 8–17)
- supersedes
- supersedes
- changeset.py
- test_inventory.py
- hostilefs_linux_test.go
- DB
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
- cli/arch_test.go
- writability_test.go
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
- impacted_tests.py
- 4. The confirmed defects
- MR-002 — Versioned Decision/Invariant knowledge lifecycle
- report.go
- WriteIfAbsent
- States
- 47. Human approval kimlik ve yetkilendirme modeli
- runWith
- writable.go
- NewRoot
- Config
- validator_test.go
- WriteFailure
- coordination_agreement_test.go
- MintFor
- ProbeWriteAccess
- github.com/spf13/cobra.Command
- scaffold_containment_test.go
- coherence_test.go
- 3. IN Scope
- Finding
- steps_test.go
- refuseUnrepresentableJSON
- record/errors.go
- 2. Requirements
- runStatus
- NewID
- storage/access_other.go
- agreement_hostilefs_linux_test.go
- timestamp_ground_test.go
- Appendix B — what the remediation did
- TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore
- 95. Knowledge schema ve CI doğrulama pipeline'ı
- FreeSpace
- builder
- NewLoader
- goldenReport
- MR-003 — design: sequential agent handover
- TestRepoConfigDirObstructionDoesNotOverFire
- MR-002 — audit package
- 29. Large repository cold-index lifecycle
- 11. SQLite concurrency ve interactive write fairness
- Audit round 2
- Check
- 121. Failure modes
- [ ] MR-013 — Yerel completion evidence gate
- LOW
- [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri
- 2. Implementation Principles
- 24. Durable symbol identity, rename ve orphan protection
- 57. Candidate invariant trigger'ları
- scaffold_hostilefs_linux_test.go
- 2. Mindrail'in temel problemi
- newInvocation
- fulldisk_linux_test.go
- 122. Dynamic dispatch ve validation budget
- 14. Code intelligence tiers
- Full support
- 20. Confidence artık global tek sayı değildir
- 61. Multi-agent çalışma modları
- 7. Data plane ayrımı
- [ ] MR-018 — Hook bypass'a dayanıklı CI verification
- created_at
- runCoordination
- NewError
- FormatTime
- contract_test.go
- time.Time
- [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı
- 2. Public API per package
- 5. Test plan
- loader/loader.go
- ParseTime
- Clock
- Open
- SQLiteCheck
- Supersede
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
- 1. Decisions this document freezes
- status/knowledge_findings_test.go
- [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü
- newStore
- validate_test.go
- requireValid
- audit_scope.py
- Code
- [ ] MR-011 — Kaynak değişince evidence geçersizleştirme
- [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision
- [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması
- [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi
- [ ] MR-008 — Scope drift ve unregistered change ambiguity
- TASK-01 dependency + schema.Validator
- TASK-02 internal/knowledge/record
- TASK-03 loader RecordRef.Body
- TASK-04+05 app codes + validate.Check
- TASK-06 bootstrap/doctor/status wiring
- TASK-07 CLI classifier + agreement matrix
- TASK-08 independent mutation sweep
- task.go
- [x] MR-001 — Yerel repository bootstrap ve tanılama yolu
- ClassifyRefusal
- MR-002 audit package — appendix: implementer self-reports
- [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme
- Görevler
- NewDecision
- [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi
- Migrations
- [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence
- [ ] MR-014 — MCP bilgi ve bağlam araçları
- Audit scoping — making audit cost follow risk, not task count
- [ ] MR-016 — MCP validation ve completion araçları
- [ ] MR-017 — Staged değişiklik için yerel Git enforcement
- [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı
- runtime.go
- readonlyfs_linux_test.go
- loadStore
- Appendix F — round 4 findings, as confirmed
- 4. Work units
- MEDIUM
- NoSpaceRefusal
- Appendix E — what the third remediation did
- TestAPatternMessageThatQuotesItsRuleFaithfullyIsStillPublished
- .insertCheckpoint

## God Nodes (most connected - your core abstractions)
1. `PayloadOf()` - 131 edges
2. `NewError()` - 111 edges
3. `Check()` - 92 edges
4. `run()` - 86 edges
5. `shippedValidator()` - 79 edges
6. `storeOf()` - 77 edges
7. `decisionDoc()` - 68 edges
8. `healthySubject()` - 67 edges
9. `decisionAt()` - 67 edges
10. `newInitializedRepo()` - 65 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `RootContext()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/context.go
- `main()` --calls--> `ExitCode()`  [EXTRACTED]
  cmd/mindrail/main.go → internal/app/exit.go
- `TestFormatTimeIsUTCRFC3339()` --calls--> `FormatTime()`  [INFERRED]
  internal/app/clock_test.go → internal/app/clock.go
- `TestDomainErrorCarriesFiveFields()` --calls--> `Code`  [INFERRED]
  internal/app/errors_test.go → internal/app/code.go
- `TestErrorPayloadJSONHasAllFourKeys()` --calls--> `RegisteredCodes()`  [INFERRED]
  internal/app/errors_test.go → internal/app/code.go

## Import Cycles
- None detected.

## Communities (244 total, 16 thin omitted)

### Community 0 - "MINDRAIL PROTOCOL Managed Section"
Cohesion: 0.33
Nodes (6): Graphify Protocol, Managed Section Markers, MINDRAIL PROTOCOL Managed Section, Not Yet Active Status Note, Protocol Call Sequence, Soft Enforcement Note

### Community 1 - "mindrail-0.1-task-list.md"
Cohesion: 0.20
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

### Community 6 - "App"
Cohesion: 0.09
Nodes (14): InitResult, io/fs.FS, AdoptCause(), asKnowledgeError(), asMigrationError(), asMigrationSetError(), asRepositoryError(), asRuntimePathError() (+6 more)

### Community 7 - "testing.T"
Cohesion: 0.08
Nodes (41): testing.T, importsOf(), modulePath(), packageFiles(), TestAppPackageIsALeaf(), TestAppUsesOnlyPermittedStdlib(), TestAReaderStaysCoherentWhileTheStoreIsRewritten(), TestTwoCommandsReadOneStoreAtTheSameTime() (+33 more)

### Community 8 - "MR-002 — findings"
Cohesion: 0.08
Nodes (25): 1. The number that matters, 2. Round 1: what was actually wrong, 3. Round 2: what closing them cost, 4. Status, 5. What generalises to MR-003, A containment gap MR-002 made load-bearing, Appendix A — round 1 findings, as confirmed, Appendix C — round 2 findings, as confirmed (+17 more)

### Community 9 - "RootKind"
Cohesion: 0.23
Nodes (12): os.FileInfo, unwritableError(), dangling(), root, RootKind, RuntimePaths, Writability, nearestExisting() (+4 more)

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

### Community 15 - "Warning"
Cohesion: 0.21
Nodes (13): Envelope, os.File, ColorEnabled(), Warning, hasEnvKey(), isTerminal(), openCharDevice(), regularFile() (+5 more)

### Community 16 - "config/loader.go"
Cohesion: 0.20
Nodes (14): fileConfig, fileOutput, fileProject, Loader, LoaderOptions, Provenance, Source, applyEnv() (+6 more)

### Community 17 - "New"
Cohesion: 0.07
Nodes (91): testing/fstest.MapFS, testing.M, alteredTables(), columnList(), columnName(), firstToken(), stripComments(), tableColumns() (+83 more)

### Community 18 - "DefaultChecks"
Cohesion: 0.12
Nodes (37): App, errorFrom(), Report, State, Subject, haltError(), kindForCode(), NewRunner() (+29 more)

### Community 19 - "run"
Cohesion: 0.14
Nodes (44): TestHealthyRepositoryIsUntouchedByRemedyCoherence(), decodeData(), newInitializedRepo(), run(), TestCoordinationHumanOutputGolden(), TestDoctorJSONContract(), TestJSONGoesToStdoutLogsToStderr(), TestRejectedCommandLineStaysSilentOnStdoutWithoutJSON() (+36 more)

### Community 20 - "Root"
Cohesion: 0.20
Nodes (9): Modes, os.FileMode, TestPrivateModesAreTheOwnerOnlyDefaults(), canonicalize(), canonicalizeHop(), escapeError(), Root, isAbsoluteInput() (+1 more)

### Community 21 - "New"
Cohesion: 0.07
Nodes (58): Recorder, refusingTransport, stepRecorder, txProbe, net/http.Request, net/http.Response, sync/atomic.Bool, sync/atomic.Int64 (+50 more)

### Community 22 - "status/render_test.go"
Cohesion: 0.19
Nodes (21): InitReport, TestKnowledgeFindingsAreCountedFromTheSubject(), assertGolden(), blockedInitReport(), decodeJSON(), Report, lookup(), readyInitReport() (+13 more)

### Community 23 - "InTx"
Cohesion: 0.22
Nodes (12): database/sql.Conn, database/sql.DB, boundCheckpointWait(), Checkpoint(), checkpointFailure(), checkpointOutcome(), InTx(), TestInTxCommitsAndRollsBack() (+4 more)

### Community 24 - "checks.go"
Cohesion: 0.16
Nodes (33): allEscapeRoot(), alsoHeading(), degradedAccount(), degradedImpact(), degradedSummary(), describeBounded(), describeFindings(), describeProblems() (+25 more)

### Community 25 - "healthySubject"
Cohesion: 0.11
Nodes (56): tail, KnowledgeCheck(), assertDiagnosable(), Result, healthySubject(), TestAnUnusableKnowledgeDirectoryIsReportedAsAPathCondition(), TestGitCheckBranches(), TestKnowledgeCheckAbsentTreeIsOK() (+48 more)

### Community 26 - "PayloadOf"
Cohesion: 0.05
Nodes (118): Error, main(), FakeResponse, Invocation, context.CancelFunc, RootContext(), ErrorPayload, PayloadOf() (+110 more)

### Community 27 - "What You Must Do When Invoked"
Cohesion: 0.08
Nodes (24): For /graphify add and --watch, For /graphify query, For the commit hook and native CLAUDE.md integration, For --update and --cluster-only, /graphify, Honesty Rules, Interpreter guard for subcommands, Part A - Structural extraction for code files (+16 more)

### Community 28 - "context.Context"
Cohesion: 0.18
Nodes (12): context.Context, Migration, bookkeepingCreateFailure(), checksumFailure(), Applied, Migrator, highestVersion(), namesAbsentFrom() (+4 more)

### Community 29 - "check_test.go"
Cohesion: 0.26
Nodes (12): State, okCheck(), stateCheck(), stateCheckIn(), TestCheckFuncFillsItsOwnIdentity(), TestCheckFuncWithoutFunctionIsNotSilent(), TestHaltErrorPointsAtTheCheckThatExplainsTheHalt(), TestReportErrExitCodeFollowsTheFailureClass() (+4 more)

### Community 30 - "RuntimePathCheck"
Cohesion: 0.26
Nodes (15): refusalCause, RuntimePathCheck(), Subject, refuseRepoConfig(), TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(), TestAnUnusableCacheDirectoryStillOutranksNothing(), TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(), TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy() (+7 more)

### Community 31 - "initReportOf"
Cohesion: 0.20
Nodes (14): blockingSummary(), Options, initReportOf(), initVerdict(), newInitCommand(), runInit(), schemaIsCurrent(), TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee() (+6 more)

### Community 32 - "newLoader"
Cohesion: 0.10
Nodes (63): encoding/json.RawMessage, reflect.Type, testing.B, decisionRef(), decodeObjects(), describeRefs(), everyKey(), fieldNames() (+55 more)

### Community 33 - "Build"
Cohesion: 0.18
Nodes (28): Subject, TestBlockingComponentIsNeverOneNobodyInspected(), TestBlockingComponentIsStillNamedWhenOneWasInspected(), TestHealthyReportNamesNoBlockingComponent(), TestRepositoryBlockIsMarkedWhenGitIsUnavailable(), TestRepositoryBlockIsObservedOnAHealthyRepository(), TestRepositoryBlockSurvivesALaterStepFailing(), TestHumanReportOmitsTheObservationMarkerWhenItLooked() (+20 more)

### Community 34 - "validate.py"
Cohesion: 0.29
Nodes (19): _cmd(), compare(), detect(), detect_go(), detect_jvm(), detect_node(), detect_python(), detect_ruby() (+11 more)

### Community 35 - "Phase 3 — Doc ↔ Code consistency"
Cohesion: 0.11
Nodes (18): A. Documentation index, A. Features, Audit Phases — Detailed Checklists, B. APIs, B. Codebase index, C. Configuration, D. Architecture, E. Database & data model (+10 more)

### Community 36 - "subject"
Cohesion: 0.16
Nodes (23): compareNodes(), ids(), newGraph(), expectedPath(), idFromPath(), activeSubjects(), Step, invalid() (+15 more)

### Community 37 - "status/render.go"
Cohesion: 0.15
Nodes (19): strings.Builder, Report, Result, State, paint(), writeBlock(), writeResult(), TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer() (+11 more)

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
Cohesion: 0.29
Nodes (22): TestOptionsThatAreNotNilAreStillApplied(), invariantProperties(), propertyOf(), TestConstructorsAcceptEveryShapeTheirSchemasAllow(), TestConstructorsCopyEverythingTheCallerStillHolds(), TestNewDecisionRefusesWhatItsSchemaWouldReject(), TestNewInvariantHasNoParameterForAPropertyItsSchemaDoesNotDeclare(), TestNewInvariantRefusesWhatItsSchemaWouldReject() (+14 more)

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

### Community 48 - "newFixture"
Cohesion: 0.16
Nodes (30): TestAPreconditionReadThatFailsIsReportedAsAReadOfThatRow(), NamedSession(), NewStore(), fixture, ids(), newFixture(), newStepClock(), openFixture() (+22 more)

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

### Community 58 - "hostilefs_linux_test.go"
Cohesion: 0.25
Nodes (14): availableBytes(), checkReadOnlyRemedy(), checkSpaceRemedy(), fillToZero(), hostileRootScenario(), leaveFree(), remountReadOnly(), remountReadWrite() (+6 more)

### Community 59 - "DB"
Cohesion: 0.10
Nodes (31): sync.Once, time.Duration, queryPlan(), schemaOnlyDatabase(), TestTheNewestCheckpointQueryDoesNotSortTheProject(), timeoutError(), busyBudget(), classifyOpenError() (+23 more)

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
Cohesion: 0.18
Nodes (62): TestOnlyAByteIdenticalFileNameOwnsAnId(), TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(), TestADecisionAndAnInvariantNeverShareAGraphNode(), TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle(), TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages(), TestAnIdNoFileIsNamedAfterSuppliesNoEdges(), TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries(), TestAValidStoreIsStillSilentWithAStrayDraftBesideIt() (+54 more)

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

### Community 83 - "cli/arch_test.go"
Cohesion: 0.11
Nodes (34): firstPartyPackage, go/ast.ImportSpec, moduleRoot(), TestTheKnowledgePipelineHasExactlyOneCallSite(), assertMainDeclaresOnlyMain(), assertNoNetworkInLinkedPackages(), fileImports(), firstPartyPackages() (+26 more)

### Community 84 - "writability_test.go"
Cohesion: 0.26
Nodes (15): chmodForTest(), assertReportsAnUnwritableDatabase(), assertRepositoryWorksAgain(), carryOutWriteRemedy(), checkNamed(), initGitRepo(), newRepoUnderAnUnrepresentablePath(), TestAHealthyDatabaseReportsThatItsWritabilityWasChecked() (+7 more)

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
Nodes (30): answer, condition, agreeingCommands(), agreementConditions(), answerFrom(), answerOf(), assertExemptionsAreStillEarned(), assertRemedyIsWhatTheRowExpects() (+22 more)

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
Cohesion: 0.18
Nodes (11): Components(), ComponentName, Readiness, Readinesses(), TestComponentsMapHasExactlySixKeys(), TestReadinessEnumIsExactlyFourValues(), TestReadinessUnmarshalRejectsUnknown(), TestTerminalStateSpellingIsTheSpecLiteral() (+3 more)

### Community 108 - "128. Hedef implementation workstream'leri"
Cohesion: 0.18
Nodes (11): 128. Hedef implementation workstream'leri, Milestone 10 — Candidate + Calibration, Milestone 1 — Durable Knowledge + Runtime Core, Milestone 2 — Project Units + Syntax Index, Milestone 3 — Resolver Manager, Milestone 4 — Change Engine, Milestone 5 — Impact + Fan-out, Milestone 6 — Coverage/Test Impact (+3 more)

### Community 109 - "impacted_tests.py"
Cohesion: 0.22
Nodes (20): changed_files(), git(), is_e2e(), is_test(), js_imports(), load_coverage_contexts(), load_explicit_map(), main() (+12 more)

### Community 110 - "4. The confirmed defects"
Cohesion: 0.11
Nodes (18): 4.10 — LOW. §4's corrected F17 bullet credits the fix to a test that does not contain it, 4.11 — LOW. The F48 figure attaches the 1,000-task measurement to 20,000 tasks, 4.12 — LOW. A minted session now starts after the row it attributes, 4.13 — LOW. The `EXISTS` rewrite degrades to a full checkpoint scan for a project with no checkpoints, 4.14 — LOW. Item 11 is recorded as "(test only)" but its commit changes production behaviour, 4.15 — LOW. AC-04.6's amendment claims all six constructors carry the id in `why`, 4.16 — LOW. The task list records fifteen commits for the remediation; there are thirteen, 4.17 — LOW. The wrong `.sql` comment stays wrong, with no marker reachable from the file (+10 more)

### Community 111 - "MR-002 — Versioned Decision/Invariant knowledge lifecycle"
Cohesion: 0.22
Nodes (9): 0. What MR-001 left here, exactly, 1. Scope decision: no CLI write surface, 2. Package plan, 3. Frozen signatures, 4. Decisions, 5. Test plan, 6. Explicit non-goals, 7. Acceptance criteria traceability (+1 more)

### Community 112 - "report.go"
Cohesion: 0.16
Nodes (17): CheckFunc, Result, componentFrom(), coordinationInfo(), currentSchemaVersion(), Report, observationOf(), repositoryObservation() (+9 more)

### Community 113 - "WriteIfAbsent"
Cohesion: 0.28
Nodes (10): DefaultTemplate(), EnsureKnowledgeDirs(), scaffoldError(), TestDefaultTemplateIsNotEmptyAndIsACopy(), TestEnsureKnowledgeDirsCreatesGitkeep(), TestEnsureKnowledgeDirsIsIdempotent(), TestScaffoldDoesNotCreateOutOfScopeFiles(), TestWriteIfAbsentCreatesFromEmbeddedTemplate() (+2 more)

### Community 114 - "States"
Cohesion: 0.19
Nodes (14): lifecycleLiteralsOutsideThisPackage(), moduleRoot(), CanTransition(), State, ParseState(), States(), TestARefusalCanSayWhatIsAvailableInstead(), TestATaskNeverMovesToTheStateItIsAlreadyIn() (+6 more)

### Community 115 - "47. Human approval kimlik ve yetkilendirme modeli"
Cohesion: 0.18
Nodes (11): 47. Human approval kimlik ve yetkilendirme modeli, Approval assurance seviyeleri, Approval payload, Audit, Authorization, EXTERNAL_VERIFIED, LOCAL_INTERACTIVE, MANAGED_IDENTITY (+3 more)

### Community 116 - "runWith"
Cohesion: 0.13
Nodes (19): brokenSetup, envelopeScenario, strictEnvelope, TestAnOccupiedCacheDirectoryIsNotAnObstruction(), assertNoErrorEnvelope(), runWith(), assertEnvelopeCoherent(), decodeStrictEnvelope() (+11 more)

### Community 117 - "writable.go"
Cohesion: 0.21
Nodes (12): TestConditionBlockerLetsTheConditionOverrideThePartItMet(), TestReadOnlyMediaKeepsBothSentinels(), noSpaceDatabaseError(), noSpaceRefusal(), readOnlyMediaDatabaseError(), conditionBlocker(), WriteAccess, WriteBlocker (+4 more)

### Community 118 - "NewRoot"
Cohesion: 0.15
Nodes (19): TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(), TestMachineLocalRootsStillOfferTheirOverride(), TestModeVariantsStillRefuseAnEscape(), TestRepositoryRootIsItsOwnKind(), TestRootKindOfStillRefusesWhatItDoesNotKnow(), TestWriteFileIfAbsentModeUsesTheGivenModes(), NewRoot(), Normalize() (+11 more)

### Community 119 - "Config"
Cohesion: 0.20
Nodes (15): OutputConfig, ProjectConfig, RuntimeConfig, Defaults(), Config, quote(), TestEnvVarWithoutValueIsStillSeen(), TestForeignEnvVarsAreIgnoredEntirely() (+7 more)

### Community 120 - "validator_test.go"
Cohesion: 0.06
Nodes (62): Registry, NewRegistry(), mustReadEmbedded(), TestEmbeddedSchemaDocumentIsACopy(), TestEmbeddedSchemasPresent(), TestNewRegistryAcceptsExtraDocuments(), TestNewRegistryRejectsIncompleteOrCorruptFS(), TestSupportsRejectsNewerVersion() (+54 more)

### Community 121 - "WriteFailure"
Cohesion: 0.14
Nodes (27): DomainError, busyFailure(), describeDatabaseFile(), describeRefusedPaths(), diskFullFailure(), IsBusy(), IsDiskFull(), IsReadOnly() (+19 more)

### Community 122 - "coordination_agreement_test.go"
Cohesion: 0.20
Nodes (17): coordinationRefusal, semanticDamageRow, assertFourErrorKeys(), isEmptyValue(), assertPublishedFailureHasACode(), commandName(), coordinationCommands(), coordinationRefusals() (+9 more)

### Community 124 - "ProbeWriteAccess"
Cohesion: 0.29
Nodes (21): ProbeWriteAccess(), assertProbeMatchesSQLite(), assertUnwritablePayload(), chmodForTest(), dirEntryNames(), initialisedDatabase(), requireModeBitsAreEnforced(), TestProbeWriteAccessAgreesWithSQLite() (+13 more)

### Community 125 - "github.com/spf13/cobra.Command"
Cohesion: 0.19
Nodes (13): classifiedArgs, Root, github.com/spf13/cobra.Command, github.com/spf13/cobra.Completion, github.com/spf13/cobra.PositionalArgs, github.com/spf13/cobra.ShellCompDirective, Execute(), helpTopics() (+5 more)

### Community 126 - "scaffold_containment_test.go"
Cohesion: 0.23
Nodes (11): TestLoadAcceptsRepoConfigThroughAnInsideSymlink(), TestLoadInAnOrdinaryRepositoryIsUnaffected(), TestLoadRefusesRepoConfigThroughAnEscapingSymlink(), requireEmptyTree(), requireEscape(), requireModeEnforcement(), requireSymlinks(), TestScaffoldAllowsSymlinkInsideTheRepository() (+3 more)

### Community 127 - "coherence_test.go"
Cohesion: 0.23
Nodes (15): remedy, remedyClass, agreementRemedyClass(), assertDocumentIsCoherent(), classOf(), contradictionsBetween(), contradictoryPathRemedies(), mindrailCommandsIn() (+7 more)

### Community 128 - "3. IN Scope"
Cohesion: 0.06
Nodes (30): 1. Goal, 2. Primary End-to-End Scenario, 3. IN Scope, 4. OUT of Scope for 0.1, 5. 0.1 Performance Targets, 6. 0.1 Acceptance Criteria, 7. Non-Goals, 8. Exit Condition (+22 more)

### Community 129 - "Finding"
Cohesion: 0.16
Nodes (17): facts(), TestAStepsMessageCarriesTheFactsItsRemedyNeeds(), TestEveryStepsMessageIsCovered(), suppressions(), TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided(), TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded(), filesTheLoaderNamed(), Finding (+9 more)

### Community 130 - "steps_test.go"
Cohesion: 0.23
Nodes (18): TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions(), TestAnInvariantCycleIsFatalAndNamesOnlyInvariants(), TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord(), TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge(), TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose(), detections(), overFires(), TestAKindThatDisagreesWithItsDirectoryIsReportedByStepFive() (+10 more)

### Community 131 - "refuseUnrepresentableJSON"
Cohesion: 0.20
Nodes (12): offender, reflect.StructField, reflect.Value, invocation, firstUnrepresentable(), memberName(), refuseUnrepresentableJSON(), TestRefusalFindsTheOffenderWhereverItIs() (+4 more)

### Community 132 - "record/errors.go"
Cohesion: 0.20
Nodes (3): DuplicateItemError, EmptyFieldError, OptionKindError

### Community 133 - "2. Requirements"
Cohesion: 0.06
Nodes (36): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's five acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation, D-43 — a Finding gets its own codes, because D-38 says it is not a Problem (+28 more)

### Community 134 - "runStatus"
Cohesion: 0.70
Nodes (4): Options, newStatusCommand(), readinessOf(), runStatus()

### Community 135 - "NewID"
Cohesion: 0.27
Nodes (9): encodeCrockford(), compareRandomness(), TestABackwardsClockStillMintsAscendingIds(), TestTheEntropyCounterCarriesAcrossAByte(), NewID(), nextEntropy(), TestAnIdIsPrefixedUniqueAndSortsInMintOrder(), TestAnIdLeaksNeitherTheClockNorTheCaller() (+1 more)

### Community 136 - "storage/access_other.go"
Cohesion: 0.83
Nodes (3): accessWritableDir(), accessWritableFile(), accessWritableMode()

### Community 139 - "agreement_hostilefs_linux_test.go"
Cohesion: 0.20
Nodes (23): hostileCondition, agreeingHostileCommands(), assertTheLoopTerminates(), emptyHostileMount(), fillHostileMount(), fillHostileMountTo(), hostileAvailableBytes(), hostileConditions() (+15 more)

### Community 140 - "timestamp_ground_test.go"
Cohesion: 0.29
Nodes (11): createdAtDescription(), everyDescriptionClaim(), everyRefusedSpelling(), measuredGround(), TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(), TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(), TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(), TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository() (+3 more)

### Community 141 - "Appendix B — what the remediation did"
Cohesion: 0.33
Nodes (6): Appendix B — what the remediation did, FIX-A `internal/knowledge/validate` — reported `GREEN`, FIX-B `internal/doctor` — reported `GREEN_WITH_DEVIATIONS`, FIX-C `internal/knowledge/loader` — reported `GREEN_WITH_DEVIATIONS`, FIX-D `internal/knowledge/record` + `schemas/` — reported `GREEN`, FIX-E sweep, docs, re-audit — reported `GREEN_WITH_DEVIATIONS`

### Community 142 - "TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore"
Cohesion: 0.25
Nodes (13): TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone(), combine(), holdsTwoReportableSelfLoops(), oracleCycleMembers(), recordsFor(), renderStore(), storesOfOneKind(), TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore() (+5 more)

### Community 143 - "95. Knowledge schema ve CI doğrulama pipeline'ı"
Cohesion: 0.20
Nodes (10): 95. Knowledge schema ve CI doğrulama pipeline'ı, Bulk migration, CI order, Feature capability, Knowledge doctor, Lazy per-record upgrade, Minimum Mindrail version, Reader-compatible schema window (+2 more)

### Community 144 - "FreeSpace"
Cohesion: 0.18
Nodes (13): FreeSpace, statFreeSpace(), ProbeFreeSpace(), statFreeSpace(), diskFullOpenFailure(), roomSQLiteWasRefused(), sizeLimitOpenFailure(), spaceCorroboratesFullDisk() (+5 more)

### Community 145 - "builder"
Cohesion: 0.18
Nodes (11): regexp.Regexp, Decision, checkRepoRelative(), isScopeLevel(), isWindowsDriveRooted(), builder, Invariant, Scope (+3 more)

### Community 146 - "NewLoader"
Cohesion: 0.36
Nodes (12): NewLoader(), TestEmbeddedTemplateLoadsStrictly(), TestInvalidColorValueIsRejected(), TestLoadWithNoFilesUsesDefaults(), TestMalformedTOMLIsAUsageError(), TestPrecedenceAcrossFiveLayers(), TestProvenanceReportsSourceLayer(), TestStrictDecodeRejectsUnknownKey() (+4 more)

### Community 147 - "goldenReport"
Cohesion: 0.42
Nodes (8): assertGolden(), Report, goldenReport(), stripANSI(), TestRenderHumanColorStaysOutOfTheContent(), TestReportJSONHasNoANSIAndStableKeys(), TestReportJSONRoundTripsThroughTheStrictDecoder(), TestReportRenderHumanGolden()

### Community 148 - "MR-003 — design: sequential agent handover"
Cohesion: 0.17
Nodes (12): 1. What the milestone is, in one paragraph, 2. What it is not, 3. Package plan, 4. Entity model, 5. Runtime schema — `migrations/000002_coordination.sql`, 6. The state table, 7. Command surface, 8. Status integration (+4 more)

### Community 149 - "TestRepoConfigDirObstructionDoesNotOverFire"
Cohesion: 0.60
Nodes (5): loadAt(), payload(), TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(), TestRepoConfigDirObstructionDoesNotOverFire(), TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission()

### Community 150 - "MR-002 — audit package"
Cohesion: 0.15
Nodes (12): 10. The independent mutation sweep, and what it found, 11. What the audit is asked to establish, 1. Original request, verbatim, 2. The task, 3. Requirements and acceptance criteria, 4. Design contract, 5. Implementation summary, 6. Change references (+4 more)

### Community 151 - "29. Large repository cold-index lifecycle"
Cohesion: 0.22
Nodes (9): 29. Large repository cold-index lifecycle, Cooperative scheduler, Foreground ve local worker, Kesilme, Priority aging, Progress, ProjectUnit reservation, Resumability (+1 more)

### Community 152 - "11. SQLite concurrency ve interactive write fairness"
Cohesion: 0.25
Nodes (8): 11. SQLite concurrency ve interactive write fairness, Cold-index write fairness, Idempotency key, Immutable/content-addressed rows, Neden global write broker yok?, Optimistic revision, Process-local writer scheduling, WAL + bounded busy retry

### Community 153 - "Audit round 2"
Cohesion: 0.25
Nodes (8): 1. The verdict, 2. The base rate, which is why this round exists, 3. The three deviations, graded, 5. What was refuted, and why, 6. What this round did not look at, 7. What generalises, 8. Remediation brief for round 2, Audit round 2

### Community 154 - "Check"
Cohesion: 0.09
Nodes (36): Check, diagnosis, halt, Probes, step, Runner, ConfigCheck(), configResult() (+28 more)

### Community 155 - "121. Failure modes"
Cohesion: 0.25
Nodes (8): 121. Failure modes, CI unavailable, Coverage map unavailable, Git hook bypass edildi, MCP unavailable, Semantic resolver unavailable, SQLite unavailable, Syntax index unavailable

### Community 156 - "[ ] MR-013 — Yerel completion evidence gate"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-013 — Yerel completion evidence gate, Ne yapılacak

### Community 157 - "LOW"
Cohesion: 0.11
Nodes (17): F01 — `mindrail init` reported READY over a repository the next command refused, F02 — an unopenable `config.toml` was remedied as a malformed one, F03 — a size limit was reported as a full disk, F12 — a defective build blamed the repository, F13 — the rebuild remedy did not name the database, F14 — three unexercised claims in the init report path, F15 — the containment remedy named no path, F16 — a guard that could not fail (+9 more)

### Community 158 - "[x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri

### Community 159 - "2. Implementation Principles"
Cohesion: 0.33
Nodes (6): 2.1 Local-first, 2.2 Deterministic core, 2.3 Standard library first, 2.4 Small dependency surface, 2.5 No premature distributed architecture, 2. Implementation Principles

### Community 160 - "24. Durable symbol identity, rename ve orphan protection"
Cohesion: 0.33
Nodes (6): 24. Durable symbol identity, rename ve orphan protection, Ambiguous identity, Orphan protection, Overload ve nested symbol, Rename/move identity migration, symbol_uid tahsisi ve determinizm

### Community 161 - "57. Candidate invariant trigger'ları"
Cohesion: 0.33
Nodes (6): 57. Candidate invariant trigger'ları, Trigger 1 — ASSERTION_CONTRACT, Trigger 2 — REGRESSION_TEST, Trigger 3 — EXPLICIT_AGENT_CANDIDATE, Trigger 4 — PUBLIC_CONTRACT_CHANGE, Trigger 5 — REPEATED_FAILURE_SIGNAL

### Community 162 - "scaffold_hostilefs_linux_test.go"
Cohesion: 0.39
Nodes (8): availableBytes(), fillToZero(), hostileScaffoldScenario(), runHostileChild(), scaffoldOnAFullFilesystem(), TestMain(), TestTheScaffoldNamesTheConditionThatStoppedIt(), userNamespacesWork()

### Community 163 - "2. Mindrail'in temel problemi"
Cohesion: 0.40
Nodes (5): 2.1 Context kaybı, 2.2 Semantic conflict, 2.3 Intent kaybı, 2.4 Kanıtsız tamamlanma, 2. Mindrail'in temel problemi

### Community 164 - "newInvocation"
Cohesion: 0.19
Nodes (14): globalFlags, humanRenderer, log/slog.Logger, Options, newDoctorCommand(), runDoctor(), blockedReason(), globalFlagsOf() (+6 more)

### Community 165 - "fulldisk_linux_test.go"
Cohesion: 0.22
Nodes (16): afterTheRemedy(), availableBytes(), checkRemedy(), directoryContents(), fillToZero(), fullDiskChild(), fullDiskScenario(), initialiseOnDisk() (+8 more)

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

### Community 172 - "[ ] MR-018 — Hook bypass'a dayanıklı CI verification"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-018 — Hook bypass'a dayanıklı CI verification, Ne yapılacak

### Community 173 - "created_at"
Cohesion: 0.40
Nodes (5): description, format, pattern, type, created_at

### Community 174 - "runCoordination"
Cohesion: 0.17
Nodes (16): scope, sessionResult, Options, newCheckpointCommand(), newCheckpointWriteCommand(), coordinationScope(), coordinationUnavailable(), invocation (+8 more)

### Community 175 - "NewError"
Cohesion: 0.18
Nodes (17): io.Writer, CauseOf(), detailLines(), NewError(), RenderError(), renderSection(), TestAdoptCauseNeverOverwritesAnExistingCause(), TestCauseOfReachesThroughTheWrapChain() (+9 more)

### Community 176 - "FormatTime"
Cohesion: 0.10
Nodes (30): checkpointResult, handoverResult, taskResult, rowScanner, Write, FormatTime(), blockedReasonMissing(), checkpointNotFound() (+22 more)

### Community 177 - "contract_test.go"
Cohesion: 0.09
Nodes (62): result, discoveryConditions(), newNonRepositoryDir(), TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(), TestEveryUninitialisedSpellingExitsZero(), addWorktree(), assertGolden(), assertShape() (+54 more)

### Community 178 - "time.Time"
Cohesion: 0.19
Nodes (12): FixedClock, lockProbingClock, stepClock, sync.Mutex, time.Time, everyTimestampSpelling(), readByInvariantRecord(), readByRecord() (+4 more)

### Community 179 - "[ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı, Ne yapılacak

### Community 180 - "2. Public API per package"
Cohesion: 0.08
Nodes (23): 0. Citation audit, 1.1 Why `internal/status` is added to §7, 1. Package plan, 2.10 `internal/doctor` — Unit D, 2.11 `internal/status` — Unit D, 2.12 `internal/bootstrap` — Unit E, 2.13 `internal/cli` — Unit E, 2.1 `internal/app` — Unit D (+15 more)

### Community 181 - "5. Test plan"
Cohesion: 0.12
Nodes (16): 5.10 `internal/doctor` (Unit D) — 11, 5.11 `internal/status` (Unit D) — 8, 5.12 `internal/bootstrap` (Unit E) — 8, 5.13 `internal/cli` — CLI contract (Unit E) — 14, 5.14 Architecture + smoke (Unit E) — 6, 5.15 Acceptance-criteria mapping, 5.1 `internal/app` (Unit D) — 10, 5.2 `internal/filesystem` (Unit A) — 8 (+8 more)

### Community 182 - "loader/loader.go"
Cohesion: 0.24
Nodes (10): io/fs.DirEntry, escapingRecord(), Loader, isRecordFile(), New(), readFailureReason(), recordPath(), stringField() (+2 more)

### Community 183 - "ParseTime"
Cohesion: 0.22
Nodes (7): SystemClock, ParseTime(), TestFixedClockSatisfiesClock(), TestFormatTimeIsUTCRFC3339(), TestParseTimeRejectsMalformedInput(), TestShutdownTimeoutMatchesBusyTimeout(), TestSystemClockIsUTC()

### Community 184 - "Clock"
Cohesion: 0.25
Nodes (12): database/sql.Tx, Clock, Registration, Store, Workspace, NewStore(), scanWorkspace(), upsertProject() (+4 more)

### Community 185 - "Open"
Cohesion: 0.09
Nodes (38): fileSize(), TestAFailedCheckpointIsReportedAsOne(), TestCheckpointGivesUpOnAHeldDatabaseRatherThanWaitingForIt(), TestCheckpointHandsTheConnectionBackAsItFoundIt(), TestCheckpointTruncatesTheLogItWroteBack(), IsUnwritten(), Open(), openTemp() (+30 more)

### Community 186 - "SQLiteCheck"
Cohesion: 0.25
Nodes (12): SQLiteCheck(), noSpaceRefusal(), TestAFullFilesystemIsNotReportedAsAHealthyDatabase(), TestAFullFilesystemSupersedesTheInitRemedyEverywhere(), TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(), TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(), TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(), Subject (+4 more)

### Community 187 - "Supersede"
Cohesion: 0.23
Nodes (12): TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(), TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(), Decision, Decision, Supersede(), activeDecision(), TestSupersedeDoesNotRepeatATargetItAlreadyCarries(), TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout() (+4 more)

### Community 188 - "MR-003 — findings"
Cohesion: 0.10
Nodes (21): 1. What the milestone does now, 2. Two defects the implementation found in itself, 3. Mutations run, and what each turned red, 4. What an audit should look at first, 5. Audit round 1, 6. The round-1 remediation, 7. The round-2 remediation, MR-003 — findings (+13 more)

### Community 189 - "version.go"
Cohesion: 0.48
Nodes (5): versionInfo, buildVersionInfo(), joinInts(), newVersionCommand(), resolveCommit()

### Community 190 - "103. Test weakening ve false-green guard"
Cohesion: 0.67
Nodes (3): 103. Test weakening ve false-green guard, Minimum heuristics, Tetikleme kapsamı

### Community 191 - "28. Interactive performance contract"
Cohesion: 0.67
Nodes (3): 28. Interactive performance contract, Interactive vs deep path, Latency telemetry

### Community 198 - "1. Decisions this document freezes"
Cohesion: 0.07
Nodes (29): 0. Baseline evidence, 1. Decisions this document freezes, 2. Requirements, 3. Traceability to the task list's four acceptance criteria, 4. Work breakdown and dependency order, 5. Definition of done, D-53 — the coordination tables are a second migration, and the reported runtime schema version becomes 2, D-54 — "Checkpoint" means two things in this repository, and neither is renamed (+21 more)

### Community 199 - "status/knowledge_findings_test.go"
Cohesion: 0.47
Nodes (5): findingOn(), rowValue(), TestACycleBlocksAndAnInvalidRecordDoesNot(), TestStatusBuildsNoValidatorOfItsOwn(), TestTheFindingsCountIsQualifiedByObservation()

### Community 200 - "[x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

### Community 201 - "newStore"
Cohesion: 0.32
Nodes (14): newStepClock(), newStore(), newStoreAt(), registration(), TestEmptyStoreListsNothing(), TestFindByRootReportsUnregisteredWorkspace(), TestRegisteredIDsAreOpaqueAndPrefixed(), TestRegisterIsIdempotent() (+6 more)

### Community 202 - "validate_test.go"
Cohesion: 0.19
Nodes (14): RecordKind, RecordRef, TestFindingsAreSortedByPathThenStep(), everyConditionStore(), fixturePath(), refFor(), renderDoc(), TestCheckOverAnAbsentStoreFindsNothing() (+6 more)

### Community 203 - "requireValid"
Cohesion: 0.25
Nodes (13): createdAtOf(), describe(), encode(), findingsFor(), readGolden(), requireValid(), shippedValidator(), TestConstructorsSerializeCreatedAtAsUTCRFC3339() (+5 more)

### Community 204 - "audit_scope.py"
Cohesion: 0.32
Nodes (12): assign_cluster(), build_plan(), changed_files(), classify(), diff_lines(), git(), load_tasks(), main() (+4 more)

### Community 205 - "Code"
Cohesion: 0.31
Nodes (11): Code, indexCodes(), IsRegistered(), RegisteredCodes(), sortedCodes(), parseDeclaredCodes(), TestCodeRegistryIsUniqueAndExhaustive(), TestRegisteredCodesIsNotAliased() (+3 more)

### Community 206 - "[ ] MR-011 — Kaynak değişince evidence geçersizleştirme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-011 — Kaynak değişince evidence geçersizleştirme, Ne yapılacak

### Community 207 - "[ ] MR-004 — Güvenli lease, idempotency ve optimistic revision"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision, Ne yapılacak

### Community 208 - "[ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması, Ne yapılacak

### Community 209 - "[ ] MR-007 — Reconcile-first gerçek değişiklik keşfi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi, Ne yapılacak

### Community 210 - "[ ] MR-008 — Scope drift ve unregistered change ambiguity"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-008 — Scope drift ve unregistered change ambiguity, Ne yapılacak

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

### Community 218 - "task.go"
Cohesion: 0.42
Nodes (9): taskListResult, Options, newTaskCommand(), newTaskListCommand(), newTaskOpenCommand(), newTaskShowCommand(), newTaskStateCommand(), parseStateFlag() (+1 more)

### Community 219 - "[x] MR-001 — Yerel repository bootstrap ve tanılama yolu"
Cohesion: 0.50
Nodes (4): Durum, Kabul kriterleri, Ne yapılacak, [x] MR-001 — Yerel repository bootstrap ve tanılama yolu

### Community 220 - "ClassifyRefusal"
Cohesion: 0.23
Nodes (8): Barrier, ClassifyRefusal(), platformBarrier(), platformBarrier(), TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(), TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(), TestFsErrPermissionDoesNotSwallowTheOtherTwo(), TestTheThreeUnwritableConditionsAreNotConfusable()

### Community 222 - "[ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme, Ne yapılacak

### Community 223 - "Görevler"
Cohesion: 0.29
Nodes (7): Görevler, Kabul kriterleri, Kabul kriterleri, [ ] MR-015 — MCP koordinasyon ve değişiklik araçları, [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi, Ne yapılacak, Ne yapılacak

### Community 224 - "NewDecision"
Cohesion: 0.31
Nodes (9): TestAConstructorNamesThePositionOfANilOption(), Decision, NewDecision(), NewInvariant(), stampedTime(), newBuilder(), TestAConstructorAgreesWithScopeValidateAboutTheDriveBoundary(), TestEveryInstantTheWriterCanStampRendersAsASpellingTheContractAccepts() (+1 more)

### Community 225 - "[ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi, Ne yapılacak

### Community 228 - "[ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence, Ne yapılacak

### Community 229 - "[ ] MR-014 — MCP bilgi ve bağlam araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-014 — MCP bilgi ve bağlam araçları, Ne yapılacak

### Community 230 - "Audit scoping — making audit cost follow risk, not task count"
Cohesion: 0.20
Nodes (9): Audit scoping — making audit cost follow risk, not task count, Rule 1 — One audit per delivery, never one per task, Rule 2 — Depth is set per cluster by risk, Rule 3 — Mutation runs targeted tests, the full suite runs once, Rule 4 — Checkpoint audits hide audit time behind implementation time, Rule 5 — Re-audits are delta-only, The audit plan artifact, The problem this solves (+1 more)

### Community 231 - "[ ] MR-016 — MCP validation ve completion araçları"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-016 — MCP validation ve completion araçları, Ne yapılacak

### Community 232 - "[ ] MR-017 — Staged değişiklik için yerel Git enforcement"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-017 — Staged değişiklik için yerel Git enforcement, Ne yapılacak

### Community 233 - "[ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı"
Cohesion: 0.67
Nodes (3): Kabul kriterleri, [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı, Ne yapılacak

### Community 234 - "runtime.go"
Cohesion: 0.28
Nodes (5): ensureDir(), RuntimePaths, hasParentSegment(), overrideOrDefault(), requireAbsolute()

### Community 235 - "readonlyfs_linux_test.go"
Cohesion: 0.44
Nodes (8): checkReadOnlyRemedy(), openOnAReadOnlyFilesystem(), probeOnAReadOnlyFilesystem(), readOnlyChild(), readOnlyScenario(), remountReadOnly(), remountReadWrite(), runReadOnlyChild()

### Community 236 - "loadStore"
Cohesion: 0.44
Nodes (7): TestAtMostOneRecordCanBeCanonicalForAnId(), TestOwnershipFollowsTheNameTheDirectoryHolds(), assertSameRefs(), loadStore(), TestTheFixtureStoreIsTheOneTheLoaderProduces(), TestTheRecordsTheLoaderIgnoresAreTheOnesTheValidatorFinds(), writeKnowledgeRecord()

### Community 237 - "Appendix F — round 4 findings, as confirmed"
Cohesion: 0.40
Nodes (5): Appendix F — round 4 findings, as confirmed, The eight, What generalises, What round 4 did not look at, What the refutations killed, and why it matters

### Community 238 - "4. Work units"
Cohesion: 0.25
Nodes (8): 4. Work units, File-ownership summary, Unit A — Repository & filesystem, Unit B — Runtime store, Unit C — Config & knowledge, Unit D — Error model & diagnostics, Unit-D T0 handshake, Unit E — Wiring (final, sequential)

### Community 239 - "MEDIUM"
Cohesion: 0.25
Nodes (8): F04, F08 — a rejected command line exited 1 and wrote no envelope, F05 — the agreement matrix never read the remedy it claimed to carry out, F06 — no matrix row for a `config.toml` that cannot be opened, F07 — the linked-worktree cross-check compared two fields of four, F09 — the read-only-media sentinel had no row in the classifier's table, F10 — `sameDirectory`'s identity comparison was untested, F11 — the linked-worktree back-pointer check was untested, MEDIUM

### Community 240 - "NoSpaceRefusal"
Cohesion: 0.38
Nodes (5): TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(), TestNoSpaceRefusalNamesTheConditionItFound(), TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(), TestNoSpaceRefusalStaysSilentWhereThereIsRoom(), NoSpaceRefusal()

### Community 241 - "Appendix E — what the third remediation did"
Cohesion: 0.40
Nodes (5): Appendix E — what the third remediation did, Graded against the whole state space, not a fixture set, The rule, in two lines of code, The twelve, and what each one took, What was deliberately not done

### Community 242 - "TestAPatternMessageThatQuotesItsRuleFaithfullyIsStillPublished"
Cohesion: 0.60
Nodes (4): schemaPattern(), TestAPatternMessageThatQuotesItsRuleFaithfullyIsStillPublished(), TestTheCreatedAtRuleTheMessageSendsTheReaderToIsThere(), TestThePatternMessageForATimestampDoesNotMisquoteTheRule()

## Knowledge Gaps
- **1158 isolated node(s):** `github.com/PsyChaos/mindrail`, `App`, `invocation`, `invocation`, `fixture` (+1153 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 1317 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **16 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewError()` connect `NewError` to `refuseUnrepresentableJSON`, `App`, `RootKind`, `adapter.go`, `Warning`, `config/loader.go`, `FreeSpace`, `DefaultChecks`, `Root`, `New`, `InTx`, `healthySubject`, `PayloadOf`, `context.Context`, `RuntimePathCheck`, `Build`, `newInvocation`, `runCoordination`, `FormatTime`, `contract_test.go`, `loader/loader.go`, `Clock`, `SQLiteCheck`, `DB`, `Probe`, `Code`, `task.go`, `writable.go`, `Config`, `WriteFailure`, `github.com/spf13/cobra.Command`?**
  _High betweenness centrality (0.023) - this node is a cross-community bridge._
- **Why does `PayloadOf()` connect `PayloadOf` to `refuseUnrepresentableJSON`, `App`, `ResolveRuntimePaths`, `Warning`, `FreeSpace`, `New`, `NewLoader`, `DefaultChecks`, `New`, `TestRepoConfigDirObstructionDoesNotOverFire`, `Check`, `context.Context`, `check_test.go`, `newLoader`, `scaffold_hostilefs_linux_test.go`, `newInvocation`, `fulldisk_linux_test.go`, `NewError`, `FormatTime`, `contract_test.go`, `newFixture`, `Clock`, `Open`, `hostilefs_linux_test.go`, `DB`, `newStore`, `Probe`, `Code`, `readonlyfs_linux_test.go`, `runWith`, `NewRoot`, `WriteFailure`, `ProbeWriteAccess`, `scaffold_containment_test.go`?**
  _High betweenness centrality (0.022) - this node is a cross-community bridge._
- **Why does `measuredGround()` connect `timestamp_ground_test.go` to `ParseTime`, `time.Time`, `testing.T`?**
  _High betweenness centrality (0.009) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `PayloadOf()` (e.g. with `TestDomainErrorWrappingSurvivesErrorsIsAndAs()` and `WriteJSON()`) actually correct?**
  _`PayloadOf()` has 2 INFERRED edges - model-reasoned connections that need verification._
- **Are the 10 inferred relationships involving `NewError()` (e.g. with `TestAdoptCauseNeverOverwritesAnExistingCause()` and `TestCauseOfReachesThroughTheWrapChain()`) actually correct?**
  _`NewError()` has 10 INFERRED edges - model-reasoned connections that need verification._
- **Are the 8 inferred relationships involving `Check()` (e.g. with `newGraph()` and `stepDuplicateActiveLineage()`) actually correct?**
  _`Check()` has 8 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/PsyChaos/mindrail`, `App`, `invocation` to the rest of the system?**
  _1158 weakly-connected nodes found - possible documentation gaps or missing edges._