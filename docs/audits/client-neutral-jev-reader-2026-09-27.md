# Client-neutral JEV — Reader audit — 2026-09-27

## Audit boundary

- Role: Reader / conformance. This report was written without reading the
  Breaker's conclusions and does not include mutation or adversarial results.
- Mode: two auditors running concurrently; this file is the independent Reader
  half of the dual-agent audit.
- Base: `cd0d1e1e5c2fc2557ec4aa717b118f32e0d65691`
- Audited state: the working tree visible on 2026-09-27, including untracked
  `internal/credential/**`, `internal/jevconnect/**`, and
  `internal/cli/jev*.go` files.
- Authoritative requirement source:
  `docs/engineering/client-neutral-jev-credentials-2026-09-27.md`.

## Original task, verbatim

> “Tool calling, agent, model/effort secimi kararlari gibi isler icin JEV AI
> kullanmayi oneriyorum”

> “jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse
> normal akis devam etsin”

> “hali hazirda proje de zaten mindrail init yapildi”

> “jev key'yi nereye girecegim?”

> “ben terminal kullanmiyorum ki!”

> “codex desktop kullanmayabilirim. belki claude ya da baska bir sey
> kullanacagim”

> “yap o zaman neyi bekliyorsun”

## R1 — Task normalization

| ID | Requirement | Source quote | Type | Verification method |
| --- | --- | --- | --- | --- |
| REQ-001 | JEV activation and credential discovery are independent of a particular coding-agent client. | “codex desktop kullanmayabilirim. belki claude ya da baska bir sey kullanacagim” | COMPATIBILITY | Trace credential resolution and inspect generated/client-facing guidance. |
| REQ-002 | A long-lived key is held below the repository and agent configuration layers in a supported OS credential store; unsupported persistence fails closed. | “jev key'yi nereye girecegim?” plus the secret-handling consequence recorded in the frozen requirements | SECURITY | Inspect platform backends and run store/sanitization tests. |
| REQ-003 | A non-terminal user can submit the key without placing it in agent chat. | “ben terminal kullanmiyorum ki!” | FUNCTIONAL | Exercise the loopback browser handler and visible `jev connect` surface. |
| REQ-004 | JEV remains optional: a non-blank automation override wins; without either source the prior flow continues. | “key girilirse jev calissin girilmezse normal akis devam etsin” | COMPATIBILITY / NON_REGRESSION | Run route-precedence, missing-key and fallback tests. |
| REQ-005 | The local connection page is loopback-only, bounded and resistant to cross-site/exposure paths. | Consequence of the requested browser credential entry, frozen in REQ-005 | SECURITY | Inspect listener/handler data flow and run handler/browser-launch tests. |
| REQ-006 | Store/browser failures are sanitized and actionable; route failures remain fail-open. | Consequence of keeping JEV optional, frozen in REQ-006 | RELIABILITY | Run store, route and CLI error-contract tests. |
| REQ-007 | README, Turkish usage guidance and managed agent instructions describe the same client-neutral workflow. | “codex desktop kullanmayabilirim...” | DOCUMENTATION | Compare docs, managed template and setup upgrade test. |
| REQ-008 | Targeted tests, the repository gate, graph refresh and independent audit complete the delivery. | Repository completion contract incorporated into the frozen task | ACCEPTANCE | Execute targeted suites; inspect graph; require final gate and Reader/Breaker reconciliation evidence. |
| REQ-009 | Stamped install/release builds identify as `0.2.0`, while unstamped builds remain `dev`. | User's earlier version question, incorporated into the frozen task | DEPLOYMENT | Execute an unstamped binary and the install test. |

No unresolved ambiguity changes the primary outcome. “No terminal” is implemented
as “ask any local agent to start a client-neutral browser flow”, not as an agent- or
desktop-specific settings screen. That interpretation satisfies all quoted user
constraints simultaneously.

## Produced evidence

| Probe | Result |
| --- | --- |
| `go test -count=1 ./internal/credential ./internal/jevconnect` | PASS: both packages. |
| `go test -count=1 ./internal/cli -run 'Test(AgentRoute|JEV|RootHelp|CommandSurface)'` | PASS. |
| `python -B -m unittest discover -s internal/agent -p 'test_jev_route.py'` | PASS: 46 tests. |
| `make install-test` with task-local Go cache and `/tmp` build temp | PASS; installed binary reported `0.2.0`, regular-file replacement passed. |
| `go run ./cmd/mindrail version --json` | PASS: unstamped version was `dev`. |
| `go run ./cmd/mindrail --help` | PASS: visible `jev` and `update` commands; internal `agent route` stayed hidden. |
| `git diff --check` | PASS. |
| `graphify query "sameOriginFormPost connectHandler credential.Store browserCommand" --budget 4000` | FAIL for freshness: the graph returned old `Store`/Python nodes and no symbols or paths from the new `internal/credential` and `internal/jevconnect` packages. |

