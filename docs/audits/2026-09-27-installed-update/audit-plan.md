# Installed update and JEV distribution — audit plan

Base ref: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`
Mode: parallel independent Reader and Breaker subagents
Scope draft: `audit_scope.py` found 23 changed files and 242 guard-shaped lines; the
plan below splits that coarse result by contract so mutation uses targeted suites.

## Original requests, verbatim

> yap onerinede uyuyorum. yanliz jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin

> bu update olayini yapman lazim. hali hazirda proje de zaten "mindrail init" yapildi.
>
> ayrica jev key'yi nereye girecegim?

Frozen requirements: `docs/engineering/installed-update-and-jev-distribution-2026-09-27.md`

## Cluster plan

| Cluster | Files | Depth | Breaker moves | Targeted test command | Status |
| --- | --- | --- | --- | --- | --- |
| Embedded JEV and bridge | `internal/agent/**`, `internal/cli/agent.go`, `internal/cli/agent_test.go`, deleted `.claude/.../scripts/jev_route.py` and test | A | 1–6 + threat, cost, class sweep, compatibility; A/B old vs embedded adapter | `python -B -m unittest discover -s internal/agent -p 'test_jev_route.py' -v && GOCACHE=/tmp/mindrail-update-audit-go go test ./internal/agent ./internal/cli -run 'TestAgent' -count=1` | pending |
| Existing-repository update | `internal/cli/init.go`, `update.go`, update/root/surface tests, `internal/status/render.go` | A | 1–6 + old-data upgrade and dual init/update readers | `GOCACHE=/tmp/mindrail-update-audit-go go test ./internal/cli ./internal/status -run 'Test(Update|CommandSurface|RootHelpShows|Init)' -count=1` | pending |
| Managed setup | `internal/setup/setup.go`, `setup_test.go`, managed block in `AGENTS.md` | A | 1–6 + hostile filesystem, old managed block, foreign hook, idempotence | `GOCACHE=/tmp/mindrail-update-audit-go go test ./internal/setup -count=1` | pending |
| Source install | `Makefile` | A | 1, 4, 5, class sweep; install-path and replacement threat experiments | disposable `DESTDIR` install, repeat install, mode/version/temp-residue checks | pending |
| User and engineering docs | `README.md`, `docs/usage-tr.md`, both engineering specs | C | Reader doc↔code conformance; Breaker may raise depth if a security contract is executable | none | pending |
| Integration seams | installed binary → `update` → managed AGENTS → `agent route` → embedded adapter | A | 3–6 + compiled-binary first-run/upgrade experiments | clean binary smoke scenarios in disposable repositories | pending |

Downgrades from automatic classification: documentation-only files are C because
their executable security claims are tested in the corresponding A clusters; no code
or deployment cluster is downgraded.

Sampling: mutation must cover every meaningful new production guard in
`internal/agent`, `internal/cli/agent.go`, the update preflight, and `Makefile`.
Existing unchanged framework guards and test-file conditionals are excluded. Any
survivor requires a discriminating input or an explicit proof of redundancy.

Safety: Breaker mutations and destructive install/upgrade experiments run only in
scratch copies and temporary repositories. The original working tree and the
pre-existing Codex session-history file containing an exposed key must not be mutated.
