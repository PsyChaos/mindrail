# Report Template

Use this exact ten-section structure. Keep tables compact — the detail belongs in
section 5, not stuffed into table cells.

---

## 1. Executive Summary

Cover: overall documentation health, overall code/doc alignment, the level of drift,
and the most dangerous inconsistencies found. Write this for someone who will read
only this section — lead with what could actually hurt them.

## 2. Documentation Inventory

| File | Purpose | Status | Authority | Notes |
| --- | --- | --- | --- | --- |

Status: `ACTIVE` / `PARTIALLY_STALE` / `STALE` / `HISTORICAL` / `UNKNOWN`.
Authority: whether the document is a binding specification or a description.

## 3. Documentation ↔ Documentation Findings

| # | Document A | Document B | Topic | Conflict | Severity | Confidence | Action |
| --- | --- | --- | --- | --- | --- | --- | --- |

## 4. Documentation ↔ Code Findings

| # | Document | Section | Code Reference | Category | Severity | Confidence | Action |
| --- | --- | --- | --- | --- | --- | --- | --- |

Code Reference must be a real path with a line number or symbol name.

## 5. Detailed Findings

For every CRITICAL / HIGH / MEDIUM finding:

### DOC-[ID]

**Documents:** paths and sections
**Code references:** paths and lines
**Current documentation:** what the docs claim
**Actual implementation:** what the code actually does
**Cross-document state:** whether other docs agree or disagree
**Root cause:** why the drift likely occurred (a refactor that missed docs, a
reverted feature, a copy-paste of an older guide, a rename)
**Impact:** developer / operational / business / security
**Authority decision:** which source should be authoritative here, and why
**Required fix:** the exact remediation, concretely enough to execute

## 6. Canonical Documentation Map

| Subject | Canonical Document | Duplicate Locations | Recommended Action |
| --- | --- | --- | --- |

Subjects typically include architecture, setup, configuration, API, deployment,
security, testing, business rules and integrations.

## 7. Fix Roadmap

- **Batch 1** — critical factual / security / production inconsistencies
- **Batch 2** — architecture, API and config drift
- **Batch 3** — stale and duplicated documentation
- **Batch 4** — missing documentation and terminology cleanup

For each batch state: affected files, dependency order, estimated scope, and the
validation required before it can be considered done.

## 8. Applied Changes

| File | Section | Previous state | Updated state | Reason |
| --- | --- | --- | --- | --- |

Include an entry only for a change actually written to disk. If no fixes were
applied, say so explicitly rather than omitting the section.

## 9. Remaining Manual Review

List every unresolved ambiguity, and for each one state exactly what information
would resolve it — a person to ask, a decision to confirm, an environment to test
against, a spec to locate.

## 10. Final Integrity Matrix

| Area | Docs Internal Consistency | Code Alignment | Status |
| --- | --- | --- | --- |
| Architecture | | | |
| API | | | |
| Configuration | | | |
| Data Model | | | |
| Business Rules | | | |
| Setup | | | |
| Deployment | | | |
| Testing | | | |
| Integrations | | | |

Status values: `CONSISTENT`, `MINOR_DRIFT`, `MAJOR_DRIFT`, `CRITICAL_DRIFT`.

---

## Final validation before delivering

1. Re-scan all Markdown files.
2. Re-check all code references cited in the report — a report with a wrong line
   number has the same credibility problem it was written to fix.
3. Re-check document-to-document contradictions after fixes.
4. Verify links and file paths resolve.
5. Verify documented commands and config examples where the environment allows.
6. Confirm no new contradictions were introduced by the fixes.
7. Confirm the canonical documents are internally consistent.
