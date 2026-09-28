# Reader Audit: Automatic JEV and Agent Presence

Date: 2026-09-28  
Base: `21f525a`  
Audit mode: independent Reader/conformance pass  
Conclusion: `COMPLIANT_WITH_MINOR_ISSUES`

This report was written before reading any Breaker conclusion. The implementation
was not modified during this audit.

## 1. Task normalization

The verbatim user request was normalized before implementation code was read. The
frozen REQ identifiers are retained so this audit can be reconciled mechanically.

| ID | Requirement | Source from the task/authoritative freeze | Type | Verification method |
| --- | --- | --- | --- | --- |
| REQ-001 | Expose client-neutral JEV advice for closed tool/agent/model/effort choices through visible `mindrail_route`. | “JEV … gercek kararlarda” and frozen REQ-001 | FUNCTIONAL | MCP discovery/subprocess test; request/result data-flow review |
| REQ-002 | Fresh managed instructions require one route call before an ambiguous supported choice without prompt ceremony. | “neden ben gorev promt'unda mindrail'den bahsetmek zorundayim” and frozen REQ-002 | FUNCTIONAL | setup golden/contract tests; generated instruction review |
| REQ-003 | JEV is optional/advisory; all unavailable or rejected states continue normal reasoning and weaken no permission/gate. | “key girilirse jev calissin girilmezse normal akis devam etsin” | BEHAVIORAL / NON_REGRESSION | router/MCP tests; failure-path review |
| REQ-004 | Automatic routing minimizes disclosure and does not persist raw request/provider/credential material. | Frozen REQ-004 | SECURITY | schema/store/API review; prohibited-column tests; dashboard projection review |
| REQ-005 | Persist bounded proof of actual JEV invocation, separate configured state from used state. | “gercekten aktive” plus frozen REQ-005 | FUNCTIONAL / SECURITY | route-store/MCP/collector tests and end-to-end data-flow review |
| REQ-006 | Record a unique short-lived MCP connection presence with 5 s heartbeat; end on disconnect/completion; crash becomes stale after 15 s. | “bir agent'in ne kadar sure calistigini aktifligini real time gormek istiyorum” plus frozen REQ-006 | FUNCTIONAL | store/lifecycle/race tests and lifecycle review |
| REQ-007 | Dashboard truthfully shows duration, heartbeat/activity and CONNECTED/IDLE/STALE/ENDED with 1.5 s SSE and 1 s client updates. | Same user quote plus frozen REQ-007 | FUNCTIONAL / NON_REGRESSION | collector and Node client tests; server/browser boundary review |
| REQ-008 | Preserve hidden CLI version-1 JSON and no-key behavior. | Frozen REQ-008 | COMPATIBILITY | CLI compatibility tests and extraction diff review |
| REQ-009 | Add deterministic additive schema 11 upgrade without changing shipped migrations or losing old rows. | Frozen REQ-009 | COMPATIBILITY | migration/load/checksum/upgrade tests |
| REQ-010 | Bound input, provider work and telemetry; serialize per session; throttle 10 s and deduplicate 30 s. | Frozen REQ-010 | SECURITY / PERFORMANCE | adapter bounds, session-state code and concurrency/throttle tests |
| REQ-011 | Document observability/model limits and restart/reconnect requirement. | Frozen REQ-011 | DOCUMENTATION | README, Turkish guide and managed-text comparison |
| REQ-012 | Targeted/full validation and independent dual-agent audit gate release. | Frozen REQ-012 | ACCEPTANCE | rerun targeted tests and `make check`; inspect audit artifacts after both agents finish |

No task ambiguity changes the acceptance target: “real time activity” is explicitly
defined by the frozen contract as MCP connection heartbeat plus Mindrail tool-call
activity, not process/model/thought/token liveness.

## 2. Requirement verdicts

