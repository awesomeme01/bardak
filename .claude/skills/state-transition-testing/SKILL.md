---
name: state-transition-testing
description: Model a system as states connected by event-triggered transitions, and find the (state, event) combinations nobody has defined — missing transitions, unreachable states, trap states, and undefined invalid-transition handling. Use eagerly whenever the user describes an entity with a status field that changes based on events (order lifecycle, payment states, approval workflows, auth flows, wizard/multi-step forms, protocol states) and wants to find gaps in the transition logic, or asks "what transitions are we missing", "can this get stuck", "map out this state machine", or "what happens if this event fires in the wrong state". If the problem is really about combinations of independent input parameters rather than a stateful entity, the scenario-design skill can route elsewhere.
---

# State Transition Testing

State transition testing models a system as a finite set of **states** connected by **transitions** (triggered by events or conditions). The goal is to enumerate all valid and invalid transitions, verify that each one produces the correct next state and output, and identify transitions that are missing, unreachable, or undefined.

A state transition model has four elements:
- **States**: distinct modes or configurations the system can be in
- **Events/triggers**: inputs or conditions that cause transitions
- **Transitions**: edges from one state to another, triggered by an event
- **Actions/outputs**: what the system does during a transition

## When to use

- Shopping cart or checkout workflows
- Order lifecycle (created → pending → processing → fulfilled → cancelled)
- Authentication flows (unauthenticated → authenticating → authenticated → session_expired)
- Document/approval workflows
- Network protocol states
- UI wizard or multi-step form flows
- Any entity with a documented status field that changes based on events

## How to apply

### Step 1 — Enumerate all states

List every distinct state the system can be in. Include:
- Initial state (where does the entity start?)
- Terminal states (states with no outgoing transitions, or only self-loops)
- Error/invalid states (if the system can enter a bad state)

### Step 2 — Enumerate all events

List every event that can trigger a transition. Events may be:
- User actions (submit, cancel, approve)
- System events (timeout, payment confirmation, webhook received)
- Time-based triggers (expiry)

### Step 3 — Build the state transition table

For every (current state, event) pair, specify:
- The **next state** (or "invalid" / "ignored" if the event doesn't apply in this state)
- The **action/output** taken during the transition

Every cell in the table is meaningful: a blank means the behavior is **undefined** — which is a gap.

### Step 4 — Identify gaps

- **Missing transitions**: (state, event) cells that are blank or unspecified
- **Unreachable states**: states that have no incoming transitions (can the system ever enter this state?)
- **Trap states**: states with no outgoing transitions that aren't intended terminal states
- **Invalid transition handling**: what happens when an event fires in a state that shouldn't receive it? Is this specified?

### Step 5 — Derive test cases

For each meaningful transition (valid and invalid), derive a test case:
- Start in the source state
- Fire the event
- Assert the resulting state and output match the expected values

Cover at minimum: every valid transition, and every invalid transition that touches a business-critical state.

## Output formats

### State transition table

```markdown
## State Transition Table: [System Name]

| Current state | Event: submit | Event: approve | Event: reject | Event: cancel | Event: timeout |
|---|---|---|---|---|---|
| **draft** | → pending / notify reviewers | — | — | → cancelled / archive | — |
| **pending** | — | → approved / notify author | → draft / notify author | → cancelled / archive | → expired / notify admin |
| **approved** | — | — | — | → cancelled / archive | — |
| **cancelled** | — | — | — | — | — |
| **expired** | submit → pending / restart | — | — | → cancelled / archive | — |

Legend: `→ next_state / action performed`. `—` = event ignored in this state (no-op). Blank = **undefined behavior (GAP)**.

**Gaps:**
- What happens if `approve` fires on an already-approved document?
- Can an expired document be cancelled? Not specified.
- No transition defined for `reject` on an approved document.
```

### Optional: Mermaid diagram

For visual review, a state diagram can accompany the table. Show **valid transitions only** — gaps belong in the table and gaps section, not in the diagram.

```markdown
\`\`\`mermaid
stateDiagram-v2
    [*] --> draft
    draft --> pending : submit
    draft --> cancelled : cancel
    pending --> approved : approve
    pending --> draft : reject
    pending --> cancelled : cancel
    pending --> expired : timeout
    approved --> cancelled : cancel
    expired --> pending : submit
    expired --> cancelled : cancel
    cancelled --> [*]
\`\`\`
```

**Mermaid syntax rules — follow these to avoid parse errors:**
- Transition labels: `state --> state : label` — keep labels short, no brackets `[]`, no backslash-n `\n`
- Notes: use the block form, not inline colon form:
  ```
  note right of STATE
      text here
  end note
  ```
  Not: `note right of STATE : text` (this causes a parse error)
- If a transition has conditional branching (e.g. two possible targets), pick the most likely one and document the alternative in the gaps section — don't try to encode it in the label
- When in doubt, omit the diagram; the table is the authoritative artifact

Include the Mermaid diagram when the user will benefit from a visual overview; always include the table as the authoritative artifact.

### Test case list (derived from the table)

```markdown
## Derived test cases

| # | Start state | Event | Expected next state | Expected action | Notes |
|---|---|---|---|---|---|
| 1 | draft | submit | pending | Notify reviewers | Happy path |
| 2 | draft | cancel | cancelled | Archive document | |
| 3 | pending | approve | approved | Notify author | Happy path |
| 4 | pending | reject | draft | Notify author with feedback | |
| 5 | pending | timeout | expired | Notify admin | |
| 6 | approved | cancel | cancelled | Archive | |
| 7 | expired | submit | pending | Restart review | |
| 8 | draft | approve | draft (unchanged) | Error / ignored | Invalid transition |
| 9 | approved | approve | ??? | ??? | **GAP: behavior undefined** |
```

## Procedure

1. Understand the entity being modeled and confirm the state and event lists with the user if either is ambiguous.
2. Apply the steps above to build the full (state × event) table, then derive test cases.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-state-transitions.md`. Ask the user if they'd prefer a different path before writing. Include the Mermaid diagram only if it adds value beyond the table.
4. Summarize: how many gaps, unreachable states, or trap states were found, and what the highest-priority fix is.
