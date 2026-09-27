# Optional Jev Routing — Breaker Delta Re-Audit

## Scope and identity

- Original findings: `BRK-001` (whole-operation deadline) and `BRK-002`
  (surviving guard mutations)
- Original report: `docs/audits/2026-09-27-optional-jev/breaker.md`
- Mode: independent Breaker delta pass
- Current adapter SHA-256:
  `41f50dd9ca00618bfa5218cc93d2e1e96cb86aa2f2f84b2cdd7ec84d29b83371`
- Current tests SHA-256:
  `3c64b12cf90d9bc88d6ac8812d585c78f396a1d80d586c3320f254b47d8b3ce1`
- Safety: production source/tests were not modified. Mutations ran only in the
  harness's disposable `/tmp/audit-mutate-*` copies or in an in-memory module.

## Verdict

**PASS.** Both prior findings are closed with reproduced evidence. No survivor or
new delta-scoped failure was produced.

## BRK-001 — CLOSED

The provider operation is now enclosed by `_provider_deadline`, which uses
`ITIMER_REAL`/`SIGALRM` on the main thread and refuses to start the optional request
when that safe deadline mechanism is unavailable or already owned by another timer.
The secret-bearing request is not moved to a worker.

### Whole-operation deadline measurement

The control and scenario used the same valid caller input and fake response. The
only change was the opener/DNS delay.

| Measure | 5 ms opener control | 50 ms opener, 10 ms budget | 50 ms patched DNS, 10 ms budget |
| --- | ---: | ---: | ---: |
| Measured elapsed | 5.55 ms | 10.39 ms | 15.45 ms |
| Result | `ok/advice_available` | `fallback/provider_timeout` | `fallback/provider_timeout` |
| Opener completed after an additional 60 ms wait | yes | **no** | n/a |
| New live threads after the wait | 0 | **0** | 0 |

The original failing arms measured 50.1 ms for the delayed opener and 57.0 ms for
patched DNS under the same 10 ms budget. The corrected delayed-opener arm therefore
returns at the deadline rather than after the 50 ms operation, and the interrupted
same-thread operation does not continue in the background. The DNS arm includes
normal signal delivery/unwind overhead but is no longer tied to the injected 50 ms
stall.

### Timer, handler, and fail-open contexts

| Scenario | Calls | Result | Restoration evidence |
| --- | ---: | --- | --- |
| Timeout with a preinstalled custom handler and no active timer | 1 interrupted | `provider_timeout` | custom handler restored; timer `(0.0, 0.0)` |
| Existing 1-second timer | 0 | `deadline_unavailable` | 1000.0 ms before, 999.9 ms after; existing timer preserved |
| Non-main thread | 0 | `deadline_unavailable` | thread terminated normally; no request |
| `setitimer` capability removed | 0 | `deadline_unavailable` | no request |
| Timer installation raises `ValueError` | 0 | `deadline_unavailable` | custom handler restored; timer `(0.0, 0.0)` |

The focused suite also passed the timer-probe failure, successful-operation
restoration, blocking read, and slow-drip cases. Unsupported/non-main/existing-timer
contexts deliberately preserve the pre-existing normal routing flow instead of
starting an operation that cannot be bounded. The documented invocation is a direct
CLI process, so on this POSIX host it runs in the supported main-thread context.

### Independent reproduction outline

```python
with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01):
    result = jev_route.route(valid_input, api_key="AUDIT_SECRET",
                             opener=opener_that_sleeps_50_ms)

with mock.patch.object(jev_route, "TIMEOUT_SECONDS", 0.01), \
     mock.patch.object(socket, "getaddrinfo", side_effect=slow_dns_50_ms):
    result = jev_route.route(valid_input, api_key="AUDIT_SECRET")
```

Both arms returned `provider_timeout` near the 10 ms deadline. Waiting longer than
the original operation afterward produced neither a completion marker nor a new
thread.

## BRK-002 — CLOSED

### Focused behavioral tests

```sh
python -B -m unittest discover \
  -s .claude/skills/engineering-orchestrator/scripts \
  -p 'test_jev_route.py' -v
```

Result: **45/45 passed** in 0.179 seconds, with no skips on this host.

The new cases use valid surrounding input to reach the intended guard, cover nested
list secret detection, run the real CLI as a subprocess, exercise production socket
timeout wiring, and cover deadline capability/restoration behavior.

### Independent mutation run

```sh
python -B .agents/skills/dual-agent-task-audit/scripts/mutate.py . \
  --files .claude/skills/engineering-orchestrator/scripts/jev_route.py \
  --test-cmd "python -B -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p test_jev_route.py" \
  --max-mutations 200 --timeout 30 \
  --output /tmp/jev-mutations-delta.json
```

| Measure | Original audit | Delta re-audit | Delta |
| --- | ---: | ---: | ---: |
| Detected guards | 76 | 82 | +6 |
| Killed | 47 | **82** | +35 |
| Survived | 29 | **0** | -29 |
| Not mutable | 0 | 0 | 0 |
| Truncated | false | false | unchanged |

There were no survivors to discriminate.

### Manual mutation sanity sample

Three critical harness mutations were independently reproduced in memory to ensure
that “killed” meant the protection was actually disabled:

| Neutralized guard | Produced mutant behavior |
| --- | --- |
| Nested list traversal in `_contains_secret` | 1 provider call; API key present in request body; status `ok` |
| `_provider_deadline.interrupt` raising `_DeadlineExceeded` | delayed opener completed; elapsed 50.3 ms despite a 10 ms budget |
| `if __name__ == "__main__"` | 0 stdout bytes and no `SystemExit`, versus control's 138 bytes and exit 0 |

These are the precise failure classes targeted by the new tests, so the 82/82 result
is not an artifact of syntactically ineffective mutations.

## Regression and compatibility observations

- Missing/blank-key behavior still returns before deadline setup, stdin parsing, or
  network access; the optional-off path is unchanged.
- Deadline unavailability is typed provider fallback with exit-success semantics,
  preserving normal routing.
- No worker or daemon request remains alive after timeout, so the Authorization
  header is not retained by background work.
- The implementation still propagates unrelated programming errors; fail-open
  handling is restricted to expected provider/deadline failures.

## Remaining open items

None within the `BRK-001`/`BRK-002` delta scope. Live TypeSafe contract validation
still requires a disposable provider credential, as recorded in the original report;
it is not evidence against either remediated finding.

## Final decision

- `BRK-001`: **CLOSED**
- `BRK-002`: **CLOSED**
- Delta verdict: **PASS**
