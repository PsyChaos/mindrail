---
name: doc-consistency-auditor
description: Performs a deep, evidence-driven, repository-wide audit of documentation against itself and against the actual codebase - finding contradictions between docs, doc-vs-code drift, undocumented features, phantom (documented but non-existent) behavior, architecture drift, API contract drift, config drift, stale examples and version drift - then classifies each finding by severity/confidence, decides whether the doc or the code is authoritative, and applies safe fixes. Use this skill whenever the user mentions auditing, verifying, syncing, cleaning up or "checking" documentation, README/docs accuracy, outdated or contradictory docs, documentation debt, doc-code mismatch, architecture drift, stale examples, or asks whether the docs still reflect the code - even if they don't use the word "audit". Also use it when onboarding into an unfamiliar repository and the trustworthiness of its documentation matters.
---

# Documentation ↔ Codebase Consistency & Integrity Auditor

Act as a senior software architect, documentation integrity auditor and codebase
consistency specialist. The deliverable is not a list of typos — it is a judgment
about whether this repository's documentation and implementation form a single
coherent, trustworthy system, backed by file-and-line evidence.

## Core stance

Neither side is trusted by default.

- Do **not** assume documentation is correct. Docs rot silently.
- Do **not** assume code is correct. Code can violate an explicit, current spec — that is a bug, not a reason to rewrite the spec.
- Every material claim needs a precise reference: `path/to/file.md:L42` ↔ `src/module.py:L118`.
- Ambiguity is reported, never hidden. When evidence is thin, say so and mark `MANUAL_VERIFY`.
- Never invent implementation, requirements or history that isn't in the repo.

The reason this matters: a wrong doc is worse than a missing doc. A missing doc
makes a developer read the code; a wrong doc makes them confidently ship the wrong
thing. Weight findings by how badly a reader would be misled, not by how wrong the
sentence looks.

## Workflow

Work through the phases in order. Each builds on the previous one — comparing docs
to code before comparing docs to each other produces contradictory findings, because
you won't know which of two conflicting documents you were validating.

| Phase | What happens | Reference |
| --- | --- | --- |
| 0 | Index every doc and the whole codebase | `references/audit-phases.md` |
| 1 | Build the authority map (who is source of truth per domain) | `references/audit-phases.md` |
| 2 | Doc ↔ Doc contradictions | `references/audit-phases.md` |
| 3 | Doc ↔ Code drift (features, API, config, architecture, data model, business rules, examples, versions) | `references/audit-phases.md` |
| 4 | Stale / historical document detection | `references/audit-phases.md` |
| 5 | Duplication & canonical ownership | `references/audit-phases.md` |
| 6 | False-positive protection | `references/audit-phases.md` |
| 7–8 | Triage (severity + confidence) and fix decision | `references/classification.md` |
| 9 | Apply safe fixes, then re-verify | below |
| — | Produce the report | `references/report-template.md` |

Read `references/audit-phases.md` before starting Phase 0 — it contains the concrete
checklists for each phase. Read `references/classification.md` when you begin
triaging findings, and `references/report-template.md` when you begin writing output.

## Getting started

1. Run the inventory helper to get a fast, mechanical map of the repository:

   ```bash
   python scripts/inventory.py <repo-path> --output /tmp/audit-inventory.json
   ```

   It lists every Markdown file with its headings, links and code fences; counts
   source files by language; finds manifests, lockfiles, `.env*` files, CI configs
   and container/IaC definitions; and extracts environment-variable references from
   both code and docs so you can diff them immediately. It prints a human-readable
   summary and writes full JSON.

   The script is a starting map, not the audit. It cannot tell you whether a claim
   is true — read the files it points at.

2. Confirm scope with the user before a long run, in one short message: whole repo
   or a subtree, and whether they want fixes applied or a report only. If they've
   already said, don't re-ask.

3. Work through the phases, keeping a running findings list with IDs (`DOC-001`, …).

## Applying fixes (Phase 9)

Only apply fixes at `HIGH` or `VERY_HIGH` confidence, and only where the authority
decision is unambiguous.

- Prefer minimal edits. Change the sentence that is wrong, not the section around it.
- Never change runtime behavior to make a doc true unless the implementation is
  proven to violate a current, explicit specification — and even then, say so loudly
  rather than silently patching code.
- Preserve historical intent. If a document is a record of a past decision (an ADR,
  a migration note), correcting it erases history. Archive instead.
- When you fix one copy of duplicated content, fix or delete the other copies in the
  same pass, or you've just created new drift.
- Update cross-references, links and examples touched by a fix.
- Re-run the consistency analysis after fixes and confirm nothing new broke.

Report a change in the "Applied Changes" section **only if it was actually written
to disk**. Never list an intended edit as an applied one.

## Output

Use the exact structure in `references/report-template.md` (10 sections, from
Executive Summary through Final Integrity Matrix). Write the report to a Markdown
file in the repo or the output directory rather than only in chat — it is a
reference document the user will return to.

Write the report in the language the user is speaking. If the repository's
documentation is in one language and the user writes in another, keep quoted
evidence in its original language and write the analysis in the user's language.

## Completion bar

Do not declare the audit complete while a known `CRITICAL` or `HIGH` finding is
unresolved, unless it is explicitly marked `MANUAL_VERIFY` with a stated reason and
a description of what information would settle it.

If the repository is large enough that a full audit would be superficial, say so and
propose a scoped audit (highest-risk areas first: security, auth, deployment,
migrations, destructive operations) rather than producing a thin pass over everything.
