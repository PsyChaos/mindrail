# Breaker Protocol — Falsification

The Breaker does not verify the change. It tries to break the running system and
reports what it managed to break.

The distinction is operational, not attitudinal: the Breaker's output is **produced
failures**, not descriptions of possible failures. If it couldn't produce it, it
didn't find it — and says so, with what was missing.

---

## B0 — Isolation and safety

Before anything else, stand up a disposable environment:

- Work on a scratch copy of the repository — `git worktree add /tmp/audit-wt <ref>` or
  a copy to a temp directory. Never mutate the user's working tree.
- Bring up your own database, cache and dependent services. Do not share an
  environment with anything else, and do not contaminate one someone else is using.
- Never run against production, shared staging, or real user data.

If the only available environment is one that matters, stop and tell the user exactly
what infrastructure is needed. Producing a failure is not worth causing one.

Confirm at the end that the original tree and any shared services are untouched.

---

## B1 — Mutation: delete each guard

For every protective condition or check the change added — validation, permission
check, bound check, null check, rate limit, error branch — remove it one at a time,
run the test suite, and observe.

```
guard removed → suite goes red   → the guard is tested. Revert, move on.
guard removed → suite stays green → the guard has no test. THIS IS A FINDING.
```

A condition that turns nothing red is an untested condition. It will be refactored
away by someone later with no signal that anything broke.

`scripts/mutate.py` automates the loop: it copies the repo, neutralizes one guard per
run, executes the test command, and reports survivors. Verify a sample of its
mutations by hand — a mutation that doesn't actually disable the guard produces a
false "killed" result and quietly hides the gap.

**Masked guards.** When two guards overlap, removing the first often leaves the second
catching the same input, so the mutation survives even though the behavior is
genuinely tested. A survivor is therefore a *lead requiring one more step*: construct
an input that reaches only the mutated guard. If such an input exists and no test
covers it, the finding is real. If the guard is genuinely unreachable behind another
one, that is also worth reporting — an unreachable check is dead code that future
readers will trust.

This is the one place where "produce it before writing it" costs an extra step, and
skipping it produces exactly the false positives that make a report get ignored.

The inverse check belongs here too: before believing any test protects something,
**break the protected thing and watch the test go red**. Three traps that make a
green test worthless:

- `pytest.raises(match=...)` is a regex **search** — an error message that merely
  *contains* the expected text satisfies it, including a different error that
  happens to mention the same name.
- A boundary test that never sits **on** the boundary passes at any limit. Move the
  limit to the old wrong value and confirm the test goes red; if it stays green, it
  pins nothing.
- A test that re-registers, re-creates, or re-mocks the very thing it verifies holds
  over nothing — it tests its own setup.

Record for each surviving guard: file, line, the condition, the command run, the green
output that should have been red, and the discriminating input you constructed.

---

## B2 — A/B of deleted behavior

If the change removed behavior, the removed code is not on the "after" side of the
diff. No amount of reading the current tree will find what it used to do.

So take it from the base commit and run it:

```bash
git show <base>:path/to/file.py > /tmp/old_version.py
```

Execute the old path and the new path **on the same input data**, and compare outputs.
Differences that nobody asked for are findings. Pay particular attention to error
paths, default values, and implicit behavior that no test named.

---

## B3 — Exercise the written threat model

If the code, comments, documentation or the task text describe an attacker profile, a
misuse case, or something the system is "protected against" — play that profile.

A threat that is described but never attempted is a threat that is not defended. The
description is a claim; the audit's job is to test it.

Construct the attempt concretely: the request, the payload, the sequence, the timing.
Record whether it succeeded, what the system did, and what the description promised it
would do.

---

## B4 — Upgrade path, not fresh install

A fresh install is the case everyone tested. Test the one nobody did:

```
old configuration + old data → apply the change → first run
```

Watch for: migrations that assume clean state, config keys renamed without a
fallback, defaults that changed under existing installations, data written by the old
version that the new version can't read, and startup ordering that only works on an
empty system.

