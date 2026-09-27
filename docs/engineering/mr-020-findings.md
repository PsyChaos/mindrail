# MR-020 — Findings

Demo driver: temporary test files (`internal/cli/mr020_demo_test.go`,
`internal/gate/mr020_demo_test.go`,
`internal/index/symbol/mr020_demo_test.go`) driving the real built
binary and the production services over scratch repositories. The
binary was built from the frozen tree with
`go build -o /tmp/mr020/bin/mindrail ./cmd/mindrail`. The drivers are
deleted before milestone close (D-241); the outputs below are pasted
from `go test -v` runs (full logs were held at `/tmp/mr020/*.log`
during the session).

Method note (D-242 honesty): every verdict below is gate output, not
narration. CLI demos exec the built binary; service demos call the
same services the MCP handlers compose (`validation.Service`,
`gate.Service`, `Indexer` + `symbol.Service`). Prior-state seeding
(DB unit rows, guard bindings, kernel task rows) reuses the suites'
own helpers — seeding is prior state, never the verdict. Legend:
`…` marks omitted hash/path middles; reason codes, verdicts, denial
counts and equality assertions are untruncated. Harness churn outside
the milestone scope (`.claude/`, `.gitignore`, `graphify-out/`,
`.wrongstack/`, `.zcode`, old `stash@{0}` — requirements §0 list,
plus newer `audit-scoping` helper files from the same harness) is
untouched and unattributed to this milestone; `git diff` on the
milestone scope shows docs only.

## TASK-01 — ALLOW yolu + `before_change` unutma (REQ-01)

Driver: `internal/cli/mr020_demo_test.go`
(`TestMR020DemoAllowTwoWorktrees`, `TestMR020DemoForgottenCall`).
Agent A acts in linked worktree `wt-a`, agent B in linked worktree
`wt-b`, of one sample repo — two genuinely separate copies (D-239),
each `mindrail init`'ed.

### Demo ALLOW (AC-01.1)

Agent A in `wt-a` stages a symbol-free docs change; no bypass is
attempted.

```
=== agent-A docs-only change: verify --staged (exit 0) ===
{"command":"verify","ok":true,"data":{"allow":true,"denials":[],"knowledge_problems":[]}}
```

Gate verdict: `allow:true`, zero denials. Honest run passes. AC-01.1 met.

### Demo forgotten `before_change` (AC-01.2)

Agent B in `wt-b` (a separate worktree and its own init) edits and
stages `pkg/a.py` with no claim and no `before_change` behind it.
(`pkg/pyproject.toml` was written but never staged, so the denial is
about the real code diff, not scaffolding.)

```
=== agent-B forgotten call: verify --staged (exit 1) ===
{"command":"verify","ok":false,"data":{"allow":false,"denials":[{"code":"UNREGISTERED_CHANGE",
"reason":"symbol [\"a.py\",\"bb2f…\"] matches no open change",
"provenance":"change CHG-… of task ",
"next_action":["Declare the scope with `before_change`, or record an explicit
attribution for …"]}],"knowledge_problems":[]},
"error":{"code":"UNREGISTERED_CHANGE","why":"verify --staged denies: 1 blocking findings", …}}
mindrail: UNREGISTERED_CHANGE: verify --staged denies: 1 blocking findings
```

Reconcile note (gate question: where is the reconcile output?):
`verify --staged` does not shell a separate `reconcile` command — it
embeds the reconcile step: `internal/verify/verify.go:78` calls
`changes.ReconcileStaged`, and the denial's `CHG-…` provenance row is
the discovery artifact (the undeclared diff reconciled into
existence, then judged unregistered). The pasted transcript is the
reconcile path's verdict, not a narration of it. AC-01.2 met.

### TASK-01 gate

- Reader: first round FAIL/BLOCKED (3 items, all record-honesty, no
  verdict overturned) → remediated: reconcile-path note added above;
  `wt-a`/`wt-b` wording corrected (separate worktrees, separate
  inits); ellipsizing legend + build command added to the method
  note. Re-gate: PASS (see Remediation §R1).
- Breaker: first round FAIL/BLOCKED on the same reconcile gap plus
  advisories → remediated: reconcile note (mechanism cited); ALLOW
  non-vacuity pinned by the ForgottenCall control on the same binary
  (same machinery denies). Re-gate: PASS.

