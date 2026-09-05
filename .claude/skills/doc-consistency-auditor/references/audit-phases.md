# Audit Phases — Detailed Checklists

Contents:
- [Phase 0 — Repository-wide indexing](#phase-0--repository-wide-indexing)
- [Phase 1 — Source of truth model](#phase-1--source-of-truth-model)
- [Phase 2 — Doc ↔ Doc consistency](#phase-2--doc--doc-consistency)
- [Phase 3 — Doc ↔ Code consistency](#phase-3--doc--code-consistency)
- [Phase 4 — Stale / historical detection](#phase-4--stale--historical-detection)
- [Phase 5 — Duplication & ownership](#phase-5--duplication--ownership)
- [Phase 6 — False-positive protection](#phase-6--false-positive-protection)

---

## Phase 0 — Repository-wide indexing

### A. Documentation index

Cover every Markdown file, not just `README.md`: `**/*.md`, `docs/**`, architecture
documents, ADRs, project plans, install/deployment guides, API docs, developer
guides, runbooks, troubleshooting notes, feature specs, migration notes, security
docs, testing docs. Include documentation-shaped configuration too: OpenAPI/Swagger,
AsyncAPI, GraphQL SDL and its comments, `.env.example`, comments in
`docker-compose.yml`, sample config files, and doc comments that serve as the public
contract (docstrings, JSDoc) when there is no prose equivalent.

For each document record: path, purpose, scope, intended audience, apparent
authority level (is this a *specification* that binds the code, or a *description*
of it?), referenced modules/features, and any dates or version references.

The specification/description distinction drives everything downstream. A spec that
the code violates is a code bug; a description that the code has outgrown is a doc
bug. Getting this backwards is the most damaging mistake this audit can make.

### B. Codebase index

Scan entrypoints, modules, packages, services, controllers/routes, models/entities,
schemas, configuration, migrations, tests, scripts, CI/CD, Docker/IaC, dependency
manifests and lockfiles, environment definitions.

Extract the *actual*: architecture, implemented features, API surface, CLI commands,
config keys, environment variables, integrations, persistence model, auth behavior,
workflows, deployment structure.

Tests deserve special attention — they encode intended behavior that is executable,
which makes them stronger evidence than prose and often the only place a business
rule is written down truthfully.

---

## Phase 1 — Source of truth model

Before hunting for mismatches, decide per topic where authority lives. Candidates,
roughly in descending order for runtime questions: executable code → tests →
schema/contracts → configuration → infrastructure definitions → documentation.

General rule: actual runtime implementation outranks descriptive documentation.
Exception: where an explicit, current specification exists and the code contradicts
it, do not rewrite the spec to match the bug. Classify as
`SPEC_IMPLEMENTATION_CONFLICT` and demand deeper validation.

Produce an authority map, e.g.:

| Domain | Primary source | Secondary source |
| --- | --- | --- |
| API routes | router/controller code | API docs |
| Request schema | validation/schema layer | docs |
| Deployment | CI / Docker / IaC | deployment docs |
| Environment vars | config-loading code | `.env.example`, docs |
| Business rules | tests + implementation | specifications |
| Architecture intent | ADR / spec | implementation |

Adapt the rows to the repository actually in front of you.

---

## Phase 2 — Doc ↔ Doc consistency

Compare documents against each other **before** comparing them to code.

**Architecture** — monolith vs microservices claims, a dependency described as
required in one place and optional in another, references to services/modules that
another document says were removed, data flows that differ between diagrams.

**Features** — the same feature described differently, different limits or defaults,
one document calling a feature current while another calls it deprecated, conflicting
workflow steps.

**APIs** — divergent paths, payloads, HTTP methods, status codes, auth requirements.

**Configuration** — the same environment variable with two meanings, conflicting
defaults, renamed variables still referenced under the old name, obsolete setup
instructions.

**Setup & deployment** — conflicting install commands, runtime versions, package
managers, Docker instructions, migration commands, deployment sequences.

**Terminology** — conceptual naming drift such as customer/client, user/account,
tenant/organization, order/transaction. Flag only where the drift can cause a
technical misunderstanding — different words for the same idea in prose are fine and
flagging them buries the real findings.

**Version drift** — obsolete framework, language, package, API or database versions.

---

## Phase 3 — Doc ↔ Code consistency

Cross-reference every meaningful documentation claim against implementation.

### A. Features

For each documented feature classify: implemented / partially implemented / removed /
renamed / feature-flagged / not found.

- `DOC_OVERPROMISE` — documentation describes functionality that does not exist.
- `CODE_UNDOCUMENTED` — significant implemented functionality is absent from the docs.

For `CODE_UNDOCUMENTED`, apply a significance filter: undocumented internal helpers
are normal and healthy. Flag undocumented things a *user or operator* must know —
public API surface, config that changes behavior, operational side effects.

### B. APIs

Verify against routing/controllers: route existence, HTTP method, path parameters,
query parameters, request body, response structure, status codes, authentication,
authorization, pagination, validation rules.

Categories: `API_ROUTE_MISMATCH`, `API_PAYLOAD_MISMATCH`, `API_RESPONSE_MISMATCH`,
`API_AUTH_MISMATCH`.

Auth mismatches are the highest-risk subclass here — a doc that says an endpoint is
authenticated when it isn't, or lists the wrong required role, points directly at a
security exposure.

### C. Configuration

Compare docs and example config against actual config usage: variable names,
required/optional state, default values, accepted values, feature flags.

Categories: `CONFIG_MISSING_IN_DOC`, `CONFIG_REMOVED_FROM_CODE`,
`CONFIG_DEFAULT_MISMATCH`, `CONFIG_NAME_MISMATCH`.

A documented variable the code no longer reads is quietly dangerous: an operator sets
it, believes the system is configured, and it silently isn't.

### D. Architecture

Compare architecture documentation to the real module/dependency structure: module
boundaries, service ownership, layer boundaries, dependency direction, external
integrations, state ownership, database responsibilities, queues/events, shared
libraries. Where implementation materially differs from documented design, that is
`ARCHITECTURE_DRIFT`.

### E. Database & data model

Verify documented entities, fields, relations, enums, lifecycle states, indexes and
migrations against ORM models, schema files, migrations and validation types.
Migrations are the strongest evidence of what actually happened to the database.

### F. Business rules

Verify limits, expiry times, permissions, statuses, transitions, quotas, validation
constraints and retry behavior across three sources: documentation → tests →
implementation. When they disagree, report all three; the pattern of disagreement
usually reveals which one was updated last.

### G. Examples & snippets

Validate commands, import paths, code snippets, API requests, JSON examples, config
samples and CLI examples wherever possible. Where the environment allows it, actually
run or lint the example rather than eyeballing it.

Categories: `INVALID_EXAMPLE`, `BROKEN_COMMAND`, `STALE_SNIPPET`.

### H. Dependencies & versions

Compare documented versions against manifests and lockfiles for language runtime,
frameworks, databases, dependencies, package managers and build tools. Manifests and
lockfiles are stronger evidence than prose unless an explicit versioning policy says
otherwise.

---

## Phase 4 — Stale / historical detection

Signals of obsolescence: references to removed modules, outdated package versions,
outdated paths, superseded architecture, old commands, old feature names, old
environment variables.

Classify each document: `ACTIVE`, `PARTIALLY_STALE`, `STALE`, `HISTORICAL`, `UNKNOWN`.

Never delete automatically. Where a stale document still carries archival value —
it explains *why* something was built a certain way — recommend moving it under an
archive/history location with a header noting it is historical, rather than removing it.

---

## Phase 5 — Duplication & ownership

Detect duplicated documentation (the classic case: `README.md` and `docs/setup.md`
both carrying the install guide). Where duplicated copies have diverged, classify as
`DUPLICATED_DOC_DRIFT`.

For each duplicated subject, identify: the canonical document, the duplicate
locations, the sections to remove, and the links to add in their place. Duplication
without divergence is still a finding — it is drift that hasn't happened yet — but
it is `LOW` severity unless the content is high-risk.

---

## Phase 6 — False-positive protection

Before reporting a mismatch, check whether the difference is intentional:

- environment-specific or platform-specific behavior
- version-specific documentation (docs for a released version vs `main`)
- experimental features
- intentionally deprecated functionality kept for compatibility
- feature flags
- migration-period backward compatibility
- enterprise vs community edition differences
- development vs production settings

If ambiguity remains after checking, do not classify it as confirmed. Mark
`MANUAL_VERIFY` and lower the confidence. A confident false positive costs more trust
than a hedged true one.
