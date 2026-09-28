# Audit Plan: Automatic JEV and Agent Presence

Mode: parallel, asymmetric subagents. The Reader and Breaker must write independent
reports before either sees the other's conclusions.

## Reader

Use the dual-agent Reader protocol. Normalize the verbatim request and frozen
REQ-001..REQ-012 before reading implementation details. Produce requirement verdicts,
scope analysis, contract/documentation comparison and concrete findings in
`reader.md`. Treat executed behavior as stronger evidence than code reading.

## Breaker

Use the dual-agent Breaker protocol in an isolated scratch copy/worktree. Execute all
six moves: guard mutation, deleted-behavior A/B, written threat model, schema-10
upgrade, weakest satisfying inputs and dual-reader comparisons. Cover the mandatory
cost, mechanical-rule, class-sweep and backward-compatibility headings. Never mutate
the user's working tree or shared Mindrail database. Write `breaker.md`.

The minimum test command for mutation probes is the narrow package test that owns the
guard; use broader suites where cost permits. Every claimed failure needs a control
arm, scenario arm and before/after numbers.

## Reconciliation

After both independent reports exist, attempt evidence-producing refutation of every
proposed finding. Record confirmed, refuted and open items in `reconciliation.md` and
produce the consolidated Turkish report in `report.md`. No finding may be silently
dropped and no implementation fix may be applied during the audit pass.

