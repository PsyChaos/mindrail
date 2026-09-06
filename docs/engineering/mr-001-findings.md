# MR-001 — findings

Every finding the MR-001 audit cycle produced, and how each was closed. Every
one was reproduced against the compiled binary by an auditor that had not
written the code; every one was closed against the same reproduction.

**Status: all 17 closed, and the pass that closed them was itself audited** —
see "The audit of this remediation" at the end, which found twenty-nine more.
`make verify` is green: gofmt, `go vet`, 1595 tests, the race detector and 3
clean-binary smoke tests.

**Provenance.** MR-001 went through seven adversarial audits and six remediation
passes before the milestone closed at `b04b1bc` with these 17 outstanding. The
audits had found 17, then 20, 15, 16, 11, 13 and finally these. They were graded
below the bar the milestone was held to — no CRITICAL, and the HIGH ones needed
an artificial environment to provoke — and were closed in a following pass
rather than left for MR-002.

| Severity | Found | Closed |
|---|---|---|
| HIGH | 3 | 3 |
| MEDIUM | 8 | 8 |
| LOW | 6 | 6 |

Go is installed through goenv and is not on the default `PATH`. Every
reproduction below assumes:

```bash
export PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH"
```

---

## What the whole set had in common

Eleven of the seventeen are one defect wearing different clothes: **the tool
asserting something it had not established.**

- F01 read the disk before it had finished writing to it and published the
  reading as a verdict.
- F03 read "no room" off a driver result code and reported it as "the filesystem
  is full", contradicting its own statfs reading three checks away in the same
  document.
- F02, F12, F13 and F15 printed remedies for conditions nobody had classified:
  an entry-level fix for a file with no readable entry, an `init` for a defect in
  the binary, a path-less instruction about a file under `.git`, and a link the
  reader was told to find without being told where.
- F05, F06, F07, F09, F10, F11, F14, F16 and F17 are the same thing one level up:
  tests asserting that *something* was reported rather than that the *right*
  thing was. Nine mutation survivors, each a claim the suite could not fail.

The remedy in every case was the same shape: ask the thing that knows, or say
plainly that nobody looked.

---

## HIGH

### F01 — `mindrail init` reported READY over a repository the next command refused

`init` derived its terminal verdict and exit code from probes of a filesystem it
was about to change. On a filesystem with room for its write-ahead log and not
for the checkpoint that folds the log back in, it printed "READY FOR TARGETED
WORK" at exit 0 while `status`, run a moment later on bytes nothing had touched,
exited 4 with BLOCKED. A CI step gated on `mindrail init` passed on a machine
where every later Mindrail command failed.

The reading was not stale by accident. Closing a WAL database is a write, the
close happened after the report was rendered, and SQLite discards a failed
checkpoint — so the log and the shared-memory index stayed on disk at full size
and took the last of the space with them.

**Reproduction** (unchanged; run it against any build to see the band)

> Build: `go build -o /tmp/mindrail ./cmd/mindrail`.
>
> Script /tmp/repro.sh:
>   mkdir -p /tmp/ns
>   mount -t tmpfs -o size=8M tmpfs /tmp/ns
>   cd /tmp/ns && git init -q repo && cd repo && git config user.email a@b.c && git config user.name t
>   echo hi > f.txt && git add -A && git commit -qm i >/dev/null 2>&1
>   dd if=/dev/zero of=/tmp/ns/filler bs=1024 count=100000 2>/dev/null
>   SIZE=$(du -k /tmp/ns/filler | cut -f1)
>   truncate -s $(( (SIZE - 128) * 1024 )) /tmp/ns/filler    # leave ~128 KiB free
>   /tmp/mindrail init --json > /tmp/o.json; echo "init_exit=$?"
>   /tmp/mindrail status; echo "status_exit=$?"
>
> Run: `unshare --user --map-root-user --mount sh /tmp/repro.sh`
>
> Before: init_exit=0, READY, and status_exit=4 with RUNTIME_PATH_UNWRITABLE.
> Reproduced at 124, 128, 132 and 140 KiB free; at 148 KiB and above everything
> succeeded, at 116 KiB and below init already reported BLOCKED.
>
> After: init_exit=4 with BLOCKED and RUNTIME_PATH_UNWRITABLE — the same code,
> the same exit class and the same remedy `status` and `doctor` give — across the
> whole band, and exit 0 at 148 KiB and above.

**Closed by** `bootstrap.App.Flush`, called by `runInit` before the report is
built. It runs `PRAGMA wal_checkpoint(TRUNCATE)`, so init's last write happens
before the reading that describes it, and a checkpoint that fails is classified
through `storage.WriteFailure` like every other refused write.

