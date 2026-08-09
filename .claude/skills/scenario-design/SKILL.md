---
name: scenario-design
description: Diagnose the shape of a scenario-enumeration problem (multiple variables, states, conditions, or failure modes) and route to the right systematic technique — decision-tables, equivalence-partitioning-bva, combinatorial-testing, state-transition-testing, fmea, design-space-exploration, scenario-analysis, or cause-effect-graphing. Use this eagerly when the user asks "what cases should I handle?", "what could go wrong?", "am I missing any scenarios?", "how do I systematically test/model this?", or describes a problem that could plausibly need more than one of those techniques (e.g. an entity with both range-constrained fields and a state machine), and it isn't obvious yet which one(s) fit. If the user already knows which technique they want (they name it, or describe a problem shape that maps cleanly to exactly one — e.g. "map out this state machine" or "what could go wrong with this integration") jump straight to that technique's own skill instead; this skill's job is diagnosis and combination, not narrow application.
---

# Scenario Design

Activate when the shape of a scenario-enumeration problem isn't yet clear, or when a problem plausibly needs more than one systematic technique. This skill's job is to diagnose the shape, confirm the plan with the user, and hand off to the technique-specific skill(s) that do the actual work — it doesn't duplicate their procedures.

## How this works

1. Understand the problem space
2. Diagnose the problem shape and select a technique (or combination) using the guide below
3. Explain your reasoning and proposed approach; ask the user to confirm
4. Invoke the matching skill(s) via the Skill tool to apply the technique and produce the output
5. If more than one technique was used, tie the outputs together in a short summary

When multiple techniques could apply, recommend the simplest one that covers the gap — don't stack techniques for the sake of completeness.

## Technique selection guide

The central question is: **what is the shape of the problem?**

| Problem shape | Technique | Skill |
|---|---|---|
| Conditional rules with discrete outcomes ("if A and B then X") | Decision table | `decision-tables` |
| Input parameters with valid/invalid ranges (integers, strings, dates) | Equivalence partitioning + BVA | `equivalence-partitioning-bva` |
| Many parameters where the combination space is too large to enumerate exhaustively | Pairwise / orthogonal array / combinatorial testing | `combinatorial-testing` |
| System with states and transitions (workflows, protocols, state machines) | State transition testing | `state-transition-testing` |
| Risk-driven: what can fail, how likely, how severe? | FMEA | `fmea` |
| Competing design options or unexplored parameter space | Design space exploration | `design-space-exploration` |
| Narrative-driven scenarios with stakeholder context | Scenario analysis | `scenario-analysis` |
| Causal chains: which inputs drive which outputs, and how? | Cause-effect graphing | `cause-effect-graphing` |

**Notes on overlap** — some techniques are closely related; knowing the distinctions prevents choosing redundantly:

- **Decision tables vs. cause-effect graphing**: Same analysis, different presentation. A cause-effect graph is a decision table drawn as a directed graph. Default to decision tables — they're more directly actionable and easier to turn into test cases. Reach for cause-effect graphing only when the causal structure itself is tangled enough to need untangling first, or a stakeholder needs the visual.
- **EP + BVA always go together**: they live in one skill (`equivalence-partitioning-bva`), not two — equivalence partitioning divides the input space into classes, BVA zooms in on the class boundaries, and you rarely apply one without the other.
- **Orthogonal arrays vs. all-pairs**: Both belong to the `combinatorial-testing` skill. All-pairs (pairwise testing) is the practical subset to reach for first; orthogonal arrays when higher-strength interaction coverage is needed.
- **FMEA vs. scenario analysis**: FMEA is quantitative (severity × probability → risk priority); scenario analysis is narrative and contextual. Use FMEA when a risk register is needed; use scenario analysis when stakeholder-facing narratives are needed.

## Procedure

### 1. Read the problem

Understand what the user is working with:
- What artifact is being analyzed? (code, requirements, design, business rules, architecture, configuration)
- What variables or dimensions are involved?
- What is the user's goal? (test cases, gap report, risk register, coverage model, design decision)

If the context is ambiguous, ask one focused question before proceeding.

### 2. Diagnose the shape

Map the problem to the technique selection guide. State which technique(s) fit and why in 2–3 sentences. If the problem has multiple distinct dimensions (e.g., input validation *and* stateful transitions), name both and explain how each will be handled — usually as separate passes with separate outputs, since forcing two different problem shapes into one technique produces a worse result than two clean, focused passes.

### 3. Explain and confirm

Present the technique selection to the user:
- Which technique(s) are being recommended and why they fit this problem shape
- What the output will look like (table, state matrix, FMEA register, etc.)
- Rough scope: how many rows/scenarios to expect

Ask: "Does this approach make sense, or would you like to adjust?"

Proceed after the user confirms. If the user redirects to a different technique, follow their lead — they may know constraints that aren't visible here.

### 4. Hand off

Invoke the confirmed technique's skill via the Skill tool (e.g. `decision-tables`, `state-transition-testing`). That skill owns the full application procedure — building the table/graph/matrix, flagging gaps, choosing the output path, and writing the file. Don't re-derive its steps here.

If the problem spans multiple dimensions that need different techniques, invoke each skill for its slice of the problem in turn.

### 5. Tie it together

If only one technique was used, its own summary is sufficient — no extra wrap-up needed.

If multiple techniques were used, add a short combined summary:
- How the outputs relate (e.g., "the state transition table above covers the workflow; the EP/BVA table in the same file covers the numeric fields on each transition")
- Total gaps found across both passes
- Suggested next steps (e.g., "5 of these have no corresponding tests — `/blind-spot-coverage` on `X` would address them")
