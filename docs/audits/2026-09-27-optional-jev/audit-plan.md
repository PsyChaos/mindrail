# Optional JEV Routing Audit Plan

- Original request: “yap onerinede uyuyorum. yanliz jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin”
- Frozen specification: `docs/engineering/optional-jev-routing-2026-09-27.md`
- Base revision: `ba35fbe`
- Audit scope: current working-tree diff and new files implementing optional JEV routing.

## Clusters

1. **Adapter and tests — depth A**
   - `.claude/skills/engineering-orchestrator/scripts/jev_route.py`
   - `.claude/skills/engineering-orchestrator/scripts/test_jev_route.py`
   - Rationale: handles a secret, an external API, network deadlines, and fail-open behavior.
   - Reader: trace every requirement and branch to code and tests.
   - Breaker: exercise all six breaker moves, including absent/blank keys, malformed inputs, provider failures, hostile responses, secret leakage, and deadline behavior.

2. **Guidance and specification — depth C**
   - `.claude/skills/engineering-orchestrator/SKILL.md`
   - `AGENTS.md`
   - `docs/engineering/optional-jev-routing-2026-09-27.md`
   - Rationale: documentation-only contract and operator guidance.
   - Reader: verify consistency, discoverability, and preservation of the managed AGENTS section.
   - Breaker: challenge ambiguous or contradictory fallback and acceptance semantics.

## Validation

```sh
python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p 'test_jev_route.py' -v
make tidy-check
GOCACHE=/tmp/mindrail-jev-gocache make verify
GOCACHE=/tmp/mindrail-jev-gocache make gate
```

Reader and Breaker work independently. Breaker mutations, if any, must occur only in a scratch copy. Any confirmed finding requires remediation and a delta re-audit before acceptance.