A broader `go test ./internal/credential ./internal/jevconnect ./internal/cli
./internal/setup` attempt was not accepted as repository-gate evidence: its test
temporary directories were placed below the repository, so Git discovery climbed to
the real repository and encountered the sandbox's read-only `.git/mindrail` runtime.
The focused JEV suites were rerun with `/tmp` and passed. This environment artifact
is not an implementation finding, but it means this Reader did not independently
produce a clean full repository gate.

## R2 — Requirement verdicts

| Requirement | Verdict | Evidence | Notes |
| --- | --- | --- | --- |
| REQ-001 | PASS | `resolveJEVKey` reads only `TYPESAFE_API_KEY` and `credential.Store` (`internal/cli/agent.go:97-134`); the keyring identity is user-global (`internal/credential/store.go:10-13`); the setup upgrade test rejects old client-specific activation text (`internal/setup/setup_test.go:225-243`). | No Codex, Claude, Cursor or repository-private credential source participates in resolution. |
| REQ-002 | PASS | Linux/Windows use `github.com/zalando/go-keyring` (`internal/credential/backend_keyring.go:1-35`, dependency `v0.2.8`); macOS explicitly returns `ErrUnsupportedPlatform` (`internal/credential/backend_darwin.go:1-26`). Store errors are sanitized and reads are bounded; focused store tests passed. Child execution receives only `TYPESAFE_API_KEY` and discards stderr (`internal/cli/agent.go:297-312`). | Runtime integration with a real Secret Service/Credential Manager was not performed by this Reader, but implementation and platform contracts agree with the requirement. |
| REQ-003 | PASS | `mindrail jev connect` is visible (`internal/cli/jev.go:34-55`); production wiring calls `jevconnect.Connect` (`cmd/mindrail/main.go:17-24`); a real loopback-shaped round trip and generic no-echo success passed in `internal/jevconnect`. | User can ask any local agent to invoke the command and enter the key only in the browser form. |
| REQ-004 | PASS | Environment-first precedence and keyring fallback are explicit at `internal/cli/agent.go:97-134`; missing credentials return typed disabled data before stdin/Python (`internal/cli/agent.go:62-73`). Route tests for environment precedence, keyring, missing store, unavailable store and blocking reads passed. | JEV is advisory and unavailable paths preserve normal routing. |
| REQ-005 | PARTIAL | Loopback `127.0.0.1:0`, 32-byte random token, exact host/path/query, same-origin Fetch Metadata, form content type, request/key bounds and security headers are implemented at `internal/jevconnect/connect.go:67-95,190-232,254-307`; hostile-origin and minimal-browser-environment tests passed. | Finding RDR-001: once storage dispatch begins, the advertised finite connection timeout no longer bounds completion. |
| REQ-006 | PARTIAL | Sanitized store/browser errors, typed route fallback, source-only status and indeterminate disconnect are implemented and their focused tests passed. | The same RDR-001 blocking storage case produces no actionable result because connect waits indefinitely after dispatch. Other failure branches conform. |
| REQ-007 | PASS | README `106-139`, Turkish guide `440-473`, managed `AGENTS.md:33-51`, and `.claude/skills/engineering-orchestrator/SKILL.md` consistently describe keyring-first, client-neutral behavior and the optional environment override. Setup upgrade/managed-text tests passed. | Client-specific MCP setup examples elsewhere in the guide are unrelated to JEV credential storage and do not contradict this contract. |
| REQ-008 | PARTIAL | Targeted Go packages, targeted CLI contract, 46 Python tests, install test and diff check passed. | The Graphify index is stale and this Reader did not produce a green full repository gate; Reader/Breaker reconciliation is necessarily pending while the two reports are being written. See RDR-002 and Open Items. |
| REQ-009 | PASS | `Makefile:140` defaults `RELEASE_VERSION` to `0.2.0`; `scripts/test-install.sh:34-51` asserts the stamped installed version and replacement path; `make install-test` passed. An unstamped `go run ... version --json` returned `dev`. | Stamped and developer identities match the frozen contract. |

## Findings

