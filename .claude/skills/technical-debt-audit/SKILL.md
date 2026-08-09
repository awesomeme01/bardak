---
name: technical-debt-audit
description: Identify structural problems in a codebase and write them up as technical debt documentation — one file per issue in a subfolder, plus an index. Use this skill when the user asks about structural problems, code smells, maintainability issues, architectural concerns, god classes, tight coupling, or "what's wrong with this code at a high level." Also use it when the user says things like "what should we clean up", "where is this hard to change", "what are the pain points", or "document our tech debt." The skill produces actionable write-ups, not vague observations.
---

The goal is to surface structural problems — concerns that make the code hard to change, test, or reason about — and write each one up clearly enough that a future reader can understand the problem, its consequences, and a credible path forward.

## Step 1 — Understand the target

If the user pointed at a specific class or file, start there. If they named a module or directory, orient first: read the main entry points and trace the key flows. If the target is vague ("the whole codebase"), start at the composition root — where dependencies are wired up. Constructor parameter counts are a fast proxy for concern count: a class registered with 10+ dependencies is a god class candidate before you've read a single method. From there, dive into the worst offenders. If the user described a symptom ("this is hard to change", "this keeps breaking") rather than a scope, skip the composition root and start from the pain site instead.

Read the code before forming opinions. Structural problems are often invisible from filenames alone — they show up in method length, constructor parameter counts, cross-cutting concerns, shared mutable state, and lifecycle coupling.

## Step 2 — Identify structural issues

Look for problems at the level of concerns and responsibilities, not style or implementation details. Good candidates follow, each with the direction it usually points — treat that as a starting hypothesis to confirm in Step 3, not a fix to apply on sight:

