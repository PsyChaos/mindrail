# Reader Remediation Delta — Automatic JEV and Agent Presence

Date: 2026-09-28  
Base: `21f525a`  
Remediation target: `BRK-001..005`, `RDR-001`  
Implementation verdict: **PASS**  
Documentation verdict: **FAIL (one LOW drift finding)**

This was a read-only implementation review. No implementation file was changed.
Only this report was written. The frozen `REQ-001..REQ-012` contract remained the
acceptance source; a remediation was not accepted merely because its new unit test
passed.

## 1. Finding-by-finding result

| Original finding | Result | Produced evidence | Requirement result |
| --- | --- | --- | --- |
| BRK-001 — caller-controlled ClientInfo could persist credentials | **PASS** | The persistence boundary now maps known names to a fixed canonical family/title, maps unknown names to a controlled placeholder, and never copies caller title/version. Store scenarios covered configured-key-as-name/title/version, common credential shapes and high-entropy unknown strings; all passed and persisted no forbidden value. Unknown clients still produce a presence row labelled `unknown-client`; known aliases such as `openai-codex` and `claude` remain useful as canonical `codex` and `claude-code`. | REQ-004/006 restored. |
| BRK-002 — throttle state followed transport pointers instead of durable sessions | **PASS** | State is keyed by `run.SessionID`; two transports for one durable session share duplicate/throttle state and serialize provider work (`maximum concurrent=1`), while a new durable session on the same transport receives a fresh provider call. The cache is capped at **256**, active entries are not evicted, and idle entries are eligible after **30 s**. Race-enabled focused scenarios passed. | REQ-010 restored. |
| BRK-003 — transient Start failure was never retried | **PASS** | A failed Start clears the in-flight marker; later tool activity retries and publishes one runtime. Sixteen concurrent recovery callers produced one Start. A scratch integration control using a real SDK client also proved that retry retained the useful `claude-code` identity rather than collapsing to unknown. | REQ-006/007 restored. |
| BRK-004 — transient End failure was permanently lost | **PASS** | A terminal trigger performs three bounded attempts, retains the binding after all three fail, and a later terminal/tool trigger succeeds on attempt four and clears it. Concurrent terminal paths wrote one End for the captured runtime and did not end/clear its replacement. A Start finishing after completion was ended as `replaced` and never attached. | REQ-006/007 restored for the tested lifecycle boundaries. |
| BRK-005 — no exact production 5 s guard | **PASS** | `TestPresenceHeartbeatProductionIntervalIsExactlyFiveSeconds` directly asserts `agentPresenceHeartbeatInterval == 5*time.Second`; the implementation uses that constant whenever no test interval is supplied. | REQ-006/012 restored. |
| RDR-001 — safe fallback ordinal/confidence evidence was dropped | **PASS** | `no_match` keeps quantized confidence without inventing an ordinal; atomic fallback keeps caller-validated ordinal and confidence. End-to-end fallback remained `status=fallback` and `accepted=false` while the recorder received ordinal **1** and confidence **550**. Candidate IDs/descriptions were not written. | REQ-005 restored without making rejected advice consumable. |

## 2. Required discriminating controls

### Privacy and useful identity

The security fix does not suppress presence. `projectClientInfo` is a narrow
persistence projection, not a Start rejection:

- known names/aliases become a fixed family and display title;
- unknown, malformed, high-entropy or credential-shaped values become a controlled
  safe name;
- caller title and version are never copied;
- the configured TypeSafe credential is additionally checked against controlled
  output.

The executed scenario matrix passed for an API key placed independently in name,
title and version. Known-client controls (`openai-codex`, `claude`,
`mcp-inspector`) remained distinguishable, and the unknown-client control still
created a usable presence row. A scratch-only real MCP connection with one injected
Start failure produced two attempts and retained `claude-code` on the successful
retry. Thus the remediation neither persists the tested secrets nor over-refuses
unknown clients.

### Durable routing and bounded cost

`internal/mcp/route.go:96-145` now acquires state by durable session ID. The same
state mutex covers provider work, duplicate history and the minimum interval across
transport boundaries. Executed controls established:

| Scenario | Observed result |
| --- | --- |
| Same durable session, second transport, identical request | `duplicate`, provider not called again |
| Same durable session, two transports concurrently | maximum provider concurrency **1** |
| New durable session on reused transport | `ok`, fresh provider call |
| 306 distinct sessions without ageing | cache length **256** |
| After 30 s idle interval | a new state was admitted |

### Presence lifecycle boundaries

