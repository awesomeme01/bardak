---
name: scenario-analysis
description: Build a set of named, narrative scenarios — each a coherent story of who does what under what conditions and what should happen — to find requirements gaps and give stakeholders a shared vocabulary to validate against. Use eagerly for requirements analysis, checking whether user stories or acceptance criteria cover all relevant situations, API usage documentation, or regulatory/compliance contexts where scenarios need sign-off. Trigger on "what scenarios are we missing", "walk through the use cases", "does this cover all the ways a user could hit this", or when the "who" and "why" matter as much as the technical "what". Not the right fit for exhaustive combinatorial coverage (use decision-tables or combinatorial-testing) or quantified risk prioritization (use fmea) — scenario-analysis is for meaningful, humanly-reasoned-about situations, not exhaustive enumeration.
---

# Scenario Analysis / Scenario Planning

Scenario analysis structures the problem space as a set of **named, narrative scenarios** — each one a coherent combination of conditions, context, and stakeholder intent. Unlike decision tables (which are exhaustive and combinatorial) or FMEA (which is risk-quantified), scenario analysis is about building a shared vocabulary of meaningful situations that stakeholders can reason about together.

Each scenario is a story: who is doing what, under what conditions, and what should happen. This makes scenario analysis especially useful for requirements work, user story analysis, and anywhere the "why" and "who" matter as much as the technical "what."

## When to use

- Requirements analysis: ensuring all stakeholder use cases are covered
- API design: documenting the meaningful ways a client can use an endpoint
- User story gap-finding: checking whether acceptance criteria cover all the relevant situations
- Regulatory or compliance contexts where scenarios must be documented and signed off
- Any domain where the vocabulary of scenarios is as important as their enumeration

Scenario analysis is not the right choice when exhaustive combinatorial coverage is needed (use `decision-tables` or `combinatorial-testing`) or when the output needs to be a quantified risk register (use `fmea`). It's the right choice when *meaningful* scenarios are what stakeholders need to reason about and validate.

## How to apply

### Step 1 — Identify the subject and participants

What is the artifact being analyzed (feature, API, business process, policy)? Who are the relevant actors (user types, systems, external parties)? Include adversarial actors alongside legitimate ones — anyone who benefits from misusing the feature rather than using it as intended (a competitor scraping data, a user probing for other users' private content, someone abusing a sharing mechanism to spam or phish). Scenario analysis is one of the few techniques with a "who" in it; skipping the adversarial who is a common way real gaps slip through.

### Step 2 — Identify the key variables

What dimensions create meaningfully different situations? These might be:
- Actor type (admin, regular user, unauthenticated visitor, adversarial actor)
- Actor intent (legitimate use vs. misuse — e.g. probing IDs to find content not meant for them, brute-forcing a guessable link, spamming an invite mechanism)
- System state (first-time setup, steady state, degraded mode)
- Data state (empty, populated, at capacity)
- External conditions (third-party service available/unavailable, time-of-day constraints)

Unlike decision tables, don't enumerate all combinations — select combinations that are *meaningful and distinct*.

### Step 3 — Draft scenarios

For each meaningful combination, write a scenario. Each scenario should have:
- **Name**: Short, memorable label
- **Preconditions**: What must be true before the scenario begins
- **Actor**: Who initiates the action
- **Action**: What they do
- **Expected outcome**: What should happen
- **Notes**: Any open questions, assumptions, or edge cases to verify

### Step 4 — Check for coverage

After drafting, review: Are there important actors not covered? Important system states not represented? Common failure modes not included? Has anyone tried to use the feature in a way it wasn't designed for — probing, brute-forcing, over-sharing, or abusing whatever trust the feature extends? Add scenarios for any gaps.

### Step 5 — Flag gaps and open questions

Scenarios that expose undefined behavior, missing requirements, or assumptions that haven't been validated become action items.

## Output format

```markdown
## Scenario Analysis: [Feature/System Name]

### Scenario 1: Happy path — new user registration

| Field | Value |
|---|---|
| **Actor** | Unauthenticated visitor |
| **Preconditions** | Registration form displayed; email not already registered |
| **Action** | Submits form with valid name, email, and password |
| **Expected outcome** | Account created; confirmation email sent; user redirected to onboarding |
| **Notes** | — |

---

### Scenario 2: Duplicate email registration attempt

| Field | Value |
|---|---|
| **Actor** | Unauthenticated visitor |
| **Preconditions** | Email address already registered to an existing account |
| **Action** | Submits registration form with the same email |
| **Expected outcome** | Error: "An account with this email already exists." No account created. |
| **Notes** | Should the error hint that the user can log in instead? Not specified. |

---

### Scenario 3: Registration during email service outage

| Field | Value |
|---|---|
| **Actor** | Unauthenticated visitor |
| **Preconditions** | SMTP service unavailable |
| **Action** | Submits valid registration form |
| **Expected outcome** | Account created; user notified that confirmation email will be delayed |
| **Notes** | **GAP**: Current spec does not specify fallback behavior when email cannot be sent. |

---

## Coverage summary

| Actor | Happy path | Duplicate data | Service outage | Admin intervention |
|---|---|---|---|---|
| Unauthenticated visitor | ✓ Sc.1 | ✓ Sc.2 | ✓ Sc.3 | — |
| Existing user | — | — | — | — |
| Admin | — | — | — | ❌ not covered |

**Gaps:**
- No scenarios covering admin-initiated registration (e.g., bulk import)
- No scenarios for registration via social auth (OAuth)
- Behavior during service outage not specified — requires product decision
```

## Procedure

1. Understand the subject and participants, and confirm the key variables with the user if the meaningful dimensions aren't obvious.
2. Draft scenarios per the steps above, then check coverage against actors/states/failure modes and add scenarios for any gaps.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-scenarios.md`. Ask the user if they'd prefer a different path before writing.
4. Summarize: how many scenarios were drafted, notable coverage gaps, and what needs a product/stakeholder decision.
