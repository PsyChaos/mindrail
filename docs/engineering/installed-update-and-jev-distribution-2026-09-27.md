# Installed update and JEV distribution — 2026-09-27

Status: FROZEN FOR IMPLEMENTATION
Tier: 3 — backwards compatibility, repository setup, deployment, and secret handling
Base: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`

## Original request

> bu update olayini yapman lazim. hali hazirda proje de zaten "mindrail init" yapildi.
>
> ayrica jev key'yi nereye girecegim?

## User outcome

An already initialized repository can be refreshed by installing the new Mindrail
binary and running one explicit repository update command. The update applies embedded
database migrations, refreshes only Mindrail-managed agent instructions and preserves
the existing hook, configuration, knowledge, identities, and user-authored files.

Optional JEV routing is supplied by the trusted installed binary rather than by
secret-handling executable code committed into each project. `TYPESAFE_API_KEY` is
read only from the coding-agent process environment. Without the variable, the normal
non-JEV flow remains unchanged.

## Requirements and acceptance criteria

| ID | Requirement | Type | Source | Acceptance criteria |
| --- | --- | --- | --- | --- |
| REQ-001 | Provide a visible `mindrail update` command for an existing initialized worktree. | FUNCTIONAL / COMPATIBILITY | User request | On an initialized repository it runs the established ModeInit migration/setup pipeline and reports `update`; repeated runs are byte-stable. On an uninitialized repository it performs no partial setup and directs the user to `mindrail init`. |
| REQ-002 | Preserve existing project and runtime data during update. | NON_REGRESSION | Existing init/upgrade contract | Repository config, knowledge, workspace identity, coordination data, user AGENTS text, and foreign hooks survive; only managed content and embedded migrations change. |
| REQ-003 | Make source-built binary replacement explicit and reproducible. | DEPLOYMENT / DOCUMENTATION | Repository currently has no publisher or self-update channel | `make install` builds and atomically installs to a configurable user prefix/bin directory; docs distinguish binary replacement from repository update. No network self-update is claimed. |
| REQ-004 | Distribute the canonical JEV adapter with the installed binary, not as mutable project executable code. | SECURITY / DEPLOYMENT | Threat analysis | A hidden `mindrail agent route` bridge executes the embedded adapter, consumes stdin, and inherits only the process environment. Updating an existing repository requires no manual adapter copy. |
| REQ-005 | Keep JEV strictly optional and fail open. | FUNCTIONAL / COMPATIBILITY | User request | Missing/blank `TYPESAFE_API_KEY` returns typed disabled JSON without reading stdin, launching Python, or making a network call. Missing Python, unsupported deadline capability, provider failures, or invalid provider responses return typed fallback data and preserve normal routing. |
| REQ-006 | Keep the JEV key out of repository state and observable command data. | SECURITY | User question; TypeSafe Bearer contract | The key is accepted only via `TYPESAFE_API_KEY` in the agent process environment, never `.mindrail/config.toml`, AGENTS, argv, request JSON, logs, or committed files. Managed guidance says to use a secret manager/keyring and inject the variable before launching the agent. |
| REQ-007 | Minimize data disclosed to TypeSafe. | SECURITY / DOCUMENTATION | Inferred consequence of external routing | Managed guidance forbids raw secrets/logs/source and requests only the smallest non-sensitive goal/context and closed candidate descriptions needed for the judgment. |
| REQ-008 | Existing and fresh installations receive consistent guidance. | COMPATIBILITY / DOCUMENTATION | Existing setup contract | `mindrail init` and `mindrail update` both render the same managed JEV instructions; existing non-managed user text is untouched. This repository's obsolete duplicate outer JEV block is removed. |
| REQ-009 | Cover update, bridge, secret, and upgrade branches with tests. | TESTING | Completion contract | Tests cover initialized update, uninitialized refusal/no writes, migration/data preservation, managed-block refresh, hook preservation, idempotence, key/no-key bridge paths, missing interpreter, canonical embedded asset, and subprocess behavior. Full repository gates pass. |

## Design decisions

| ID | Decision | Evidence | Impact |
| --- | --- | --- | --- |
| DEC-001 | `mindrail update` reuses the same ModeInit application path as `init`, after a read-only initialized-state preflight. | Existing migrations and setup are already idempotent and extensively upgrade-tested. | No second migration implementation or divergent setup semantics. |
| DEC-002 | Binary replacement remains an explicit local build/install step. | There is no release publication endpoint, package-manager contract, signature channel, or safe self-update source. | `make install` plus `mindrail update` is honest and reproducible; no implicit network access. |
| DEC-003 | Embed one canonical Python adapter and expose it through hidden `mindrail agent route`. | A repository-owned executable receiving the key can be modified by collaborators; production Go intentionally excludes HTTP. | Trusted adapter bytes come from the installed binary while network logic remains outside the Go core. Python 3 is an optional JEV-only runtime. |
| DEC-004 | The bridge itself is fail-open and treats a valid typed adapter result as data, not a Mindrail gate verdict. | JEV must never become an availability or authorization dependency. | Agent guidance decides whether to consume `status: ok`; every other status continues normal routing. |
| DEC-005 | Do not store or auto-load the key from repository files. | `.mindrail/config.toml` is version-controlled and strict; generic `.env` files are easy to commit and are not currently loaded. | Users inject `TYPESAFE_API_KEY` from a password manager/keyring into the process that launches Codex/Claude/VS Code. |

## Work breakdown

| Task | Objective | Requirements | Expected scope | Depends on |
| --- | --- | --- | --- | --- |
| TASK-001 | Add the embedded JEV adapter package and hidden CLI bridge. | REQ-004–007, REQ-009 | `internal/agent/**`, `internal/cli/agent.go`, CLI wiring/tests | none |
| TASK-002 | Add the explicit repository update command by sharing the init pipeline safely. | REQ-001, REQ-002, REQ-008, REQ-009 | `internal/cli/init.go`, `internal/cli/update.go`, CLI tests/goldens | none |
| TASK-003 | Move JEV guidance into the managed setup contract and remove obsolete repository-local duplication. | REQ-005–008 | `internal/setup/**`, `AGENTS.md`, old `.claude` adapter/tests | TASK-001 |
| TASK-004 | Add install/update/key documentation and a reproducible install target. | REQ-003, REQ-006–008 | `Makefile`, `README.md`, `docs/usage-tr.md` | TASK-001–003 |
| TASK-005 | Integrate, validate, independently audit, and refresh Graphify. | REQ-001–009 | audit artifacts, `graphify-out/**` | TASK-001–004 |

## Validation map

| Requirements | Verification |
| --- | --- |
| REQ-001, REQ-002, REQ-008 | Targeted CLI update/upgrade and setup tests |
| REQ-004–REQ-007 | Embedded adapter Python suite plus compiled-binary subprocess tests |
| REQ-003 | Make install target test/dry-run inspection and clean binary version/smoke evidence |
| REQ-001–REQ-009 | `make tidy-check`, `make verify`, and `make gate` after independent audit |

## Out of scope

- Downloading or replacing the Mindrail binary from an unauthenticated remote source.
- Storing, prompting for, printing, or rotating a user's TypeSafe key.
- Adding JEV as a fourteenth Mindrail MCP tool or allowing it to authorize actions.
- Loading arbitrary project `.env` files.
- Deleting or rewriting the pre-existing Codex session log that exposed a key; that
  requires explicit user authorization after the key is rotated.
