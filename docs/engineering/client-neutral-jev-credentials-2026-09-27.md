# Client-neutral JEV credentials — 2026-09-27

Base: `cd0d1e1e5c2fc2557ec4aa717b118f32e0d65691`

Tier: 3 — this changes credential storage, a user-visible CLI contract, deployment
behavior, and compatibility across coding-agent hosts.

## Frozen requirements

| ID | Requirement | Type | Source | Acceptance criteria |
| --- | --- | --- | --- | --- |
| REQ-001 | JEV configuration must not depend on Codex, Claude, Cursor, or another agent's private configuration. | COMPATIBILITY | User: “belki claude ya da başka bir şey kullanacağım” | A credential stored once is available to `mindrail agent route` regardless of the parent agent, and no client-specific file is read. |
| REQ-002 | Long-lived JEV credentials must be stored in a secure operating-system credential store, never in the repository or Mindrail config. | SECURITY | User correction; inferred secret-handling consequence | Supported production backends use Secret Service on Linux and Credential Manager on Windows. A platform without an application-bound native backend refuses persistence. Tests prove repository files, stdout, stderr, argv, and request JSON never receive the credential. |
| REQ-003 | A user who does not use a terminal must be able to connect JEV without pasting the key into agent chat. | FUNCTIONAL | User: “ben terminal kullanmıyorum” | `mindrail jev connect` opens a loopback-only browser form, accepts the key through a bounded POST, stores it, shows a generic success page, and never renders the key. |
| REQ-004 | Environment configuration remains an optional automation override. | COMPATIBILITY | Existing public behavior; CI/container use | A non-blank `TYPESAFE_API_KEY` wins over keyring. When neither source has a key, routing returns the existing disabled result and normal flow continues. |
| REQ-005 | The local setup surface must resist cross-site requests and accidental exposure. | SECURITY | Inferred from browser-based secret entry | Server binds only to loopback on an ephemeral port; requires an unguessable per-run token, exact host/path, POST content type and bounded body; emits no-store/CSP/referrer/frame protections; and times out while awaiting submission. Once a credential mutation crosses its commit point, Mindrail waits for the authoritative result instead of reporting a false timeout. |
| REQ-006 | Credential-store and browser failures must be actionable without disclosing secrets or breaking normal routing. | RELIABILITY | Inferred | Connect fails visibly with sanitized errors; route fails open with a typed result; status reports only connected/source metadata; an indeterminate bounded disconnect says deletion may still complete and directs the caller to verify status. |
| REQ-007 | The new workflow must be documented for agent-neutral use and generated Mindrail guidance must not prescribe Codex-specific configuration. | DOCUMENTATION | User correction | README, Turkish usage guide, and managed AGENTS text describe keyring-first behavior and the environment override without naming a required client. |
| REQ-008 | The delivery must be regression-tested and independently audited. | TESTING | Repository completion contract | Targeted credential/UI/route tests, full repository gate, Graphify refresh, and Reader/Breaker consensus are green. |
| REQ-009 | The release line that contains the public connect/update commands must identify as `0.2.0` when stamped. | DEPLOYMENT | Prior version question; semantic-versioning consequence | Make release/install default is `0.2.0`; unstamped developer builds continue to identify as `dev`. |

## Design and decisions

| Decision | Context | Evidence | Impact |
| --- | --- | --- | --- |
| DEC-001 | Credential backend | Cross-client use requires the secret to live below the agent layer. | Mindrail owns a small credential-store interface backed by the OS keyring. |
| DEC-002 | Credential precedence | Existing automation must not break. | `TYPESAFE_API_KEY` (non-blank) → OS keyring → disabled normal flow. |
| DEC-003 | GUI-neutral setup | There is no portable configuration UI shared by all coding agents. | A loopback browser form is launched by `mindrail jev connect`; any agent capable of invoking Mindrail can start it for the user. |
| DEC-004 | Secret transport | The existing Python adapter reads `TYPESAFE_API_KEY`. | The Go bridge injects only the resolved key into the child environment and never mutates the parent environment or adds it to argv/stdin. |
| DEC-005 | Stored identity | One local user needs one TypeSafe credential independent of repositories. | Keyring service `mindrail`, account `typesafe-api-key`; no project identifier is included. |
| DEC-006 | macOS secure-storage boundary | The generic adapter shells out to `/usr/bin/security`, which would bind access to that utility rather than to Mindrail. | Refuse persistent storage on macOS until an application-bound native Keychain backend exists; the optional environment override remains available. |
| DEC-007 | Credential mutation commit point | Opaque native credential APIs cannot simultaneously promise cancellation and prove that no write/delete will complete after a timeout. | Cancellation is authoritative before dispatch. After dispatch, connect waits for the actual write result; disconnect remains bounded but reports an explicit indeterminate outcome and requires a status check. |

## Work breakdown

| Task | Objective | Requirements | Scope | Depends on |
| --- | --- | --- | --- | --- |
| TASK-001 | Add OS-keyring credential abstraction and route resolution. | REQ-001, REQ-002, REQ-004, REQ-006 | `internal/credential`, `internal/cli/agent*`, dependency manifests | — |
| TASK-002 | Add loopback browser connection/status/disconnect workflow. | REQ-003, REQ-005, REQ-006 | `internal/cli/jev*`, root command registration | TASK-001 |
| TASK-003 | Update managed guidance, docs, release version and tests. | REQ-007, REQ-009 | setup/docs/Makefile/tests | TASK-001, TASK-002 |
| TASK-004 | Integrate, validate, refresh graph, and audit. | REQ-008 | repository-wide | TASK-001–003 |

Dependency graph: `TASK-001 → TASK-002 → TASK-003 → TASK-004`.

## Test map

| Requirement | Verification |
| --- | --- |
| REQ-001, REQ-002, REQ-004 | Credential resolver and agent-route subprocess tests with fake keyring and sentinel secrets. |
| REQ-003, REQ-005 | HTTP handler tests plus real loopback connect integration using a fake browser and fake keyring. |
| REQ-006 | Missing keyring, denied keyring, browser failure, timeout, invalid request and sanitized-output tests. |
| REQ-007 | Setup golden/contract tests and documentation review. |
| REQ-008 | `make gate`, focused Python suite, Graphify update, dual-agent audit. |
| REQ-009 | Version stamp/install tests asserting `0.2.0`; developer default remains `dev`. |

## Explicit non-goals

- TypeSafe account creation, billing, OAuth, or key rotation.
- A client-specific Codex/Claude/Cursor settings integration.
- Persisting the key in `.mindrail/config.toml`, `.env`, project files, logs, or
  agent conversation state.
- Sending arbitrary repository content to TypeSafe.
- Guaranteeing cancellation of an operating-system credential mutation after
  the native API has accepted it. Mindrail waits for the authoritative result
  during connect; a bounded disconnect that cannot observe the result reports
  `JEV_DISCONNECT_INDETERMINATE` and requires a subsequent status check.
