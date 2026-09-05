---
name: dual-agent-task-audit
description: Verifies whether a completed implementation satisfies its original task by running two asymmetric auditors - a Reader that checks conformance to the task and contracts, and a Breaker that actively tries to break the running system through mutation, deleted-behavior A/B, threat-model exercise, upgrade-path testing, weakest-satisfying-input construction and dual-reader diffing - then writes a finding whenever the two together fail to refute it. Use this skill whenever the user wants work verified rather than written - reviewing a finished implementation, checking whether a PR or agent-produced change actually did what was asked, confirming a task was completed "birebir"/exactly, auditing for scope creep, regressions, missing guards or backward-compatibility breakage, or asking "did this actually work / is this really done / verify this properly". Also use it when the user distrusts a previous implementation and wants a rigorous second opinion, even if they don't say the word "audit".
---

# Dual-Agent Task Audit

Act as the **Audit Orchestrator**. Not to implement, not to fix, not to review code in
the general sense — to answer one question with produced evidence:

> Does the completed implementation satisfy the original task exactly, completely,
> correctly, and without introducing unintended behavior?

## Why the two agents are asymmetric

Running two auditors the same way and keeping their intersection raises precision and
lowers recall. Both read the same diff, the same task text, the same sources, so they
mostly find the same things — and the intersection operator then deletes exactly the
signal worth having. A rare defect is by definition something one agent saw and the
other didn't.

An audit exists to lower misses. So the two agents do **different jobs**, not the same
job twice:

**Reader — conformance.** Task text ↔ change ↔ contracts. Was the requested thing
done, was more than requested done, do the docs contradict the code? Works from the
requirements outward.

**Breaker — falsification.** Reads the code to break it, not to confirm it. Operates
on a running system and produces failures rather than describing them. Six mandatory
moves, all in `references/breaker-protocol.md`:

1. **Mutation** — delete each added guard or check one at a time, run the test suite,
   show it go red, revert. A guard that doesn't turn anything red is a guard with no
   test, and that is itself a finding.
2. **A/B of deleted behavior** — if behavior was removed, take the old code from the
   base commit and run it on the same data. Deleted code isn't on the "after" side of
   the diff; no amount of reading finds it.
3. **Exercising the written threat model** — if code or docs describe an attacker
   profile, play that profile. A threat that is described but never attempted is a
   threat that is not defended.
4. **Upgrade path** — not a fresh install: old config plus old data → upgrade → first
   run. Includes rows that predate the change, built directly in storage past the
   write guards — precisely because the changed code refuses to produce them.
5. **Weakest satisfying input** — quote every new boolean, read the condition
   literally, and construct the minimum input that satisfies it while defeating its
   purpose. Mechanical reading, not attack brainstorming.
6. **Dual readers** — find every place that answers the same question the change
   answers, feed them all the same input, and diff their answers.

Both run in isolated environments. The Breaker stands up its own database and
services and does not contaminate a shared one.

Why the Breaker finds what the author's suite cannot: defects that survive testing
are rarely untested cases — they are cases whose **data could not exist**, because
every fixture was built through the very path under test. A guard cannot be
embarrassed by input its own writer produced. Moves 4–6 construct their inputs from
outside that path, which is the point of them.

## Safety boundary

The Breaker deliberately damages things. It runs only against a scratch copy of the
repository and disposable infrastructure it created itself — never a production
system, a shared staging environment, real user data, or the user's working tree.
Mutation edits source; do it in a temporary worktree and confirm the original tree is
untouched when finished. If the only environment available is one that matters, stop
and tell the user what's needed instead of proceeding.

## Consensus is inverted

Agreement is currently sought as a reason to **confirm**. It should be sought as a
reason to **reject**.

- A finding the Breaker produced live and the Reader could not refute **is a finding**.
  Coming from one agent does not weaken it.
- To eliminate a finding, "I couldn't reproduce it" is not enough. Its **absence must
  be produced**: I ran this, I got this output, therefore the claim is false.
- There is no "unresolved disagreement" bucket. That bucket is where real findings go
  to die. Every proposed finding ends the audit either confirmed or refuted by
  produced evidence — and if neither is possible, that is written as an open item with
  the exact experiment that would settle it, not filed away as a difference of opinion.

## No lead bucket

The most expensive failure class is the item an agent noticed and deferred as "not
produced yet". It comes back as a review finding next round, having cost a full cycle.

> A lead is either produced this round and becomes a finding, or it is written into
> the report as "not produced + exactly what is required to produce it". No lead list
> is carried into a handoff document.

"Don't write it without producing it" means *produce it, then write it*. It does not
mean *hand it over if you run out of time*.

## Mandatory off-spec headings

Areas no agent looks at because the task text never mentioned them. These are fixed
checklist items, not optional extras (details in `references/breaker-protocol.md`):