**What is closed, precisely.** The direction the finding is about — `init`
reporting READY over a repository the next command refuses — is gone across the
whole band. The blanket claim that "init, status and doctor cannot report
different codes for one broken installation" was *also* written here, and it is
false: below about 100 KiB free, `init` refuses with RUNTIME_PATH_UNWRITABLE
while `status` exits 0 reporting an uninitialised repository and offering
`mindrail init`. That is not this finding coming back. It is the documented
trade-off in `filesystem.FreeSpace.Exhausted`, which calls a filesystem full
only at zero bytes so that a nearly-full machine is not reported as broken:
`status` observes a repository that genuinely has not been initialised, and
decision D-03 puts that at exit 0. What matters is that the reader gets out, and
they do — running the `mindrail init` `status` offers prints the space condition
and the remedy that clears it. `assertTheLoopTerminates` asserts exactly that,
and the sweep now runs down to 8 KiB so the direction is covered rather than
assumed.

**Fails if it regresses:** `TestInitNeverCallsARepositoryReadyTheNextCommandRefuses`
(`internal/cli/agreement_hostilefs_linux_test.go`). It mounts a tmpfs in a user
namespace and sweeps the headroom from 112 KiB to 256 KiB, asserting that init
exiting 0 implies status exiting 0. Removing the `Flush` call fails it at 120,
124, 128, 132 and 140 KiB and leaves the healthy ends passing.

### F02 — an unopenable `config.toml` was remedied as a malformed one

A `.mindrail/config.toml` at mode 0000, or a directory standing at that path, was
reported as CONFIG_INVALID with "Fix or remove the offending entry in <path>" —
an instruction with nothing to carry out. There is no entry to fix, and the file
cannot be opened to look for one. All three commands agreed on it, so the
agreement matrix saw no contradiction.

**Reproduction**

