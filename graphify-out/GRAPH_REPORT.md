# Graph Report - Mindrail  (2026-09-05)

## Corpus Check
- 6 files · ~25,494 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 312 nodes · 833 edges · 11 communities
- Extraction: 93% EXTRACTED · 7% INFERRED · 0% AMBIGUOUS · INFERRED: 60 edges (avg confidence: 0.88)
- Token cost: 227,014 tokens combined this run (input/output split not reported by the extraction harness; not estimated)

## Community Hubs (Navigation)
- MCP Protocol Surface
- Reconcile & Completion Gate
- Impact & Coverage Analysis
- Runtime Coordination & CLI
- Implementation Stack Choices
- Validation, Evidence & Approval
- Versioned Engineering Knowledge
- Semantic Resolver Lifecycle
- Syntax Index & Change Detection
- Git & CI Enforcement
- Performance & Resource Budget

## God Nodes (most connected - your core abstractions)
1. `Mindrail Technology Stack and Implementation Guide` - 31 edges
2. `Mindrail Technical Specification 1.0` - 29 edges
3. `MCP Tool Surface` - 26 edges
4. `Reconcile Workflow` - 23 edges
5. `Acceptance Criteria` - 23 edges
6. `In Scope` - 19 edges
7. `Agent Protocol Instructions` - 18 edges
8. `MR-014 MCP knowledge and context tools` - 17 edges
9. `Mindrail 0.1 Kernel Scope` - 17 edges
10. `Protocol Call Sequence` - 16 edges

## Surprising Connections (you probably didn't know these)
- `Graphify Protocol` --semantically_similar_to--> `CLAUDE.md graphify Protocol`  [INFERRED] [semantically similar]
  AGENTS.md → CLAUDE.md
- `Managed Section Markers` --implements--> `Repository Layout`  [INFERRED]
  AGENTS.md → .docs/mindrail-technical-specification-1.0.md
- `Out Of Scope` --conceptually_related_to--> `0.1 Deferred Delivery Backlog`  [INFERRED]
  .docs/mindrail-0.1-kernel-scope.md → .tasks/mindrail-0.1-task-list.md
- `MR-001 Local repository bootstrap and diagnostics path` --implements--> `SQLite Migrations`  [EXTRACTED]
  .tasks/mindrail-0.1-task-list.md → .docs/mindrail-tech-stack.md
- `First Implementation Milestones` --conceptually_related_to--> `Dependency Waves 1-8`  [INFERRED]
  .docs/mindrail-tech-stack.md → .tasks/mindrail-0.1-task-list.md

## Hyperedges (group relationships)
- **Reconcile-First Correctness Spine** — _docs_mindrail_technical_specification_1_0_reconcile_workflow, _docs_mindrail_technical_specification_1_0_session_correctness_path, _docs_mindrail_technical_specification_1_0_unregistered_change, _docs_mindrail_tech_stack_reconcile_implementation_rule, _docs_mindrail_tech_stack_protocol_skipped_scenario, _docs_mindrail_0_1_kernel_scope_ac_05 [EXTRACTED 1.00]
- **Thirteen-Tool Agent Protocol Surface** — _docs_mindrail_technical_specification_1_0_mcp_tool_surface, _docs_mindrail_technical_specification_1_0_agent_protocol_instructions, _docs_mindrail_technical_specification_1_0_managed_section_tool_coverage, agents_mindrail_protocol_managed_section, agents_protocol_call_sequence, _docs_mindrail_0_1_kernel_scope_mcp_parameter_surface [EXTRACTED 1.00]
- **Milestone Alignment Between Documents** — _docs_mindrail_tech_stack_milestone_mapping_table, _docs_mindrail_tech_stack_first_implementation_milestones, _docs_mindrail_tech_stack_reconcile_in_fifth_milestone, _docs_mindrail_technical_specification_1_0_milestone_4, _docs_mindrail_technical_specification_1_0_milestone_5, _docs_mindrail_technical_specification_1_0_milestone_8 [EXTRACTED 1.00]

## Communities (11 total, 0 thin omitted)

### Community 0 - "MCP Protocol Surface"
Cohesion: 0.15
Nodes (40): AC-14, AC-21, MCP Parameter Surface, Core E2E Workflow, MCP Contract Tests, MCP Go SDK, MCP Layering, Protocol Compliant Scenario (+32 more)

### Community 1 - "Reconcile & Completion Gate"
Cohesion: 0.09
Nodes (39): AC-05, AC-07, AC-09, AC-11, AC-18, Exit Condition, Goal Hypothesis, Primary E2E Scenario (+31 more)

### Community 2 - "Impact & Coverage Analysis"
Cohesion: 0.08
Nodes (39): AC-22, Mindrail 0.1 Kernel Scope, In Scope, Non Goals, Out Of Scope, Structural Precision Limits, Coverage Provider Interface, Explainability Storage (+31 more)

