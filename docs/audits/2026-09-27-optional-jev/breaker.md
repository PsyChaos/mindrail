# Optional Jev Routing — Breaker Audit

## Audit identity

- Original request: “yap onerinede uyuyorum. yanliz jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin”
- Base revision: `ba35fbe`
- Mode: parallel, independent Breaker subagent
- Adapter SHA-256: `cb8523cea27270fd5d5d4c3d545ed5433a6348f4d99cac0b9e282c554914781b`
- Test SHA-256: `6c8fd3c5adf81e916fbc1ea6786aae7d18485e8fb7c393b8596780af099c2a1b`
- Safety: all source mutations were made by the mutation harness in disposable
  `/tmp/audit-mutate-*` copies or in an in-memory module. Production files and
  shared services were not mutated.

## Verdict

**REJECT**. The no-key compatibility path and the principal fail-open/security
behaviors work, but two independently produced defects remain:

1. the advertised total timeout does not bound DNS/open time; and
2. the focused tests do not kill 29 of 76 guard mutations, including 26
   reachable or operationally meaningful guard lines.

## Severity-ranked findings

### BRK-001 — The total deadline does not bound connection setup or DNS

- Severity: **MEDIUM**
- Confidence: **HIGH**
- Requirements: REQ-004, REQ-006
- Location: `.claude/skills/engineering-orchestrator/scripts/jev_route.py:391-397`

`deadline` is computed before `opener.open`, but enforcement is delegated to the
opener's socket timeout. DNS resolution is not interrupted by that timeout. A fake
opener that respects the same public call shape and a production-path probe with
`socket.getaddrinfo` both exceeded the total budget.

Control and scenario used the same valid request and a 10 ms configured budget; the
only difference was the open/DNS delay:

| Measure | Control | Delayed opener | Patched production DNS |
| --- | ---: | ---: | ---: |
| Configured budget | 10 ms | 10 ms | 10 ms |
| Injected delay | 5 ms | 50 ms | 50 ms |
| Measured elapsed | 5.1 ms | 50.1 ms | 57.0 ms |
| Budget multiple | 0.51x | 5.01x | 5.70x |
| Result | `ok` | `fallback/provider_timeout` | `fallback/provider_unavailable` |

Reproduction for the production path:

```python
def slow_dns(*args, **kwargs):
    time.sleep(0.05)
    raise socket.gaierror("audit dns failure")

jev_route.TIMEOUT_SECONDS = 0.01
with mock.patch("socket.getaddrinfo", side_effect=slow_dns):
    result = jev_route.route(caller_input(), api_key="AUDIT_SECRET")
```

The result is sanitized and fail-open, but it arrives after the promised bound.
At the production value the nominal 10-second limit can therefore be exceeded by an
unbounded resolver/open stall, blocking the normal routing path that the optional
feature is required to preserve.

Class sweep:

```sh
rg --hidden -n "urlopen|build_opener|opener\.open|TIMEOUT_SECONDS|socket\.getaddrinfo|settimeout" \
  --glob '!graphify-out/**' --glob '!.git/**' .
```

No other production network adapter with this shape was found.

Suggested fix (not applied or measured in this audit): enforce the deadline around
the complete DNS/connect/write/read operation with a genuinely cancellable boundary,
and add a test that patches `socket.getaddrinfo` to block beyond the budget. A daemon
thread that merely returns while a secret-bearing request continues in the
background is not an adequate fix.

### BRK-002 — Guard mutations reveal masked and missing behavioral tests

- Severity: **MEDIUM**
- Confidence: **HIGH**
- Requirement: REQ-008 (with security relevance to REQ-004/005)
- Locations: `.claude/skills/engineering-orchestrator/scripts/jev_route.py` and
  `test_jev_route.py`

Command:

```sh
python .agents/skills/dual-agent-task-audit/scripts/mutate.py . \
  --files .claude/skills/engineering-orchestrator/scripts/jev_route.py \
  --test-cmd "python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p test_jev_route.py" \
  --max-mutations 100 --timeout 30 --output /tmp/jev-mutations.json
```

| Measure | Untouched control | One guard removed at a time | Delta |
| --- | ---: | ---: | ---: |
| Focused suite | 29/29 pass | baseline remains green for 29 mutations | 29 surviving guards |
| Guard mutations | 76 | 76 | — |
| Killed | — | 47 | — |
| Survived | — | 29 | 38.2% of guards |
| Meaningful/reachable survivors after discrimination | — | 26 | 34.2% of guards |

Three survivors were refuted rather than counted as missing behavior: line 117 is
unreachable through the documented JSON CLI contract, and lines 274-275 are
redundant with the subsequent integer parse and exact candidate-index comparison.
The remaining 26 guard lines belong to reachable checks or user-visible CLI/network
behavior.

Representative discriminating inputs were run against the untouched module and an
in-memory module with only the named guard neutralized:

