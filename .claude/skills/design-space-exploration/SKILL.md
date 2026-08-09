---
name: design-space-exploration
description: Map the dimensions along which an architectural or design decision can vary, place known options in that space, and surface the unexplored regions and implicit assumptions before a decision gets locked in. Use eagerly when the user is deciding *how* to build something and multiple valid approaches exist — sync vs. async, monolith vs. services, REST vs. GraphQL, storage tier choices, algorithm/data-structure selection — or asks "what are all the ways we could solve this", "what are we not considering", "help me think through this design tradeoff", or wants a systematic pass before committing to an architecture. Unlike the other scenario-design techniques, this isn't about testing an existing system — it's about reasoning about the space of possible systems before one is built. If the goal is instead to enumerate test scenarios for something already built or specified, another scenario-design technique fits better.
---

# Design Space Exploration

Design space exploration maps the **parameter space of a design decision** — the dimensions along which a design can vary — to understand what tradeoffs exist, which regions of the space have been explored, and which haven't. Unlike techniques that test an existing system, design space exploration is about reasoning about the *space of possible systems* before committing to one.

The goal is to surface implicit assumptions ("we assumed X had to be true, but does it?"), identify regions of the design space that haven't been considered, and make tradeoffs explicit so decisions are deliberate rather than accidental.

## When to use

- Architectural decisions with multiple valid approaches (e.g., sync vs. async, push vs. pull, monolith vs. services)
- Algorithm or data structure selection where multiple options have non-obvious tradeoffs
- API design where parameter choices have cascading implications
- Anywhere the user is deciding *how* to build something and wants to be systematic about what they're not considering
- Research and spike work: "what are all the ways we could solve this?"

## How to apply

### Step 1 — Identify the design dimensions

What are the axes along which this design can vary? Each dimension is a parameter the designer controls. Examples:
- Storage: in-memory vs. persisted vs. distributed
- Consistency: strong vs. eventual
- API style: REST vs. GraphQL vs. RPC
- Execution model: synchronous vs. asynchronous vs. event-driven
- Coupling: tight vs. loose

Aim for 3–6 independent dimensions. Too many makes the space unnavigable; too few suggests the problem hasn't been fully decomposed.

### Step 2 — Enumerate values for each dimension

For each dimension, list the meaningful design choices. These don't have to be binary — they can be points on a spectrum.

### Step 3 — Map known design points

For each design option already considered, place it in the space by specifying its value on each dimension. This makes implicit choices explicit.

### Step 4 — Identify unexplored regions

Look for combinations of dimension values that don't correspond to any design considered so far. Ask: is this region unexplored by accident, or is it genuinely unviable? A region nobody has looked at might contain a better design; a region ruled out should have an explicit reason.

### Step 5 — Evaluate tradeoffs

For each design point (or region), characterize the tradeoffs:
- Performance vs. consistency
- Simplicity vs. flexibility
- Cost vs. reliability

The goal is not to find the "best" design but to make the tradeoffs visible so the decision-maker can choose deliberately.

### Step 6 — Identify constraints that eliminate regions

Some combinations are impossible given system constraints (latency requirements, team skill, cost budget, existing infrastructure). Document these as constraints — they explain why certain regions aren't being explored.

## Output format

```markdown
## Design Space: [Decision Name]

### Dimensions

| Dimension | Values |
|---|---|
| Data consistency model | Strong / Eventual / Causal |
| Storage tier | In-memory / Database / Distributed cache |
| API coupling | Synchronous request-response / Async message / Event-driven |
| Deployment topology | Single node / Active-passive / Active-active |

### Constraints
- Active-active topology requires eventual or causal consistency (strong consistency is prohibitively expensive across nodes)
- In-memory storage is eliminated by the 99.9% availability requirement

### Design points evaluated

| # | Design | Consistency | Storage | API | Topology | Tradeoff summary |
|---|---|---|---|---|---|---|
| A | Current design | Strong | Database | Sync | Single node | Simple; bottleneck at scale; single point of failure |
| B | Proposed redesign | Eventual | Distributed cache | Async | Active-passive | More complex; resilient; requires conflict resolution logic |
| C | Unexplored | Causal | Database | Event-driven | Active-passive | May balance simplicity and resilience; not yet analyzed |

### Unexplored regions

| Region | Why unexplored | Worth investigating? |
|---|---|---|
| Causal consistency + event-driven API | Team unfamiliar with causal consistency semantics | Yes — could be a middle ground; recommend a spike |
| In-memory + active-active | Eliminated by availability constraint | No |
| Async + strong consistency | Technically feasible but operationally expensive | Low priority given current scale |

### Recommendation

Design B addresses the resilience requirement but introduces conflict resolution complexity. Region C (causal + event-driven) may offer a better tradeoff and is worth a short spike before committing.

**Open questions:**
- What are the actual consistency requirements? "Strong" may be over-specified for this use case.
- Does the team have operational experience with distributed caches?
```

## Procedure

1. Understand the decision being made and confirm the dimensions with the user — this step benefits most from a quick back-and-forth, since the "right" dimensions are rarely obvious upfront.
2. Apply the steps above: enumerate values, map known design points, identify unexplored regions, evaluate tradeoffs.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<decision-name>-design-space.md`. Ask the user if they'd prefer a different path before writing.
4. Summarize: the recommendation (if one emerges), and which unexplored region is most worth a follow-up spike.
