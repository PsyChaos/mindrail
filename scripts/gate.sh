#!/bin/sh
# Test-gate matrix (MR-019 AC-03.3): ten test categories as explicit,
# hand-re-runnable commands. Each Go category runs verbose exactly once and
# asserts at least one top-level PASS; the install category runs its own
# hostile-filesystem assertions. (`go test -list` ignores `-run` when
# counting, so counting is done on the run output, never on a listing.)
#
# Usage: ./scripts/gate.sh (or `make gate`). Needs Go, Git, make, and
# Linux/coreutils (including realpath, sha256sum, and mv -T).
set -eu

run() {
	name="$1"
	shift
	echo "gate: running $name: go test $*"
	out="$(mktemp)"
	# shellcheck disable=SC2064
	trap "rm -f '$out'" EXIT
	rc=0
	GOCACHE="${GOCACHE:-/tmp/mindrail-go-build}" go test -v "$@" >"$out" 2>&1 || rc=$?
	count=$(grep -c '^--- PASS' "$out" || true)
	if [ "$rc" -ne 0 ]; then
		echo "gate: category '$name' FAILED (exit $rc), last lines:" >&2
		tail -30 "$out" >&2
		exit 1
	fi
	if [ "$count" -eq 0 ]; then
		echo "gate: category '$name' ran no tests ($*)" >&2
		exit 1
	fi
	echo "gate: $name: $count passed"
	rm -f "$out"
	trap - EXIT
}

run_step() {
	name="$1"
	shift
	echo "gate: running $name: $*"
	if "$@"; then
		echo "gate: $name: passed"
	else
		rc=$?
		echo "gate: category '$name' FAILED (exit $rc)" >&2
		exit "$rc"
	fi
}

# 1. unit: the whole fast suite.
run unit ./...

# 2. domain: packages owning domain rules (gate, impact, attribution,
# coordination, guards, validation, verify, knowledge).
run domain ./internal/gate/ ./internal/impact/ ./internal/changes/ \
	./internal/coordination/ ./internal/testguard/ ./internal/validation/ \
	./internal/verify/ ./internal/knowledge/...

# 3. integration: cross-seam tests by name.
run integration -run 'Integration|EndToEnd|E2E' ./...

# 4. race: concurrency hotspots under the detector (full `make race`
# remains the slow authority; this cut runs inside the gate).
run race -race -run 'Concurren' ./internal/storage/ ./internal/coordination/ \
	./internal/changes/ ./internal/index/... ./internal/migration/

# 5. knowledge schema: records, schemas, validation, migrations.
run knowledge-schema ./internal/knowledge/... ./migrations/...

# 6. MCP contract: discovery, schemas, errors, stdio end-to-end.
run mcp-contract ./internal/mcp/

# 7. Git worktree: discovery, ranges, hooks, linked worktrees.
run git-worktree ./internal/git/
run git-worktree-named -run 'Worktree' ./internal/cli/ ./internal/bootstrap/ ./internal/mcp/

# 8. SQLite concurrency: busy-retry, idempotency, revision conflicts.
run sqlite-concurrency -run 'Concurren|Busy|Idempoten|Revision' \
	./internal/storage/ ./internal/coordination/ ./internal/changes/ ./internal/index/...

# 9. end-to-end: named journeys plus the clean-binary smoke suite.
run end-to-end -run 'EndToEnd|E2E' ./...
run end-to-end-smoke -tags smoke -count=1 -timeout 300s ./cmd/...

# 10. install boundary: hostile filesystem targets plus atomic replacement.
run_step install-boundary ./scripts/test-install.sh

echo "gate: all ten categories green"
