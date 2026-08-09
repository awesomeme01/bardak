---
name: decision-tables
description: Build a decision table that maps every meaningful combination of conditions to a specific outcome, and flags the combinations nobody has specified. Use eagerly whenever behavior is governed by combinations of independent yes/no conditions — authorization rules ("if role is admin AND resource is active AND user owns it..."), validation logic with multiple interacting constraints, discount/pricing stacking rules, feature-gating logic, or any business rule the user would naturally sketch as a truth table. Trigger on "what happens when multiple conditions apply", "map out these rules", "decision table", "truth table", "combination of conditions", or "what if a user qualifies for more than one X". If the problem also involves numeric ranges, states, or many independent parameters beyond simple yes/no conditions, the scenario-design skill can help route to the better-fitting technique instead.
---

# Decision Tables

A decision table maps every meaningful combination of conditions (inputs, states, flags) to a specific outcome (action, result, behavior). Each column is one rule (scenario); each row is a condition or action.

Decision tables are the right tool when behavior is governed by **combinations of independent yes/no conditions** and you want to ensure every combination is specified — and none are missing or contradictory. The value isn't the table itself, it's the columns nobody thought about: the multi-condition combinations that the spec never actually addresses.

## When to use

- Authorization logic ("if role is admin AND resource is active AND user owns it...")
- Validation rules with multiple interacting constraints
- Business rules expressed as conditional chains (discount stacking, pricing tiers, feature gating)
- Any place where you'd naturally write a truth table

If the logic is complex enough that the causal structure itself is hard to see (deeply nested ANDs/ORs, or you're explaining the rules to a non-technical stakeholder), consider the `cause-effect-graphing` skill instead — it produces the same table but derives it visually first. For straightforward conditional logic (≤ 5 conditions), skip straight to the table below.

## How to apply

### Step 1 — List all conditions

Identify every independent boolean condition that affects the outcome. Keep them binary (Y/N) where possible. If a condition has more than two values (e.g., role = admin/user/guest), either split it into two binary conditions or include all values as distinct columns.

Aim for 4–6 conditions. More than 6 creates 2^N explosion; consider whether some conditions are truly independent or whether the problem should be split.

### Step 2 — List all actions/outcomes

Identify every distinct thing the system does or returns for a given rule. There is usually one primary outcome plus secondary effects (error messages, audit logs, etc.).

### Step 3 — Generate all rules

For N binary conditions, there are 2^N possible combinations. Start by generating all of them. Use a systematic order (e.g., binary counting: YY, YN, NY, NN for 2 conditions).

Give each rule a short descriptive name alongside its number (e.g., `Rule 3: Unauthenticated`), built from whichever conditions are true. "Rule 3" on its own means nothing to someone skimming the table later — the name is what lets a reader recognize a scenario without tracing Y/N cells column by column.

### Step 4 — Fill in actions

For each rule (column), specify whether each action fires (✓ or ✗). If a rule's behavior is undefined by the spec, mark it as **GAP** — this is usually the entire point of the exercise.

### Step 5 — Simplify with don't-care conditions

If two adjacent rules have the same actions and differ only in one condition, that condition is irrelevant for this outcome. Merge those columns and mark the condition as `-` (don't-care). This reduces clutter.

### Step 6 — Check for gaps and contradictions

- **Gap**: A rule column where no action is specified — the system has undefined behavior
- **Contradiction**: Two identical condition columns with different actions — the spec is inconsistent
- **Dead rule**: A condition combination that is logically impossible — mark and remove

### Step 7 — Write a one-line summary per rule

After the table, write one plain-English sentence per rule stating the scenario and its outcome (e.g., "A logged-in user without permission is forbidden"). The table shows *what* happens in Y/N/✓/✗ notation; the summary says *why* in words a non-technical stakeholder can read without decoding the grid. Skip this only if the table has just 2-3 trivial rules where the grid is already self-explanatory.

## Output format

Write the table in markdown with two sections: **Conditions** and **Actions**. One column per rule.

```markdown
## Decision Table: [Name]

| | Rule 1: Authenticated & Permitted | Rule 2: Authenticated, No Permission | Rule 3: Unauthenticated | Rule 4: Unauthenticated, No Permission |
|---|---|---|---|---|
| **Conditions** | | | | |
| User is authenticated | Y | Y | N | N |
| User has permission | Y | N | - | - |
| **Actions** | | | | |
| Allow access | ✓ | ✗ | ✗ | ✗ |
| Redirect to login | ✗ | ✗ | ✓ | ✓ |
| Return 403 Forbidden | ✗ | ✓ | ✗ | ✗ |

**Rule summaries:**
- **Rule 1** (Authenticated & Permitted): A logged-in user with permission gets access.
- **Rule 2** (Authenticated, No Permission): A logged-in user without permission is forbidden.
- **Rule 3** (Unauthenticated): Anyone not logged in is redirected to login, regardless of permission — permission is irrelevant while unauthenticated (don't-care).
- **Rule 4** (Unauthenticated, No Permission): Same as Rule 3; kept distinct here only to show the merge candidate before simplification.

**Gaps identified:**
- Rule 3 and Rule 4 produce the same action but the spec only documents Rule 3. Verify Rule 4 behavior.
```

After the table, list:
- **Rule summaries**: one plain-English sentence per rule (see Step 7)
- **Gaps**: undefined or undocumented rules
- **Contradictions**: rules with conflicting outcomes
- **Impossible combinations**: condition sets that can never occur (note why)

## Procedure

1. Understand what's being analyzed (code, requirements, business rules) and confirm the conditions and actions with the user if either is ambiguous.
2. Apply the steps above to build the full table, including the "don't-care" simplification pass.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-decision-table.md`. Ask the user if they'd prefer a different path before writing.
4. Summarize: how many rules were generated, how many are gaps or contradictions, and what the highest-value next step is (e.g., "4 of these 8 combinations have no defined behavior — worth a product decision before shipping").
