# Upgrading a build from Maven 3 to Maven 4

**Docs:** [Starting with Maven 4 (migration guide)](https://maven.apache.org/guides/mini/guide-migration-to-mvn4.html)
· [What's new in Maven 4?](https://maven.apache.org/whatsnewinmaven4.html)
· [Maven Upgrade Tool (mvnup)](https://maven.apache.org/tools/mvnup.html)

Maven 4 is **backward compatible**: a Maven 3 (model 4.0.0) project builds under Maven 4 unchanged.
Model 4.1.0 features are **opt-in**. Migrate in three phases — prepare on 3.9, switch the runtime,
then optionally adopt 4.1.0 features.

## Contents

- Requirements
- Phase 1 — Prepare (still on Maven 3.9)
- Phase 2 — Switch to Maven 4 (minimal changes; warnings that became failures, changed plugin defaults)
- Phase 3 — Adopt model 4.1.0 (`mvnup`, project root, `<subprojects>`, version inference, CI-friendly versions, BOM packaging, multiple source dirs)
- Build POM vs consumer POM
- Other notable Maven 4 changes (reactor, lifecycle, profiles, security, tooling, plugins)

## Requirements

- **Java 17 to run Maven 4** (you can still compile to older bytecode via the compiler `release`).
- Baseline on the **latest Maven 3.9** before upgrading.

## Phase 1 — Prepare (still on Maven 3.9)

1. Use the latest Maven 3.9.x.
2. Upgrade plugins to their latest Maven-3-compatible versions:
   `mvn versions:display-plugin-updates` (avoid plugins that require Maven 4 yet).
3. Optionally clean the POMs first with OpenRewrite's `org.openrewrite.maven.BestPractices`
   recipe (duplicates, prefixless expressions, and redundant versions are exactly what Maven 4
   complains about) — see the Automated migrations section of the `java-maven` skill.

## Phase 2 — Switch to Maven 4 (minimal changes)

1. Move the build environment (local + CI) to **Java 17**.
2. Install Maven 4 and update version references: `maven-wrapper.properties`, the Enforcer
   `requireMavenVersion` rule, and CI scripts.
3. **Fix misconfigurations that Maven 3.9 warned about and Maven 4 fails on.** Run with
   `--fail-on-severity WARN` (`-fos WARN`) to surface them:
    - **Duplicate plugin declarations** — declare each plugin once.
    - **Removed directory properties:**
        - `${executionRootDirectory}` → `${session.rootDirectory}` or `${project.rootDirectory}`
        - `${multiModuleProjectDirectory}` → `${session.topDirectory}` or `${project.rootDirectory}`
    - **Renamed lifecycle phases:** `pre-integration-test` → `before:integration-test`,
      `post-integration-test` → `after:integration-test` (legacy `pre-*`/`post-*` are now aliases).
4. **Changed plugin defaults (breaking):** Install and Deploy plugins now default
   `installAtEnd=true` / `deployAtEnd=true`. Remove explicit `true` values; add `false` only in the
   rare case you need per-module deploys.

At this point the project builds on Maven 4 with model 4.0.0. Everything below is optional.

## Phase 3 — Adopt model 4.1.0 (optional)

Model 4.1.0 is only needed for new features (root attribute, `<subprojects>`, BOM packaging,
optional profiles, multi-source dirs, version inference). Update the schema:

```xml
<project xmlns="http://maven.apache.org/POM/4.1.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.1.0 http://maven.apache.org/xsd/maven-4.1.0.xsd">
  <modelVersion>4.1.0</modelVersion>
```

### `mvnup` — the upgrade tool

Maven 4 ships **`mvnup`** (since 4.0.0-rc-4). It automates most of Phase 3: rewrites POMs to model
4.1.0, flags deprecated features, and migrates configuration.

```bash
mvnup check                          # dry run — report what would change
mvnup apply                          # apply the upgrade
mvnup apply --model-version 4.1.0 --plugins --infer   # target 4.1.0, upgrade plugins, infer versions
```

Review the diff afterward. ([Maven Upgrade Tool docs](https://maven.apache.org/tools/mvnup.html))

### Project root: `${project.rootDirectory}`

Mark the root so root-relative paths resolve:

- A `.mvn/` directory at the root (works with Maven 3 too), **or**
- `root="true"` on the root POM (Maven 4 only): `<project root="true">`.

### `<modules>` → `<subprojects>`

`<modules>` is deprecated in 4.1.0 (removed in a future version):

```xml
<subprojects>
  <subproject>acme-core</subproject>
  <subproject>acme-api</subproject>
</subprojects>
```

Maven 4 can also **auto-discover** subprojects: with `pom` packaging and no explicit
`<subprojects>`/`<modules>`, direct subdirectories containing a `pom.xml` are included.

### Version inference (less boilerplate)

In 4.1.0 you can omit the parent `<version>` and inter-subproject dependency versions — Maven infers
them. `<parent/>` is shorthand for a parent at `..`:

```xml
<parent/>                                  <!-- groupId/artifactId/version inferred -->
<dependency>
  <groupId>com.acme</groupId>
  <artifactId>acme-core</artifactId>       <!-- version inferred -->
</dependency>
```

### CI-friendly versions without the flatten plugin

Maven 4 + model 4.1.0 fully supports `${revision}` / `${changelist}` natively — **drop
`flatten-maven-plugin`**. Set the value via `-Drevision=...`, `.mvn/maven.config`, or a root property.
([CI-friendly versions](https://maven.apache.org/maven-ci-friendly.html))

### BOM packaging

A dedicated packaging type distinguishes a BOM from a parent POM:

```xml
<packaging>bom</packaging>
```

Available only as a **build POM** type in 4.1.0+; Maven generates a Maven-3-compatible consumer POM.
Supports `<exclusions>` on imported BOMs and importing classified BOMs via `<bomClassifier>`.

### Multiple source directories

```xml
<build>
  <sources>
    <source><scope>main</scope><directory>src/main/java</directory></source>
    <source><scope>test</scope><directory>src/test/java</directory></source>
  </sources>
</build>
```

## Build POM vs consumer POM

Maven 4 separates the **build POM** (what you commit — full config, model 4.1.0, `.mvn/`) from the
**consumer POM** (what is deployed — flattened, parent references resolved, BOM imports expanded,
kept at model 4.0.0 for ecosystem compatibility). This is automatic; consumers never see 4.1.0.

## Other notable Maven 4 changes

- **Reactor:** `mvn --resume`/`-r` resumes the last failed build, skipping already-built
  subprojects; reliable subfolder builds; consistent SNAPSHOT timestamps across subprojects.
- **Lifecycle is a tree, not a list:** `before:`/`after:` variants on every phase, optional phase
  skipping, and a concurrent builder (`mvn -b concurrent verify`).
- **Optional profiles:** `mvn -P?maybe-missing` succeeds (info message) if the profile is absent;
  richer activation via a new `<condition>` element (file existence, property comparison, logical
  operators) in `<activation>`.
- **New dependency `<type>`s** (4.1.0): `classpath-jar`/`modular-jar` force classpath vs module
  path; `processor`/`classpath-processor`/`modular-processor` declare annotation processors
  (requires Maven Compiler Plugin 4.x).
- **Security:** redesigned password encryption via `mvnenc` (real encryption + `decrypt` + vaults).
  ([Password Encryption (Maven 4)](https://maven.apache.org/guides/mini/guide-encryption-4.html))
- **Tooling:** Maven Daemon (`mvnd`) and Maven Shell (`mvnsh`) for faster repeated builds; Maven
  Resolver 2.0 (Java 17 native HTTP client).
- **Plugins:** Plexus DI removed — plugins must use JSR-330. Validate old plugins with
  `-Dmaven.plugin.validation=verbose`. Prefer `mvn verify` over `mvn clean install`.