---

## B5 — Weakest satisfying input: read your own predicate

Quote every new boolean the change introduces. For each, ask: what is the weakest
input that **satisfies** the condition while **defeating** what it was written for?

Do not brainstorm attacks. Read the condition literally and construct its minimum:

- `is not None` → the emptiest non-`None` value (`""`, `{}`, `[]`, `0`, a no-op
  template)
- `len(x) > 0` → one character
- `"=" in s` → the string `"="`
- `x != y` → values differing only by whitespace, case, or normalization form
- `startswith(prefix)` → the bare prefix
- `isinstance(x, dict)` → an empty dict

The classic shape this catches: a predicate testing **existence** where it meant to
test **content**. `if template is not None: return` is satisfied by a template that
does nothing — which is exactly the value the guard existed to refuse, and no amount
of case-brainstorming finds it, because it isn't an edge case; it's the condition's
own literal minimum.

Send each constructed input at the running system and record the response. Accepted
when it should refuse → finding, with the predicate quoted.

---

## B6 — Dual readers: one question answered in two places

Count how many places answer the question the change answers. Grep for the constant,
the lookup, the fold, the comparison: `lower(`, `strip`, `trim`, `casefold`,
`normalize`, the shared limit, the enum, the duplicated regex. A validation done in
SQL and again in application code, a limit checked at write and again at read, a
normalization in the UI layer and another on the wire path — each pair is a
candidate split.

Then give **every reader the same input**, chosen to expose normalization
differences: trailing/leading whitespace, a non-breaking space, case variants,
locale-sensitive characters (Turkish `İ/ı` where relevant — real characters, never
escapes), the same string in NFC and NFD, empty and whitespace-only values.

Diff the answers. Two readers that disagree on one input is a finding even when each
reader, read alone, looks correct — the defect lives in the *pair*, not in either
line, which is why single-file review never sees it.

One check before writing the case up: confirm the input you constructed actually
reaches both readers and **can fail**. A locale-case test over a value that has no
mapping in either spelling passes for the wrong reason and proves nothing.

---

## Mandatory off-spec headings

Areas the task text didn't mention, which is exactly why nobody checks them. Every one
gets an explicit entry in the Breaker report — including "checked, nothing found".

### Cost

Actually multiply the limits the endpoint or operation permits. Page size × maximum
pages × per-item work. Retries × timeout. Fan-out × payload size. Compute the worst
case; do not estimate it.

An operation whose own declared limits allow an unacceptable worst case is a finding
even if nobody has hit it yet.

### Mechanical rule = automated test

If the repository states a rule anywhere — a convention in the README, a constraint in
a comment, a policy in a contributing guide, an invariant in a docstring — ask whether
a machine enforces it.

A mechanical rule with no automated check is a finding. Rules that only live in prose
are followed until the first person who hasn't read that prose.

### Class sweep

Every finding closes with: **is there another place with the same shape?**

Grep for the pattern, not the instance. Fixing one occurrence and leaving its siblings
alive means the same defect returns under a different filename, and the audit gets
blamed for missing it.

Record the sweep and its result for each finding, even when the answer is "no other
occurrences".

### Backward compatibility

Which combination of configuration and data, in installations that already exist,
breaks under this change?

Enumerate concretely: which config keys, which data shapes, which versions. "Should be
fine" is not an answer to this heading.

---

## Breaker report

For each produced failure:

- What was attempted, exactly (commands, inputs, sequence)
- **Control arm** — the untouched case and its result
- **Scenario arm** — same data, one difference, and its result
- **Before/after numbers** in a table
- Class sweep result
- Severity and confidence (same scale as the Reader protocol)

For each move that produced nothing: say so, and say what was attempted. A move that
was skipped is not the same as a move that found nothing, and the report must not blur
them. All six moves get a line either way.

Open items: anything not produced this round, each with exactly what is required to
produce it. No lead lists carried forward.