| Requirement | Verdict | Evidence | Notes |
| --- | --- | --- | --- |
| REQ-001 | PASS | `cmd/mindrail` subprocess discovery exposes exactly 14 tools including `mindrail_route`; `internal/mcp/route.go` validates provider selections against caller candidates through the shared router. | Executed `go test ./cmd/mindrail -run TestMCPSubprocessServesFourteenTools -count=1`: PASS. |
| REQ-002 | PASS | `internal/setup/setup.go` makes the visible MCP tool primary, says “exactly once,” and does not require the user to mention Mindrail/JEV; setup tests pin ordering and forbidden legacy wording. | Focused setup/CLI suite: PASS. |
| REQ-003 | PASS | Missing key returns disabled; credential/provider/Python/malformed/throttle cases return fallback; successful advice is cleared when telemetry cannot be written; routing never enters completion/permission paths. | Router, CLI and MCP failure-path tests: PASS. |
| REQ-004 | PASS | `RouteEventInput` has no raw request fields; migration 000011 has metadata-only columns; dashboard selects only those columns. Credential is restricted to child env, stderr is discarded, and validated output is checked for the credential value. | Store prohibited-column test and Python adapter suite (46 tests): PASS. |
| REQ-005 | PARTIAL | Attribution, enums, counts, accepted ordinal/confidence, credential source and duration are persisted; configured and accepted-use states are distinct. | RDR-001: rejected/no-match/atomic-fallback confidence and safe ordinal evidence is discarded even though the validated result contains it and the schema explicitly supports it. |
| REQ-006 | PASS | Bootstrap creates a connection-owned runtime; 5 s heartbeat is independent of the 20 min lease; conditional updates cannot resurrect ended rows; disconnect/completion/replacement/shutdown end bindings. | Focused MCP/store lifecycle and race-oriented tests: PASS. |
| REQ-007 | PASS | Server uses inclusive 15 s/30 s boundaries and ENDED precedence; browser duplicates those boundaries, updates counters every 1 s, freezes ended duration, and labels client identity self-reported. SSE remains 1500 ms. | Dashboard Go + Node contract tests: PASS. |
| REQ-008 | PASS | CLI delegates to the shared router while preserving the raw version-1 JSON; exact disabled JSON and existing success/fallback/security cases remain pinned. | Focused agent/CLI suite: PASS. |
| REQ-009 | PASS | 000011 only creates two tables and indexes after 000010; migration loader/schema/upgrade/checksum tests passed, including preservation of older rows and schema version 11. | Migration and shipped-migration suites: PASS. |
| REQ-010 | PASS | Existing 64 KiB request, 1 MiB bridge-output, 256 KiB provider-response, deadline and 32-candidate limits remain; each MCP session owns a serialized route state with 10 s minimum provider interval and 30 s duplicate window. | Python bounds suite and concurrent/throttle MCP tests: PASS. |
| REQ-011 | PASS | README, `docs/usage-tr.md`, managed instructions and UI explicitly deny private ambiguity/thought/token/process observability, deny changing an already-running host model/effort, and require restart/reconnect for discovery. | Documentation/implementation comparison: consistent. |
| REQ-012 | NOT_VERIFIABLE | Targeted suites and an independent `make check` run passed. | Reader and Breaker reports plus reconciliation/security acceptance were not all available when this independent report was written; final release-gate verdict belongs to reconciliation. |

## 3. Findings

| ID | Severity | Requirement | File / symbol | Problem | Evidence | Confidence |
| --- | --- | --- | --- | --- | --- | --- |
| RDR-001 | MEDIUM | REQ-005 | `internal/mcp/route.go:202`, `routeDimension` | Safe decision evidence is erased for every rejected or atomic-fallback selection. | The shared router retains each validated selection's caller-checked candidate ordinal and confidence (`internal/agent/router.go:248-254`). The provider emits confidence for `no_match`, low-confidence, and atomic-fallback answers (`internal/agent/jev_route.py:272-305`). But `routeDimension` returns immediately whenever `Accepted` is false or the candidate is nil, before writing either confidence or an available ordinal. Migration 000011 and `validateRouteDimension` deliberately allow confidence without an ordinal, and the store test proves that shape. Therefore a real provider invocation that returns `no_match` records only candidate count, while an atomic multi-dimension fallback loses even the safe ordinals/confidences of all provider answers. This is deterministic and makes the durable “actually invoked” proof materially less complete than the frozen per-dimension selected-ordinal/quantized-confidence contract. Expected: preserve quantized confidence for every validated selection and preserve a safe ordinal whenever the provider selected a caller-supplied candidate, while still preventing fallback advice from being consumed. Actual: both are null on every non-OK result. | CONFIRMED |