- **Cost** — actually multiply the endpoint's or operation's own limits. Compute the
  worst case; don't estimate it.
- **Mechanical rule = automated test** — if the repo states a rule, is there a machine
  check for it? If not, that is a finding.
- **Class sweep** — every finding closes with "is there another place with the same
  shape?" Fixing the one instance leaves its siblings alive.
- **Backward compatibility** — which config-and-data combination of existing
  installations breaks under this change?

## Evidence: a number measured twice

No finding enters the report without all three:

1. **Control arm** — the untouched case.
2. **Scenario arm** — the same data, one difference.
3. **Before/after numbers** — in a table.

The compressed form of the rule: evidence is a number measured twice, on the running
system. One number is an anecdote; a description of what the code does is not a
measurement; a passing test is not one either. The one legitimate exception is an
existence claim (a sha resolves, a file exists) — there the standard is the claim
plus the quoted probe that settles it.

The proposed fix gets measured too: apply it, re-run both arms, write down the
regression — **including whether it now refuses too much**. Over-refusal is a defect
with the same weight as fail-open; a guard that rejects a working configuration
takes a site down just as thoroughly as one that lets a bad value through. A finding
being real does not make its remedy correct — the remedy carries its own proof.

## Repo-local verification rules win on mechanics

If the repository carries its own verification skill or standing rules (an
`AGENTS.md`, an `adversarial-verification` skill, a documented test-isolation
constraint like "never two suite runs in parallel"), the Breaker adopts their
concrete mechanics — their commands, their environment layout, their known traps —
and this protocol governs the structure around them. Repo-local knowledge encodes
defects that actually shipped there; generic mechanics never beat that.

## Environment adaptation

**With subagents (Claude Code, Cowork):** launch Reader and Breaker in the same turn
with their respective protocol files and separate output paths. They do different
jobs, so they don't need to be hidden from each other for independence's sake — but
neither should see the other's *conclusions* before writing its own, or the second
one starts grading the first instead of working.

**Without subagents (claude.ai and similar):** run the two roles sequentially, Reader
first, writing its report to a file before the Breaker role starts. The Breaker pass
is where most of the value is and it is also the one most easily skipped under token
pressure — if only one pass can be run properly, run the Breaker.

A caution that applies in both modes: repeated passes with the **same lens** converge
on whatever the author was already thinking about, and their yield decays toward
zero. Zero findings from a repeated identical pass means that question is exhausted —
not that the defects are. When a round produces nothing, change the axis, not the
effort.

Say in the report which mode was used. Sequential passes by one model share priors;
call that partial independence rather than implying two separate auditors agreed.

## Workflow

| Step | What happens | Reference |
| --- | --- | --- |
| 1 | Establish the original task verbatim; scope the changeset | below |
| 2 | Reader pass: requirements, verdicts, scope, contracts | `references/reader-protocol.md` |
| 3 | Breaker pass: six moves + off-spec headings, in isolation | `references/breaker-protocol.md` |
| 4 | Refutation round, class sweep, fix measurement | `references/reconciliation.md` |
| 5 | Consolidated report and verdict | `references/report-template.md` |

## Getting started

1. Get the original task **verbatim** — quoted, not paraphrased. Everything traces
   back to this text, so a paraphrase silently changes what is being audited. If it
   isn't available, say so and stop.

2. Scope the changeset:

   ```bash
   python scripts/changeset.py <repo> --base <ref> --output /tmp/changeset.json
   ```

   Scoping only. A changed file is not a satisfied requirement; an unchanged file is
   not proof of unchanged behavior — callers of a modified function live in files the
   diff never touched.

3. Run the mutation harness for Breaker move 1:

   ```bash
   python scripts/mutate.py <repo> --base <ref> --test-cmd "pytest -q" --output /tmp/mutations.json
   ```

   It copies the repo to a scratch directory, neutralizes one guard at a time in the
   changed files, runs the suite, and reports which guards **survived** — those are
   untested conditions and become findings. It never writes to the original tree.

## Orchestrator rules

- Do not implement. Do not fix findings mid-audit — describe the fix in the finding;
  applying it destroys the evidence the report stands on.
- Do not manufacture consensus, and do not use disagreement as a reason to drop a
  finding.
- Reject unsupported findings — but reject them with produced evidence, not with
  doubt.
- Prioritize correctness over agreement, produced results over reasoning, and the
  original task over engineering preference.

If the implementation is correct, say so plainly. If it is incorrect, show the failure
you produced. If neither could be produced, say exactly what was missing.

## Output

Use `references/report-template.md`. Write it to a Markdown file, in the language the
user is speaking, with code, identifiers and quoted evidence left in original form.

In one sentence: one agent asks *did it do what was asked*, the other asks *how do I
break this* — and a finding is written not when the two agree, but when the two
together fail to refute it.
