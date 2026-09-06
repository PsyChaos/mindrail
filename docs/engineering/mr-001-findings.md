# MR-001 — open findings

Findings that survived the MR-001 audit cycle. Every one was reproduced against
the compiled binary by an auditor that had not written the code.

**Provenance.** MR-001 went through seven adversarial audits and six remediation
passes. The audits found 17, then 20, 15, 16, 11, 13 and finally these
17. The list below is what remained when the milestone closed at commit
`b04b1bc`, with `make verify` green: gofmt, `go vet`, 1479 tests, the race
detector, and 3 clean-binary smoke tests.

**Why these are still open.** They were graded below the bar the milestone was
held to — no CRITICAL, and the HIGH ones that remain need an artificial
environment to provoke (a full filesystem, a disk quota, a mode-0000 file). They
are real defects, not noise; they are simply not what stops MR-002 from starting.

**How to read a finding.** The reproduction is complete on its own: it names the
commands, the environment and the observed output. Nothing here depends on the
session that produced it.

| Severity | Count |
|---|---|
| HIGH | 3 |
| MEDIUM | 8 |
| LOW | 6 |

Go is installed through goenv and is not on the default `PATH`. Every
reproduction below assumes:

```bash
export PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH"
```

---

## HIGH

### F01 — `internal/cli/init.go`

`mindrail init` derives its terminal verdict and exit code from writability probes captured at startup step 3, before it performs its own writes, so a run that exhausts the filesystem prints "READY FOR TARGETED WORK" with ok=true and exit 0 while `status` and `doctor` run a moment later on the identical on-disk state exit 4 with BLOCKED / ERROR.

**Reproduction**

