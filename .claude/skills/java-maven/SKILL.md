---
name: java-maven
description: Use when working with Maven builds — editing pom.xml, structuring or building multi-module/multi-project (reactor) projects, managing dependencies/plugins/BOMs, cleaning up POM hygiene (scopes, ordering, redundant versions), upgrading Maven 3 to Maven 4, or running large mechanical migrations (Java version, Spring Boot, JUnit upgrades) where OpenRewrite recipes beat hand-editing. Load the reference files for multi-module details and the Maven 4 upgrade.
---

# Java Maven

## Overview

**Docs:** [Maven Wrapper](https://maven.apache.org/wrapper/)
· [POM reference](https://maven.apache.org/pom.html)
· [CI-friendly versions](https://maven.apache.org/maven-ci-friendly.html)
· [What's new in Maven 4](https://maven.apache.org/whatsnewinmaven4.html)

The house standard for Maven builds. Everyday conventions, POM hygiene, and migrations live here;
multi-module/reactor detail and the Maven 4 upgrade live in `references/` (routing table below).

## Everyday conventions

- **Use the Maven Wrapper** (`./mvnw`, `.mvn/wrapper/maven-wrapper.properties`) so everyone builds
  with the same Maven version. Pin it; don't rely on the machine's installed Maven.
- **Pin every plugin version** (in `<pluginManagement>` at the parent), and pin dependency versions
  via **`<dependencyManagement>`** (or an imported BOM) — never leave versions to chance/transitive
  resolution. Run `mvn versions:display-plugin-updates` / `display-dependency-updates` to review.
- **Prefer `mvn verify` over `mvn clean install`.** `verify` runs the full check (incl. integration
  tests) without polluting the local repo; reserve `install` for when a downstream local build needs
  the artifact.
- **CI-friendly versions:** use `${revision}` for a single source of truth across modules (built-in
  in Maven 4; needs the flatten plugin in Maven 3 — see the upgrade reference).
- **Know your Maven version.** Check `.mvn/wrapper/maven-wrapper.properties` or `mvn -v`. Maven 4
  requires **Java 17 to run** (it can still compile to older bytecode via the compiler `release`).
  Don't use model-4.1.0-only POM features on a Maven 3 build.
- Keep `pom.xml` ordered and minimal; don't duplicate a plugin/dependency declaration (Maven 4 fails
  the build on duplicates that Maven 3 only warned about).

```bash
./mvnw verify                                  # the default full check — not clean install
./mvnw -pl :acme-api -am verify                # one subproject plus everything it depends on
./mvnw versions:display-dependency-updates     # review pins (also: display-plugin-updates)
```

## POM hygiene

Apply these to any POM you write or review:

- **Explicit `<groupId>` on every plugin**; explicit versions everywhere (centralized in
  `*Management`) — and conversely, children must **not** re-declare a version that
  `dependencyManagement`/`pluginManagement` already provides.
- **Never `<scope>system</scope>`** — install the artifact into a repo instead.
- **No prefixless/`pom.` expressions:** `${artifactId}`/`${pom.version}` → `${project.artifactId}`/`${project.version}`.
- Keep the **canonical element order** in the POM and the dependency list sorted; keep `<scm>` in
  sync with the actual git origin.
- Run **`mvn dependency:analyze`** periodically: fix *used-undeclared* (add them) and
  *unused-declared* (remove them) dependencies.

## Automated migrations (OpenRewrite)

For well-known mechanical migrations, don't hand-edit dozens of files — run the matching
[OpenRewrite](https://docs.openrewrite.org/) recipe, then review the diff and `./mvnw verify`:

| Migration                             | Recipe (`rewrite.activeRecipes`)                                                                  | Artifact                     |
|---------------------------------------|---------------------------------------------------------------------------------------------------|------------------------------|
| POM cleanup (the hygiene rules above) | `org.openrewrite.maven.BestPractices`                                                             | built in                     |
| Java upgrade (17/21/25)               | `org.openrewrite.java.migrate.UpgradeToJava21` (also `...UpgradeToJava17`/`...25`)                | `rewrite-migrate-java`       |
| Spring Boot upgrade                   | `org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_5` (Boot 4: `boot4.UpgradeSpringBoot_4_0`) | `rewrite-spring`             |
| JUnit 4 → 5                           | `org.openrewrite.java.testing.junit5.JUnit4to5Migration`                                          | `rewrite-testing-frameworks` |
| Maven 3 → 4                           | use **`mvnup`** instead (ships with Maven 4) — see `references/maven-4-upgrade.md`                | —                            |

Invoke the plugin directly (no POM changes); the `dryRun` goal first writes a `rewrite.patch` preview:

```bash
mvn -U org.openrewrite.maven:rewrite-maven-plugin:run \
  -Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-migrate-java:RELEASE \
  -Drewrite.activeRecipes=org.openrewrite.java.migrate.UpgradeToJava21
```

Recipes are a starting point, not gospel: review the diff, build, and test before committing.

## When to open which reference

| Situation                                                                                     | Read                            |
|-----------------------------------------------------------------------------------------------|---------------------------------|
| Setting up or building part of a multi-module repo; reactor/build-order/BOM questions         | `references/multi-module.md`    |
| Moving a build to Maven 4, or asked about `<subprojects>`, `mvnup`, model 4.1.0, consumer POM | `references/maven-4-upgrade.md` |

## Red flags — stop

- Plugin or dependency versions not centralized in `<pluginManagement>` / `<dependencyManagement>`,
  or a child re-declaring a managed version
- `<scope>system</scope>`; `${pom.*}` or prefixless `${artifactId}`-style expressions
- Relying on the machine's Maven instead of the wrapper
- Using model-4.1.0 features (`<subprojects>`, `root="true"`, `bom` packaging) on a Maven 3 build
- Duplicate plugin declarations in a POM