>   git init /tmp/cfg && mindrail -C /tmp/cfg init
>   chmod 000 /tmp/cfg/.mindrail/config.toml
>   mindrail -C /tmp/cfg status --json
>
> Before: exit 2, CONFIG_INVALID, next_action ["Fix or remove the offending
> entry in /tmp/cfg/.mindrail/config.toml"]. Same for a directory at that path.
>
> After: next_action ["Check the permissions on /tmp/cfg/.mindrail/config.toml"],
> and for the directory ["Remove or move aside …, so that Mindrail can write a
> configuration file there"].

**Closed by** `configOpenError` in `internal/config/loader.go`, which classifies
the open failure through `filesystem.ClassifyRefusal` — the same classifier
`.mindrail`, `.mindrail/knowledge` and the record buckets already use. The code
stays CONFIG_INVALID and the exit stays 2; only the sentence and the remedy
change.

**Fails if it regresses:** the agreement-matrix rows "config.toml cannot be
opened" and "a directory where config.toml belongs", both of which declare
their expected remedy class and carry it out (see F05).

### F03 — a size limit was reported as a full disk

`SQLITE_IOERR_SHMSIZE` was read as "the filesystem is full" with no
corroboration. On a machine with a file-size limit or an exhausted quota, every
command asserted a false fact about a filesystem with gigabytes free and
prescribed a remedy that cannot succeed — while the same `doctor --json`
reported `runtime_root_usable: true` three checks away. This one was a
regression the fifth remediation pass introduced.

**Reproduction**

>   mkdir -p /tmp/fz && cd /tmp/fz && git init -q . && git config user.email a@b.c \
>     && git config user.name t && echo hi > f && git add -A && git commit -qm i
>   mindrail init            # exits 0
>   bash -c 'ulimit -f 20; cd /tmp/fz; mindrail status --json'
>
> Before: RUNTIME_PATH_UNWRITABLE, "the filesystem holding the runtime database
> at … is full", next_action ["free space on the filesystem holding …"], exit 4
> forever; freeing space changed nothing.
>
> After: "… was refused the room it asked for, but the filesystem holding it
> reports 13791625216 bytes available, so a size limit is refusing it rather than
> a full disk", with remedies naming `ulimit -f`, the user's quota,
> `PRAGMA max_page_count` and `MINDRAIL_RUNTIME_DIR`. Raising the limit clears it.

**Closed by** `storage.spaceCorroboratesFullDisk`, asked before either
`diskFullOpenFailure` or `diskFullFailure` asserts "full", plus the
`ErrSizeLimit` sentinel so a caller matching on `ErrDiskFull` does not inherit
the wrong condition. An unknown statfs reading corroborates: a probe that could
not look is not evidence of room.

**Fails if it regresses:** `TestWriteFailureNamesALimitWhenTheFilesystemStillHasRoom`
(`internal/storage/diskfull_test.go`), and its over-fire guard
`TestAFullFilesystemIsRefusedAndDiagnosed`, which mounts and fills a real
filesystem and holds the disk-full sentence to every word of itself.

---

## MEDIUM

### F04, F08 — a rejected command line exited 1 and wrote no envelope

An unknown subcommand, an unknown flag and a surplus argument all exited 1, the
code decision D-03 reserves for a gate that ran and refused, while every other
usage mistake this binary knows exits 2. With `--json` they wrote nothing at all
to stdout, so a consumer parsing stdout on a non-zero exit got a parse error
rather than an envelope.

**Closed by** `cli.Root`, which records the command line, classifies cobra's
flag and argument errors as `COMMAND_LINE_INVALID` / `KindUsage`, and writes the
envelope when `--json` was asked for. The decision is taken from the raw line
rather than the flag set, because pflag stops at the first flag it does not
recognise: reading `Changed()` would emit the envelope for `--json --nope` and
swallow it for `--nope --json`.

**Fails if it regresses:** three rows in `TestExitCodeMatrix`, plus
`TestRejectedCommandLineStillProducesAnEnvelope` (which asserts both `--json`
orderings) and `TestRejectedCommandLineStaysSilentOnStdoutWithoutJSON`.

### F05 — the agreement matrix never read the remedy it claimed to carry out

The matrix's final phase says "the sentence all three printed, carried out" and
ran a hard-coded per-row closure instead. `next_action` appeared only in the
failure message. A remedy that was wrong, useless or impossible passed as long as
all three commands printed it identically — proved by replacing the config
loader's remedy with "Reinstall Mindrail from your package manager" and watching
1479 tests stay green.

**Closed by** a `remedy` field on each row, asserted against the printed
`next_action` before `clear` runs, through a classifier that extends
`coherence_test.go`'s with the four remedies only this matrix asks about. A row
that declares neither a class nor a `remedyNotCarryable` reason fails, so a new
row cannot skip the check. The one genuinely non-carryable remedy — "upgrade to
a binary whose reader window covers these records" — now says so in the row
instead of silently substituting a deletion.

**Fails if it regresses:** the original mutation now fails both config rows of
`TestOneConditionIsNamedTheSameWayByEveryCommand`.

### F06 — no matrix row for a `config.toml` that cannot be opened

The matrix had rows for a `.mindrail` that is a file, unwritable, dangling or
escaping the root, and rows for config.toml's *contents* being wrong — and none
for the file being unopenable, which is the gap that hid F02.

**Closed by** two rows: `config.toml` at mode 0000, and a directory occupying
the path. Both are reachable without unusual permissions — a tree may
legitimately commit a path named `.mindrail/config.toml`, and then every command
in every fresh clone meets the second.

### F07 — the linked-worktree cross-check compared two fields of four

`TestLinkedWorktreeIsReportedEndToEnd` cross-checked `doctor` against `status`
for `common_dir` and `worktree_root` only, never `git_dir` or
`is_linked_worktree` — the two fields that distinguish a linked worktree from a
main one. Hardcoding either in `internal/doctor/checks.go` left the whole gate
green while `doctor` printed `is_linked_worktree: false` in a directory where
`status`, from the same binary, printed true.

**Closed by** asserting all four fields, and the workspace check's own copy of
`is_linked_worktree` beside them. All three mutations the finding names are now
killed.

### F09 — the read-only-media sentinel had no row in the classifier's table

Making `ClassifyRefusal` answer `BarrierPermission` for its own
`ErrReadOnlyMedia` sentinel passed the entire suite. The table that documents
itself as "the whole detection rule" had a row for the ENOSPC sentinel and none
for the EROFS one, so the branch was unverified — and on non-unix builds, where
`platformBarrier` answers nothing, it is the only classification available.

**Closed by** the mirroring row. The mutation is killed.

### F10 — `sameDirectory`'s identity comparison was untested

Reducing the documented defence against a symlinked repository root to string
equality passed unit and smoke tests, while changing which of two E7 sentences a
reader gets and dropping the `ErrInsideGitDirectory` sentinel a caller branches
on.

**Closed by** `TestStandpointIsDecidedByIdentityRatherThanSpelling`, which drives
discovery through a symlink to the repository root, and its over-fire guard.

### F11 — the linked-worktree back-pointer check was untested

Deleting the check that the recorded worktree root points back at the
administrative directory naming it passed the whole gate, and made `mindrail`
inside `.git/worktrees/<name>` direct the reader into a directory belonging to a
different repository.

**Closed by** `TestLinkedWorktreeRecordIsOnlyBelievedWhenItPointsBack`, covering
both ways the back-pointer can disagree: it names somewhere else, and it is not
there at all.

---

## LOW

### F12 — a defective build blamed the repository

A migration set the binary cannot read — two files at one version, a name the
loader cannot parse — was reported with "Run `mindrail init` to apply the pending
migrations", which re-runs the command that just failed and reproduces the
condition verbatim forever. No repository is at fault: the migrations are
compiled in.

**Closed by** `asMigrationSetError`, whose remedy names the build rather than the
repository. **Fails if it regresses:**
`TestAMalformedMigrationSetBlamesTheBuildRatherThanTheRepository`, with
`TestAPendingMigrationStillSendsTheReaderToInit` as the over-fire guard.

### F13 — the rebuild remedy did not name the database

MIGRATION_FAILED said "Move the runtime database aside" while the corruption
remedy for the file beside it printed the full path. The database lives under
`.git`, which most users have never opened — and a remedy naming no absolute path
is invisible to the invariant that compares what two documents say about one
path, so it could never be caught contradicting the error object it travels with.

**Closed by** `Migrator.rebuildRemedy`, which asks the connection for its own
file through the newly exported `storage.MainDatabaseFile`.

**Fails if it regresses:** `TestEveryRemedyAboutAFileNamesThatFileAbsolutely`.
This line was missing for a round, and its absence was the finding: the fix was
declared closed while nothing could fail. The agreement matrix classifies a
remedy on a phrase, and `"to rebuild it"` is in both the path-less sentence and
the one that replaced it, so reverting the fix left the whole suite green.

### F14 — three unexercised claims in the init report path

Both halves of the disjunction deciding the terminal line, and the pending-count
arm of `schemaIsCurrent`, were each documented as load-bearing and none was
exercised. The justification the comment gave for the disjunction was also
false: an unreadable knowledge record leaves the repository DEGRADED and exit 0,
not BLOCKED with healthy components.

**Closed by** extracting `terminalStateOf` as a pure function of the two
answers, which makes each clause reachable from a value, and by correcting the
comment: the two coincide in this binary, the disjunction is a fail-safe rather
than a live branch, and each clause alone is one change away from reproducing
F01. `TestTerminalStateNeedsEitherAnswerAlone` and
`TestSchemaIsCurrentNeedsAllThreeAnswers` kill all three mutations.

### F15 — the containment remedy named no path

PATH_ESCAPES_ROOT told the reader to replace the symbolic link pointing outside
the repository without saying which one, in a repository that may have many.

**Closed by** quoting the offending path — and, once quoted, by naming the right
one. The first attempt quoted the path each command had asked for, which made
`status` name `.mindrail/config.toml` and `init` name `.mindrail` for one link
and broke the agreement matrix. `Root.escapingComponent` now names the
shallowest prefix that already leaves the root, which is the entry a reader has
to change and the same entry whichever command met it.

**Fails if it regresses:** `TestEveryRemedyAboutAFileNamesThatFileAbsolutely`
(the sentence and the `inspect_path` metadata) and
`TestEscapingComponentNamesTheEntryThatLeaves` (the choice of path). Like F13,
this line was missing for a round and both halves of the fix could be reverted
in silence.

### F16 — a guard that could not fail

`worktreeHoldingGitDir`'s `os.SameFile` comparison stat'ed the same path twice:
once the base name is `.git`, `filepath.Join(filepath.Dir(commonDir), ".git")`
rebuilds `commonDir` byte for byte. The function shipped exactly the name-only
test its own comment said was insufficient.

**Closed by** deleting the guard and correcting the comment. The shape it
claimed to reject — `git init --separate-git-dir=<parent>/.git` — must not be
rejected: git resolves that parent as a worktree of that Git directory, so
agreeing with it is agreeing with the oracle.
`TestWorktreeHoldingGitDirAcceptsASeparateGitDirectoryNamedGit` pins that, which
is what the deleted guard never did.

### F17 — two uncovered parsing guards, one of them wrong

`readGitFile` trimmed only `"\n "` off the target where git's
`read_gitfile_gently` strips every trailing `isspace` byte. The gap is reachable:
a `.git` file written with CRLF line endings left a carriage return on the end of
the path, so Mindrail looked for a directory git never named. `inspectGitEntry`'s
`os.Lstat` was load-bearing for the walk's own contract — stop at the first
`.git` entry of any kind — and nothing could fail it.

**Closed by** trimming git's own set (`gitTrailingSpace`), three discriminating
rows in `TestReadGitFileMirrorsGit` (trailing tab, CRLF, and the leading space
git deliberately keeps), and `TestTheWalkStopsAtAGitEntryItCannotFollow`. All
three mutations are killed.

---

## The audit of this remediation

Closing the seventeen above was itself audited, by three independent reviewers
that had not written any of it: one on the runtime write path, one on error
classification and remedies, one on test quality. They produced twenty-nine
findings between them. All are closed; `make verify` is green with 1595 tests.

**Five were defects the fixes introduced.** This is the number worth carrying
forward: a third of what a remediation pass produces is new.

- Ctrl-C during the new checkpoint was reported as `RUNTIME_DB_UNAVAILABLE` at
  exit 4, in the same output whose readiness line said READY and whose printed
  remedy, `mindrail doctor`, then exited 0 and found nothing. A cancelled
  checkpoint finds nothing wrong; it is reported as busy.
- The size-limit classification over-fired into a new false claim. Reading "more
  than zero bytes available" as room meant a filesystem with four kilobytes left
  was told a limit was refusing the write and that *freeing space would not
  change it* — on a disk where freeing two megabytes cleared it at once. The
  filesystem now has to show room for what SQLite was actually refused: the
  32 KiB shared-memory index plus the log on disk.
- `wal_checkpoint(TRUNCATE)` inherited the database's five-second busy timeout,
  so one held read lock took `mindrail init` from about ten milliseconds to
  five seconds. The wait is now capped per connection at 250 ms. The cap has to
  be the busy timeout: a context deadline around the query changed nothing,
  because the wait happens inside SQLite's own busy handler.
- `mindrail help bogus` printed the whole root help on stdout and exited 0,
  because a non-nil `Args` on the root stops cobra consulting `legacyArgs`.
- The exit-code fix did not reach cobra's own commands. `help`, `completion` and
  one command per shell are added inside `ExecuteC`, after any loop over
  `Commands()` in the constructor has finished, so `mindrail completion bash
  extra --json` still exited 1 with an empty stdout — the exact symptom F04 and
  F08 describe. The tree is now classified at execution rather than at
  construction.

**Two were false claims in this document.** F13 and F15 were recorded as closed
while nothing could fail if either were reverted, and both were reverted to
prove it. The regression-test lines above were written afterwards, against
assertions that exist.

**One gutted the fix that was supposed to prevent all of this.** The
remedy-class assertion added for F05 was defeated by the bare substring
`"make "` in the shared classifier: "Delete the whole repository to make it
stop" classified as a permission remedy, so the seven matrix rows declaring
`classPermission` were satisfied by any sentence containing an English verb. The
classifier matches shapes now, and has a test of its own. The matrix's escape
hatch was the same kind of hole — free text that skipped the assertion entirely
— and no longer excuses a row from declaring its class.

**One reported success without running.** The flagship F01 regression test
re-executes the test binary inside a mount namespace and the parent only checked
the child's exit code, which is zero for a child that skipped. A refused mount,
a `-test.run` matching nothing and `go test -short` all reported a pass. The
child now says how many rows it ran and the parent refuses an answer without it.

**The rest were unfalsifiable claims**: comments asserting behaviour no
assertion pinned. `sizeLimitOpenFailure` was reached by no test at all — its
four remedies could be replaced with the very "free space" sentence F03 exists
to prevent, in silence. `Flush` could swallow the error it exists to surface.
`TRUNCATE` could become `PASSIVE`. Each now has a test; where a claim turned out
to be genuinely untestable, the code says so instead of implying otherwise —
`escapingComponent`'s redundant error clause was deleted the way F16's dead
guard was, and the trim in `adapter.go` records that nothing can currently fail
if it regresses.

### What generalises to MR-002

1. **A remediation pass needs its own audit.** Five of these were introduced by
   the fixes, and two were false claims in the document recording them. The
   fixes were made by someone who had just read the findings and believed them.
2. **A test that classifies on a phrase asserts the phrase, not the property.**
   Both F13 and F15 survived reversion because the matrix matched a sentence
   fragment that both the right and the wrong answer contained. Assert the
   property — the absolute path, the metadata key — not the wording.
3. **A test that re-executes anything must prove it ran.** Exit codes are not
   evidence; a marker the parent requires is.
4. **A widened classification over-fires before it under-fires.** F03's fix and
   its own correction are the same mistake twice, one in each direction: first
   asserting a full disk without asking, then asserting *not* a full disk on the
   strength of four kilobytes.
