# Final Closure Reader — Automatic JEV and Agent Presence

Date: 2026-09-28  
Base: `21f525a`  
Scope: TASK-R3 / TASK-R4 closure  
Final verdict: **REJECT**

This pass audited the verbatim user intent, frozen REQ-001..REQ-012 contract and
the explicit post-audit security amendment. No implementation file was changed;
only this report was written.

## 1. Executive result

| Item | Result | Evidence |
| --- | --- | --- |
| TASK-R3 — canonical identity amendment, UI and docs | **FAIL** | The visible UI and operator docs now accurately describe a server-owned canonical client family and retain the runtime ID, but the dashboard collector still serializes legacy/raw `client_name`, `client_title` and `client_version` database values into the SSE snapshot. A produced legacy-row probe leaked all three markers. |
| TASK-R4 — bounded fallback telemetry | **PASS** | 1,000 saturated-cache calls produced **0 provider calls / 0 telemetry writes**; 2,001 rapid admitted-session calls produced **1 provider call / 3 total telemetry writes**; 1,000 duplicate retries after a telemetry failure produced **2 total write attempts**. |
| BRK-R01 | **Resolved as a product-identity conflict by the explicit amendment, but security closure fails through RDR-F01 below.** | Unknown/custom clients remain present as `unknown-client`; known aliases produce useful canonical families; runtime ID distinguishes individual connections. Raw identity preservation is no longer required. |
| BRK-R02 | **CLOSED** | Provider work and durable fallback evidence are now bounded both for admitted state and full-cache rejection. |
| RDR-RM-001 | **CLOSED** | README, Turkish guide, UI note and frozen document amendment agree on canonical family, `unknown-client`, runtime-ID distinction and non-persistence/non-display of raw title/version. |

Severity inventory at this snapshot:

| Severity | Open |
| --- | ---: |
| CRITICAL | 0 |
| HIGH | 1 |
| MEDIUM | 0 |
| LOW | 0 |

## 2. Verbatim user intent and amended identity contract

The user asked to see how long an agent has been associated with work and its
real-time activity. They did not require arbitrary caller-controlled identity
text. The amended projection still answers the request:

- canonical family identifies a recognized MCP client class;
- `unknown-client` truthfully avoids inventing identity for custom clients;
- the runtime ID distinguishes simultaneous or repeated connections even when the
  family is unknown;
- connected duration, 5 s heartbeat, last MCP activity and
  CONNECTED/IDLE/STALE/ENDED remain visible and live;
- wording continues to deny model/process/thought/token liveness.

The fixed-clock browser contract passed and directly checked canonical
`claude-code`, `unknown-client`, runtime IDs, identity-source wording, status and
duration. Therefore dropping raw title/version does not defeat the user's actual
observability goal and does not constitute over-refusal under the explicit
security amendment.

## 3. TASK-R4 and 1,000-request bounds

The route-state cache remains capped at 256 durable sessions. When full, an
unadmitted session now returns non-consumable `fallback/rate_limited` with
`telemetry_recorded=false` and performs neither provider work nor a database write.
For admitted sessions, duplicate and rate-limit telemetry each has its own bounded
window under the same per-session mutex.

Executed measurements:

| Probe | Requests | Provider calls | Telemetry writes / attempts | Result |
| --- | ---: | ---: | ---: | --- |
| Full 256-entry state cache, unadmitted session | 1,000 | 0 | 0 | PASS |
| Admitted session: initial + duplicate spam + distinct rate-limit spam | 2,001 | 1 | 3 | PASS |
| Successful provider result whose proof write fails, then duplicate spam | 1,001 | 1 | 2 failed attempts | PASS |

The last control is important: successful advice still becomes
`fallback/telemetry_failed` with no usable selections, and repeated requests do not
turn a database outage into one write attempt per request. BRK-R02 is closed without
weakening fail-closed-on-success behavior.

## 4. TASK-R3 UI/docs closure

The user-facing layer now states the amended truth:

- README and `docs/usage-tr.md` say the family is server-owned and derived from
  self-reported `ClientInfo.name`;
- known aliases map to fixed families and unknown/custom names map to
  `unknown-client`;
- runtime ID distinguishes connections;
- raw title/version are promised neither persistence nor display;
- the card label is `CANONICAL CLIENT FAMILY · SOURCE: SELF-REPORTED CLIENTINFO NAME`.

