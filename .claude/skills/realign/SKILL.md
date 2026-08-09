---
name: realign
description: Use this skill when the user wants to audit code consistency issues — when the same problem is solved in different ways across the codebase. Activate on /realign.
---

# realign

Activate when the user invokes `/realign`.

## Input

The user provides two arguments:

- **Scope** (required): A file or directory path where they noticed the inconsistency — the anchor, not a search boundary
- **Hint** (required): A short free-text description of what they noticed (e.g. `"user group assignment"`, `"volume migration"`, `"error handling"`)

Example: `/realign src/services/payments "error handling"`

## Procedure

### 1. Understand the anchor

Read the scope path. Use the hint to identify the specific pattern or operation the user noticed. Understand how the anchor implements it.

### 2. Search the whole repo

Search beyond the scope for every other place in the codebase that solves the same or related problem. The scope is where the user noticed it — not a fence around the search.

### 3. Identify inconsistencies

Look for two categories of inconsistency only:

- **Structural**: The same goal achieved through different sequences of steps or different data flows
- **Abstraction choice**: The same operation performed using different tools, libraries, primitives, or levels of indirection (e.g. ORM vs raw SQL, `fetch` vs `axios`, declarative config vs imperative script)

Do NOT flag:

- Style differences (formatting, quoting, casing) — those belong to linters
- Naming conventions unless they reflect a structural difference

### 4. Report findings

Group findings by pattern. For each group:

1. Give it a short name describing the inconsistency
2. Show a brief excerpt from each instance with a clickable link — use `[file:line](path#Lline)` format so the user can jump to surrounding context in their editor
3. State clearly what varies across instances

Example group:

````
## Error handling in service calls

Found 3 different approaches:

**Try/catch with logger** — [src/services/payments/charge.ts:42](src/services/payments/charge.ts#L42)
\```ts
try {
  await stripe.charge(amount);
} catch (err) {
  logger.error(err);
  throw err;
}
\```

**Result type** — [src/services/payments/refund.ts:18](src/services/payments/refund.ts#L18)
\```ts
const result = await safeCall(() => stripe.refund(chargeId));
if (!result.ok) return { error: result.error };
\```

**Unhandled** — [src/services/payments/webhook.ts](src/services/payments/webhook.ts)
(no error handling found around external call at line 31)
````

### 5. Propose and ask

For each pattern group, propose a canonical form — the clearest, most explicit implementation found in the repo, or your own recommendation if none stands out. State your reasoning in one sentence.

Ask the user which groups they want to standardize and whether they agree with the proposed canonical. Continue conversationally from there — no menus or numbered choices.
