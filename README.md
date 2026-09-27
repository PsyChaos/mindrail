# Mindrail

A local-first engineering gate for AI coding agents. Mindrail discovers the
actual change from Git, binds repository Decisions and Invariants to that
change, requires snapshot-bound evidence, and denies completion when the proof
is insufficient. The interactive path is local: no account, cloud service, or
network connection is required.

## Start here

Build or install `mindrail`, then run this once in each repository/worktree:

```bash
mindrail init
```

Sıfırdan kurulum, mevcut repository'ye güvenli entegrasyon ve Codex, Claude
Code, VS Code ya da başka bir stdio MCP istemcisi için gerçek yapılandırma
örnekleri: [Türkçe kullanım kılavuzu](docs/usage-tr.md).

That single command creates the repository configuration and knowledge
directories, initializes the runtime database, updates only Mindrail's managed
section in `AGENTS.md`, and installs a pre-commit gate while preserving and
chaining an existing repository hook. It is safe to repeat.

Normal human use is four commands:

```bash
mindrail init       # once per repository/worktree
mindrail update     # refresh an already-initialized worktree after binary replacement
mindrail status     # readiness summary
mindrail doctor     # detailed, read-only diagnosis
mindrail verify     # verify staged changes locally
```

To watch the local runtime as an operations board, start the read-only live
dashboard and open the printed tokenized URL:

```bash
mindrail dashboard
```

It binds only to `127.0.0.1`, embeds all frontend assets in the binary and
streams bounded snapshots over SSE. Factory, Agents and Events views show task
states and revisions, lease history, agent sessions, checkpoint/handoff
metadata, validation evidence metadata and repository readiness. Checkpoint text, evidence
output, command arguments, provenance, credentials and unbounded checkpoint
text are never published. CI, completion/testguard and merge state are labelled
unavailable or on-demand when the runtime schema has no persisted provider
result; the dashboard does not invent GitHub or merge facts.

CI uses the committed-range gate:

```bash
mindrail verify --ci
```

Session IDs, task IDs, revisions, leases, and operation IDs are not part of the
normal human workflow. They are managed by the agent through MCP. Existing
low-level commands remain callable for compatibility and diagnostics; see the
[Turkish usage guide](docs/usage-tr.md#ileri-seviye-ve-uyumluluk-referansı).

## Automatic agent flow

Configure the coding agent to launch `mindrail mcp` in the target repository.
The server keeps the original 13-tool MCP surface. The minimal lifecycle is:

```text
mindrail_bootstrap {"goal":"Implement the requested change","run_key":"<opaque-stable-agent-key>","paths":["planned/file.go"]}
mindrail_before_change {"paths":["another/file.go"]}
mindrail_complete {"finalize":true}
```

Bootstrap `paths` may be omitted when the scope is not known yet, but then
`mindrail_before_change` is required before the first edit. It is also required
before editing a file outside the declared scope. The call is optional only when
bootstrap paths already cover every file being edited.
The agent creates and retains the opaque `run_key`; a person should
not need to copy it. Finalization reconciles the real Git diff, runs every
configured validation profile in deterministic order, evaluates the shared
gate, and completes only on ALLOW.

If no validation profiles are configured, the result explicitly reports
`no_profiles_configured:true`. That means no profile ran; it must never be
reported as “tests passed.” Project tests, lint, and build checks must be
configured as validation profiles and/or run separately in CI.

## Build from source

Requirements: Go 1.27.x, Git, and a C toolchain.

```bash
make build      # -> bin/mindrail
make install    # -> $HOME/.local/bin/mindrail (atomic replacement)
make check      # format check, go vet, tests
make race       # race detector
```

For a complete local release gate, use `make verify`. See `make help` for all
targets. `PREFIX`, `BINDIR`, and packaging `DESTDIR` are configurable, for example
`make install PREFIX=/opt/mindrail` or
`make install DESTDIR=/tmp/package-root BINDIR=/usr/bin`. On the verified
Linux target, installation refuses destination symlinks/directories and any
resolved `DESTDIR` escape; regular-file replacement remains an atomic
same-directory rename.

Mindrail has no network self-update channel. To update a source installation,
update this source checkout, run its checks, replace the binary, and then refresh
each already-initialized repository's managed files and migrations:

```bash
make check
make install
cd /path/to/initialized/repository
mindrail update
```

Binary replacement and `mindrail update` are separate: the first installs the new
program; the second updates one existing worktree without replacing user-owned
`AGENTS.md` text or hooks.

## Optional JEV routing

JEV advice is opt-in and coding-agent neutral. Ask any local agent to run
`mindrail jev connect`, or invoke it directly: Mindrail opens a loopback-only browser
form and stores the submitted credential in the operating-system keyring. The key
must not appear in agent chat, Codex/Claude/Cursor settings, stdin, repository
configuration, an `.env` file, command arguments, logs, or committed content.
Secure persistent storage is currently available through Secret Service on Linux
and Credential Manager on Windows. Mindrail deliberately refuses persistent storage
on macOS until an application-bound native Keychain backend is available; use the
optional environment override there.
`TYPESAFE_API_KEY` remains an optional higher-precedence override for CI and
container automation. With neither source configured, the normal routing flow is
unchanged.

For an ambiguous tool, agent, model, or reasoning-effort choice, the agent can send
the bounded routing request without including the key:

```bash
printf '%s\n' '{"goal":"select a search tool","tools":[{"id":"rg","description":"search repository text"}]}' |
  mindrail agent route
```

The JSON requires a `goal` string, accepts optional `context` JSON, and requires one
or more `tools`, `agents`, `models`, or `efforts` arrays. Each candidate contains
exactly `id` and `description`. Send TypeSafe only minimal non-sensitive summaries
and closed candidate descriptions—never raw secrets, logs, or source code.

The installed binary carries the canonical adapter behind `mindrail agent route`;
agents consume advice only when the top-level result is `ok` and every selection is
accepted. Missing credentials and all disabled/fallback/error results continue
normally. `mindrail jev status` reports only whether JEV is connected and which
non-secret source is active; `mindrail jev disconnect` removes the stored keyring
credential. Python 3 is needed only when this optional route is enabled.

## Storage model

`.mindrail/` holds configuration plus Decision and Invariant records as
version-controlled files. It is deliberately not gitignored: repository
knowledge should be reviewed like code and must survive a clean clone. Runtime
state—the SQLite database and caches—lives under
`<GIT_COMMON_DIR>/mindrail/` and is not tracked.

## Documentation

| Document | Role |
| --- | --- |
| [`docs/usage-tr.md`](docs/usage-tr.md) | User guide, automatic MCP flow, CI, troubleshooting, and advanced compatibility reference |
| [`docs/specification/mindrail-technical-specification-1.0.md`](docs/specification/mindrail-technical-specification-1.0.md) | Target architecture and acceptance criteria |
| [`docs/specification/mindrail-0.1-kernel-scope.md`](docs/specification/mindrail-0.1-kernel-scope.md) | First-deliverable scope |
| [`docs/specification/mindrail-tech-stack.md`](docs/specification/mindrail-tech-stack.md) | Go implementation strategy, subordinate to the specification |
| [`docs/adr/`](docs/adr/) | Architecture decision records |

## License

Not yet decided.
