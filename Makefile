BINARY   := mindrail
PKG      := ./...
CMD      := ./cmd/mindrail
PREFIX   ?= $(HOME)/.local
BINDIR   ?= $(PREFIX)/bin
DESTDIR  ?=

# SQLite driver build tags.
#
# The driver is provisional until ADR-0002 records benchmark evidence
# (tech-stack §20). `mattn/go-sqlite3` only compiles FTS5 with the
# `sqlite_fts5` tag, so whichever driver ADR-0002 selects must have its
# required tags recorded here, in the release CI workflow and in ADR-0002.
# `modernc.org/sqlite` needs no tag for FTS5.
TAGS ?=
GOFLAGS_TAGS := $(if $(TAGS),-tags $(TAGS),)

# The smoke tests carry their own build tag. It is appended to TAGS rather than
# replacing it, because a second -tags flag would silently drop whichever tags
# the driver needs.
COMMA := ,
SMOKE_TAGS := $(if $(TAGS),$(TAGS)$(COMMA)smoke,smoke)

.DEFAULT_GOAL := check

.PHONY: build
build: ## Build the mindrail binary into ./bin
	go build $(GOFLAGS_TAGS) -o bin/$(BINARY) $(CMD)

.PHONY: install
install: ## Build and atomically install mindrail into DESTDIR+BINDIR
	@set -eu; \
	destdir="$(DESTDIR)"; \
	bindir="$(BINDIR)"; \
	if [ -n "$$destdir" ]; then \
		root="$$(realpath -m -- "$$destdir")"; \
		case "$$bindir" in /*) requested="$$destdir$$bindir" ;; *) requested="$$destdir/$$bindir" ;; esac; \
		dir="$$(realpath -m -- "$$requested")"; \
		case "$$root" in \
			/) case "$$dir" in /*) ;; *) echo "install: target escapes DESTDIR: $$dir" >&2; exit 2 ;; esac ;; \
			*) case "$$dir" in "$$root"|"$$root"/*) ;; *) echo "install: target escapes DESTDIR: $$dir" >&2; exit 2 ;; esac ;; \
		esac; \
	else \
		dir="$$(realpath -m -- "$$bindir")"; \
	fi; \
	dest="$$dir/$(BINARY)"; \
	if [ -L "$$dest" ] || { [ -e "$$dest" ] && [ ! -f "$$dest" ]; }; then \
		echo "install: destination must be absent or a regular file: $$dest" >&2; exit 2; \
	fi; \
	mkdir -p -- "$$dir"; \
	resolved="$$(realpath -e -- "$$dir")"; \
	if [ -n "$$destdir" ]; then \
		case "$$root" in \
			/) case "$$resolved" in /*) ;; *) echo "install: resolved target escapes DESTDIR: $$resolved" >&2; exit 2 ;; esac ;; \
			*) case "$$resolved" in "$$root"|"$$root"/*) ;; *) echo "install: resolved target escapes DESTDIR: $$resolved" >&2; exit 2 ;; esac ;; \
		esac; \
	fi; \
	dir="$$resolved"; dest="$$dir/$(BINARY)"; \
	if [ -L "$$dest" ] || { [ -e "$$dest" ] && [ ! -f "$$dest" ]; }; then \
		echo "install: destination changed to an unsafe type: $$dest" >&2; exit 2; \
	fi; \
	tmp="$$(mktemp "$$dir/.mindrail.install.XXXXXX")"; \
	trap 'rm -f "$$tmp"' EXIT HUP INT TERM; \
	go build $(GOFLAGS_TAGS) -ldflags "$(LDSTAMP)" -o "$$tmp" $(CMD); \
	chmod 0755 "$$tmp"; \
	mv -fT -- "$$tmp" "$$dest"; \
	trap - EXIT HUP INT TERM; \
	echo "installed $$dest"

.PHONY: install-test
install-test: ## Exercise hostile and normal install targets
	@./scripts/test-install.sh

.PHONY: test
test: ## Run the unit and integration test suite
	go test $(GOFLAGS_TAGS) $(PKG)

.PHONY: race
race: ## Run the suite under the race detector (tech-stack §96)
	go test $(GOFLAGS_TAGS) -race $(PKG)

# The smoke tests compile the binary and drive it as a subprocess, so they are
# slower and need a real git. They sit behind a build tag rather than in the
# default suite because `make test` has to stay fast enough to run on every
# save (tech-stack §136). -count=1 keeps a cached pass from standing in for a
# build that has since changed.
.PHONY: smoke
smoke: ## Run the clean-binary smoke tests (tech-stack §136)
	go test -tags $(SMOKE_TAGS) -count=1 -timeout 300s ./cmd/...

.PHONY: cover
cover: ## Run the suite and write a coverage profile
	go test $(GOFLAGS_TAGS) -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out | tail -1

.PHONY: vet
vet: ## Run go vet
	go vet $(GOFLAGS_TAGS) $(PKG)

# The warm-path benchmark suite (MR-019): the five operations against
# fixture repos, graded against the STRUCTURAL p95 targets. A p95 over
# target fails this target; recalibration lands as a documented decision
# with numbers, never a quiet constant change (spec-1.0 §28).
.PHONY: bench
bench: ## Run the warm-path benchmarks with p50/p95 grading
	go test $(GOFLAGS_TAGS) -run='^$$' -bench='Benchmark(Status|Context|BeforeChange|AfterChange|Reconcile)' -benchtime=100x -count=1 -v ./internal/mcp/
	go test $(GOFLAGS_TAGS) -run='^$$' -bench='BenchmarkAfterChangeSvc|BenchmarkReconcileSvc' -benchtime=100x -count=1 -v ./internal/changes/
	go test $(GOFLAGS_TAGS) -run='^$$' -bench='BenchmarkAnalyze' -benchtime=100x -count=1 -v ./internal/impact/

.PHONY: fmt
fmt: ## Format all Go sources
	go fmt $(PKG)

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is unformatted
	@unformatted=$$(gofmt -l . | grep -v '^testdata/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted files:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: tidy-check
tidy-check: ## Fail if go.mod or go.sum are not tidy
	@cp go.mod go.mod.bak; cp go.sum go.sum.bak 2>/dev/null || true; \
	go mod tidy; \
	status=0; \
	cmp -s go.mod go.mod.bak || { echo "go.mod is not tidy"; status=1; }; \
	cmp -s go.sum go.sum.bak 2>/dev/null || { echo "go.sum is not tidy"; status=1; }; \
	mv go.mod.bak go.mod; mv go.sum.bak go.sum 2>/dev/null || true; \
	exit $$status

.PHONY: check
check: fmt-check vet test install-test ## Local quality gate (fast: runs on every save)

# Release stamping (tech-stack §103). Dirty detection covers tracked
# modifications, staged changes AND untracked files: an untracked .go
# file compiles into the binary, so ignoring it would stamp "clean" over
# unknown contents. A tree git cannot read at all stamps "unknown"; a
# tree whose git answers brokenly stamps "dirty" — a repository no git
# command can inspect must never claim clean.
RELEASE_VERSION ?= 0.1.0
RELEASE_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
RELEASE_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
RELEASE_DIRTY := $(shell if ! git rev-parse --git-dir >/dev/null 2>&1; then echo unknown; else \
	out="$$(git status --porcelain 2>/dev/null)"; rc=$$?; \
	if [ $$rc -ne 0 ] || [ -n "$$out" ]; then echo dirty; else echo clean; fi; fi)
LDSTAMP := -X github.com/PsyChaos/mindrail/internal/cli.version=$(RELEASE_VERSION) \
	-X github.com/PsyChaos/mindrail/internal/cli.commit=$(RELEASE_COMMIT) \
	-X github.com/PsyChaos/mindrail/internal/cli.buildDate=$(RELEASE_DATE) \
	-X github.com/PsyChaos/mindrail/internal/cli.dirty=$(RELEASE_DIRTY)

# The gate that has to be green before a change is proposed. `check` stays fast
# enough to run continuously; `verify` adds the two suites that are too slow for
# that but too load-bearing to leave to a human's memory — the race detector and
# the clean-binary smoke tests, which are the only thing that exercises the
# compiled binary as a subprocess. `gate` (MR-019) enumerates the same ground
# by test category so each cut is re-runnable by hand.
.PHONY: verify
verify: check race smoke ## Full quality gate: check + race detector + smoke tests

# Release automation (MR-019, tech-stack §106 rule): stamp, build the
# platform matrix, publish checksums, smoke the native binary. Each matrix
# target ends built or not-built-with-reason in dist/MATRIX.txt; only
# build+smoke-proven targets are advertised, which in 0.1 is linux/amd64
# (cross C toolchains are absent — the reasons are on record, not silent).
.PHONY: release
release: ## Stamp, build the platform matrix, checksums, native smoke
	@mkdir -p dist
	@rm -f dist/MATRIX.txt
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS_TAGS) -ldflags "$(LDSTAMP)" -o dist/mindrail-linux-amd64 $(CMD)
	@echo "linux/amd64: built" | tee -a dist/MATRIX.txt
	@for target in "linux arm64" "darwin amd64" "darwin arm64" "windows amd64"; do \
		set -- $$target; \
		name="mindrail-$$1-$$2"; \
		if [ "$$1" = "windows" ]; then name="$$name.exe"; fi; \
		if GOOS=$$1 GOARCH=$$2 go build $(GOFLAGS_TAGS) -ldflags "$(LDSTAMP)" -o dist/$$name $(CMD) 2>dist/$$name.err; then \
			echo "$$1/$$2: built" | tee -a dist/MATRIX.txt; rm -f dist/$$name.err; \
		else \
			echo "$$1/$$2: not-built: $$(head -1 dist/$$name.err)" | tee -a dist/MATRIX.txt; rm -f dist/$$name dist/$$name.err; \
		fi; \
	done
	cd dist && sha256sum mindrail-* > SHA256SUMS
	@./dist/mindrail-linux-amd64 version --json > dist/version.json
	@grep -q '"build_date"' dist/version.json || (echo "release: stamped fields missing"; exit 1)
	@grep -q '"version":"$(RELEASE_VERSION)"' dist/version.json || (echo "release: version stamp mismatch"; exit 1)
	@grep -Eq '"dirty":"(clean|dirty)"' dist/version.json || (echo "release: dirty stamp not stamped"; exit 1)
	go test -tags $(SMOKE_TAGS) -count=1 -timeout 300s ./cmd/...

# Test-gate matrix (MR-019 AC-03.3): ten categories, each an explicit
# command a reader can re-run by hand. Every Go category asserts a non-empty
# selection; the install category runs its hostile-filesystem assertions.
.PHONY: gate
gate: ## Run ten test categories by explicit selection
	@./scripts/gate.sh

# Supply-chain scan (tech-stack §102). The binary is absent from developer
# machines, so the target reports absence loudly instead of presenting a
# missing scan as a pass; CI installs it and runs the scan for real.
.PHONY: vuln
vuln: ## Run govulncheck when present, report absence otherwise
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "vuln: SKIPPED — govulncheck not installed (CI runs the scan; this is not a pass)"; \
	fi

.PHONY: clean
clean: ## Remove build and coverage output
	rm -rf bin coverage.out

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'