### RDR-001 data flow

1. The embedded adapter converts a provider `no_match` to a validated selection
   with `candidate:null`, numeric `confidence`, `accepted:false`; low-confidence and
   atomic fallback retain caller-supplied candidates and numeric confidence.
2. `validateResult` accepts those shapes and computes an ordinal for every non-null,
   caller-supplied candidate.
3. `routeDimension` discards the metadata solely because `Accepted` is false.
4. The route event is still successfully written and shown as recorded, but its
   dimension columns cannot establish what bounded provider result was observed.

The correction should affect telemetry projection only. It must not make fallback
selections consumable or expose candidate identifiers/descriptions.

## 4. Scope compliance

- `UNDER_IMPLEMENTATION`: limited to RDR-001's incomplete fallback proof.
- `OVER_IMPLEMENTATION`: none found.
- `SCOPE_CREEP`: none found. Changes are confined to router extraction, schema/stores,
  MCP integration, dashboard projection, managed instructions/docs, contract tests,
  the frozen engineering/audit artifacts, and the required graph refresh.
- `BEHAVIORAL_DRIFT`: none found in the hidden CLI or existing 13 lifecycle tools.
  The fourteenth tool is additive.
- `REQUIREMENT_MISINTERPRETATION`: none found. The implementation correctly treats
  presence as connection/tool telemetry and not thought or process liveness.

## 5. Contract and documentation conformance

- MCP discovery, README and Turkish usage guide agree on 14 tools and the visible
  `mindrail_route` name.
- The MCP response is additive version 1 and always includes
  `telemetry_recorded`; `route_id` appears only after a durable write.
- On successful advice, a telemetry write failure clears selections and returns
  `fallback/telemetry_failed`; the persisted-evidence promise is therefore
  fail-closed while engineering remains fail-open.
- Migration/store/dashboard schemas agree and contain no raw goal, context,
  description, candidate ID, provider body or credential column.
- Server and browser agree at the exact inclusive 15 s heartbeat and 30 s activity
  boundaries and freeze ENDED duration.
- Managed instructions and docs are truthful about client self-reporting and MCP
  observability limits.
- One `SPEC_IMPLEMENTATION_CONFLICT` remains: the durable-event contract says route
  dimensions contain selected ordinal and quantized confidence, while RDR-001 shows
  that valid fallback provider answers do not retain those safe fields.

## 6. Executed evidence

The following commands were executed independently in this Reader pass:

- `go test ./internal/agent ./internal/cli -run 'JEV|AgentRoute' -count=1` — PASS
- `python -B -m unittest discover -s internal/agent -p 'test_jev_route.py'` — PASS, 46 tests
- `go test ./internal/migration ./internal/agent ./migrations -count=1` — PASS
- `go test ./internal/mcp -run 'Route|Presence|Tool|Automatic|Disconnect' -count=1` — PASS
- `go test ./internal/dashboard -count=1` — PASS
- `go test ./internal/setup ./internal/cli -run 'Init|Setup|Usage|JEV|Tool' -count=1` — PASS
- `go test ./cmd/mindrail -run TestMCPSubprocessServesFourteenTools -count=1` — PASS
- focused migration/checksum probes — PASS
- `make check` — PASS

## 7. Open items

1. REQ-012 requires the independent Breaker report, security acceptance and final
   reconciliation. Produce those artifacts, refute or confirm RDR-001 with a
   discriminating telemetry probe, and ensure no unresolved CRITICAL/HIGH finding
   remains before release.
2. To close RDR-001, add a route telemetry test with both `no_match` and an atomic
   multi-dimension fallback. The test must prove confidence (and ordinal when a
   caller candidate was selected) is persisted without candidate IDs/descriptions,
   while the MCP response remains fallback and cannot influence the choice.

## 8. Independent conclusion

`COMPLIANT_WITH_MINOR_ISSUES`

The primary objectives are implemented and the exercised compatibility, privacy,
rate-control, presence-lifecycle and dashboard contracts are strong. The one
confirmed medium issue is loss of bounded fallback decision evidence, not exposure
of sensitive data or consumption of rejected advice. Final release acceptance is
still contingent on the independent Breaker/reconciliation phase required by
REQ-012.
