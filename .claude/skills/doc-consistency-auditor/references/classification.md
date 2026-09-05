# Classification — Categories, Severity, Confidence, Fix Decisions

## Finding categories

| Category | Meaning |
| --- | --- |
| `DOC_OVERPROMISE` | Docs describe functionality that does not exist |
| `CODE_UNDOCUMENTED` | Significant implemented functionality is absent from docs |
| `DOC_OUTDATED` | Docs describe a previous, superseded state |
| `SPEC_IMPLEMENTATION_CONFLICT` | Code violates an explicit, current specification |
| `API_ROUTE_MISMATCH` | Path, method or existence differs |
| `API_PAYLOAD_MISMATCH` | Request body / parameters differ |
| `API_RESPONSE_MISMATCH` | Response shape or status codes differ |
| `API_AUTH_MISMATCH` | Authentication or authorization requirements differ |
| `CONFIG_MISMATCH` | Config name, default, required-state or accepted values differ |
| `ARCHITECTURE_DRIFT` | Real structure materially differs from documented design |
| `DATA_MODEL_DRIFT` | Entities, fields, relations, enums or states differ |
| `BUSINESS_RULE_DRIFT` | Limits, permissions, transitions or validation differ |
| `INVALID_EXAMPLE` | Example, snippet or command does not work as written |
| `VERSION_DRIFT` | Documented versions contradict manifests/lockfiles |
| `DUPLICATED_DOC_DRIFT` | Duplicated documentation has diverged |

Use the config sub-categories (`CONFIG_MISSING_IN_DOC`, `CONFIG_REMOVED_FROM_CODE`,
`CONFIG_DEFAULT_MISMATCH`, `CONFIG_NAME_MISMATCH`) where they add precision.

---

## Severity

Severity answers: *what happens to someone who trusts the wrong side?*

**🔴 CRITICAL** — incorrect documentation can cause security exposure, data loss,
a broken production deployment, a destructive migration, or incorrect
authentication/authorization.

**🟠 HIGH** — major operational or integration mismatch: wrong API contract, broken
installation guide, materially incorrect architecture documentation, invalid
required configuration.

**🟡 MEDIUM** — outdated or incomplete technical documentation that costs time but
not correctness.

**🟢 LOW** — minor terminology, formatting or informational inconsistency.

Judge by consequence, not by how large the textual difference is. A single wrong
character in a documented `chmod`, `DROP`, or auth-header example can be CRITICAL,
while three paragraphs of outdated prose about a deprecated UI may be LOW.

---

## Confidence

| Level | When to use |
| --- | --- |
| `VERY_HIGH` | Directly verified in code; no plausible alternative reading |
| `HIGH` | Verified, with a small residual chance of context you can't see |
| `MEDIUM` | Strong signal but indirect evidence, or dynamic/meta-programmed code |
| `LOW` | Suspicion worth recording; needs human confirmation |

Anything at `MEDIUM` or below is reported but not auto-fixed.

---

## Fix decisions

| Decision | Use when |
| --- | --- |
| `FIX_DOC` | Implementation is clearly correct; documentation is stale |
| `FIX_CODE` | Documentation/specification is clearly authoritative and the implementation violates it |
| `SYNC_BOTH` | Both sides are incomplete or inconsistent |
| `REMOVE_DOC` | Docs describe removed functionality with no historical value |
| `ARCHIVE_DOC` | Document is obsolete but historically useful |
| `ADD_DOC` | Significant implementation is undocumented |
| `MANUAL_VERIFY` | Evidence is insufficient to decide |

Two rules that prevent the most damaging mistakes:

- Never choose `FIX_CODE` *solely* because documentation differs from code. Changing
  behavior to satisfy a sentence someone wrote is how working systems break.
- Never choose `FIX_DOC` *solely* because code differs from documentation. Rewriting
  a spec to match a bug launders the bug into intended behavior.

The deciding question is always the Phase 1 authority map, plus evidence: what do the
tests say, what does the schema say, what does the migration history say, what did
the ADR intend?

Treat security, deployment, migrations, authentication and destructive operations as
high-risk areas — bias toward `MANUAL_VERIFY` there rather than an automatic fix,
even at high confidence, because the cost of being wrong is asymmetric.