- **God classes / monoliths** — a single class handles multiple distinct concerns (detection: many unrelated injected dependencies, long files, unrelated method groups). _Usually points to:_ extract each concern into its own focused class, wired together at the composition root.
- **Inverted or borrowed lifecycle** — a component's behavior depends on state owned or timed by a different component (detection: comments explaining "this fires too late", tokens passed as proxies for state, ObjectDisposedException guards). _Usually points to:_ give the component ownership of its own state, or inject the state/signal explicitly instead of leaning on another component's timing.
- **Polling where events would do** — a loop reads a file or checks a flag on a timer when a channel, callback, or event would eliminate the latency and fragility. _Usually points to:_ replace the timer loop with a channel, callback, or event raised by the producer.
- **Inlined pipelines** — a sequence of distinct processing steps that are serialized in one function, making it hard to add, remove, or retry a single step. _Usually points to:_ model each step as a discrete stage (handler/middleware) so steps can be added, removed, or retried independently.
- **N round-trips where one would do** — multiple sequential queries that each fetch a partial view of the same data, especially inside a timed loop. _Usually points to:_ fetch the data in a single batched query or projection.
- **Abstraction mismatch** — interfaces introduced at internal seams where only one implementation exists and no test requires stubbing, or conversely, concrete coupling across a boundary that should be swappable. _Usually points to:_ collapse the single-implementation interface to a concrete class, or add an interface only where a real swap or test stub needs one.
- **Misplaced responsibility / leaky layer** — a generic component (often an infrastructure adapter) bakes in application or domain policy that belongs to its caller (detection: the type's name and contract promise generality but its body hard-codes a specific narrower domain, so it is no longer reusable outside the one use case it was written for, and a domain rule ends up living where nobody would look for it). _Usually points to:_ move the domain policy up to the caller and keep the component faithful to its general contract.
- **Contract divergence / hidden behavior** — an implementation does more or less than its signature and documentation promise, so callers cannot predict it from the contract (detection: arguments silently rewritten, e.g. an empty value swapped for a default; filtering, fan-out, or side effects the interface never discloses; behavior that contradicts the doc comment). _Usually points to:_ make behavior match the contract — either surface the hidden step in the signature and docs, or remove it and let the caller decide.
- **Silent error suppression** — an error or exceptional condition is caught and discarded (or replaced with a default), so the failure surfaces later as a loud, misleading error that points away from the real source (detection: empty catch blocks, catch-and-log-then-continue, nullable returns standing in for thrown exceptions, boolean success flags that callers forget to check). _Usually points to:_ let the exception propagate, or wrap and rethrow with the original cause preserved, so the call stack leads back to the true source of the problem.
- **Non-local coupling** — code whose correctness, behavior, or intent cannot be assessed by reading the local file alone. The reviewer must trace effects to distant handlers, discover preconditions established in other layers, or reconstruct a decision tree scattered across nested branches or multiple files. The cost shows up at review time (understanding requires jumping between files), at change time (one policy change touches many files), and at debug time (a behavior has no obvious cause at the site where it manifests). (Detection: to verify correctness, a reviewer must open files not referenced at the call site; a domain policy change ripples across many files; decisions and their sub-decisions are nested when they could be sequential.) _Usually points to:_ co-locate the decision with its effect, surface hidden preconditions in the signature or a guard, or flatten nested branching into sequential guard clauses — so the code can be read and evaluated in one place.

For each problem: be specific. Name the class, method, and file. Describe the concrete consequence (what breaks, what's hard, what has already broken in production if known). Avoid vague criticisms like "this is too coupled" without saying what is coupled to what and why that matters.

Prioritize by impact: lead with the issues that cause real pain (production errors, hard-to-add features, frequent regressions) before cosmetic concerns.

## Step 3 — Think about solutions before writing

For each issue, form a view on the preferred approach before writing. The write-up is more useful when it says "use a queue or channel instead of polling a file" than "consider alternatives to the current approach."

When the solution involves splitting a class, ask whether the seams need interfaces. Interfaces at internal seams add ceremony without benefit unless tests need to stub them or there are multiple real implementations. The default for internal splits is concrete classes; add interfaces when a test forces it.

## Step 4 — Write the debt files

### Index

Create or update `docs/technical-debt.md`. It is a table — one row per issue — concise enough to scan in 30 seconds:

```markdown
# Technical debt index

| Item             | File                              | Area           | Summary                                  |
| ---------------- | --------------------------------- | -------------- | ---------------------------------------- |
| Short issue name | [slug.md](technical-debt/slug.md) | Module / layer | One-line description of the core problem |
```

If the index already exists, add new rows; don't overwrite existing ones unless they're stale.

### Individual files

Create `docs/technical-debt/<slug>.md` for each issue. Use a short kebab-case slug that names the problem, not the symptom (e.g. `god-class.md`, `polling-ipc.md`, `shared-state-lifetime.md`).

Each file follows this structure:

```markdown
# [Issue name]

[One or two sentences identifying exactly what the problem is — class name, method name, file path. Enough that a reader can find it immediately.]

## Problems

[Concrete consequences. What breaks, what's hard to change, what has already failed. Be specific — "adding a new post-processing step means touching this function" is better than "low cohesion".]

## Preferred approach

[Actionable recommendation. Name the pattern, type, or refactoring. Explain why it's better, not just that it is.]

## Related debt

[Links to other debt files this one connects to, as relative markdown links. Omit if none.]
```

Cross-reference liberally. If fixing issue A is a prerequisite for fixing issue B, say so in both files.

### AGENTS.md pointer

- Check the repo root for `AGENTS.md` (or `CLAUDE.md`).
- If one exists and doesn't already reference the index, append:

  ```markdown
  ## Technical debt

  Before changing code in an area, check [docs/technical-debt.md](docs/technical-debt.md) for relevant entries and read the linked write-up first. Treat it as context, not a mandate — let it inform the change instead of expanding scope to fix it.
  ```

- Skip if a pointer already exists.
- If neither file exists, don't create one. Flag it in the Step 5 summary and ask whether to add one.

## Step 5 — Confirm with the user

After writing, summarize what was documented in a short message: how many issues, which area they're in, and whether any cross-reference each other in a meaningful dependency order. Note whether a pointer to the index was added to `AGENTS.md` (or `CLAUDE.md`), or whether neither file exists and the user should decide if one should be created. If the user wants to track priority or effort, that's a separate conversation — don't add it to the files unless asked.
