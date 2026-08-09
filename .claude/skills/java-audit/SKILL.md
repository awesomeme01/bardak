---
name: java-audit
description: Use when asked to audit, sweep, or review Java code in bulk against the house conventions — the whole repository, the current branch's diff, or named paths/classes. Also invoked directly as /java-skills:java-audit [paths | diff]. Read-only; reports findings without modifying files.
argument-hint: [paths | diff]
---

# Java Audit

Audit Java code in the current repository against the java-skills conventions.

## Scope

Determine the scope from the arguments: `$ARGUMENTS`

- **Empty**: audit the repository's main Java source — every module's production code and tests.
  If that exceeds ~40 files, audit a representative slice per module (entry points, core
  services, persistence, tests, build files) and say what was sampled.
- **`diff`**: audit only files changed on the current branch (staged + unstaged + commits not on
  the default branch).
- **Paths**: audit exactly the named files/directories, plus their immediate tests.

## How to run it

Dispatch the `java-reviewer` agent from this plugin with the scope above; it loads the relevant
skills and produces severity-grouped findings. For very large scopes, dispatch one agent per
module in parallel rather than one giant sweep.

If the agent is unavailable, do it inline: read the relevant `SKILL.md` files from this plugin's
`skills/` directory (`java-development` always; `java-testing` for tests; `java-concurrency`,
`java-observability`, `java-maven`, `java-gitignore` as the file types warrant), then review every
in-scope file against the skills' rules and "Red flags — stop" lists.

## Report

Whether delegated or inline, end with a single consolidated report:

1. One-line verdict (clean / nits only / needs changes).
2. Findings grouped by severity (blocker / should fix / nit), each with `file:line`, the rule and
   skill violated, and a concrete fix.
3. What was checked and found clean, and what was out of scope.

Do not modify any files — this is a read-only audit.
