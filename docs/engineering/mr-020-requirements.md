# MR-020 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `9f9fbb6`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-020-design.md](mr-020-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-237…D-244.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-020
- **Tier:** 3 (Program). It demonstrates the 0.1 exit condition
  (kernel-scope §8) against a clean sample repository with two
  agent/worktree scenarios, and records the product owner's release
  decision. No database schema, no wire codes, no MCP surface change,
  no implementation change is expected.
- **Blocked by:** MR-019 (bench report, gate matrix and release stamp
  are the measurement inputs).

This file is frozen **before** any demo run so that an audit has
something to grade against that the demonstrations did not shape. The
milestone runs **one task at a time** as MR-004 through MR-019 did:
each task in §4 is executed, gated by an independent Reader/Breaker
pair and recorded before the next is begun. MR-020's tasks are
demonstrations, not implementation — the gates grade the honesty of
the record, not the shape of new code.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `9f9fbb6` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 36 | `go list ./... \| wc -l` |
| Top-level test functions | 1308 | `go test -list '.*' ./... \| grep -c '^Test'` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| `make verify` | green at MR-019 close | `make verify` |
| `make tidy-check` | green at MR-019 close | `make tidy-check` |
| `make bench` | green at MR-019 close (STRUCTURAL p95 under target) | `make bench` |
| `make gate` | green at MR-019 close (nine categories) | `make gate` |
| Release stamp | linux/amd64 advertised (`make release`) | `make release` |
| Scope hygiene | `internal/ docs/ migrations/ go.mod go.sum Makefile scripts/` clean | `git status --short -- <scope>` |

Pre-existing workspace dirt (`.claude/`, `graphify-out/`, `.gitignore`,
`.wrongstack/`, `.zcode`, old `stash@{0}`) is out of scope — never
touched, never attributed to this milestone.

---

## 1. The scenario, in one paragraph

The product owner watches six bypass attempts fail and one honest run
pass. On a clean sample repository — worked by two agents in separate
worktrees — an agent that forgets `before_change` is still caught by
`reconcile`; a commit pushed with `--no-verify` is re-denied by
`verify --ci` on a fresh clone; a protected-symbol rename either keeps
its `symbol_uid` or blocks with explicit ambiguity; a weakened critical
test blocks even when production code is untouched; and a stale
validation result is refused as completion proof. The one run that
does everything right gets ALLOW. Each attempt is denied (or allowed)
by the independent gate, not by the demonstrating agent's narration,
and each is on record with commands and outputs. The owner then
records the 0.1 release decision — ship or not — by name.

---

## 2. Decisions

MR-019 ended at D-236. These continue the sequence.

### D-237 — demos run against the built binary on scratch sample repos, never against this repo

The demonstrating agent builds `mindrail` once from the frozen tree
and runs every demo inside throwaway repositories under `/tmp`
(fresh `git init`, fresh `mindrail init`). This repo's own history,
knowledge records and worktrees are never the demo subject — a demo
that depends on ambient state proves nothing, and a demo that mutates
this repo risks the baseline above.

### D-238 — the gate, not the narrator, delivers the verdict

For every denial demo, the record must show the independent gate's
own output: the `reconcile`/`verify --staged`/`verify --ci`/
completion-gate DENY with its reason code and `next_action`. An
agent's prose claim ("this would be blocked") is not evidence. The
ALLOW demo likewise shows the gate's ALLOW output, not a claim of
success.

### D-239 — two agents means two worktrees (or two clones), genuinely separate

The "iki ajan/worktree senaryosu" is satisfied by two separate working
copies (worktrees or clones of the sample repo) acting in sequence or
in parallel where the demo requires it — e.g. one agent writes while
the other reconciles, or one commits `--no-verify` while CI checks the
pushed head. A single working copy playing both roles is not a
two-agent demo and fails its task's gate.

### D-240 — a demo that deviates blocks the release, it does not bend the demo