| Removed guard / input | Control | Mutant |
| --- | --- | --- |
| Nested-list secret descent; `context={"nested":[API_KEY]}` | fallback, 0 calls, key absent from body | `ok`, 1 call, key present in request body |
| Candidate list type; `tools=1` | `fallback/invalid_candidates` | uncaught `TypeError` |
| Unknown-field rejection plus a valid tool candidate | fallback, 0 calls | `ok`, 1 call |
| Goal length guard plus a valid candidate | `fallback/input_too_large`, 0 calls | `ok`, 1 call |
| Answer type guard with otherwise valid probabilities | `fallback/invalid_response`, 0 accepted | `ok`, 4 accepted |
| Confidence type guard with `confidence=true` and otherwise valid probabilities | `fallback/invalid_response`, 0 accepted | `ok`, 4 accepted |
| Choice parse guard with `candidate_x` and valid probabilities | `fallback/invalid_response` | uncaught `UnboundLocalError` |
| Choice index guard with `candidate_99` and valid probabilities | `fallback/invalid_response` | uncaught `IndexError` |
| Direct CLI entry guard, no key | 138 stdout bytes, exit 0 | 0 stdout bytes, no `SystemExit` |

The current invalid-choice unit cases use a one-candidate probability map for the
two-candidate `tool` dimension. Consequently several cases are rejected by the
probability-key check before reaching the choice/type/confidence guard named by the
test. This is a concrete masked-test failure, not a coverage percentage preference.

Class sweep covered every survivor in `/tmp/jev-mutations.json`:

- semantic JSON/secret descent: lines 117, 131;
- local input shape and bounds: 139-181 and 213-214;
- provider answer validation: 233-281;
- production socket/read validation: 331, 348-349;
- executable CLI entry: 440-441.

Suggested fix (not applied or measured in this audit): add table-driven tests whose
surrounding fields are valid enough to reach exactly the intended guard, add nested
list secret coverage and a real subprocess CLI test, then re-run the mutation command
until all meaningful guards are killed. Paired `if`/`raise` mutations may share one
discriminating case, but both mutations must turn the suite red.

## Six mandatory Breaker moves

### B1 — Mutation

Executed all 76 guard-shaped mutations in a disposable copy. Result: 47 killed, 29
survived; 26 survivors remained findings after discriminating inputs. See BRK-002.

### B2 — A/B of deleted behavior

No executable behavior was deleted: the adapter and tests are new files and the two
tracked documentation files only add guidance. Existence probe:

```sh
git cat-file -e ba35fbe:.claude/skills/engineering-orchestrator/scripts/jev_route.py
# exit 128: path did not exist in ba35fbe
```

Therefore an old-adapter/new-adapter A/B is inapplicable. The compatibility control
is the pre-existing no-key workflow exercised under B4.

### B3 — Written threat model

Concrete hostile attempts and results:

| Attempt | Calls / accepted | Result |
| --- | ---: | --- |
| Missing key | 0 calls | `disabled/api_key_missing` |
| Whitespace-only key | 0 calls | `disabled/api_key_missing` |
| Provider error containing `Bearer AUDIT_SECRET` | 1 call, 0 leaked occurrences | sanitized `fallback/provider_unavailable` |
| Malformed response | 0 accepted | `fallback/invalid_response` |
| `candidate_99` with otherwise valid probabilities | 0 accepted | `fallback/invalid_response` |
| Probability sum 1.8 | 0 accepted | `fallback/invalid_response` |
| API key nested in a JSON list | 0 calls, 0 output occurrences | `fallback/secret_in_input` |
| Unicode API key (`é`) | 0 calls | `fallback/invalid_api_key` |
| One low-confidence dimension plus three high-confidence dimensions | 0 accepted | atomic `fallback` |
| Redirect to `https://evil.invalid` | handler returned `None` | redirect refused |
| Caller-controlled URL attempt | no URL input field exists | fixed `https://api.typesafe.ai/v1/systemone` |

No secret disclosure, redirect follow, SSRF, unknown candidate acceptance, or partial
atomic acceptance was produced in the untouched implementation. The deadline attack
did produce BRK-001.

### B4 — Upgrade path

There is no persisted schema or old Jev configuration. The relevant upgrade is an
existing checkout with no new environment variable, followed by first invocation:

| Existing configuration | Exit | stdout lines | status | stderr bytes |
| --- | ---: | ---: | --- | ---: |
| `TYPESAFE_API_KEY` absent | 0 | 1 | `disabled` | 0 |
| `TYPESAFE_API_KEY="   "` | 0 | 1 | `disabled` | 0 |

Both paths return before stdin parsing and before any HTTP call. No backward-
compatibility failure was produced.

### B5 — Weakest satisfying input

New predicates were exercised at their literal minimums:

