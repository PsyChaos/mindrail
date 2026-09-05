# Mindrail

A local-first engineering gate for AI coding agents.

Mindrail discovers the actual code change from Git, binds durable engineering
constraints — Decisions and Invariants — to that change, requires
snapshot-bound evidence, and denies completion when the proof is insufficient.
It runs entirely on the developer's machine: no cloud service, no account, no
network dependency on the interactive path.

**Status: 0.1 kernel under construction.** The command surface described in the
specification is not complete yet; see the task list for what has landed.

## Documentation

| Document | Role |
|---|---|
| [`.docs/mindrail-technical-specification-1.0.md`](.docs/mindrail-technical-specification-1.0.md) | Target architecture and acceptance criteria |
| [`.docs/mindrail-0.1-kernel-scope.md`](.docs/mindrail-0.1-kernel-scope.md) | First deliverable scope, AC-01…AC-22 |
| [`.docs/mindrail-tech-stack.md`](.docs/mindrail-tech-stack.md) | Go implementation strategy, subordinate to the specification |
| [`.tasks/mindrail-0.1-task-list.md`](.tasks/mindrail-0.1-task-list.md) | Implementation tasks and dependency waves |
| [`docs/adr/`](docs/adr/) | Architecture decision records |

## Requirements

- Go 1.27.x
- Git
- A C toolchain (required from MR-005 onwards for the Tree-sitter binding)

## Build

```bash
make build      # -> bin/mindrail
make check      # gofmt check, go vet, go test
make race       # go test -race
make help       # list all targets
```

## Repository-owned knowledge

`.mindrail/` holds Decision and Invariant records as version-controlled JSON.
It is deliberately **not** gitignored: those records must be reviewed like code
and reloaded from a clean clone in CI. Runtime state — the SQLite database and
caches — lives under `<GIT_COMMON_DIR>/mindrail/`, inside `.git/`, and is
therefore never tracked.

## License

Not yet decided.
