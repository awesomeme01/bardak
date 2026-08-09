---
name: cause-effect-graphing
description: Build a directed graph connecting causes (inputs, preconditions) to effects (outputs, actions) through AND/OR/NOT logic nodes, then mechanically derive a decision table from it — for when conditional logic is too tangled to see clearly in prose or needs to be communicated visually to non-technical stakeholders. Use when the user has complex validation logic with nested conditions (more than ~5 interacting conditions), says the spec seems to have implicit logical structure they can't quite see, or needs to communicate business rules to stakeholders via a diagram. For straightforward conditional logic with 5 or fewer conditions, the decision-tables skill is the faster, more direct path to the same result — reach for this only when the graph itself adds clarity.
---

# Cause-Effect Graphing

A cause-effect graph is a directed graph that maps **causes** (inputs, conditions, preconditions) to **effects** (outputs, outcomes, system actions) through intermediate logical nodes (AND, OR, NOT). It captures the same information as a decision table but makes the causal structure visible — which is valuable when the logic is complex and non-obvious.

Cause-effect graphing was developed as a precursor to decision table derivation: build the graph first, then mechanically derive the decision table from it.

**In practice**: for most software problems, go straight to a decision table (see the `decision-tables` skill). Use cause-effect graphing when:
1. The logical structure between causes is complex (nested ANDs and ORs) and validating the structure before building the table adds value
2. Communicating to stakeholders who benefit from seeing the causal chain visually

## When to use

- Complex validation logic with nested conditions
- Communicating business rules to non-technical stakeholders via diagrams
- When the spec seems to have implicit logical structure that isn't visible in prose form
- As a precursor to decision table derivation when conditions > 5 and interactions are complex

For straightforward conditional logic with ≤ 5 conditions, use the `decision-tables` skill directly instead.

## How to apply

### Step 1 — Identify causes and effects

**Causes** (numbered C1, C2, ...): Independent input conditions. Each cause is binary: true or false.

**Effects** (numbered E1, E2, ...): Outputs or system actions. Each effect is binary: fires or doesn't.

### Step 2 — Build the graph

Connect causes to effects using logical operators:
- **AND**: Effect fires only if all connected causes are true
- **OR**: Effect fires if any connected cause is true
- **NOT**: Inverts the truth value of a cause
- **Intermediate nodes**: Complex conditions can be broken into intermediate nodes (labeled I1, I2, ...) that combine causes before feeding into effects

Draw the graph from left (causes) to right (effects).

### Step 3 — Add constraints (optional)

Some cause combinations are impossible:
- **E (Exclusive)**: At most one of these causes can be true simultaneously
- **I (Inclusive)**: At least one must be true
- **R (Requires)**: C1 being true requires C2 to be true

Document constraints on the graph or as annotations.

### Step 4 — Derive the decision table

Once the graph is complete, mechanically enumerate the decision table: for each combination of causes, trace through the graph to determine which effects fire. This is the authoritative output for test derivation.

### Step 5 — Simplify

Merge rules where changing one cause value doesn't change any effect (don't-care cases).

## Output format

### Graph (Mermaid)

```markdown
\`\`\`mermaid
graph LR
    C1["C1: User is authenticated"] --> AND1
    C2["C2: User has role 'editor'"] --> AND1
    AND1["AND"] --> E1["E1: Show edit button"]

    C1 --> OR1
    C3["C3: Resource is public"] --> OR1
    OR1["OR"] --> E2["E2: Allow read access"]

    C1 --> NOT1["NOT"]
    NOT1 --> E3["E3: Show login prompt"]
\`\`\`
```

### Derived decision table

After building the graph, derive the decision table (see the `decision-tables` skill for the table format). The decision table is the output used for test case derivation — the graph is a working artifact to reach it.

```markdown
## Cause-Effect Graph: [Name]

### Causes
- C1: User is authenticated
- C2: User has role 'editor'
- C3: Resource is public

### Effects
- E1: Show edit button
- E2: Allow read access
- E3: Show login prompt

### Constraints
- C2 requires C1 (you can't have editor role without being authenticated)

### Derived decision table

| | Rule 1 | Rule 2 | Rule 3 | Rule 4 | Rule 5 |
|---|---|---|---|---|---|
| C1: Authenticated | Y | Y | Y | N | N |
| C2: Editor role | Y | Y | N | - | - |
| C3: Resource public | Y | N | - | Y | N |
| E1: Show edit button | ✓ | ✓ | ✗ | ✗ | ✗ |
| E2: Allow read access | ✓ | ✓ | ✓ | ✓ | ✗ |
| E3: Show login prompt | ✗ | ✗ | ✗ | ✗ | ✓ |

**Gaps:**
- Rule combining C2=Y and C3=Y not explicitly tested — covered by Rule 1
- Behavior when C2=Y and C1=N is constrained-impossible; document this explicitly
```

## Procedure

1. Understand the logic being modeled and confirm the causes and effects with the user if either is ambiguous.
2. Apply the steps above: build the graph, add constraints, derive the decision table, simplify.
3. Write the output to a markdown file. Suggested default: `docs/analysis/<artifact-name>-cause-effect.md`. Ask the user if they'd prefer a different path before writing. Include the Mermaid graph alongside the derived table.
4. Summarize: the number of effects/rules derived and any gaps or constrained-impossible combinations flagged.
