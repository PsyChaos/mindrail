<!-- BEGIN MINDRAIL MANAGED SECTION -->
## MINDRAIL PROTOCOL

Mindrail is the repository's local engineering gate. The MCP server command is
`mindrail mcp`, started in this repository. Use the existing 13 MCP tools.

At the start of a task, call mindrail_bootstrap with goal, an agent-generated
stable run_key, and paths when known. Keep the same run_key when retrying or
reconnecting. To continue an existing handoff, also pass resume_task_id explicitly.
Do not choose another agent's active task. Sessions, task revisions, operation
identities and leases are managed automatically; do not ask a person to copy them.

Read context through mindrail_status, mindrail_search and mindrail_context.
If bootstrap omitted paths, call mindrail_before_change with paths before the first edit.
Call it again before editing any file outside the declared scope. It may be skipped
only when bootstrap paths already cover every file being edited. Relative
repository file paths are accepted in automatic mode. The task and session may
be omitted when this connection has bootstrapped automatic work. Use
mindrail_after_change or mindrail_reconcile to inspect changes when useful.
Record durable choices through mindrail_decide and mindrail_invariant.

To finish, call mindrail_complete with finalize: true. It reconciles changes,
runs configured validation profiles, applies the shared completion gate, and
completes the task only when allowed. A denial is work to resolve, never success.
No configured profiles means no test profiles ran; do not claim tests passed.
For handoff, call mindrail_checkpoint with a note and handoff: true. The next
run uses a fresh run_key and explicitly resumes that task.

People normally run mindrail init once, and mindrail status, mindrail doctor or
mindrail verify when needed. CI runs mindrail verify --ci. Advanced commands and
explicit-ID MCP payloads remain available for diagnostics and legacy clients.
<!-- END MINDRAIL MANAGED SECTION -->

## Optional Jev routing

Before an ambiguous tool, agent, model, or reasoning-effort choice, check
`TYPESAFE_API_KEY`. When it is non-blank, run
`.claude/skills/engineering-orchestrator/scripts/jev_route.py` with a bounded JSON
request on standard input. Supply the goal, relevant context, and closed candidate
arrays as top-level `tools`, `agents`, `models`, and/or `efforts` fields, with an
`id` and `description` for each candidate. The required `goal` is a string;
`context` is optional JSON.

This path is opt-in through a non-blank `TYPESAFE_API_KEY`. If the key is absent or
blank, do not make a TypeSafe network request; continue the existing workflow. Jev
output is shadow/advisory only. Routing acceptance is atomic: apply selections only
when the top-level status is `"ok"` and every selection is marked `accepted: true`.
Any low-confidence or no-match selection makes the whole result a fallback with all
selections unaccepted, so continue normal flow.

Disabled routing and provider or response failures return typed JSON with exit
status zero and preserve the normal flow. Malformed, oversized, or otherwise
contract-invalid local input returns typed fallback JSON with exit status 2; fix the
caller invocation before retrying instead of treating it as provider fallback. Jev
must never change task scope, permissions, the Mindrail lifecycle, validation, or
completion gates.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
