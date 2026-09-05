# ADR-0002 — SQLite driver

- **Status:** Proposed — provisional driver in use, decision open
- **Date:** 2026-09-05
- **Scope:** `internal/storage`, runtime database

## Context

Tech-stack §20 deliberately leaves the concrete SQLite driver open until
Mindrail-specific benchmark evidence exists. The usual "avoid CGO" argument
does not apply here, because the official Tree-sitter Go binding already
requires a native C toolchain (§25, §26).

Candidates:

- `modernc.org/sqlite` — pure Go, no CGO, FTS5 available without a build tag.
- `mattn/go-sqlite3` — CGO binding to upstream SQLite; FTS5 only compiles with
  the `sqlite_fts5` build tag.

MR-001 has to create and migrate the runtime database before any benchmark can
run against a realistic workload, so a driver must be chosen provisionally.

## Decision

**Provisional:** use `modernc.org/sqlite` for MR-001 onwards.

Rationale for the provisional pick, not for the final one:

- It needs no build tag for FTS5, so a missing tag cannot silently produce a
  build that fails Mindrail's search requirement.
- It keeps the early milestones free of a second native toolchain dependency;
  Tree-sitter introduces that requirement at MR-005 on its own schedule.

The choice is **not** settled. Per §20, storage code MUST stay behind
`database/sql` and MUST NOT depend on driver-specific APIs outside a small
adapter/bootstrap boundary, so that swapping the driver stays a local change.

## Open — benchmark matrix required before this ADR is accepted

Both candidates must be measured, with FTS5 explicitly enabled in each build:

```bash
go test -tags sqlite_fts5 -bench . ./internal/storage/...
```

Matrix (§20):

- WAL concurrent read/write
- 100 / 500 / 1000 row symbol+edge batch inserts
- FTS5 search
- short interactive writes during cold index
- `SQLITE_BUSY` contention
- open/reopen recovery
- supported OS build matrix

Acceptance criteria: correct `database/sql` behaviour, WAL support, FTS5
support, acceptable interactive p95 under index load, coverage of the
supported release platforms, acceptable maintenance and security profile.

The benchmark becomes meaningful once a realistic symbol and edge workload
exists, which is MR-005. This ADR must be revisited no later than MR-019,
where the warm-path SLOs are measured and enforced.

Whichever driver wins, any build tag it requires must be recorded here, in the
`Makefile` (`TAGS`) and in the release CI workflow.

## Consequences

- Until this ADR is accepted, no code outside `internal/storage` may import a
  SQLite driver package.
- A driver swap is expected to touch only the storage bootstrap, so the cost
  of being wrong now is bounded.