If any bypass attempt unexpectedly succeeds (gate allows what the
exit condition says it must deny), or the honest run unexpectedly
fails, the deviation is recorded verbatim and the release decision in
TASK-04 MUST be "not yet" (or conditional on a named follow-up). Demo
scripts are never edited to make the gate say the desired thing —
that would be the exact bypass the milestone exists to disprove.

### D-241 — no implementation change in this milestone

Expected diff: `mr-020-requirements.md`, `mr-020-design.md`,
`mr-020-findings.md`, the task-list `#### Durum` block, and optional
throwaway demo scripts under `/tmp` (not committed). If a demo
exposes a product bug, the bug is recorded in findings and the
release decision reflects it; fixing it is a new milestone's work,
not a quiet TASK-05.

### D-242 — transcripts are the evidence, kept verbatim

Each demo's record in `mr-020-findings.md` carries: the exact commands
run, the gate's verdict output (reason codes included), and the
sample-repo state that produced it (what changed, which agent, which
worktree). Outputs are pasted, not paraphrased. A gate whose output
cannot be reproduced from the recorded commands fails its task's gate.

### D-243 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-019 process repeats with demo-shaped tasks: one task at
a time, each gated before the next, each recorded in
`mr-020-findings.md` (new file — MR-019's record stays closed). The
gates grade record-honesty (D-238, D-242), two-agent reality (D-239),
deviation handling (D-240) and Turkish-text hygiene. Post-gate hygiene
(`git status`/`git diff` on tracked paths, unique mutant-backup names
where applicable, md5-verified restore + full-suite re-run) applies
to every subagent.

### D-244 — the owner's decision is a named record, not an implication

TASK-04 closes only with an explicit release verdict spoken by the
product owner (the user) in-session and written into findings and the
task-list Durum block: ship 0.1, or not, with conditions. Silence,
absence of objection, or agent inference is not approval. The
milestone — and with it the 0.1 kernel — stays open until that line
exists.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-04. Each task's criteria are dotted under it, in the form
the gates grade. A criterion not met at the end of its task is
recorded as not met, with the reason, in `mr-020-findings.md`.

**REQ-01 — honest-run and forgotten-call demos** (ALLOW path, reconcile catch).
**REQ-02 — hook-bypass and rename demos** (CI catch, identity/ambiguity).
**REQ-03 — test-weakening and stale-evidence demos** (guard block, freshness refusal).
**REQ-04 — release decision and the record** (owner verdict, Durum block, proof).

### TASK-01 — ALLOW yolu + `before_change` unutma

Owns REQ-01 (task-list AC-1, AC-2).

- **AC-01.1** Birincil end-to-end senaryonun başarılı ALLOW yolu demo
  edilir: iki worktree'lü örnek repoda, protokol çağrıları eksiksiz
  yapan ajan current evidence ile completion ister; bağımsız gate
  ALLOW döner (D-238 — çıktıyla).
- **AC-01.2** Aynı senaryoda `before_change` çağrılmadan yapılan
  düzenleme `reconcile` tarafından yakalanır: gate'in gördüğü gerçek
  diff, beyan edilmemiş değişikliği içerir ve completion bu diff
  üzerinden hüküm verir (D-238 — reconcile çıktısıyla).
- **AC-01.3** Her iki demo da iki-ajan gerçekliğini taşır (D-239) ve
  kayıtları komut + çıktı + depo-durumu üçlüsünü içerir (D-242).

### TASK-02 — `--no-verify` + protected-symbol rename

Owns REQ-02 (task-list AC-3, AC-4).

- **AC-02.1** `--no-verify` ile yerelde geçirilmiş bir commit, yeni
  klon üzerinde `verify --ci` tarafından yeniden reddedilir
  (D-238 — CI çıktısıyla, DENY reason code görünür).
- **AC-02.2** Korunan bir sembolün rename'i ya aynı `symbol_uid`
  üzerinde kimliği korur ya da explicit ambiguity ile bloklanır;
  her iki durumda da bağlı invariant sessizce orphan olmaz
  (D-238 — gate/satır çıktısıyla).
- **AC-02.3** Kayıtlar D-239 ve D-242'yi karşılar; sapma varsa D-240
  uygulanır (release hükmü "henüz değil" yönünde etkilenir).

### TASK-03 — kritik test zayıflatma + stale evidence

Owns REQ-03 (task-list AC-5, AC-6).

- **AC-03.1** Production kodu değişmeden yalnızca kritik
  verification testini zayıflatan düzenleme (assertion
  kaldırma/azaltma, skip/xfail marker veya test silme)
  `TEST_GUARD_WEAKENED` blocking bulgusuyla reddedilir
  (D-238 — bulgu çıktısıyla).
- **AC-03.2** Kaynak değiştikten sonra eski validation sonucu
  completion proof'u olarak reddedilir: stale evidence required
  evidence şartını karşılamaz, gate hangi profilin yeniden
  çalışacağını söyler (D-238 — karar çıktısıyla).
- **AC-03.3** Kayıtlar D-239 ve D-242'yi karşılar; sapma varsa D-240
  uygulanır.

### TASK-04 — ürün sahibi kararı ve kapanış kaydı

Owns REQ-04 (task-list AC-7).

- **AC-04.1** Ürün sahibi (kullanıcı) 0.1 release kararını session
  içinde açıkça kayda geçirir: ship / not-yet (+ varsa koşullar).
  Ajan çıkarımı onay sayılmaz (D-244).
- **AC-04.2** `mr-020-findings.md` altı demo + kararı eksiksiz taşır;
  task-list'in MR-020 entriesi MR-019'unki gibi yazılmış bir
  `#### Durum` bloğu alır (kutular: karar varsa ve doğruysa `[x]`).
- **AC-04.3** Kapanış kanıtı: `make verify` + `make tidy-check`
  yeşil, test sayımı tek komutla
  (`go test -list '.*' ./... | grep -c '^Test'`), kod 46, göç v8
  değişmemiş (D-241), graph refresh kendi commit'inde.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is begun.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | ALLOW + forgotten-`before_change` demos | REQ-01 | — |
| TASK-02 | `--no-verify` CI catch + rename demos | REQ-02 | TASK-01 |
| TASK-03 | test-weakening + stale-evidence demos | REQ-03 | TASK-02 |
| TASK-04 | owner verdict, Durum block, proof | REQ-04 | TASK-03 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met
   with the reason in `mr-020-findings.md` (per D-240, an unmet demo
   criterion forces a not-yet release verdict).
2. `make verify` green, `make tidy-check` green, and the test count
   reported with `go test -list '.*' ./... | grep -c '^Test'` — that
   command and no other.
3. No implementation diff: registry still 46, no migration beyond
   `000008`, no MCP surface change (D-241 — proven by grep).
4. Every task in §4 has passed its Reader/Breaker gate before the next
   was begun, and the gate's findings — confirmed, refuted,
   unconfirmed — are in `mr-020-findings.md` per task, with what was
   done about each.
5. The task list's MR-020 entry carries a `#### Durum` block written
   the way MR-019's is, including the owner's named release verdict.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's seven acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| ALLOW yolu demo edilir | AC-01.1 (REQ-01) |
| `before_change` unutma reconcile yakalar | AC-01.2 (REQ-01) |
| `--no-verify` CI yakalar | AC-02.1 (REQ-02) |
| rename kimliği korur / ambiguity bloklar | AC-02.2 (REQ-02) |
| test zayıflatma bloklanır | AC-03.1 (REQ-03) |
| stale evidence reddedilir | AC-03.2 (REQ-03) |
| ürün sahibi kararı kayda geçer | AC-04.1 (REQ-04) |
