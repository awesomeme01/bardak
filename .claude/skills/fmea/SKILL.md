---
name: fmea
description: Systematically enumerate failure modes for a system or process, score each by severity, likelihood, and detectability, and produce a prioritized risk register (Risk Priority Number = S × O × D). Use eagerly whenever the user asks "what could go wrong", wants a pre-launch risk review of a critical feature, is turning an incident postmortem into a broader risk analysis, needs a documented risk assessment for compliance, or describes a new system/integration where failure modes haven't been catalogued yet. Trigger on "failure modes", "risk register", "what are the risks here", "RPN", "risk priority", or "what should we worry about before we ship this". Prefer this over scenario-analysis when the output needs to be quantified and prioritized rather than narrative; if the user wants stakeholder-facing stories instead of a scored register, the scenario-analysis skill fits better.
---

# FMEA — Failure Mode and Effects Analysis

FMEA is a systematic method for identifying how a system can fail, what the consequences of each failure are, and which failures to prioritize based on their risk. The core output is a **risk register**: a table where each row is a failure mode, scored by severity, probability of occurrence, and detectability. The product of these three scores — the **Risk Priority Number (RPN)** — determines what to address first.

FMEA originated in hardware engineering (aerospace, automotive) but applies equally well to software systems, API integrations, data pipelines, and any complex process.

## When to use

- Designing a new system or integration where failure modes aren't yet documented
- Pre-launch risk review of a critical feature
- Incident post-mortems (to turn "what happened" into "what else could happen")
- Compliance contexts that require documented risk assessment
- Any situation where the user asks "what could go wrong?" and needs a prioritized, structured answer

FMEA is quantitative — it produces a ranked register. If the user needs narrative, stakeholder-facing situations instead (who does what, under what conditions), the `scenario-analysis` skill is the better fit.

## Scoring

Each failure mode is scored on three dimensions (1–10 scale):

**Severity (S)**: How bad is the impact if this failure occurs?
- 1–3: Minor inconvenience, no data loss, easily recoverable
- 4–6: Degraded functionality, recoverable with effort, some data risk
- 7–9: Major functionality loss, potential data loss, user-facing outage
- 10: Catastrophic — data corruption, security breach, irrecoverable loss

**Occurrence (O)**: How likely is this failure to occur?
- 1–2: Remote — unlikely under normal conditions
- 3–4: Low — occasional, known edge case
- 5–6: Moderate — intermittent, known to happen under certain conditions
- 7–8: High — frequent or expected under normal load
- 9–10: Very high — near-certain without mitigation

**Detection (D)**: How easy is it to detect this failure before it impacts users?
- 1–2: Certain detection — monitored, alerted, automatically caught
- 3–4: High — likely caught by existing tests or monitoring
- 5–6: Moderate — may be caught, but not reliably
- 7–8: Low — likely to go undetected until user reports
- 9–10: Undetectable — no current mechanism to detect it

**RPN = S × O × D**. Higher RPN = higher priority. Focus mitigation on items with RPN > 100 (or whatever threshold fits the user's risk appetite).

## How to apply

### Step 1 — Define the scope

Agree on what system, component, or process is being analyzed. FMEA works best on a bounded scope — one service, one data flow, one integration.

### Step 2 — Identify components or process steps

Break the system into logical components or sequential steps. Each component/step is a candidate source of failure modes.

### Step 3 — Enumerate failure modes for each component

For each component, ask: "In what ways could this fail?" Think about:
- Input failures (malformed data, missing fields, unexpected values)
- Dependency failures (downstream service unavailable, database timeout, external API rate limit)
- State failures (concurrent modification, stale cache, race condition)
- Resource failures (memory exhaustion, disk full, network partition)
- Human/operator failures (misconfiguration, deployment error)

### Step 4 — Identify effects

For each failure mode: what is the user-facing or system-level consequence? Think about the immediate effect and any downstream cascading effects.

### Step 5 — Score and calculate RPN

Score each failure mode on S, O, and D. Calculate RPN = S × O × D.

### Step 6 — Identify current controls

Note any existing mitigations: circuit breakers, retries, input validation, monitoring alerts, rate limits. These inform the Detection score and suggest where to invest.

### Step 7 — Recommend actions

For high-RPN items, propose specific mitigations that reduce S, O, or D. Prioritize reducing Detection (add monitoring) and Occurrence (add guards) before Severity (which often can't be changed).

## Output format

```markdown
## FMEA: [System/Component Name]

| # | Component | Failure mode | Effect | Severity (S) | Occurrence (O) | Detection (D) | RPN | Current controls | Recommended action |
|---|---|---|---|---|---|---|---|---|---|
| 1 | Payment API | External API timeout | Order stuck in pending; user sees no feedback | 7 | 5 | 4 | 140 | None | Add timeout + retry with exponential backoff; add monitoring alert for pending orders > 5 min |
| 2 | Inventory check | Race condition: two users buy last item | Oversell; fulfillment failure | 8 | 3 | 7 | 168 | None | Add optimistic locking on inventory decrement |
| 3 | Cart serialization | Invalid JSON in session store | Cart lost silently | 6 | 2 | 8 | 96 | None | Add schema validation on cart deserialization; log corruption events |
| 4 | Email notification | SMTP server unavailable | Order confirmation not sent | 4 | 3 | 3 | 36 | Retry queue | Existing retry queue adequate; consider dead-letter queue for visibility |

**High-priority items (RPN > 100):**
- Item 2: Inventory race condition (RPN 168) — recommend immediate action before launch
- Item 1: Payment timeout (RPN 140) — implement retry logic and alerting

**Gaps identified:**
- No analysis of authentication failures — should be added as a separate FMEA pass
- Detection scores are estimates; actual monitoring coverage not verified
```

## Procedure

1. Understand the system/component/process being analyzed and confirm the scope with the user — FMEA sprawls badly on an unbounded scope.
2. Apply the steps above: decompose into components, enumerate failure modes, score, calculate RPN, recommend mitigations.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-fmea.md`. Ask the user if they'd prefer a different path before writing.
4. Summarize: the highest-RPN items and the single most urgent recommended action.
