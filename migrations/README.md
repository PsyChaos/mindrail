# Migrations

Numbered `.sql` files, embedded into the binary by `embed.go` and applied in
order by `internal/migration`. The migrator records each file's sha256 in the
ledger when it applies it and compares the two on every later start, so **an
applied file is never edited** — not for a typo, not for a comment. A
comment-only edit reports `MIGRATION_CHECKSUM_MISMATCH` to every database that
already applied the file, while the clean binary stays `READY`.
`shipped_test.go` pins the bytes of every file that has shipped, so that edit
goes red in `go test ./...` instead of in a user's repository.

Corrections to what an applied file *says* live in this README. It is neither
embedded (`embed.go`'s glob is `*.sql`) nor checksummed (`internal/migration`
skips entries that are not `.sql`), so it can change freely.

## `000002_coordination.sql`

The comment above the three indexes reads:

> The two queries status and `task list` actually run: tasks of a project by
> state, and the newest checkpoint of a task. The second index is on
> (task_id, checkpoint_id) so the newest is the last entry of a contiguous range
> rather than a sort over the task's whole history.

Two things in it are wrong, and the file stays as it is.

- **Which command runs which query.** `status` runs the newest checkpoint *of a
  project* — `selectNewestCheckpointOfProject` in
  `internal/coordination/store.go`, whose doc comment carries this correction
  too. The task-scoped newest checkpoint is what `task show` runs, through
  `Handover`. (Audit round 1, finding F48.)
- **How the newest is chosen.** Since the round-1 remediation the newest
  checkpoint is `ORDER BY rowid DESC`, the row the database inserted last, not
  the largest `checkpoint_id`: two processes writing in the same millisecond
  mint ids that order by random bits (decision D-59 as amended, finding F37).
  `idx_checkpoints_task(task_id, checkpoint_id)` therefore no longer serves the
  ordering. It still serves the `task_id` equality, and it is kept because the
  file that created it cannot change. (Audit round 2, §4.4.)