### Community 3 - "Runtime Coordination & CLI"
Cohesion: 0.09
Nodes (31): AC-01, AC-03, AC-04, AC-15, AC-16, Agent Workspace Modes, CLI JSON Output, Cobra CLI (+23 more)

### Community 4 - "Implementation Stack Choices"
Cohesion: 0.11
Nodes (30): ADR List, Application Wiring, Dependency Matrix, Mindrail Technology Stack and Implementation Guide, Durable Symbol Identity Impl, Explicit Non Choices, Fixture Repositories, Go Primary Language (+22 more)

### Community 5 - "Validation, Evidence & Approval"
Cohesion: 0.12
Nodes (29): AC-08, AC-17, Approval Provider Architecture, Filesystem Safety, Milestone Mapping Table, Process Tree Cleanup, Secret Redaction Pipeline, Security Boundaries (+21 more)

### Community 6 - "Versioned Engineering Knowledge"
Cohesion: 0.14
Nodes (23): AC-02, Schema Policy v1, Compatibility Surfaces, JSON Schema Validation, Knowledge Schema Compatibility, AC-11, AC-12, AC-16 (+15 more)

### Community 7 - "Semantic Resolver Lifecycle"
Cohesion: 0.12
Nodes (23): Managed Resolver Cache, Pyright Resolver, Resolver Interface, Semantic Resolver Out Of Process, TypeScript Resolver, AC-01, AC-02, AC-03 (+15 more)

### Community 8 - "Syntax Index & Change Detection"
Cohesion: 0.15
Nodes (22): AC-06, Cold Index Worker Model, Content Addressed Parse Cache, Fsnotify Watcher, Telemetry and GC, AC-13, AC-14, AC-15 (+14 more)

### Community 9 - "Git & CI Enforcement"
Cohesion: 0.21
Nodes (20): AC-10, AC-12, AC-19, AC-20, Acceptance Criteria, CI In Scope Rationale, CI Integration, Git Hooks (+12 more)

### Community 10 - "Performance & Resource Budget"
Cohesion: 0.20
Nodes (16): AC-13, Performance Targets, Concurrency Model, Interactive Latency Engineering, Priority Scheduler, Resolver Pool Eviction, AC-25, AC-26 (+8 more)

## Knowledge Gaps
- **37 isolated node(s):** `0.1 Deferred Delivery Backlog`, `CLAUDE.md graphify Protocol`, `AC-01`, `AC-02`, `AC-03` (+32 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 49 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Mindrail Technical Specification 1.0` connect `Semantic Resolver Lifecycle` to `MCP Protocol Surface`, `Reconcile & Completion Gate`, `Impact & Coverage Analysis`, `Runtime Coordination & CLI`, `Implementation Stack Choices`, `Validation, Evidence & Approval`, `Versioned Engineering Knowledge`, `Syntax Index & Change Detection`, `Git & CI Enforcement`, `Performance & Resource Budget`?**
  _High betweenness centrality (0.213) - this node is a cross-community bridge._
- **Why does `Mindrail Technology Stack and Implementation Guide` connect `Implementation Stack Choices` to `MCP Protocol Surface`, `Reconcile & Completion Gate`, `Impact & Coverage Analysis`, `Runtime Coordination & CLI`, `Validation, Evidence & Approval`, `Semantic Resolver Lifecycle`, `Performance & Resource Budget`?**
  _High betweenness centrality (0.186) - this node is a cross-community bridge._
- **Why does `Mindrail 0.1 Kernel Scope` connect `Impact & Coverage Analysis` to `MCP Protocol Surface`, `Reconcile & Completion Gate`, `Implementation Stack Choices`, `Versioned Engineering Knowledge`, `Semantic Resolver Lifecycle`, `Git & CI Enforcement`, `Performance & Resource Budget`?**
  _High betweenness centrality (0.092) - this node is a cross-community bridge._
- **Are the 3 inferred relationships involving `MCP Tool Surface` (e.g. with `AC-14` and `AC-21`) actually correct?**
  _`MCP Tool Surface` has 3 INFERRED edges - model-reasoned connections that need verification._
- **Are the 3 inferred relationships involving `Reconcile Workflow` (e.g. with `AC-05` and `Core E2E Workflow`) actually correct?**
  _`Reconcile Workflow` has 3 INFERRED edges - model-reasoned connections that need verification._
- **What connects `0.1 Deferred Delivery Backlog`, `CLAUDE.md graphify Protocol`, `AC-01` to the rest of the system?**
  _37 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `MCP Protocol Surface` be split into smaller, more focused modules?**
  _Cohesion score 0.14871794871794872 - nodes in this community are weakly interconnected._