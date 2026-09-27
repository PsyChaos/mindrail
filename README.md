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
mindrail status     # readiness summary
mindrail doctor     # detailed, read-only diagnosis
mindrail verify     # verify staged changes locally
```

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
make check      # format check, go vet, tests
make race       # race detector
```

For a complete local release gate, use `make verify`. See `make help` for all
targets.

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
