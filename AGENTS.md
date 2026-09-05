<!-- BEGIN MINDRAIL MANAGED SECTION -->
## MINDRAIL PROTOCOL

> **Status: not yet active.** Mindrail 0.1 is not implemented, so the `mindrail_*` MCP
> tools below do not exist yet and calling them will fail. This block is the managed
> section defined by Technical Specification 1.0 §117; `mindrail init` will own and
> rewrite it once the binary ships. Do not hand-edit the content between the markers.

```text
MINDRAIL PROTOCOL

Session start:
mindrail_bootstrap

Orientation:
mindrail_status
mindrail_search

Before task work:
mindrail_claim
mindrail_context

Before source mutation:
mindrail_before_change

After mutation:
mindrail_after_change

If a change was made without before_change,
or the actual diff must be re-derived:
mindrail_reconcile

When a durable design choice is made:
mindrail_decide

When a protected contract is discovered:
mindrail_invariant

Validation:
mindrail_validate

Completion:
mindrail_complete

Handoff:
mindrail_checkpoint
```

Enforcement model (Technical Specification 1.0 §118): this section is **soft**
enforcement. Skipping a protocol call does not bypass the gate — `reconcile`
discovers the actual diff from Git regardless, and leases, the completion gate,
pre-commit and `mindrail verify --ci` are the hard gates.

<!-- END MINDRAIL MANAGED SECTION -->

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