The Start generation/in-flight state prevents duplicate publication and detects a
late result after terminal transition. End is success-latched rather than
attempt-latched: failure leaves a retryable binding, while a successful End becomes
idempotent. Focused race execution covered concurrent Start recovery, late Start,
retryable End and stale-terminal-versus-replacement behavior with no race report.
The exact production heartbeat guard passed at **5 s**.

### Fallback telemetry remains non-consumable

The remediation changes only durable metadata projection. In the exercised atomic
fallback, the response remained `fallback`, the selection remained
`accepted=false`, and safe ordinal/confidence were written as numbers. The response
therefore still instructs the caller to continue normal reasoning; evidence of the
provider result cannot be mistaken for accepted advice. Successful advice still
fails closed to `fallback/telemetry_failed` if its durable event cannot be recorded.

## 3. New finding

### RDR-RM-001 — LOW — canonical identity tradeoff is not documented

**Files:** `README.md:54`, `docs/usage-tr.md:149`,
`internal/dashboard/assets/app.js:183`  
**Requirements:** REQ-007, REQ-011

The remediation intentionally stopped persisting caller title/version and now shows
a fixed canonical family/title or a controlled unknown label
(`internal/agent/presence_store.go:265-293`). The public documentation still says
cards show the MCP client's self-reported **name/version**, and the card itself says
`SELF-REPORTED CLIENT`. Version is now always absent, title is canonical rather than
self-reported, aliases may display under another canonical name, and an unknown
client displays a Mindrail-generated label.

This is a truthful-security tradeoff, not a reason to restore arbitrary durable
text. Update the README, Turkish guide and card note to say that Mindrail derives a
safe canonical client family from the self-reported name, uses a controlled unknown
label when it cannot, and deliberately does not retain caller title/version. The
frozen design's `ClientInfo name/title/version` persistence statement should be
recorded as superseded by the credential-safety invariant rather than silently left
as the operative contract.

Confidence: **CONFIRMED**. This is direct documentation/UI-to-code disagreement.

## 4. Executed evidence

```text
go test ./internal/agent ./internal/mcp ./internal/dashboard ./internal/setup ./internal/cli -count=1
PASS: agent 0.147s; mcp 11.793s; dashboard 0.151s; setup 0.003s; cli 43.691s

go test ./internal/agent -run 'TestPresenceStore(ProjectsUnsafeClientMetadataAndRejectsMismatchedAttribution|NeverPersistsCredentialsOrCallerControlledFreeText|PreservesOnlyKnownCanonicalClientIdentity)' -count=1 -v
PASS: all scenarios

go test ./internal/mcp -run 'Test(RouteStateFollowsDurableSessionAcrossTransportBoundaries|RouteCallsAcrossTransportsForSameDurableSessionAreSerialized|RouteStateCacheIsBounded|RouteDimensionRetainsSafeFallbackEvidence|RouteFallbackResponseStaysUnusableWhileSafeEvidenceIsRecorded|PresenceStartFailureRetriesOnLaterToolActivity|ConcurrentPresenceRecoveryStartsOnlyOneRuntime|LatePresenceStartCannotPublishAfterCompletion|PresenceEndFailureRetriesOnSubsequentTerminalTrigger|ConcurrentTerminalPathsEndOnlyCapturedRuntimeOnce|PresenceHeartbeatProductionIntervalIsExactlyFiveSeconds)' -count=1 -v
PASS: 11/11

go test -race ./internal/mcp -run 'Test(RouteCallsAcrossTransportsForSameDurableSessionAreSerialized|ConcurrentPresenceRecoveryStartsOnlyOneRuntime|LatePresenceStartCannotPublishAfterCompletion|PresenceEndFailureRetriesOnSubsequentTerminalTrigger|ConcurrentTerminalPathsEndOnlyCapturedRuntimeOnce)' -count=1
PASS

scratch overlay: TestProbePresenceRetryRetainsKnownClientIdentity
PASS: attempts=2; successful retry client_name=claude-code
```

The first narrow MCP rerun encountered host `/tmp` exhaustion while linking. The
identical command was rerun with `GOTMPDIR` on the workspace filesystem and passed;
this was an environment-capacity failure, not a product failure. All scratch/cache
artifacts were removed after the probes.

## 5. Delta conclusion

All six remediated implementation findings are closed by discriminating behavior,
not only by source inspection. No new CRITICAL, HIGH or implementation MEDIUM issue
was found. The remaining LOW finding is that public/user-facing identity wording
still describes the pre-remediation data model. Release conformance should remain
`PASS_WITH_LOW_DOCUMENTATION_FINDING` until that wording and the frozen-design
supersession note are aligned.