| Predicate | Weakest/boundary input | Result |
| --- | --- | --- |
| non-blank key | `!` | 1 provider call; advisory result |
| blank-key opt-out | one ASCII space | disabled; 0 calls |
| non-blank goal | one ASCII space | `fallback/invalid_goal`; 0 calls |
| non-empty candidate | one-character id and description | `ok`; 1 call |
| candidate set present | empty list | `fallback/no_candidates`; 0 calls |
| confidence accepted | `0.599999` versus exactly `0.60` | fallback/0 accepted versus `ok`/4 accepted |
| printable ASCII key | embedded ASCII space | `fallback/invalid_api_key`; 0 calls |

No fail-open input was produced. The guard-removal versions of these inputs are
accounted for in BRK-002.

### B6 — Dual readers

The same questions are answered in these places:

- key activation: `route`, `main`, `AGENTS.md`, orchestrator `SKILL.md`;
- atomic acceptance: `_parse_response`, `AGENTS.md`, orchestrator `SKILL.md`;
- local/provider exit semantics: `main`, `AGENTS.md`, orchestrator `SKILL.md`.

Absent, empty, ASCII-whitespace, Unicode, and printable one-character keys were fed
through both executable readers. `route` and `main` share `_normalize_api_key`; no
normalization split was produced. Code and both guidance readers agree that only
top-level `ok` plus all `accepted: true` advice may be applied. No dual-reader
disagreement was produced.

## Mandatory off-spec checks

### Cost

The adapter performs no retries and at most one provider call:

| Limit | Exact worst case |
| --- | ---: |
| Candidate fan-out | 4 dimensions × 32 candidates = 128 candidates |
| Provider requests | 1 × request = 1 request |
| Bounded body traffic | 65,536 request bytes + 262,144 response bytes = 327,680 bytes (320 KiB), excluding transport headers |
| Nominal blocking budget | 1 × 10 seconds = 10 seconds |

Monetary cost is one `systemone` invocation; the repository contains no price value
from which to calculate currency. The byte/call fan-out is bounded. Wall-clock cost
is not actually bounded because of BRK-001.

### Mechanical rule = automated test

No-key/no-network, atomic fallback, fixed URL, redirect refusal, response size, and
provider fallback rules have automated tests. However, the tests do not mechanically
pin 26 meaningful guard lines and do not execute the CLI as a subprocess; this is
BRK-002. The advisory-only scope/permission rule is structural here: the adapter only
returns data and contains no executor or Mindrail gate mutation path.

### Class sweep

Each finding includes its sweep. BRK-001's network shape occurs only in this adapter.
BRK-002 swept all 76 detected guard lines, not only a sample.

### Backward compatibility

The concrete existing-installation combination is: repository at `ba35fbe`, no
`TYPESAFE_API_KEY`, arbitrary pre-existing task data, then first run after upgrade.
The adapter returns typed disabled output with exit 0 without reading task data or
calling the network. No old config key, persisted data shape, or migration exists.

## Refuted leads

| Lead | Refuting evidence |
| --- | --- |
| Unicode-equivalent key can leak through normalization | Non-ASCII keys (`é`, NFD forms in the focused suite) are rejected before input or network; 0 calls. |
| Redirect or caller URL creates SSRF | Redirect handler returns `None`; endpoint is a constant and no URL input field is accepted. |
| Hostile choice/probability enables partial advice | Properly formed `candidate_99`, sum-1.8 probabilities, low confidence, and `no_match` all produced fallback with 0 accepted selections. |
| All 29 mutation survivors represent unique defects | Three were refuted: documented CLI JSON cannot produce line 117's non-JSON object; lines 274-275 are redundant with later exact parsing. |

## Open items

| Item | Why not produced | Exact settling experiment |
| --- | --- | --- |
| Live TypeSafe contract compatibility | No disposable TypeSafe credential was available; using a real credential is an external side effect and unnecessary for hostile offline testing. | With a disposable scoped key, submit one private-data-free request containing one candidate per dimension to `https://api.typesafe.ai/v1/systemone`; require HTTP 2xx, exact answer keys, valid normalized probabilities, and adapter `status=ok`, then revoke the key. |
| Corrected deadline design | The audit must not implement production fixes, and cancellation semantics require an explicit platform design. | Apply the proposed cancellable whole-operation boundary in a scratch branch; run 5 ms and 50 ms patched-DNS arms with a 10 ms budget. Accept only if the control stays `ok`, scenario returns fallback by 20 ms, no worker remains alive, and the Authorization header is not retained by background work. |

## Commands executed

```sh
python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p 'test_jev_route.py' -v
# 29/29 passed

python .agents/skills/dual-agent-task-audit/scripts/mutate.py . \
  --files .claude/skills/engineering-orchestrator/scripts/jev_route.py \
  --test-cmd "python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p test_jev_route.py" \
  --max-mutations 100 --timeout 30 --output /tmp/jev-mutations.json
# 47 killed / 29 survived
```

Full repository gates were intentionally left to the audit orchestrator; this
Breaker pass ran the cluster-specific command from the audit plan.
