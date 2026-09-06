# ADR-0001 — Primary language and CLI stack

- **Status:** Accepted
- **Date:** 2026-09-05
- **Scope:** 0.1 kernel and beyond

## Context

Mindrail ships as a local-first engineering gate that agents and CI invoke on
the interactive path. The technology strategy in `docs/specification/mindrail-tech-stack.md`
already fixes the language and the CLI framework; this record captures that
decision in the repository so later changes have something to supersede.

The binding constraints are:

- The product is distributed as a single native executable per platform
  (tech-stack §27), so a runtime-dependent language is excluded.
- Warm-path commands carry hard p95 budgets — `status` under 150 ms, `context`
  under 250 ms (kernel scope AC-13) — so process startup cost matters.
- The official Tree-sitter Go binding needs a native C toolchain (§25, §26),
  so CGO is available and is not an argument against any dependency by itself.
- The command surface is hierarchical: `mindrail knowledge validate`,
  `mindrail doctor --resolver`, `mindrail verify --ci` (§11).

## Decision

- Implement the core in **Go**, baseline **1.27.x**, with `go 1.27` in `go.mod`
  (tech-stack §3).
- Build the command tree with **`github.com/spf13/cobra`** (§11).
- Keep `cmd/mindrail/main.go` limited to the root context, bootstrap, CLI
  execution and the exit code (§85).
- Use the small exit-code convention from §13: `0` success, `1` operation
  failed, `2` invalid usage or config, `3` verification denied, `4` temporary
  or runtime unavailable. Detail belongs in structured output, not in new
  exit codes.

## Consequences

- A single static-ish binary per platform, no runtime install on the user's
  machine, and startup cost compatible with the warm-path SLOs.
- CGO stays enabled, which constrains cross-compilation to a per-platform
  build matrix rather than a single `GOOS`/`GOARCH` sweep.
- Cobra is the only CLI dependency; help, completion and flag parsing come
  from it rather than from hand-written parsing.
- Exit codes stay coarse, so agents and CI must consume `--json` output to
  distinguish failure reasons.
