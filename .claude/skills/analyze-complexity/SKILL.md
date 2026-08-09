---
name: analyze-complexity
description: Analyze a code path, system, workflow, or architecture to separate inherent complexity imposed by requirements and external constraints from accidental complexity introduced by the current implementation, then define the invariants a simplification must preserve. Use when the user asks what complexity is necessary or unavoidable, asks for inherent vs. accidental complexity, says a pipeline or subsystem feels too complex, wants to understand why something cannot be simpler, or wants a complexity map before refactoring. Also use before a technical-debt audit when unavoidable constraints and removable implementation complexity are mixed together. Do not use for exploring competing greenfield designs (`design-space-exploration`) or for a pure structural-debt inventory that does not need an invariant boundary (`technical-debt-audit`).
---

# Analyze Complexity

Identify what the problem fundamentally requires before criticizing how the current implementation satisfies it. Treat complexity as relative to an explicit system boundary, requirements, and dependencies—not as a permanent property of the universe.

## Output mode

- For an analysis, review the artifacts and report the split without modifying files.
- When the user asks to document it, create one canonical current-state complexity map under `docs/`; prefer `<area>-complexity.md` or `<area>-pipeline.md`.
- Do not create one file per constraint by default. Extract separate constraint documents only when one constraint needs substantial independent evidence or operational guidance.
- Do not turn accidental findings into debt files unless the user requests a debt audit. Use `technical-debt-audit` for that workflow and link the outputs.

## Procedure

### 1. Fix the boundary

State the subsystem, requirements, external dependencies, and time horizon being held constant. A constraint imposed by the selected database, protocol, or media tool is inherent relative to that boundary, but may disappear if replacing that dependency is in scope.

Start at the user's pain site. Trace the complete behavior through producers, consumers, persistence, external processes, and cleanup paths before forming conclusions. Read existing architecture, testing, and debt documentation when present.

### 2. Trace the flows

Reconstruct the minimum flows needed to explain behavior:

- data and backpressure;
- control, cancellation, and ownership;
- lifecycle and terminal states;
- timeouts and liveness signals;
- failures, cleanup, and diagnostic outcomes.

Use a small dataflow or state diagram only when concurrency, ownership, or ordering is difficult to explain linearly.

### 3. Derive obligations before mechanisms

For every candidate complexity, write the chain:

`external cause → required invariant → current representation → freedom to simplify`

Classify it with these tests:

- **Inherent:** every acceptable implementation inside the stated boundary must satisfy the invariant.
- **Accidental:** the complexity exists because of the chosen representation, duplicated policy, implicit protocol, leaky abstraction, or historical layering.
- **Mixed:** preserve the obligation but challenge the mechanism. Split it into an inherent row and one or more accidental findings.

Common inherent causes include ordering imposed by a protocol, bounded-resource requirements, partial failure, atomicity, concurrency, distributed ownership, external-process semantics, and the need to distinguish healthy idleness from stalled work.

Common accidental forms include parallel supervisors, overlapping timers, lifecycle encoded in booleans and call order, resource ownership conveyed by comments, duplicated error policy, lossy failure aggregation, and indirection unsupported by a current requirement.

Do not call a current class, queue, timer, pipe, or state machine inherently necessary. Name the behavioral obligation it currently serves.

### 4. Build the constraint register

Assign stable identifiers such as C1, C2, and C3. Use this table:

| ID | Obligation | External cause | Invariant | Current representation | Freedom to simplify |
| --- | --- | --- | --- | --- | --- |
| C1 | What the system must accomplish | Why the obligation exists | What every valid implementation preserves | How it is represented now | What may change safely |

Each row must make a useful falsifiable claim. “The system is complex because it is asynchronous” is not sufficient; “a slow consumer must eventually apply bounded backpressure without blocking unrelated inputs” is.

### 5. Add only the views the problem needs

- Add **lifecycle phases** when opening, running, graceful completion, abort, and terminal outcomes differ.
- Add a **liveness map** when several timeouts or progress signals exist. Identify pending work, relevant progress, deadline ownership, and reaction.
- Add a **failure/termination matrix** when tasks or processes fail concurrently. Distinguish clean drain, cancellation, timeout, transport failure, and nonzero process exit.
- Add a **verification map** connecting each constraint to existing tests, telemetry, or an explicit evidence gap.

These views elaborate the register; they are not independent sources of truth.

### 6. Name the accidental complexity

Cluster accidental findings by structural cause rather than symptom. For each cluster, identify:

- the concrete files, types, or components involved;
- the invariant the mechanism is trying to protect;
- the change, review, or debugging cost caused by the current representation;
- the likely simplification seam, without designing an unrequested replacement architecture.

Examples of the split:

- Bounded backpressure is inherent; three watchdogs inferring it from unrelated events are accidental.
- Graceful completion and abort are inherently distinct; requiring callers to remember `AbortAsync` before `DisposeAsync` is accidental.
- Concurrent operations can fail independently; swallowing pump errors because process exit is assumed authoritative is accidental.

### 7. Check the boundary

Before presenting the result, verify:

- every inherent item has an external cause and invariant;
- every current mechanism has been separated from the obligation it serves;
- every accidental item has a concrete consequence;
- proposed freedom does not erase backpressure, atomicity, ordering, liveness, or failure guarantees;
- the document describes current constraints without becoming a changelog or implementation defense.

## Canonical document shape

```markdown
# <Area> complexity map

[Scope, fixed boundary, and purpose]

## Scope and terminology

## Data flow

[Optional diagram]

## Inherent complexity register

| ID | Obligation | External cause | Invariant | Current representation | Freedom to simplify |
| --- | --- | --- | --- | --- | --- |

## Lifecycle phases

[Include only when useful]

## Current liveness map

[Include only when useful]

## Failure and termination matrix

[Include only when useful]

## Verification map

## Accidental complexity boundary

[Named clusters or links to technical-debt entries]
```

## Relationship to adjacent skills

- Use `technical-debt-audit` after this analysis when the user wants accidental findings documented as actionable debt.
- Use `design-space-exploration` when the question is which replacement architecture to choose.
- Use `state-transition-testing` when an identified lifecycle needs a complete transition matrix and tests.
- Use `fmea` when the user wants failure modes prioritized by severity, occurrence, and detectability.