## TASK-02 — `--no-verify` + protected-symbol rename (REQ-02)

### Demo `--no-verify` → CI (AC-02.1)

Driver: `TestMR020DemoNoVerifyCI`. Origin repo, real binary on PATH,
`hook install`'d. Violating `pkg/a.py` staged.

Local gate first (the hook's own check):

```
=== local gate: verify --staged (exit 1) ===
… "code":"UNREGISTERED_CHANGE" … "why":"verify --staged denies: 1 blocking findings" …
```

Plain commit against the installed hook:

```
=== plain commit against the hook (exit 1) ===
UNREGISTERED_CHANGE
Why: verify --staged denies: 1 blocking findings
…
```

The hook blocks. The agent ships it anyway:

```
=== commit --no-verify (exit 0) ===
(empty output — the bypass ships)
```

Fresh clone + `init` + `verify --ci --base <base>`:

```
=== fresh clone: verify --ci (exit 1) ===
… "code":"UNREGISTERED_CHANGE" … "why":"verify --ci denies: 1 blocking findings",
"impact":"The committed range fails the shared gates; the push would not complete." …
```

The driver asserts the exact denial-code multiset staged-vs-CI
(equal counts per code, no missing, no extra — not a subset check).
CI reproduces the identical denial over an independent checkout.
AC-02.1 met.

### Demo rename (AC-02.2)

Driver: `internal/index/symbol/mr020_demo_test.go`
(`TestMR020DemoRename`). Two-agent shape: SEQUENTIAL agents on one
knowledge lifecycle — agent A protects `pkg/a.py:Box.run` (CRITICAL
`INV-1000`), agent B renames later in the same checkout.

```
agent-A protect: status=bound uid=SYM-… blocking=false finding=<nil>
agent-B reliable rename: status=bound uids=[SYM-…] blocking=false finding=<nil>
agent-B ambiguous rename: status=ambiguous uids=[SYM-…] blocking=true
  finding=SYMBOL_IDENTITY_AMBIGUOUS: removed SYM-… is undecided among 2 heirs
stored row after ambiguity: uid=SYM-… status=ambiguous (carried, not orphaned)
agent-B deletion: status=orphaned uids=[] blocking=true
  finding=ORPHANED_PROTECTED_SYMBOL: invariant INV-1000 protects pkg/a.py:Box.run, which no longer resolves
```

Reliable rename carries the same `symbol_uid` (equality-asserted, not
re-minted); twinned rename blocks with `SYMBOL_IDENTITY_AMBIGUOUS`
while the stored row keeps the uid (read back from storage — no
silent orphan); deletion blocks with `ORPHANED_PROTECTED_SYMBOL`.
AC-02.2 met.

D-239 note (gate question: why not two checkouts for the carry?):
a two-checkout staging was run and is logged verbatim
(`TestMR020DemoRenameTwoWorktrees`, `rename-demo.log`): two working
copies (`wt-a`, `wt-b`) share ONE lifecycle database while each agent
acts only in its own copy — the exact topology the gate proposed.

```
agent-A protect (wt-a): status=bound uid=SYM-01M3E9BQAH6R2BYCW4MQ4383VA
agent-B checkout rebind (wt-b, identical bytes): status=bound uids=[SYM-01M3E9BQC3EXKPT5093JTXPES2]
uid independence: wt-a=SYM-01M3E9BQAH6R2BYCW4MQ4383VA wt-b=SYM-01M3E9BQC3EXKPT5093JTXPES2
  (per-lifecycle durability, not cross-checkout)
agent-B reliable rename (wt-b): status=bound uids=[SYM-01M3E9BQC3EXKPT5093JTXPES2]
```

Identical bytes in the second copy mint an independent uid, because
uid durability is per knowledge lifecycle (one database — units are
absolute-path scoped, so two copies are two lifecycles for identity).
Staging "carry" across checkouts would demonstrate something the
product explicitly does not guarantee. The same run proves the
mechanism is copy-independent: wt-b's own rename carries wt-b's uid.
The product's two-agent model for this mechanism is sequential
handoff on one checkout (MR-003 model), which the AC demo above
stages; the two-copy scenario requirement is carried by the CLI
worktree demos sharing the repo. No D-240 trigger: every verdict
behaved as designed.

### TASK-02 gate

- Reader: first round FAIL/BLOCKED (D-239 letter for rename) →
  remediated with the experiment-backed note above + bypass-commit
  exit logged + exact code-multiset assertion. Re-gate: PASS (see
  Remediation §R1).
- Breaker: first round FAIL/BLOCKED (same D-239 item; verdict attacks
  all refuted: uid-carry proven by string equality, ambiguity blocks,
  stored row read back; CI equality now exact-multiset) → re-gate:
  PASS.

## TASK-03 — kritik test zayıflatma + stale evidence (REQ-03)

### Demo test weakening (AC-03.1)

Driver: `TestMR020DemoTestWeakening`. Agent B works in its own linked
worktree `wt-c` (own init; seeding targets the shared common-dir
database the product resolves via `--git-common-dir`, which the
suite's `.git`-is-a-directory helpers cannot open — demo-local
helpers cover this). Only `test_helper` is removed in `wt-c`.

Production-untouched pin (asserted, not by-construction): the helper
bytes `def helper():\n    return 1\n` are present in both the
committed revision (`git show HEAD:…`) and the staged file, and the
staged file no longer contains `test_helper`.

```
=== agent-B weakened critical test, production untouched: verify --staged (exit 1) ===
… {"code":"TEST_GUARD_WEAKENED","reason":"verification test removed (CRITICAL_TEST_REMOVED)",
"key":"…/pkg/test_a.py::test_helper",
"next_action":["Restore the weakened test or record an explicit allowance for …"]} …
mindrail: TEST_GUARD_WEAKENED: verify --staged denies: 3 blocking findings
```

DENY multiset (all three pasted in the log): 1× `TEST_GUARD_WEAKENED`
(shown above) + 2× `UNREGISTERED_CHANGE` (the undeclared test-file
diff — expected fellow-travellers, not the verdict; the envelope
error code names the guard).

Blocking guard finding with remedy, production byte-identical.
AC-03.1 met.

### Demo stale evidence (AC-03.2)

Driver: `internal/gate/mr020_demo_test.go`
(`TestMR020DemoStaleEvidence`). Two separate checkouts: agent A acts
only in copy A, agent B only in copy B (D-239). Copy A's rows claimed
over copy B's tree fail safe by design — logged above as
`scope escapes the root` (provenance scope is absolute, so foreign
trees are refused, never called current); each copy therefore
validates its own proof.

```
agent-A evidence (copy A): profile=test snapshot=cfba9df6785c…
honest completion (copy A, unedited): allow=true denials=[]
cross-copy verdict: profile=test status=stale reason="scope escapes the root: …/tests/t.py"
freshness verdict: profile=test status=stale reason="scope content differs from snapshot cfba9df6785c"
freshness verdict: profile=test status=stale reason="scope content differs from snapshot cfba9df6785c"
stale-proof completion: allow=false denials=[{Code:REQUIRED_EVIDENCE_NOT_CURRENT
  Reason:scope content differs from snapshot cfba9df6785c Key:test
  Provenance:required profile test without current evidence
  NextAction:[Run the test validation profile and re-check completion.]}]
re-run completion: allow=true denials=[]
```

Agent B validates in copy B (fresh coverage satisfied), edits
`tests/t.py`, claims the old result — refused as stale with reason +
named re-run; a fresh run restores ALLOW through the same gate.
Staleness is recomputed from the live tree (`rehashScope`), not
asserted. AC-03.2 met.

### TASK-03 gate

- Reader: first round FAIL/BLOCKED (D-239 letter for both demos) →
  remediated: weakening now in `wt-c`, stale across copy A/B.
  Re-gate: PASS (see Remediation §R1).
- Breaker: first round FAIL/BLOCKED (same) → remediated; weakest-pin
  advisory closed by the prod-bytes pin (weakening) and the
  copy-B fresh-coverage pin (stale). Re-gate: PASS.

## Remediation §R1 — gate round 1 → round 2

Round-1 gates (independent Reader + Breaker via subagent): both
FAIL/BLOCKED on record-honesty only — six verdicts real, pastes
verbatim, no implementation diff, no D-240 deviation. Load-bearing
items: (R1) D-239 letter for rename/stale/weakening demos;
(R2) AC-01.2 showed no reconcile-path evidence. Polish: wt wording,
bypass-commit log line, ellipsizing legend, build command, harness
churn ack, `fresh` → `yeni klon` in requirements, exact code-set
(TASK-02) and prod-bytes (TASK-03) pins.

Remediation (this file's round-2 record): weakening re-staged into
linked worktree `wt-c` with common-dir seeding + prod-bytes pin;
stale re-staged across copy A/B with fresh-coverage pin (cross-copy
refusal `scope escapes the root` recorded as attempted);
rename kept sequential with the refuting experiment on record
(`SYM-…RR3SPQ` vs `SYM-…VP65Z3`); reconcile note cites
`internal/verify/verify.go:78` (`changes.ReconcileStaged`) with the
`CHG-…` provenance row as discovery artifact; CI equality is now an
exact multiset assertion; all LOW polish applied.

Round-2 re-gate: Reader PASS (+2 LOW advisories, closed in §R2;
Reader round-2 ses_f236d5f0dffe5StesBFzDVZjTS); Breaker FAIL/BLOCKED
on F2 (rename two-agent staging; Breaker round-2
ses_f236d5f0affezN55Iapf94Dbxq) — remediated with the logged
two-worktree experiment; F3/F4 LOW notes closed by the cross-copy
log line and the multiset statement. (Round-1 sessions: Reader
ses_f27ac3537ffeSatDnC3qVZmLxZ, Breaker ses_f27ac3536ffel4I5Y6w0eUih34;
demo-path mapping: rename ses_f27b11772ffeOr0GXvyXsbDig3, stale/ALLOW
ses_f27b11770ffeEfgh60DoDfPaLd.)

## Remediation §R2 — Breaker round-2 F2 + LOW notes

- F2 (HIGH): `TestMR020DemoRenameTwoWorktrees` stages wt-a/wt-b on
  one shared database, each agent in its own copy, and logs full
  uids proving independence (`…4383VA` vs `…XPES2`) plus wt-b's own
  temporal carry. The D-239 note above now quotes the verbatim
  transcript instead of narrating an unlogged run.
- Reader advisory A + Breaker F3-note: cross-copy `Check` is now a
  logged run (`cross-copy verdict: … status=stale reason="scope
  escapes the root: …"`), not a narration.
- Breaker F4-note: weakening DENY multiset stated (1× guard + 2×
  unregistered fellow-travellers); full set is in the log.
- Reader advisory B: refuting-experiment uids now full-length in the
  log and quoted above.

Round-3 re-gate (narrow, F2 + LOWs): PASS (Breaker
ses_f2368c051ffesM72tNPGTbYVqf).

Freeze amendment (DoD-1 kaydı, sessiz değil): freeze sonrası
`mr-020-requirements.md` AC-02.1'de tek kelime değişti — "fresh
clone" → "yeni klon" (Türkçe hijyen, anlamsal değişiklik yok).
Round-2 Reader doğruladı (requirements diff'i bu satırdan ibaret).
Anlam daralması/genişlemesi yoktur; AC-02.1'in hükmü aynıdır.

## TASK-04 — ürün sahibi kararı ve kapanış kaydı (REQ-04)

Owner verdict (AC-04.1, in-session, explicit): **Ship 0.1.** Ürün
sahibi (kullanıcı) altı demonun tasarlandığı gibi davrandığını ve
kapıların PASS olduğunu görerek 0.1 release'i onayladı. Koşul yok.
Tarih: 2026-09-26 (session ses_f27b5da48ffeUkeqgyedOpnmA8).

Closing proof (AC-04.3): temp driver'lar silindi (D-241 —
`internal/cli/mr020_demo_test.go`,
`internal/gate/mr020_demo_test.go`,
`internal/index/symbol/mr020_demo_test.go`); final diff'te
implementation yok (kod 46, göç v8). `make verify` yeşil (check +
race + smoke), `make tidy-check` yeşil, test sayımı 1308
(`go test -list '.*' ./... | grep -c '^Test'` — driver silme sonrası
MR-019 baseline'ine döndü, beklenen).
