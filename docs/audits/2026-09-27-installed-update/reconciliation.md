# Installed update and JEV distribution — audit reconciliation

## Consensus

Reader: **APPROVE** after delta re-audit
Breaker: **APPROVE** after two delta rounds
Final audit verdict: **VERIFIED**, subject only to the final repository validation run.

The auditors worked independently from the frozen requirements and did not
participate in implementation. Reader and Breaker conclusions were written to
separate artifacts before reconciliation.

## Finding disposition

| Finding | Severity | Produced evidence | Remediation | Closing evidence | Status |
| --- | --- | --- | --- | --- | --- |
| RDR-001 | MEDIUM | Active Claude skill invoked a deleted `.claude` adapter path; command exited 2 with no JSON. | Skill now invokes `mindrail agent route` with the bounded schema, env-only key, minimal-data, atomic, and fail-open contract. | Active stale invocation count 0; disabled and invalid-input bridge probes returned typed JSON at exit 0. | CLOSED |
| RDR-002 | LOW | Graphify still indexed the deleted adapter instead of the embedded bridge. | Ran `graphify update .` after code stabilization. | Graph query/explain resolves `internal/agent` and `internal/cli/agent.go`; deleted adapter node count 0. | CLOSED |
| BRK-001 | HIGH | Destination-directory install falsely succeeded; parent symlink escaped `DESTDIR` and wrote a 27,712,040-byte binary outside it. | Canonical path containment, destination-shape refusal, same-directory temp + `mv -T`, hostile install suite, and explicit gate category. | Seven-arm hostile matrix passed; traversal/symlink/directory refused without outside hash change or temp residue; normal/repeat installs passed. | CLOSED |
| BRK-002 | MEDIUM | Eight meaningful Go bridge guard mutations survived the permanent targeted suite. | Added one discriminating permanent test per guard without changing production bridge behavior. | All eight prior mutants killed, zero survived; source restoration byte comparison passed. | CLOSED |
| BRK-003 | LOW | Gate claimed only Go and Git were required while category ten invoked `make` and Linux/coreutils. | Corrected the executable prerequisite contract. | Seven claim tokens and six concrete executables matched; stale claim count 0; shell syntax passed. | CLOSED |

## Audit coverage

- Embedded Python adapter: 82/82 guard mutations killed in the initial Breaker run.
- Go bridge remediation: 8/8 previously surviving mutations killed.
- Real old-binary initialization → new `mindrail update`: config, knowledge, hook,
  workspace/project identities, and coordination task preserved; repeat update stable.
- Uninitialized update: zero filesystem changes and actionable `mindrail init` remedy.
- Install: normal, replacement, destination-directory, destination-symlink,
  parent-symlink escape, traversal, `DESTDIR=/`, outside hash, and temp residue.
- Secret/path threat model: PATH-controlled interpreter not executed; repository key
  persistence and real exposed session data were not used or modified.

Historical rejected reports remain unchanged as evidence. The authoritative closure
sequence is `reader-delta.md`, `breaker-delta.md`, and `breaker-delta-2.md`.