| ID | Severity | Requirement | File / symbol | Problem | Evidence | Confidence |
| --- | --- | --- | --- | --- | --- | --- |
| RDR-001 | MEDIUM | REQ-005, REQ-006 | `internal/jevconnect/connect.go:112-143` `awaitConnectVerdict`; `internal/credential/store.go:138-152` `osStore.Set` | The five-minute connection timeout is finite only before credential storage dispatch. After `connectProcessing`, timeout/cancellation takes `return <-result`; `osStore.Set` deliberately ignores cancellation after dispatch. A backend call that never returns therefore hangs `mindrail jev connect` indefinitely and cannot emit an actionable failure. | Control: focused connector suite passes a normal completed write. Scenario: `TestConnectTimeoutAfterStorageDispatchWaitsForAuthoritativeSuccess` closes the timeout channel and proves the command remains blocked for at least 30 ms until the fake store is manually released (`internal/jevconnect/connect_test.go:180-207`). The same control flow has no later deadline. Expected upper bound: 5 minutes plus bounded shutdown; actual upper bound after dispatch: none. | CONFIRMED |
| RDR-002 | LOW | REQ-008 | `graphify-out/graph.json` | The required graph refresh has not indexed the new credential and browser-connector implementation. | Control query finds existing `internal/agent/jev_route.py` nodes. Scenario query naming `sameOriginFormPost`, `connectHandler`, `credential.Store`, and `browserCommand` returns no new package/symbol paths. `git status --short graphify-out` is empty, consistent with no refresh in this changeset. | CONFIRMED |

### RDR-001 expected and actual behavior

- Expected: REQ-005 says the local setup surface “has a finite timeout”; REQ-006
  requires credential-store failures to become actionable, sanitized results.
- Actual data flow: HTTP POST sets state to `connectProcessing`; any connection
  timeout then blocks on `<-result`; production `Store.Set` waits synchronously for
  the native backend even after its ten-second context expires.
- Contract conflict: the document's explicit non-goal says Mindrail waits for the
  authoritative connect result, while its frozen REQ-005 still promises a finite
  timeout. The code implements the non-goal, not the unqualified acceptance
  criterion. The authoritative requirements must either define an intentionally
  unbounded post-commit phase or the implementation must return a truthful bounded
  indeterminate result analogous to disconnect.

## R3 — Scope compliance

- `UNDER_IMPLEMENTATION`: finite timeout and completion evidence are incomplete as
  described in RDR-001/RDR-002.
- `OVER_IMPLEMENTATION`: none identified.
- `SCOPE_CREEP`: none with product impact. The `.mindrail-task-tmp/` ignore entry is
  validation hygiene; the engineering-orchestrator skill update is directly related
  because it removes client-specific credential discovery from an agent-facing JEV
  caller.
- `BEHAVIORAL_DRIFT`: none identified outside RDR-001's documented timeout
  semantics.
- `REQUIREMENT_MISINTERPRETATION`: none for the user's primary client-neutral,
  optional and browser-based workflow.

The changes to application error codes, root wiring and the narrow `net/http`
architecture exception are necessary consequences of the requested public browser
workflow. Existing setup user text and hooks remain protected by the unchanged
managed-section/update path and its tests.

## R4 — Contract and documentation conformance

The following contracts agree:

- credential precedence: environment override → OS keyring → disabled;
- supported persistence: Secret Service (Linux), Credential Manager (Windows),
  deliberate refusal on macOS;
- public commands: `jev connect`, `jev status`, `jev disconnect`;
- secret boundary: browser POST → store, or resolved key → isolated adapter child;
- response boundary: status/source metadata and sanitized error codes, never the key;
- release identity: stamped `0.2.0`, unstamped `dev`.

One `SPEC_IMPLEMENTATION_CONFLICT` remains: REQ-005 promises a finite timeout, but
the implementation and explicit non-goal deliberately allow an unbounded
post-dispatch wait (RDR-001). No other `DOC_DRIFT` was found.

## Open items

1. Produce the repository-wide acceptance evidence after all audit artifacts are in
   place: run `make gate` (and the repository's required full verification command)
   with temporary directories outside the repository, record the exit status, and
   attach the ten category results. This settles the still-unverified portion of
   REQ-008.
2. Run `graphify update .`, then repeat
   `graphify query "sameOriginFormPost connectHandler credential.Store browserCommand"`.
   The refresh is complete only when the result includes the new Go packages and
   symbols. This settles RDR-002.
3. Reconcile the independent Reader and Breaker reports according to the dual-agent
   protocol. REQ-008 cannot become PASS until every produced finding is either
   confirmed or refuted with the required evidence.
4. To settle RDR-001 without changing the contract, run a production-shaped helper
   whose `Store.Set` never returns, submit a valid same-origin POST, and observe the
   process beyond the documented connection deadline. To close it, either produce a
   truthful bounded terminal/indeterminate result with a regression test, or amend
   REQ-005 so the post-commit wait is explicitly exempt from the finite timeout and
   accept that availability tradeoff.

## Independent conclusion

`PARTIALLY_COMPLIANT`

The user-visible objective is substantially implemented: JEV is optional,
client-neutral, keyring-backed on supported platforms, accessible through a browser
flow, fail-open for routing, documented, and version-stamped. It is not yet exact and
complete against the frozen contract because a credential write can make connect
unbounded, the graph is stale, and final full-gate/dual-audit evidence has not yet
been produced.
