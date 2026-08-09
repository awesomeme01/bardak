---
name: java-gitignore
description: Use when initializing or auditing a Java project's .gitignore, or when the wrong files were committed — build output, IDE files, OS cruft, or (worst) secrets/local config. Covers what must and must NOT be ignored, a ready Maven+Gradle template, and how to find and un-track files already checked in by mistake (git rm --cached) including the secrets-in-history caveat.
---

# Java .gitignore

## Overview

Two jobs: **initialize** a correct `.gitignore` for a Java project, and **audit** a repo that has
already committed things it shouldn't. The expensive failure is a **secret** checked in — fixing the
`.gitignore` after the fact does NOT remove it from history.

## What MUST be ignored

| Category                                     | Patterns                                                                                                           |
|----------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| **Secrets / local config** (the costly leak) | `*.env`, `application-local.*`, `*-secret*.*`, `*.pem`, `*.p12`, `*.jks`, `*.keystore`, `credentials*`, `.envrc`   |
| Build output                                 | `target/`, `build/`, `out/`, `bin/`, `*.class`                                                                     |
| Packaged artifacts                           | `*.jar`, `*.war`, `*.ear` (except wrappers — below)                                                                |
| IDE files                                    | `.idea/`, `*.iml`, `*.ipr`, `*.iws`, `.vscode/`, `.settings/`, `.classpath`, `.project`, `.metadata`, `nbproject/` |
| OS cruft                                     | `.DS_Store`, `Thumbs.db`, `Desktop.ini`                                                                            |
| Logs / crash dumps                           | `*.log`, `hs_err_pid*`, `replay_pid*`                                                                              |
| Git/merge backups                            | `*.orig`, `*.BACKUP.*`, `*.LOCAL.*`, `*.REMOTE.*`                                                                  |

Drop in `references/java.gitignore` as a starting point (Maven + Gradle + IDE + OS + secrets).

## What must NOT be ignored

- **Wrapper files** — keep `gradle/wrapper/gradle-wrapper.jar`, `gradle/wrapper/gradle-wrapper.properties`,
  and `.mvn/wrapper/maven-wrapper.properties` committed so `./gradlew` / `./mvnw` work for everyone.
  A broad `*.jar` or `build/` rule can swallow these — add negations:

  ```gitignore
  !gradle/wrapper/gradle-wrapper.jar
  !**/src/main/**/build/
  ```

- Source, `pom.xml`/`build.gradle`, and `.mvn/` config (other than `timing.properties`).

## Initialize

1. One `.gitignore` at the repo root; module-specific files only if a module genuinely differs.
2. Copy `references/java.gitignore`, then trim to the project's build tool (Maven vs Gradle).
3. OS/IDE cruft is per-developer — a **global** excludes file keeps it out of every repo:

   ```bash
   git config --global core.excludesfile ~/.gitignore_global   # .DS_Store, .idea/, *.iml, etc.
   ```

## Audit a repo (the handy part)

`.gitignore` only affects **untracked** files — anything already committed keeps being tracked even
if it now matches a pattern. To find and fix what was checked in by mistake:

```bash
# List tracked files that SHOULD be ignored (already committed but match .gitignore)
git ls-files --cached --ignored --exclude-standard

# Un-track exactly those files WITHOUT deleting your working copy, then commit.
# Drive git rm from the list above (don't guess paths — a non-tracked path aborts the whole command):
git ls-files --cached --ignored --exclude-standard -z | xargs -0 git rm --cached
git commit -m "chore: stop tracking ignored files"
```

> Files covered by a **global** excludes file (`.idea/`, `.DS_Store`) won't appear here — that's fine.

Sanity sweeps for common mistakes:

```bash
git ls-files | grep -E '\.(class|jar|war|log|iml)$|(^|/)(target|build|out|bin)/|\.idea/'
git ls-files | grep -iE 'secret|credential|\.env$|application-local|\.(pem|p12|jks|keystore)$'
```

## Secrets already committed — important

`git rm --cached` removes a file from the **current** commit, **not from history** — the secret is
still recoverable from earlier commits. If a real secret was committed:

1. **Rotate the secret immediately** (assume it's compromised). This is the only true fix.
2. Then scrub history with `git filter-repo` (or BFG), and force-push (coordinate with the team).
3. Add the pattern to `.gitignore` so it can't recur.

## Red flags — stop

- `target/`/`build/`/`.class`/`.idea/` showing up in `git status` as tracked
- A `*.env`, `application-local.*`, keystore, or `credentials*` file staged or committed
- A broad ignore (`*.jar`, `build/`) with no negation for the wrapper jar
- "I'll just `git rm --cached` the secret" — that doesn't remove it from history; rotate it