> Build: `export PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH"; go build -o /tmp/mindrail ./cmd/mindrail`.
>
> Script /tmp/repro.sh:
>   mkdir -p /tmp/ns
>   mount -t tmpfs -o size=8M tmpfs /tmp/ns
>   cd /tmp/ns && git init -q repo && cd repo && git config user.email a@b.c && git config user.name t
>   echo hi > f.txt && git add -A && git commit -qm i >/dev/null 2>&1
>   dd if=/dev/zero of=/tmp/ns/filler bs=1024 count=100000 2>/dev/null      # fills the tmpfs
>   SIZE=$(du -k /tmp/ns/filler | cut -f1)
>   truncate -s $(( (SIZE - 128) * 1024 )) /tmp/ns/filler                    # leave ~128 KiB free
>   echo "free=$(df -k --output=avail /tmp/ns | tail -1) KiB"
>   /tmp/mindrail init --json > /tmp/o.json; echo "init_exit=$?"
>   python3 -c "import json;d=json.load(open('/tmp/o.json'));print(d['ok'],d['data']['terminal_state'],d['data']['status']['readiness'],{k:v['state'] for k,v in d['data']['status']['components'].items()})"
>   echo "free after init: $(df -k --output=avail /tmp/ns | tail -1) KiB"
>   /tmp/mindrail status; echo "status_exit=$?"
>   /tmp/mindrail doctor >/dev/null 2>&1; echo "doctor_exit=$?"
>
> Run: `unshare --user --map-root-user --mount sh /tmp/repro.sh`
>
> Observed, reproduced identically on 3 consecutive runs:
>   init_exit=0, ok=True, terminal_state="READY FOR TARGETED WORK", readiness=READY, runtime_db=OK, knowledge=OK, error=null
>   free after init: 0 KiB   (init's own 74 KiB uncheckpointed -wal plus the 32 KiB -shm consumed the remainder)
>   status_exit=4  -> RUNTIME_PATH_UNWRITABLE, readiness BLOCKED, 'the filesystem holding the runtime root "/tmp/ns/repo/.git/mindrail" is full'
>   doctor_exit=4  -> runtime_paths ERROR, sqlite ERROR
> Nothing changes on disk between init's exit and status's start. The band is not a knife edge: free space of 124, 128, 132 and 140 KiB all reproduce it; at 148 KiB and above init succeeds cleanly and status exits 0; at 116 KiB and below init already reports BLOCKED at exit 4.
>
> Root cause: `runInit` (internal/cli/init.go L58-72) builds the report from `a.Diagnosis(ctx)` and `status.Build(a.Subject(), ...)`, and the Subject's `Probes.RuntimeDir` / `Probes.CacheDir` / `Probes.RepoConfigDir` are the `filesystem.ProbeWritable` / `ProbeRepoConfig` answers taken at §87 step `resolve_runtime_paths`, before `open_sqlite` and `migrate_db` wrote anything. `internal/doctor/checks.go` `runtimePathResult` reads those cached fields (`s.Probes.RuntimeDirKnown`, etc.) rather than re-probing, so init's verdict describes a filesystem state that its own run has since invalidated. Note this directly falsifies the invariant asserted in the comment at internal/cli/init.go L59-61 ("so `init`, `status` and `doctor` can never report different codes for the same broken installation").
>
> Impact: a CI step gated on `mindrail init` returns success on a machine where every subsequent Mindrail command exits 4.
>
> Suggested fix: re-run the root writability probes (or at minimum `filesystem.NoSpaceRefusal` on the runtime root) after the write phase, before `initReportOf` computes TerminalState; alternatively checkpoint/truncate the WAL at shutdown and surface a failed checkpoint.

### F02 — `internal/config/loader.go`

A `.mindrail/config.toml` that cannot be opened — mode 0000, or a directory standing at that path — is reported as CONFIG_INVALID with the remedy "Fix or remove the offending entry in <path>", an instruction that cannot be carried out: there is no entry to fix, and the file cannot be read or is not a file at all. This is finding D5's exact sentence, fixed for the `.mindrail` directory and left in place one level down for the file inside it.

**Reproduction**

> Build the binary and run:
>
>   git init /tmp/cfg && mindrail -C /tmp/cfg init
>   chmod 000 /tmp/cfg/.mindrail/config.toml
>   mindrail -C /tmp/cfg status --json
>
> Observed on all three of status, doctor and init (they agree with each other, so the agreement matrix is satisfied):
>   exit 2, ok:false, code CONFIG_INVALID
>   why:  "/tmp/cfg/.mindrail/config.toml cannot be read: open /tmp/cfg/.mindrail/config.toml: permission denied"
>   next_action: ["Fix or remove the offending entry in /tmp/cfg/.mindrail/config.toml"]
> The doctor `config` check prints the same remedy, so the coherence invariant sees no contradiction.
>
> The same happens for an obstruction:
>   git init /tmp/cfg2 && mindrail -C /tmp/cfg2 init
>   rm /tmp/cfg2/.mindrail/config.toml && mkdir /tmp/cfg2/.mindrail/config.toml
>   mindrail -C /tmp/cfg2 doctor --json
>   -> exit 2, CONFIG_INVALID, why "...cannot be read: read ...: is a directory", next_action "Fix or remove the offending entry in /tmp/cfg2/.mindrail/config.toml".
>
> Carry the remedy out as written: open the path in an editor to fix the offending entry. It fails with "permission denied" / "is a directory". The actions that actually clear the conditions are `chmod 644` and `rm -rf` respectively, and neither is named. (Deleting the file does clear it — `rm .mindrail/config.toml` then status/doctor/init all exit 0 — but the sentence tells the reader to remove an *entry in* the file, not the file.)
>
> Root cause: internal/config/loader.go:214 turns every non-ENOENT `os.ReadFile` failure into `configError(path, "cannot be read: "+err, nil)`, and configError (internal/config/loader.go:287-300) hard-codes the single remedy "Fix or remove the offending entry in "+path for all CONFIG_INVALID causes. EACCES, EISDIR, ENOTDIR and ELOOP are all funnelled into the malformed-content remedy.
>
> Suggested fix: classify the open failure through internal/filesystem (ClassifyRefusal / UnwritablePath), the way `.mindrail`, `.mindrail/knowledge` and the record buckets already are, so that a permission refusal prints "check the permissions on <path>" and an obstruction prints "remove or move aside <path>". Add both conditions as rows to internal/cli/agreement_test.go.
>
> Reachability: a checkout shared between two users leaves config.toml owned by whoever ran init first with a restrictive umask; a repository can also legitimately commit a tree named .mindrail/config.toml, in which case every Mindrail command in every fresh clone hits the directory case.

### F03 — `internal/storage/driver.go` — **regression introduced during remediation**

SQLITE_IOERR_SHMSIZE is unconditionally reported as "the filesystem is full", so on a machine with a file-size limit or a disk quota every command asserts a false fact about a filesystem with gigabytes free and prescribes a remedy that cannot succeed. The tool's own statfs reading, printed in the same doctor document, says the directory is usable.

**Reproduction**

> Build the binary: `export PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH"; go build -o /tmp/mindrail ./cmd/mindrail`.
>
> Setup (no namespaces needed; /tmp had 13 GB free throughout):
>   mkdir -p /tmp/fz && cd /tmp/fz && git init -q . && git config user.email a@b.c && git config user.name t && echo hi > f && git add -A && git commit -qm i
>   /tmp/mindrail init            # exits 0, healthy install
>
> Now simulate any environment that caps file size or per-user space (systemd `LimitFSIZE=`, /etc/security/limits.conf `fsize`, or an exhausted ext4/XFS user quota). RLIMIT_FSIZE is the member of that class reproducible without root:
>   bash -c 'ulimit -f 20; cd /tmp/fz; /tmp/mindrail status --json; /tmp/mindrail doctor --json; /tmp/mindrail init'
>
> Observed (df says /tmp has 13G available):
>   code    = RUNTIME_PATH_UNWRITABLE
>   why     = the filesystem holding the runtime database at /tmp/fz/.git/mindrail/mindrail.db is full, so the database could not be opened
>   next    = ['free space on the filesystem holding /tmp/fz/.git/mindrail/mindrail.db']
>   cause   = no space left for the runtime database: disk I/O error (4874)
>   exit    = 4 for status, doctor and init alike
> Freeing space changes nothing; the loop is permanent while the limit is in force. On a FRESH repo under the same limit, `mindrail doctor` first exits 0 and prescribes `mindrail init`, which then exits 4 forever (`for i in 1 2 3 4; do mindrail init; echo $?; done` -> 4 4 4 4).
>
> Self-contradiction inside one document: in that same `doctor --json`, check `runtime_paths` is state OK with details `runtime_root_usable: true`, `cache_dir_usable: true`, `repo_config_dir_usable: true` (its statfs probe correctly sees 13 GB free), while check `sqlite` is state ERROR saying the filesystem is full. The SQLite classification wins the remedy.
>
> Root cause: internal/storage/driver.go L143-145, `diskFullCode()` returns true for `code == sqliteIOErrShmSize` (4874 = SQLITE_IOERR_SHMSIZE) with no corroboration. The doc comment above it assumes "on a Unix VFS the way that happens is that the filesystem will not give it the bytes", which is false for EFBIG (rlimit) and EDQUOT (quota): both make the ftruncate of the 32 KiB `-shm` fail while the filesystem has room. internal/storage/db.go L391 `diskFullOpenFailure` then hard-codes "is full" / "free space on" without asking `filesystem.ProbeFreeSpace`, unlike its sibling `readOnlyWriteFailure` (internal/storage/classify.go L113) which does corroborate with `ProbeWriteAccess` before choosing its sentence.
>
> Suggested fix: before asserting "full", call `filesystem.ProbeFreeSpace(dir)`; when statfs reports space available, emit a distinct condition ("SQLite could not size the shared-memory index beside <db>; the filesystem reports N bytes free, so this is a per-file or per-user limit, not a full disk") with a remedy naming RLIMIT_FSIZE / quota / MINDRAIL_RUNTIME_DIR.

## MEDIUM

### F04 — `internal/app/exit.go`

Command-line usage errors exit 1 instead of the 2 that decision D-03 and tech-stack §13 assign to "invalid usage/config", and `--json` emits nothing at all for them, so the most common user mistake is reported under the code reserved for a genuine operation failure.

**Reproduction**

> In any initialised repository (`cd /tmp/final` after `mindrail init`):
>   mindrail bogus            ; echo $?   -> prints 'mindrail: unknown command "bogus" for "mindrail"', exit 1 (expected 2)
>   mindrail status --nope    ; echo $?   -> prints 'mindrail: unknown flag: --nope', exit 1 (expected 2)
>   mindrail init extraarg    ; echo $?   -> exit 1 (expected 2)
>   mindrail status --nope --json > /tmp/out ; echo $?; wc -c /tmp/out  -> exit 1, 0 bytes on stdout (no envelope)
>
> Contrast with the conditions that ARE mapped: not a Git repository, a bare repository and a malformed config.toml all exit 2 from all three commands, and internal/app/exit_test.go asserts ExitUsage=2 for exactly those.
>
> Root cause: internal/cli/root.go L50-51 sets SilenceErrors/SilenceUsage and returns cobra's own flag/command error unwrapped. `app.ExitCode` (internal/app/exit.go L67-81) finds neither a *DomainError nor an *app.Error on it and falls through to `return ExitFailed` (L79). No test covers the exit code of an unknown command or an unknown flag.
>
> Suggested fix: wrap cobra's `SetFlagErrorFunc` result and the unknown-command error in `app.Usage(err)` in cmd/mindrail/main.go (or in newRootCommand), and add a row to the existing exit-code matrix test.

### F05 — `internal/cli/agreement_test.go`

The agreement matrix's final phase claims to carry out the remedy the three commands printed, but it never reads `next_action` — it runs a hard-coded fixture action instead, so a remedy that is wrong, useless or impossible passes the matrix as long as all three commands print it identically.

**Reproduction**

> Read internal/cli/agreement_test.go:120-132. The comment on line 124 says "The sentence all three printed, carried out." The code calls `tc.clear(t, repo)` — a per-row closure written by the test author — and then asserts the three commands exit 0. `answers[first].nextAction` appears only inside the failure message on line 129; nothing compares the fixture action to the printed sentence.
>
> Proof by mutation (source repo untouched; copy it to /tmp first):
>   cp -a <repo> /tmp/mut
>   edit /tmp/mut/internal/config/loader.go:293, replacing
>       "Fix or remove the offending entry in "+path,
>   with
>       "Reinstall Mindrail from your package manager",
>   cd /tmp/mut && go test ./...
> Result: 1479 tests pass, every package ok. Both config rows of the agreement matrix ("config.toml cannot be parsed" and "config.toml carries a key this binary does not know") stay green while the binary now prints a remedy that cannot possibly clear a TOML syntax error. The coherence invariant also stays green, because the new sentence matches none of the phrases in remedyPhrases (coherence_test.go:270-281) and so classifies as classUnknown.
>
> Proof without any mutation, in the repository as it stands: the row at agreement_test.go:397-409, "a knowledge record written by a newer schema than this binary reads", has the binary print next_action ["Upgrade Mindrail to a version whose reader window covers these records."] while `clear` deletes .mindrail/knowledge/decisions/DEC-0001.json. Two unrelated actions. The same mismatch, in milder form, is in the three database rows (agreement_test.go:492, 544, 556): the binary prints "Move the runtime database aside and run `mindrail init` to rebuild it." and `clear` only removes the file.
>
> Suggested fix: make the row declare the remedy class it expects (obstruction / permission / space / read-only / remove-and-init / upgrade), assert that class against `answers[first].nextAction` using the same classifier coherence_test.go already owns, and only then run `clear`. A row whose printed remedy is not machine-carryable (the upgrade case) should say so explicitly rather than silently substituting a different action.

### F06 — `internal/cli/agreement_test.go`

The broken-setup matrix has rows for a `.mindrail` that is a file, unwritable, dangling or escaping the root, and rows for config.toml's *contents* being wrong, but no row for config.toml being unopenable — which is exactly the gap that hides the wrong remedy reported above.

**Reproduction**

> Read repositoryConfigConditions() at internal/cli/agreement_test.go:202-316. Its seven rows are: a regular file at .mindrail, a worktree that refuses .mindrail, a dangling symlink at .mindrail, a symlink out of the worktree, .mindrail unwritable with an incomplete scaffold, config.toml unparseable, config.toml with an unknown key. Missing: config.toml at mode 0000, and config.toml occupied by a directory.
>
> Confirm no other test covers them:
>   grep -rn 'cannot be read' internal/config/*_test.go internal/cli/*_test.go   -> no hits
>   grep -n 'func Test' internal/config/loader_test.go  -> only TestStrictDecodeRejectsUnknownKey, TestMalformedTOMLIsAUsageError, TestInvalidColorValueIsRejected and friends; none exercises an open failure.
>
> Add two rows to repositoryConfigConditions():
>   - setup: newInitializedRepo then chmod 0000 on .mindrail/config.toml; clear: chmod 0644.
>   - setup: newInitializedRepo, remove config.toml, mkdir config.toml; clear: os.RemoveAll.
> Both are currently `broken: true`. With the remedy-class assertion from the finding above in place, both would fail against today's binary; without it they would pass, which is the second reason that assertion is worth adding.

### F07 — `internal/cli/contract_test.go`

The AC-3 end-to-end test compares `doctor` against `status` for only two of the four repository fields, so `doctor` can report the wrong `git_dir` and the wrong `is_linked_worktree` in a linked worktree with the entire quality gate green.

**Reproduction**

> TestLinkedWorktreeIsReportedEndToEnd (internal/cli/contract_test.go:926) is the only test that runs `doctor` in a linked worktree. Its doctor loop (lines 982-1000) cross-checks the doctor git check's Details against the status report for `common_dir` (line 995) and `worktree_root` (line 997) only. It never checks `git_dir` or `is_linked_worktree` — the two fields that actually distinguish a linked worktree from a main one.
>
> To reproduce, copy the repo to /tmp (never mutate the source tree), then apply either mutation to internal/doctor/checks.go:
>   (A) line 144: change `"is_linked_worktree": strconv.FormatBool(s.Repo.IsLinkedWorktree),` to `strconv.FormatBool(false),`
>   (B) line 142: change `"git_dir": s.Repo.GitDir,` to `map[bool]string{true: s.Repo.CommonDir, false: s.Repo.GitDir}[s.Repo.IsLinkedWorktree],` (wrong only when linked, so the non-linked golden files still match)
> Then with PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH" run `go test ./...` (1479 tests, 16 packages) and `go test -tags smoke -count=1 ./cmd/...` (3 tests). Both mutations leave the entire gate green — I verified each separately.
>
> The defect they hide is user-visible. Build mutation (A) and run:
>   git init main; (cd main; git commit --allow-empty -m x; git worktree add ../wt -b b)
>   cd wt; ./mindrail init; ./mindrail doctor | head; ./mindrail status
> `doctor` prints `is_linked_worktree: false` while `status`, from the same binary in the same directory, prints `Linked worktree: true` — the same condition diagnosed differently by two commands AC-3 names, with `make verify` passing. Mutation (B) makes `doctor` print git_dir = <common>/.git while status prints <common>/.git/worktrees/wt.
>
> Fix: extend the loop at internal/cli/contract_test.go:982-1000 to also assert `check.Details["git_dir"] == want.GitDir` and `check.Details["is_linked_worktree"] == strconv.FormatBool(want.IsLinkedWorktree)`. The same gap covers the doctor workspace check's `is_linked_worktree` at internal/doctor/checks.go:805 — hardcoding that one to false also survives unit+smoke — so add an assertion for it too.

### F08 — `internal/cli/root.go`

An unknown subcommand or unknown flag exits 1 ("operation failed") instead of the documented 2 ("invalid usage or configuration"), and with `--json` writes nothing at all to stdout; the CLI contract test's exit-code table has no row for either case.

**Reproduction**

> mindrail -C <any-repo> frobnicate --json ; echo $?
>   -> stderr: 'mindrail: unknown command "frobnicate" for "mindrail"', stdout: empty, exit 1
>   mindrail -C <any-repo> status --nope --json ; echo $?
>   -> stderr: 'mindrail: unknown flag: --nope', stdout: empty, exit 1
>
> tech-stack §13 and decision D-03 reserve exit 2 for "invalid usage or configuration" and exit 1 for "operation failed", and this binary already returns 2 for every other usage mistake (not a Git repository, bare repository, malformed config, unknown config key, -C at a path that does not exist). A typo'd subcommand is invalid usage, so a CI step or wrapper that branches on 2-means-my-invocation-was-wrong versus 1-means-the-gate-failed gets the wrong answer.
>
> Cause: internal/cli/root.go:76 returns cobra's flag/command error unchanged; app.ExitCode (internal/app/exit.go:64-80) finds neither a *DomainError nor an *app.Error on it and falls through to ExitFailed.
>
> Secondary effect: `--json` was accepted on the command line and stdout is empty, so a consumer that unconditionally parses stdout as JSON on a non-zero exit gets a parse error rather than an envelope. Decision D-15 promises stdout is "human or JSON result".
>
> Test gap: the exit-code table at internal/cli/contract_test.go:1085-1145 has rows for healthy status, uninitialised status, not-a-git-repository, bare repository, unknown configuration key, malformed configuration and unwritable runtime path — and no row for an unknown command or an unknown flag. Add both, asserting app.ExitUsage.

### F09 — `internal/filesystem/refusal.go`

Mutation survivor: making `ClassifyRefusal` return BarrierPermission for its own ErrReadOnlyMedia sentinel passes the entire 1479-test suite. The table that documents itself as "the whole detection rule" has a row for the ENOSPC sentinel but none for the EROFS sentinel, and the branch is currently unreachable in production, so nothing would notice if a caller started depending on it.

**Reproduction**

> Copy the repo to /tmp (`rsync -a --exclude graphify-out --exclude .git <repo>/ /tmp/mut/`), then in /tmp/mut/internal/filesystem/refusal.go L94-96 change:
>     case errors.Is(cause, ErrReadOnlyMedia):
>         return BarrierReadOnlyMedia
> to:
>     case errors.Is(cause, ErrReadOnlyMedia):
>         return BarrierPermission
> and run `export PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH"; go test ./...`.
>
> Observed: the suite passes (1479 tests, 16 packages) with the mutation applied. This is precisely the defect the Barrier type's own doc comment says it exists to prevent ("a classifier that reached the permission branch first would answer 'permission' for a read-only mount ... which is the exact shape of the bug this replaces"). The twin mutation for the ENOSPC sentinel (`case errors.Is(cause, ErrNoSpace): return BarrierPermission`) IS killed, by TestNoSpaceRefusalNamesTheConditionItFound.
>
> Why the tests miss it: TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther (internal/filesystem/refusal_unix_test.go L27) has a row for `syscall.EROFS` (which is handled earlier, by platformBarrier in refusal_unix.go) and a row for the ErrNoSpace sentinel, but no row for the ErrReadOnlyMedia sentinel.
>
> I confirmed the branch is currently unreachable rather than actively wrong: I built a mutant binary and diffed its output against the baseline on a real read-only bind mount (`mount --bind /tmp/ns /tmp/ns && mount -o remount,bind,ro /tmp/ns`), for both a fresh and an already-initialised repository, across init/status/doctor in --json form. Every code, why and next_action was byte-identical. The only producer of a wrapped ErrReadOnlyMedia is `readOnlyMediaDatabaseError` (internal/storage/space.go L89), whose result is a finished DomainError that is never fed back through ClassifyRefusal. On non-unix builds (refusal_other.go, where platformBarrier always returns BarrierNone) the sentinel branch is the only classification available, so the gap matters more there.
>
> Suggested fix: add a row `{cause: fmt.Errorf("probe %q: %w", "/mnt", filesystem.ErrReadOnlyMedia), want: filesystem.BarrierReadOnlyMedia}` to the table, mirroring the ErrNoSpace row; or delete the branch if the sentinel is never meant to reach the classifier.

### F10 — `internal/git/adapter.go`

sameDirectory's os.SameFile identity comparison — the documented defence against a repository reached through a symlink — is not covered by any test; reducing it to string equality passes the whole gate while changing the E7 standpoint classification and dropping the ErrInsideGitDirectory sentinel.

**Reproduction**

> internal/git/adapter.go:390 `sameDirectory` compares two paths by os.Stat + os.SameFile after the fast string-equality path, and its comment says this is what keeps "a repository reached through a symlink — or a /tmp that is really /private/tmp" from reading as a different place. It feeds pathWithin/standpointOf, which decides which of the two E7 sentences and which sentinel a user gets.
>
> Copy the repo to /tmp, then in internal/git/adapter.go:390 replace the body's first clause so the function is only string equality:
>   if true { return left == right }
> With PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH": `go test ./...` (1479 tests) and `go test -tags smoke -count=1 ./cmd/...` (3 tests) both stay green.
>
> The behaviour it hides:
>   R=/tmp/g1; mkdir -p $R; git init -q $R/repo
>   git -C $R/repo -c user.email=a@b -c user.name=a commit -q --allow-empty -m x
>   mkdir -p $R/elsewhere; git -C $R/repo config core.worktree $R/elsewhere
>   ln -s $R/repo $R/repolink
>   ./mindrail -C $R/repolink/.git status
> Pristine binary: why = "the directory is inside a Git directory whose configured working tree \"/tmp/g1/elsewhere\" does not point back at it…", impact = the Git-directory impact, cause = ErrNotARepository+ErrInsideGitDirectory.
> Mutated binary: why = "this repository's configured working tree \"/tmp/g1/elsewhere\" does not point back at it…", impact = the outside-worktree impact, cause = ErrNotARepository only. A caller branching on ErrInsideGitDirectory silently stops matching, and the reader standing inside a .git directory is told they are not.
>
> Fix: add a test that drives discovery with a start directory reached through a symlink to the repository root (and one where core.worktree's target is reached through a symlink), asserting both the standpoint wording and the joined sentinel.

### F11 — `internal/git/adapter.go`

The back-pointer agreement check in linkedWorktreeOfAdminDir is untested; deleting it passes the whole gate and makes `mindrail` inside .git/worktrees/<name> direct the reader into a directory that belongs to a different repository.

**Reproduction**

> internal/git/adapter.go:1005-1008 requires that the worktree root recorded in `<common>/worktrees/<name>/gitdir` has a `.git` that points back at that same administrative directory before the E9 message names it. Its comment says a pruned or moved worktree must fall through to the generic classification.
>
> Copy the repo to /tmp and delete the check at internal/git/adapter.go:1005-1008, replacing it with `_ = adminDir`. With PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH": `go test ./...` (1479) and `go test -tags smoke -count=1 ./cmd/...` (3) both stay green.
>
> The behaviour it hides:
>   R=/tmp/g6; mkdir -p $R
>   git init -q $R/main; git -C $R/main -c user.email=a@b -c user.name=a commit -q --allow-empty -m x
>   git -C $R/main worktree add -q $R/lw -b b1
>   git init -q $R/other; git -C $R/other -c user.email=a@b -c user.name=a commit -q --allow-empty -m y
>   rm $R/lw/.git; echo "gitdir: $R/other/.git" > $R/lw/.git    # lw now belongs to `other`
>   cd $R/main/.git/worktrees/lw && ./mindrail status
> Pristine binary correctly falls back: "the directory is inside the repository's .git directory … the working tree is \"/tmp/g6/main\"", remedy `change into "/tmp/g6/main"`.
> Mutated binary: "the directory is inside the administrative directory this repository keeps for the linked working tree \"/tmp/g6/lw\"", remedy `change into "/tmp/g6/lw"` — a directory whose --git-common-dir is /tmp/g6/other/.git, i.e. a different repository, described as this one's worktree.
>
> Fix: add a test for the E9 fall-through — an administrative directory whose recorded root exists and is enterable but whose `.git` points at a different Git directory (and a second one where the root's `.git` is missing entirely) — asserting the generic .git-directory classification, not the linked-worktree one.

## LOW

### F12 — `internal/bootstrap/app.go`

When the embedded migration set itself cannot be loaded, every command including `init` prints `next_action: ["Run `mindrail init` to apply the pending migrations."]`, which is a remedy that re-runs the command that just failed and can never clear the condition.

**Reproduction**

> Reachable only from a defective build, so this is developer-facing rather than user-facing. Reproduce by copying the repo to /tmp/mut and adding a second migration file that collides with an existing version, e.g. create both /tmp/mut/migrations/000002_drop.sql and /tmp/mut/migrations/000002_bad.sql (any SQL), then `go build -o /tmp/mindrail_dup ./cmd/mindrail` and run it in a git repository:
>   /tmp/mindrail_dup init --json
> Observed:
>   code = MIGRATION_FAILED
>   why  = the runtime schema could not be established: duplicate migration version 2 in "000002_bad.sql" and "000002_drop.sql"
>   next = ['Run `mindrail init` to apply the pending migrations.']
>   exit = 1, and re-running init reproduces it verbatim forever.
>
> Root cause: `asMigrationError` (internal/bootstrap/app.go L884-889) is the generic domainizer for any non-DomainError out of the migration layer, including internal/migration/load.go L130's duplicate-version and other file-grammar failures. Its single next_action assumes the failure is "migrations are pending", which is true for the common case but false for a load failure.
>
> Suggested fix: give load-time failures their own diagnosis and a remedy that names the binary rather than the repository (e.g. "this Mindrail build ships a malformed migration set; report the version shown by `mindrail version`"), the way `unreadableFailure` in internal/migration/migrator.go already separates its cases.

### F13 — `internal/bootstrap/app.go`

The MIGRATION_FAILED remedy says "Move the runtime database aside" without naming the database, while the RUNTIME_DB_CORRUPT remedy for the neighbouring condition names the full path.

**Reproduction**

> git init /tmp/mg && mindrail -C /tmp/mg init
>   sqlite3 /tmp/mg/.git/mindrail/mindrail.db 'DROP TABLE IF EXISTS workspaces; DROP TABLE IF EXISTS projects;'
>   mindrail -C /tmp/mg status --json
> -> exit 1, MIGRATION_FAILED, next_action ["Move the runtime database aside and run `mindrail init` to rebuild it."]
> The same is produced by `sqlite3 ... 'DELETE FROM schema_migrations'`.
> Compare the adjacent corruption case (`printf 'garbage' > .git/mindrail/mindrail.db`), which prints "Move /tmp/mg/.git/mindrail/mindrail.db aside and run `mindrail init` to rebuild it." — same instruction, path included. The database lives under .git/mindrail, which most users have never opened, and the report does print db_path elsewhere in the document, so this is inconvenience rather than a dead end. Consequence for the invariants: a remedy with no absolute path in it is invisible to coherence_test.go's path-class comparison, so it can never be caught contradicting the error object.

### F14 — `internal/cli/init.go`

Three more mutation survivors in the init report path: both halves of the disjunction that decides TerminalState, and the pending-migration arm of schemaIsCurrent, are each documented as load-bearing but neither is exercised by any test, and I could not construct a scenario in which they differ.

**Reproduction**

> Copy the repo to /tmp/mut as above, apply each mutation separately, and run `go test ./...` — all three pass the full 1479-test suite:
>   (a) internal/cli/init.go L162: `if verdict != nil || current.Readiness == status.ReadinessBlocked {` -> `if verdict != nil {`
>   (b) same line -> `if current.Readiness == status.ReadinessBlocked {`
>   (c) internal/cli/init.go L179: `return s.DBPresent && s.MigrateErr == nil && s.PendingCount == 0` -> `return s.DBPresent && s.MigrateErr == nil`
>
> For (a) and (b) the doc comment at L137-143 states both halves are needed ("a repository can end up unusable without any single check failing outright — an unreadable knowledge record blocks work while every component that did resolve is healthy"). For (c) the doc comment at L170-177 states "All three conditions are needed ... a run that left migrations pending ... may not claim the schema is current (finding F14)"; only the DBPresent and MigrateErr arms are covered — the M7 mutation `return true` IS killed by TestInitNeverClaimsASchemaItDidNotEstablish, so the test only pins two of the three arms.
>
> I built mutant binaries for (a) and (b) and hunted for a divergent scenario: malformed JSON in .mindrail/knowledge/decisions (all three binaries: READY, DEGRADED readiness, exit 0) and a read-only worktree with MINDRAIL_RUNTIME_DIR pointed elsewhere (all three: BLOCKED, exit 4). The clauses always agreed, so this is a coverage gap rather than a demonstrated behaviour bug — but the documented justification for the disjunction is currently unverified, and a future MR that makes them diverge will get no signal.
>
> Suggested fix: either add tests that force verdict!=nil with a non-BLOCKED readiness and vice versa (and a PendingCount>0 init report), or simplify the condition to the one clause that is actually reachable and delete the claim from the comment.

### F15 — `internal/config/loader.go`

The PATH_ESCAPES_ROOT remedy tells the reader to replace the symbolic link that points outside the repository but never says which path it is, even though the error's own metadata carries it.

**Reproduction**

> git init /tmp/esc && mkdir /tmp/escout && ln -s /tmp/escout /tmp/esc/.mindrail
>   mindrail -C /tmp/esc status --json
> Gives, on all three commands, exit 2 / PATH_ESCAPES_ROOT with
>   why: "the path resolves outside the repository root"
>   next_action: ["use a path inside the repository", "replace any symbolic link on the path that points outside the repository"]
>   metadata: {"check":"config", "path":".mindrail/config.toml"}
> Neither the why nor either next_action names `.mindrail`, so in a repository with many symlinks the reader has to hunt. Every neighbouring condition (obstruction, permission, space, read-only mount) quotes the absolute path in the remedy. A side effect is that coherence_test.go's contradictoryPathRemedies cannot classify this remedy at all, because it extracts absolute paths out of the sentence and there are none. Suggested fix: quote the offending path in the remedy, as unwritableSubjectError already does for its four classes.

### F16 — `internal/git/adapter.go`

worktreeHoldingGitDir's os.SameFile guard is tautological — it stats the same path twice — so the function ships exactly the name-only test its own comment says is insufficient.

**Reproduction**

> internal/git/adapter.go:1021-1044. The function returns early unless `filepath.Base(commonDir) == ".git"`, sets `parent := filepath.Dir(commonDir)`, then compares `os.Stat(filepath.Join(parent, ".git"))` with `os.Stat(commonDir)` via os.SameFile, documented as "the question that decides it is not 'is it called .git' but 'would git find this same Git directory from the parent'".
>
> But when Base(commonDir) == ".git", `filepath.Join(filepath.Dir(commonDir), ".git")` reconstructs commonDir byte for byte, and commonDir is always filepath.Clean'ed (probeOptionalPath / absolutePath at internal/git/adapter.go:446 and :460). Both os.Stat calls therefore stat the identical path and os.SameFile is always true. The guard cannot fail.
>
> Demonstrate: copy the repo to /tmp, delete lines 1030-1037 (both os.Stat calls and the SameFile early return), replace with `_ = os.Stat`. With PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH", `go test ./...` reports 1479 tests passing. Build both binaries and run them in a Git directory created by `git init --separate-git-dir=/tmp/m6/srv/x/.git /tmp/m6/realwt` (cd /tmp/m6/srv/x/.git && ./mindrail status): output is byte-identical — both say the working tree is /tmp/m6/srv/x. No test distinguishes them because nothing can.
>
> No user is harmed today (git itself also resolves /tmp/m6/srv/x as a worktree of that Git directory, so mindrail still agrees with the oracle), but the safety property the comment promises is not implemented, and a future change that relies on it would be wrong. Fix: either delete the dead guard and reword the comment to say the name test is sufficient here and why, or make it a real check (e.g. run `git -C parent rev-parse --path-format=absolute --git-common-dir` and compare) and add a test for a Git directory named `.git` whose parent git would not resolve.

### F17 — `internal/git/gitfile.go`

Two .git-file parsing guards in internal/git/gitfile.go are uncovered: swapping readGitFile's git-mirroring TrimRight for TrimSpace, and inspectGitEntry's Lstat for Stat, both pass unit and smoke tests.

**Reproduction**

> Copy the repo to /tmp and apply either mutation, then run, with PATH="$HOME/.goenv/bin:$HOME/.goenv/shims:$PATH", `go test ./...` (1479 tests) and `go test -tags smoke -count=1 ./cmd/...` (3 tests). Both stay green for both mutations, verified separately.
>
> (a) internal/git/gitfile.go:165 — `target := strings.TrimRight(line, "\n ")` changed to `strings.TrimSpace(line)`. The comment says the parse is deliberately mirrored from git's read_gitfile_gently because "looking at a different path than git did would be the one way to get it wrong" — the same class of defect as finding H5 (trimOutputTerminator), whose own TrimSpace mutation IS killed. Divergence is observable only for a `.git`/`gitdir` target with leading whitespace after the `gitdir: ` prefix or a trailing tab/CR, which is why no test notices. Add a table test over readGitFile covering `gitdir:  /path` (extra leading space, which git keeps and TrimSpace eats) and a target ending in a tab.
>
> (b) internal/git/gitfile.go:116 — `os.Lstat(path)` changed to `os.Stat(path)`. The comment justifies Lstat as protecting against "a `.git` symlink to a deleted target" being read as an absent entry. I could not construct any shape where the two differ: with `ln -s /tmp/gone wt/.git`, git 2.55 prints the bracket form ("not a git repository (or any parent up to mount point /)"), so gitReportedBrokenIndirection at internal/git/gitfile.go:87 returns false and findDanglingGitFile is never reached; with `.git` a symlink to a real directory or to a dangling gitfile, both stat calls stop the walk identically. Output was byte-identical between the pristine and mutated binaries in every shape tried. Either the rationale is unreachable (in which case the comment should say the Lstat is defensive, not load-bearing) or there is a shape nobody has written down — decide which, and add the test or the correction.

