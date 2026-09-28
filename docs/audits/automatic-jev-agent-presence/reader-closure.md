# Reader Closure — RDR-F01

Date: 2026-09-28  
Scope: legacy client-identity read boundary only  
Final verdict: **APPROVE**

No implementation file was changed during this pass. The exact prior scenario was
recreated with three independent markers in a legacy `agent_runtimes` row:

```text
client_name    = legacy-name-keyring-marker
client_title   = legacy-title-typesafe-marker
client_version = legacy-version-secret-marker
```

## Produced control/scenario evidence

| Boundary | Database markers after read | Markers in projection | Raw title/version JSON keys | Safe identity retained |
| --- | ---: | ---: | --- | --- |
| Raw SQLite control | **3/3** | n/a | n/a | `RUN-LEGACY` present |
| `PresenceStore.Get` | **3/3** | **0/3** | n/a; internal `ClientInfo` title/version values are empty | canonical safe client info + runtime ID |
| `PresenceStore.ListProject` | **3/3** | **0/3** | n/a; internal `ClientInfo` title/version values are empty | canonical safe client info + runtime ID |
| Collector snapshot JSON | **3/3** | **0/3** | `client_title` absent; `client_version` absent | `client_name=unknown-client`, `id=RUN-LEGACY` |
| Real loopback REST `/api/snapshot` | **3/3** | **0/3** | both absent | `unknown-client` + `RUN-LEGACY` |
| Real loopback SSE `/events` first snapshot frame | **3/3** | **0/3** | both absent | `unknown-client` + `RUN-LEGACY` |

The real transport probe used an actual loopback `httptest` TCP listener and
`http.Client`, not direct handler invocation. The sandbox initially denied socket
creation; the identical local-only probe was rerun with loopback permission and
passed.

The database intentionally remains unchanged at 3/3. Safety is enforced on every
read through `agent.CanonicalClientInfo`, so old rows cannot escape through internal
Get/List consumers, collector JSON, REST or SSE. New writes already pass through the
same projection. This closes the upgrade/remediation path that produced RDR-F01.

## Contract consistency

The public dashboard model now exposes only:

- `client_name`, whose value is the server-owned canonical family;
- runtime ID and operational attribution/timestamps/status.

It has no title/version fields or self-reported boolean. The browser consumes
`client_name` only, labels it `CANONICAL CLIENT FAMILY`, and continues to show the
runtime ID. Unknown/custom legacy input therefore remains operationally
distinguishable by connection without claiming a raw caller identity.

README, Turkish usage documentation, UI wording and the post-audit security
amendment all describe the same behavior: canonical family derived from the
self-reported name, controlled unknown projection, runtime-ID distinction and no raw
title/version persistence or display. No new documentation/model drift was found.

## Executed commands

```text
go test ./internal/agent \
  -run '^TestPresenceStoreProjectsLegacyClientMetadataOnEveryReadWithoutRewriting$' \
  -count=1 -v
PASS

go test ./internal/dashboard \
  -run '^Test(CollectorProjectsLegacyClientMetadataBeforePublicJSON|ServerSnapshotNeverExposesLegacyRawClientMetadata|EmbeddedClientNeverPromisesOrProjectsRawClientTitleVersion|OperatorDocsDescribeCanonicalClientFamilySecurityAmendment|EmbeddedClientBehaviorWithFixedClock)$' \
  -count=1 -v
PASS: 5/5

node --test internal/dashboard/testdata/client_behavior_test.cjs
PASS

scratch overlay: TestReaderClosureRealRESTAndSSEProjectLegacyIdentity
PASS: DB 3/3; REST 0/3; SSE 0/3; retired keys absent; safe family/runtime ID present

make check
PASS

make gate
PASS: all ten categories green
  unit 1584; domain 454; integration 12; race 9;
  knowledge-schema 216; mcp-contract 81; git-worktree 79;
  git-worktree-named 8; sqlite-concurrency 17; end-to-end 12;
  end-to-end-smoke 7; install-boundary PASS
```

## Closure decision

RDR-F01 is **CLOSED**. The original 3/3 leak is now 0/3 at every exercised internal
and public read boundary while canonical family, runtime ID and real-time presence
semantics remain intact. No CRITICAL, HIGH, MEDIUM or LOW finding remains from this
closure pass. **APPROVE.**
