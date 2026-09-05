BINARY   := mindrail
PKG      := ./...
CMD      := ./cmd/mindrail

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
check: fmt-check vet test ## Local quality gate

.PHONY: clean
clean: ## Remove build and coverage output
	rm -rf bin coverage.out

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'