The UI no longer reads `client_title` or `client_version`, and contract tests reject
the old `SELF-REPORTED CLIENT` / `name/version` promise. The security amendment is
explicitly recorded rather than silently rewriting the historical freeze.
Consequently BRK-R01's contract objection and RDR-RM-001's documentation drift are
closed at the UI/documentation layer.

## 5. Open finding

### RDR-F01 — HIGH — legacy raw client identity is still emitted in dashboard SSE

**Files:** `internal/dashboard/collector.go:242-296`,
`internal/dashboard/model.go:204-220`, `internal/dashboard/collector_test.go:31`  
**Requirements:** REQ-004, amended REQ-006/007 contract  
**Confidence:** confirmed by execution

The persistence writer now projects new ClientInfo safely, and the browser ignores
raw title/version. The dashboard read boundary, however, still selects all three
database columns, copies them into `AgentRuntime.ClientName/ClientTitle/ClientVersion`
and JSON-serializes them. `ClientSelfReported` is also still set to true for the
whole projection. No read-side canonicalization or legacy-row cleanup exists.

This matters specifically in a remediation: the previously audited implementation
could persist credential-shaped values in any of these columns. Updating the binary
does not rewrite such rows. A dashboard opened after the update therefore sends the
old raw values to the browser over SSE even though the rendered card ignores two of
them; a legacy raw name is also rendered as the apparent family.

Produced scratch probe:

```text
TestReaderFinalCollectorDoesNotExposeLegacyRawIdentity
input row:
  client_name    = credential-in-name
  client_title   = credential-in-title
  client_version = credential-in-version

serialized dashboard snapshot:
  "client_name":"credential-in-name"
  "client_title":"credential-in-title"
  "client_version":"credential-in-version"

result: FAIL; leaked markers 3/3
```

This is not hypothetical direct-SQL hardening alone. It is the exact persisted row
shape produced by the pre-remediation BRK-001 behavior, and the schema/update path
preserves existing rows. The normal redactor is not an absolute remedy: it may not
know a keyring credential and cannot recognize arbitrary secret-shaped text. The
current collector fixture itself inserts noncanonical title/version values and does
not assert that they are absent from the serialized snapshot.

**Required closure:** project the amended safe identity again at the dashboard read
boundary (or durably sanitize all existing rows), remove raw title/version from the
public snapshot model, and add a regression test beginning with a legacy unsafe row.
The output should retain only a safe canonical family plus runtime ID and operational
timestamps/status. Do not restore arbitrary client identity.

Severity is HIGH because caller-controlled credential material that was already at
rest can cross the dashboard output boundary after the purported security fix. The
loopback/token restriction reduces reach but does not satisfy the explicit absolute
privacy contract.

## 6. Executed evidence

```text
go test ./internal/mcp -run '^TestRoute(FullStateCacheDoesNotAmplifyFallbackTelemetry|AdmittedSessionFallbackTelemetryIsBoundedPerWindow|FallbackTelemetryFailureDoesNotRetryOnEveryRequest)$' -count=1 -v
PASS: 3/3

go test ./internal/dashboard -run '^Test(EmbeddedClientNeverPromisesOrProjectsRawClientTitleVersion|OperatorDocsDescribeCanonicalClientFamilySecurityAmendment|EmbeddedClientBehaviorWithFixedClock)$' -count=1 -v
PASS: 3/3

node --test internal/dashboard/testdata/client_behavior_test.cjs
PASS: 1/1

scratch overlay: TestReaderFinalCollectorDoesNotExposeLegacyRawIdentity
FAIL as designed: name/title/version markers present in serialized snapshot

make check
PASS

make gate
PASS: all ten categories green
  unit 1581; domain 454; integration 12; race 9;
  knowledge-schema 216; mcp-contract 81; git-worktree 79;
  git-worktree-named 8; sqlite-concurrency 17; end-to-end 12;
  end-to-end-smoke 7; install-boundary PASS
```

The green full check/gate evidence is internally consistent but incomplete: existing
dashboard tests inspect browser source and rendered behavior, not the serialized
legacy-row boundary exercised by RDR-F01. Thus green gates cannot override the
produced security counterexample.

## 7. Final decision

**REJECT.** TASK-R4 is complete, and TASK-R3's amended user-facing semantics are
correct and still satisfy the user's real-time presence request. Release approval is
blocked solely by RDR-F01. After read-boundary/legacy-row sanitization and a matching
regression test, rerun the targeted privacy probe plus `make check` and `make gate`;
no other CRITICAL, HIGH, MEDIUM or LOW issue remains from this closure pass.

