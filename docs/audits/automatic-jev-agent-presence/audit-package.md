# Audit Package: Automatic JEV and Agent Presence

## Original request (verbatim)

> JEV su anda gercek kararlarda yer almiyor diye hatirliyorum. bunu ne zaman gercekten aktive edecegiz?

> P0-03 basladi. biz gelistirmeyi yapalim. diger task''e gecmeden once update ederiz. ayrica bir agent'in ne kadar sure calistigini aktifligini real time gormek istiyorum. yapilabilir mi?

The user also established that JEV must remain optional: when a key is configured it
may advise decisions; without a key the normal flow must continue.

## Frozen contract

The authoritative implementation contract is
`docs/engineering/automatic-jev-and-agent-presence-2026-09-28.md`, requirements
REQ-001 through REQ-012. Audit the implementation against that file without rewriting
the requirements to match the code.

## Changeset

- Base: `21f525a`
- Head under audit: working tree on `dev`
- Primary areas: `internal/agent`, `internal/mcp`, `internal/dashboard`,
  `internal/setup`, migration `000011`, CLI compatibility tests and user docs.
- Compatibility boundary: the hidden `mindrail agent route` version-1 JSON contract
  must remain intact while the visible MCP tool count grows from 13 to 14.

## Produced pre-audit evidence

- `make check`: PASS after two explicitly recorded remediation rounds (formatting and
  subprocess tool-count golden).
- `make gate`: PASS; all ten categories green.
- `graphify update .`: PASS.
- Component-focused race, migration, dashboard, setup, CLI and MCP tests were run by
  their scoped implementation tasks; do not accept these claims without checking the
  repository evidence and rerunning discriminating probes.

## High-risk questions

1. Can any raw prompt, goal, source, path, candidate identifier, provider response or
   credential value reach persistent storage or the dashboard?
2. Can successful JEV advice be consumed after telemetry persistence fails?
3. Can rate limiting, duplicate suppression or concurrent calls cross session
   boundaries or block normal reasoning?
4. Can heartbeat/activity/end races resurrect an ended runtime, end a replacement
   runtime or report a disconnected process as connected?
5. Do upgrade paths from schema 10 preserve old data and produce schema 11 exactly?
6. Do dashboard server and browser derive the same status at the exact 15s/30s
   boundaries and freeze ended duration?
7. Do managed instructions activate the visible MCP route automatically while being
   truthful about model/effort and activity observability limits?

