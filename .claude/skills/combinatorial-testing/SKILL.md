---
name: combinatorial-testing
description: Shrink an exponential combination space (feature flags, config options, API parameters) down to a minimal test matrix that still guarantees every pair of values is covered — pairwise/all-pairs testing, escalating to orthogonal arrays when 3-way interaction coverage is needed. Use eagerly whenever the user has 4+ independent parameters or flags and wants a test suite for them, mentions "too many combinations to test", "feature flag combinations", "config options interacting badly", "pairwise testing", "all-pairs", "orthogonal array", or describes a system where multiple independent settings can combine in ways nobody has fully tested. Don't reach for this when parameters are tightly interdependent (that's a decision-tables problem) or when there are only 2-3 conditions (exhaustive enumeration is cheaper than setting up pairwise coverage).
---

# Combinatorial Testing (Pairwise / Orthogonal Arrays)

When a system has many independent parameters (feature flags, configuration options, API inputs), the number of possible combinations grows exponentially — testing all of them is impractical. Combinatorial testing solves this by selecting a carefully chosen subset that guarantees every pair (or triple) of parameter values appears together at least once.

**Pairwise (all-pairs) testing** is the practical starting point: it catches the vast majority of bugs (empirically, ~70–90% of combination-triggered bugs involve exactly two factors interacting) with a fraction of the test cases. **Orthogonal arrays** generalize this to higher interaction strengths when you need coverage of 3-way or higher interactions.

The key insight: most bugs in configuration-driven code are caused by two factors interacting unexpectedly, not by any single value. Covering all pairs is orders of magnitude cheaper than covering all combinations, and it's the number that makes this technique worth reaching for — always state the exhaustive count next to the pairwise count so the reduction is visible.

## When to use

- Feature flags (A/B flags, rollout flags, killswitches)
- Configuration files with multiple independent settings
- APIs with multiple independent optional parameters
- Database query filters that can be combined in many ways
- Any system where N > 4 independent binary or multi-value parameters interact

Do not use when parameters are heavily interdependent (use the `decision-tables` skill instead) or when the interaction strength is already known to be higher than pairwise and 3-way+ coverage is required from the start.

## How to apply

### Step 1 — List all factors and their values

Identify each independent variable (factor) and its possible values (levels). Keep the list focused: parameters with only one meaningful value aren't factors.

Example: 4 feature flags, each ON/OFF = 4 factors × 2 levels = 16 total combinations.

### Step 2 — Identify constraints and exclusions

Some combinations are invalid (e.g., "feature B requires feature A to be ON"). List these constraints explicitly — they'll reduce the test set and prevent testing impossible states.

### Step 3 — Generate pairwise test cases

For pairwise coverage: construct a set of rows such that for every pair of factors, every combination of their values appears in at least one row.

For small factor sets (up to ~6 factors, 2 levels each), you can construct this manually:
- Start with all combinations of the two most important factors
- For each remaining factor, assign values greedily to maximize coverage

For larger sets, use a pairwise generation algorithm or tool (e.g., PICT, AllPairs, or any combinatorial testing tool). The output is a minimal set of test cases.

### Step 4 — Annotate expected outcomes

For each generated test case (row), determine the expected outcome. Note any rows where the expected behavior is undefined or ambiguous — those are gaps.

### Step 5 — Check pair coverage

Verify that every pair of factor values appears at least once. If any pair is missing, add a row.

## Output format

```markdown
## Combinatorial Test Matrix: [System/Feature Name]

### Factors

| Factor | Values |
|---|---|
| Auth mode | local, SSO, API key |
| Cache enabled | ON, OFF |
| Rate limiting | ON, OFF |
| Debug mode | ON, OFF |

**Constraints:**
- Debug mode ON is only valid when Auth mode = local

### Pairwise test cases

| # | Auth mode | Cache | Rate limiting | Debug | Expected outcome |
|---|---|---|---|---|---|
| 1 | local | ON | ON | ON | Request processed, cached, rate checked |
| 2 | local | ON | OFF | OFF | Request processed, cached, no rate check |
| 3 | local | OFF | ON | OFF | Request processed, not cached, rate checked |
| 4 | SSO | ON | ON | OFF | SSO token validated, cached |
| 5 | SSO | OFF | OFF | OFF | SSO token validated, not cached |
| 6 | API key | ON | OFF | OFF | API key validated, cached |
| 7 | API key | OFF | ON | OFF | API key validated, rate checked |
| ... | | | | | |

**Pair coverage summary:** All pairs covered in N test cases (vs. 48 exhaustive).

**Gaps identified:**
- Expected behavior for SSO + debug mode not in scope (constraint violation)
- Behavior when all features are OFF simultaneously not specified
```

After the table, note:
- How many exhaustive combinations exist vs. how many pairwise cases you generated
- Any pairs that expose undefined behavior
- Whether higher-strength (3-way) coverage is warranted for any specific subset

## Procedure

1. Understand the system and confirm the factor list and any known constraints with the user if unclear.
2. Apply the steps above: list factors, apply constraints, generate the pairwise matrix, verify pair coverage.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-combinatorial.md`. Ask the user if they'd prefer a different path before writing.
4. Summarize: the exhaustive vs. pairwise count, any constraint-driven exclusions, and gaps that need a decision.
